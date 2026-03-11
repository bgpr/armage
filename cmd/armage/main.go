package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/user/armage/pkg/agent"
	"github.com/user/armage/pkg/provider"
)

func main() {
	// 1. Get API Key and Model from Env
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: OPENROUTER_API_KEY environment variable is not set.")
		os.Exit(1)
	}
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "google/gemma-3-12b-it" 
	}

	// 2. Setup Provider and Agent
	llm := provider.NewOpenRouter(apiKey, model)
	reg := agent.NewRegistry()
	reg.Register(&agent.ShellTool{})      // Register the shell tool
	reg.Register(&agent.ReadTool{})       // Register surgical read
	reg.Register(&agent.SearchTool{})     // Register grep search
	reg.Register(&agent.WriteTool{})      // Register atomic write
	reg.Register(&agent.DiffTool{})       // Register surgical edit (Search/Replace)
	reg.Register(&agent.ListDirTool{})    // Register project navigation
	reg.Register(&agent.SymbolsTool{})    // Register code mapping
	reg.Register(&agent.ApplyPatchTool{}) // Register robust multi-line edits
	
	a := agent.New(llm, reg)
	a.RequireApproval = true // Enable Safety Governor
	a.AddSystemPrompt(`You are Armage, an expert coding agent for Termux on Android. 
You follow the ReAct pattern strictly: 
Thought: [Your reasoning here]
Action: ToolName([JSON Arguments])

Available Tools:
- shell: Executes a shell command and returns the output. Use it for complex system operations.
- list_dir: {"path": "...", "depth": 1}. Lists files/directories. Use depth to see subdirectories (max 3).
- get_symbols: {"path": "..."}. Lists functions, classes, and types in a file. Very efficient for mapping out code.
- read_file: {"path": "...", "start": 1, "end": 10}. Reads a file with line numbers for context.
- grep_search: {"pattern": "...", "path": "..."}. Recursively searches for a pattern in files.
- edit_file_diff: {"path": "...", "find": "...", "replace": "..."}. Surgically updates a file. Use for small, single-turn changes.
- apply_patch: {"path": "...", "patch": "..."}. Applies a standard unified diff (patch). Use for complex, multi-line refactors.
- write_file: {"path": "...", "content": "..."}. Writes content to a file atomically. Use this for new or small files.

Example:
Thought: I need to apply a complex multi-line change.
Action: apply_patch({"path": "pkg/agent/agent.go", "patch": "--- pkg/agent/agent.go\n+++\n..."})
`)

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

		if input == "" {
			continue
		}

		fmt.Println("Thinking...")
		res, err := a.Step(ctx, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		
		if res.Thought != "" {
			fmt.Printf("\nThought: %s\n", res.Thought)
		}

		if res.Status == agent.StatusPending {
			fmt.Printf("\nAction: %s(%s)\n", res.ToolName, res.ToolArgs)
			fmt.Print("Approve? [y/N]: ")
			confirm, _ := reader.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(confirm)) == "y" {
				res, err = a.Approve(ctx)
				if err != nil {
					fmt.Printf("Approval Error: %v\n", err)
				}
			} else {
				fmt.Println("Action cancelled.")
				// Record cancellation in history so agent knows
				a.History = append(a.History, provider.Message{Role: "user", Content: "Observation: User cancelled the action."})
			}
		}

		// Save state after every step for resilience
		if err := a.Save(statePath); err != nil {
			fmt.Printf("Error saving state: %v\n", err)
		}
	}
}
