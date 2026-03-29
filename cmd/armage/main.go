package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/user/armage/pkg/agent"
	"github.com/user/armage/pkg/provider"
)

type Config struct {
	OpenRouterKey   string `json:"openrouter_key"`
	OpenRouterModel string `json:"openrouter_model"`
	LocalScrubber   struct {
		Enabled    bool   `json:"enabled"`
		URL        string `json:"url"`
		BinaryPath string `json:"binary_path"`
		ModelPath  string `json:"model_path"`
	} `json:"local_scrubber"`
}

func main() {
	// 1. Load Configuration
	config := Config{
		OpenRouterModel: "meta-llama/llama-3.2-3b-instruct:free",
	}
	config.LocalScrubber.URL = "http://localhost:8080/v1/chat/completions"

	configData, err := os.ReadFile("armage.json")
	if err == nil {
		json.Unmarshal(configData, &config)
	}

	// Environment variable overrides
	if apiKey := os.Getenv("OPENROUTER_API_KEY"); apiKey != "" {
		config.OpenRouterKey = apiKey
	}
	if model := os.Getenv("OPENROUTER_MODEL"); model != "" {
		config.OpenRouterModel = model
	}
	if scrubberURL := os.Getenv("LOCAL_SCRUBBER_URL"); scrubberURL != "" {
		config.LocalScrubber.URL = scrubberURL
		config.LocalScrubber.Enabled = true
	}

	if config.OpenRouterKey == "" {
		fmt.Println("Error: OpenRouter API key is not set in armage.json or via environment.")
		os.Exit(1)
	}

	// 2. Setup Provider and Registry
	var llm provider.LLM
	orProvider := provider.NewOpenRouter(config.OpenRouterKey, config.OpenRouterModel)
	
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
	if config.LocalScrubber.Enabled {
		fmt.Printf("Privacy Shield: Enabled (Local scrubbing via %s)\n", config.LocalScrubber.URL)
		scrubber := &provider.LocalScrubber{
			BaseURL:    config.LocalScrubber.URL,
			BinaryPath: config.LocalScrubber.BinaryPath,
			ModelPath:  config.LocalScrubber.ModelPath,
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
	
	systemPrompt := `You are Armage, an expert coding agent for Termux on Android. 
You MUST follow the ReAct pattern strictly for EVERY turn:

Thought: [Your detailed reasoning about the current state and next steps]
Action: ToolName([JSON Arguments])

CRITICAL RULES:
1. ALWAYS provide at least one Action if the task is not complete.
2. Do NOT just "think" without acting. If you need information, call a tool.
3. You can provide multiple Actions in one turn (up to 5) by repeating the Action: line.
4. All your tools are located in "pkg/agent/". Use this path for code analysis.
5. When the task is fully finished, end your Thought with the phrase "Final Answer:" followed by your summary.

Available Tools:
- shell: Executes a shell command and returns the output. Use it for system tasks.
- list_dir: {"path": "...", "depth": 1}. Lists files/directories.
- get_symbols: {"path": "..."}. Lists functions, classes, and types in a file. Very efficient for mapping.
- propose_plan: {"plan": "..."}. Documents a strategy in PLAN.md and pins it. USE THIS FIRST for complex tasks.
- pin_file: {"path": "..."}. Pins a file's content to your history permanently.
- read_file: {"path": "...", "start": 1, "end": 100}. Reads a file with line numbers.
- grep_search: {"pattern": "...", "path": "..."}. Recursively searches for a pattern.
- apply_patch: {"path": "...", "patch": "..."}. Applies a unified diff (patch). Use for code edits.
- write_file: {"path": "...", "content": "..."}. Writes content to a file atomically. Use for NEW files.

Example Turn:
Thought: I need to see the project structure to find the main entry point.
Action: list_dir({"path": ".", "depth": 1})
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

	// 3. Simple CLI Loop
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
				// Use Transient Step to avoid history pollution
				// We don't append the result of a transient step to the permanent history
				res, err = a.StepTransient(ctx, "Please continue with your next Action or provide your Final Answer. Remember to use the Action: ToolName() format.")
				if err != nil {
					fmt.Printf("Error during nudge: %v\n", err)
					break
				}
				// We display the thought but DO NOT loop back to the 'assistant' message saving part of Step()
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
