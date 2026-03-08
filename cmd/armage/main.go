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
	// 1. Get API Key from Env
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: OPENROUTER_API_KEY environment variable is not set.")
		os.Exit(1)
	}

	// 2. Setup Provider and Agent
	llm := provider.NewOpenRouter(apiKey, "google/gemini-2.0-flash-001")
	reg := agent.NewRegistry()
	reg.Register(&agent.ShellTool{}) // Register the shell tool
	
	a := agent.New(llm, reg)
	a.AddSystemPrompt(`You are Armage, an expert coding agent for Termux on Android. 
You follow the ReAct pattern: 
Thought: [Your reasoning here]
Action: [ToolName]([Arguments])

Available Tools:
- shell: Executes a shell command and returns the output. Use it for ls, cat, grep, etc.

Example:
Thought: I need to see what's in the current directory.
Action: shell("ls -F")
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
		thought, err := a.Step(ctx, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		
		if thought != "" {
			fmt.Printf("\nThought: %s\n", thought)
		}

		// Save state after every step for resilience
		if err := a.Save(statePath); err != nil {
			fmt.Printf("Error saving state: %v\n", err)
		}
	}
}
