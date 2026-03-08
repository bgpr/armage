package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/user/armage/pkg/provider"
)

func TestIntegrationFullCycle(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: OPENROUTER_API_KEY not set")
	}

	// 1. Setup with a FREE model (Gemma 3 12B is very reliable)
	model := "google/gemma-3-12b-it"
	llm := provider.NewOpenRouter(apiKey, model)
	reg := NewRegistry()
	reg.Register(&ShellTool{})
	reg.Register(&ReadTool{})
	reg.Register(&WriteTool{})

	a := New(llm, reg)
	a.AddSystemPrompt(`You are Armage, an expert coding agent for Termux. 
Follow the ReAct pattern strictly: 
Thought: [Your reasoning]
Action: ToolName([JSON Arguments])

Available Tools:
- shell: Executes a shell command and returns the output.
- read_file: {"path": "...", "start": 1, "end": 10}. Reads a file with line numbers.
- write_file: {"path": "...", "content": "..."}. Writes content to a file atomically.

Example:
Thought: I need to check the files.
Action: shell("ls")
`)

	fmt.Printf("\n--- Multi-Step Surgical Integration Test ---\n")

	// 2. Multi-Step Task: Read, then Write, then Verify.
	ctx := context.Background()
	task := "Read the first 5 lines of tools.go, then create a new file named 'BUILD.md' with the content 'Project: Armage'. Finally, list the files to confirm."
	
	fmt.Printf("[TASK]: %s\n", task)

	// We run up to 4 steps to allow the agent to complete the multi-turn task
	for i := 1; i <= 4; i++ {
		fmt.Printf("\n--- STEP %d ---\n", i)
		thought, err := a.Step(ctx, task)
		if err != nil {
			t.Fatalf("Step %d failed: %v", i, err)
		}

		// Print the latest turns
		lastIdx := len(a.History) - 1
		if lastIdx >= 1 {
			assistantMsg := a.History[lastIdx-1] // The Thought/Action
			observationMsg := a.History[lastIdx] // The Observation

			fmt.Printf("[ASSISTANT]:\n%s\n", assistantMsg.Content)
			fmt.Printf("[OBSERVATION]:\n%s\n", observationMsg.Content)
		}

		// If the assistant didn't call a tool, it might be done
		if thought != "" && !strings.Contains(a.History[len(a.History)-1].Content, "Action:") {
			// If the last message from assistant has no Action, it's a Final Answer.
			// But in our ReAct loop, observations are from the User role.
		}
		
		// Break if the task seems complete (Agent usually stops calling tools)
		if strings.Contains(strings.ToLower(thought), "task complete") || 
		   strings.Contains(strings.ToLower(thought), "final answer") {
			break
		}
		
		// Reset task input for subsequent steps so the agent relies on history
		task = "" 
	}
	
	fmt.Printf("\n--- END OF FULL CYCLE ---\n")

	// 4. Final Verification: Check the real file system
	data, err := os.ReadFile("BUILD.md")
	if err != nil {
		t.Fatalf("FAIL: BUILD.md was not created on the file system: %v", err)
	}
	if !strings.Contains(string(data), "Project: Armage") {
		t.Errorf("FAIL: File content mismatch. Expected to contain 'Project: Armage', got: '%s'", string(data))
	} else {
		fmt.Printf("SUCCESS: Verified file content on disk.\n")
	}

	// Cleanup
	os.Remove("BUILD.md")
}
