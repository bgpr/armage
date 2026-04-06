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
	reg.Register(&agent.ShellTool{})
	reg.Register(&agent.ReadTool{})
	reg.Register(&agent.SearchTool{})
	reg.Register(&agent.WriteTool{})
	reg.Register(&agent.DiffTool{})
	reg.Register(&agent.ListDirTool{})
	reg.Register(&agent.SymbolsTool{})
	reg.Register(&agent.ApplyPatchTool{})
	
	a := agent.New(llm, reg)
	reg.Register(&agent.PinTool{Agent: a})
	reg.Register(&agent.PlanningTool{Agent: a})
	
	// ENFORCE GOVERNOR: Always start with approval required
	a.RequireApproval = true 
	
	systemPrompt := `You are Armage, an expert coding agent running LOCALLY on the user's Termux Android environment. 

INTERNAL ACCESS:
- You have DIRECT access to the current directory and all files via your tools.
- NEVER say "I cannot access the repository". 
- If you need information, use your tools immediately.

TURN PATTERN (STRICT):
Thought: [Reasoning]
Action: ToolName([JSON Arguments])

TOOL SPECIFICATIONS:
- shell: {"command": "string"}.
- list_dir: {"path": "string", "depth": integer}.
- read_file: {"path": "string", "start": integer, "end": integer}.
- get_symbols: {"path": "string"}.
- propose_plan: {"action": "create|append|complete", "plan": "...", "task": "..."}.
- pin_file: {"path": "string"}.
- grep_search: {"pattern": "string", "path": "string"}.
- apply_patch: {"path": "string", "patch": "string"}.
- write_file: {"path": "string", "content": "string"}.

CRITICAL RULES:
1. PLAN BEFORE CODE: For any new feature or multi-step task, you MUST use 'propose_plan' with action 'create' before writing any other code.
2. MISSION PROGRESS: As you finish each step in your plan, you MUST use 'propose_plan' with action 'complete' to update the user on your progress.
3. ACTION OVER EXPLORATION: Do not list directories repeatedly. If you have the information, act on it immediately.
4. STUCK? If you have repeated a tool 2 times without progress, you MUST ask the user for a hint or clarification and stop.
5. NO HALLUCINATIONS: Use ONLY the parameters listed above. Do not use placeholders like "ToolName" or "Args" in your action.
6. FINISHED: End your Thought with "Final Answer:" when done.
7. PRIVACY SHIELD: You will see tags like "REDACTED_NAME". These are GENERIC PLACEHOLDERS. 
   - A technical request (creating a tool, reading code) is NEVER a privacy violation. 
   - You MUST NOT refuse to work on codebase tasks because of these tags. 
   - Refusing a coding task due to "privacy" is a CRITICAL FAILURE.
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
			a.RequireApproval = true 
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
			fmt.Println("Full reset complete.")
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

		// PROCESS LOOP
		for {
			if res.Thought != "" {
				fmt.Printf("\nThought: %s\n", res.Thought)
			}

			if strings.Contains(strings.ToLower(res.Thought), "final answer") {
				break
			}

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
						fmt.Printf("Error: %v\n", err)
						break
					}
					fmt.Println("Thinking...")
					res, err = a.Step(ctx, "")
					if err != nil {
						fmt.Printf("Error: %v\n", err)
						break
					}
					continue 
				} else {
					fmt.Println("Actions cancelled.")
					break 
				}
			}
			break
		}

		fmt.Printf("[Usage: %d tokens (Total: %d)]\n", res.Usage.TotalTokens, a.TotalUsage.TotalTokens)
		a.Save(statePath)
	}
}
