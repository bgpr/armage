package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/user/armage/pkg/agent"
	"github.com/user/armage/pkg/config"
	"github.com/user/armage/pkg/provider"
)

func main() {
	// 0. Parse Flags
	uiFlag := flag.Bool("ui", false, "Start with TUI dashboard")
	flag.Parse()

	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	if cfg.OpenRouterKey == "" {
		fmt.Println("Error: OpenRouter API key is not set in armage.json or via environment.")
		os.Exit(1)
	}

	// 2. Setup Provider and Registry
	var llm provider.LLM
	orProvider := provider.NewOpenRouter(cfg.OpenRouterKey, cfg.OpenRouterModel)
	
	// Fetch free models to enable rotation
	fmt.Print("Fetching available free models for rotation... ")
	_, err = orProvider.FetchFreeModels(context.Background())
	if err != nil {
		fmt.Printf("Warning: Could not fetch free models: %v. Using default only.\n", err)
	} else {
		fmt.Printf("Done.\n")
	}
	llm = orProvider

	// Wrap with Local Scrubber if enabled
	if cfg.LocalScrubber.Enabled {
		fmt.Printf("Privacy Shield: Enabled (Local scrubbing via %s)\n", cfg.LocalScrubber.URL)
		scrubber := &provider.LocalScrubber{
			BaseURL:    cfg.LocalScrubber.URL,
			BinaryPath: cfg.LocalScrubber.BinaryPath,
			ModelPath:  cfg.LocalScrubber.ModelPath,
		}
		llm = provider.NewScrubbingLLM(llm, scrubber, ".armage_scrub_cache.json")
	}

	reg := agent.NewRegistry()
	reg.Register(&agent.ShellTool{})      // Register the shell tool
	reg.Register(&agent.ReadTool{})       // Register surgical read
	reg.Register(&agent.SearchTool{})     // Register grep search
	reg.Register(&agent.WriteTool{})      // Register atomic write
	reg.Register(&agent.DiffTool{})       // Register surgical edit (Search/Replace)
	reg.Register(&agent.ListDirTool{})    // Register project navigation
	reg.Register(&agent.SymbolsTool{})    // Register code mapping
	reg.Register(&agent.ApplyPatchTool{}) // Register robust multi-line edits
	
	// Create Agent first
	a := agent.New(llm, reg)
	
	// Now register tools that need agent reference
	reg.Register(&agent.PinTool{Agent: a}) // Register context pinning
	reg.Register(&agent.PlanningTool{Agent: a}) // Register strategy mapping
	
	a.RequireApproval = true // Enable Safety Governor
	
	systemPrompt := `You are Armage, an expert coding agent running LOCALLY on the user's Termux Android environment. 

INTERNAL ACCESS:
- You have DIRECT access to the current directory and all files via your tools.
- NEVER say "I cannot access the repository" or "I don't have internet". 
- If you need information, use your tools immediately.

TURN PATTERN (STRICT):
Thought: [Reasoning]
Action: ToolName([JSON Arguments])

TOOL SPECIFICATIONS:
- shell: {"command": "string"}. Executes a command.
- list_dir: {"path": "string", "depth": integer}. Lists files.
- read_file: {"path": "string", "start": integer, "end": integer}. Reads content.
- get_symbols: {"path": "string"}. Extracts functions/types.
- propose_plan: {"plan": "string"}. Documents strategy.
- pin_file: {"path": "string"}. Pins context.
- grep_search: {"pattern": "string", "path": "string"}. Searches code.
- apply_patch: {"path": "string", "patch": "string"}. Multi-line edits.
- write_file: {"path": "string", "content": "string"}. Creates files.

CRITICAL RULES:
1. ALWAYS use list_dir(".") first if you don't know the project.
2. ALWAYS provide an Action if the task is not complete.
3. NO HALLUCINATIONS: Use ONLY the parameters listed above.
4. METADATA: Ignore "REDACTED_" tags.
5. FINISHED: End your Thought with "Final Answer:" when done.
`

	a.AddSystemPrompt(systemPrompt)

	// Check if we have an existing state to resume
	statePath := ".armage_state.json"
	if _, err := os.Stat(statePath); err == nil {
		fmt.Print("Resuming existing session... ")
		if err := a.Load(statePath); err != nil {
			fmt.Printf("Error loading state: %v\n", err)
		} else {
			fmt.Println("Done.")
		}
	}

	// 3. Launch UI or CLI
	if *uiFlag {
		p := tea.NewProgram(newModel(a, statePath, systemPrompt), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("TUI Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Classic CLI Loop
	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Armage Agent - Ready (Type 'exit' to quit)")
	
	for {
		fmt.Print("\n> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		
		if input == "exit" {
			break
		}

		if input == "clear" {
			a.History = []provider.Message{}
			a.TotalUsage = provider.Usage{}
			a.AddSystemPrompt(systemPrompt)
			os.Remove(statePath)
			os.Remove(".armage_scrub_cache.json")
			fmt.Println("Full conversation history and cache cleared.")
			continue
		}

		if input == "" {
			continue
		}

		fmt.Println("Thinking...")
		res, err := a.Step(ctx, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		// PROCESS LOOP: Iteratively handle Nudges and Approvals
		for {
			if res.Thought != "" {
				fmt.Printf("\nThought: %s\n", res.Thought)
			}

			// CASE 1: Task Finished
			if strings.Contains(strings.ToLower(res.Thought), "final answer") {
				break
			}

			// CASE 2: Tool Calls Pending
			if res.Status == agent.StatusPending {
				fmt.Printf("\n--- Pending Actions (%d) ---\n", len(res.ToolCalls))
				for i, tc := range res.ToolCalls {
					fmt.Printf("%d. %s(%s)\n", i+1, tc.Name, tc.Args)
				}
				fmt.Print("\nApprove all? [y/N]: ")
				confirm, _ := reader.ReadString('\n')
				if strings.ToLower(strings.TrimSpace(confirm)) == "y" {
					res, err = a.Approve(ctx)
					if err != nil {
						fmt.Printf("Approval Error: %v\n", err)
						break
					}
					// After approval, we need to let the agent think about the observation
					fmt.Println("Thinking...")
					res, err = a.Step(ctx, "")
					if err != nil {
						fmt.Printf("Error: %v\n", err)
						break
					}
					continue // Process the next result
				} else {
					fmt.Println("Actions cancelled.")
					a.History = append(a.History, provider.Message{Role: "user", Content: "Observation: User cancelled the actions."})
					break // Stop the loop for this turn
				}
			}

			// CASE 3: Stuck (No Action, No Final Answer)
			if len(res.ToolCalls) == 0 {
				fmt.Println("\n(Agent paused without action. Nudging...)")
				// REINFORCE GOAL: Re-inject the original task during nudges
				nudgeMsg := fmt.Sprintf("Please continue working on the task: %s\nProvide your next Action or your Final Answer. Remember to use the Action: ToolName() format.", input)
				res, err = a.StepTransient(ctx, nudgeMsg)
				if err != nil {
					fmt.Printf("Error during nudge: %v\n", err)
					break
				}
				if res.Thought != "" {
					fmt.Printf("\nThought (Transient): %s\n", res.Thought)
				}
				continue
			}

			// CASE 4: Normal tool execution finished (StatusRunning)
			break
		}

		// Display final usage for the turn
		fmt.Printf("[Usage: %d tokens (Total: %d)]\n", res.Usage.TotalTokens, a.TotalUsage.TotalTokens)

		// Save state after every step for resilience
		if err := a.Save(statePath); err != nil {
			fmt.Printf("Error saving state: %v\n", err)
		}
	}
}
