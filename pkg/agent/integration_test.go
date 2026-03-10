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

	// 1. Setup with a FREE model
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "google/gemma-3-12b-it"
	}
	llm := provider.NewOpenRouter(apiKey, model)
	reg := NewRegistry()
	reg.Register(&ShellTool{})
	reg.Register(&ReadTool{})
	reg.Register(&WriteTool{})
	reg.Register(&SearchTool{})
	reg.Register(&DiffTool{})

	a := New(llm, reg)
	a.AddSystemPrompt(`You are Armage, an expert coding agent for Termux. 
Follow the ReAct pattern strictly: 
Thought: [Your reasoning]
Action: ToolName([JSON Arguments])

Available Tools:
- shell: Executes a shell command and returns the output.
- read_file: {"path": "...", "start": 1, "end": 10}. Reads a file with line numbers.
- write_file: {"path": "...", "content": "..."}. Writes content to a file atomically.
- grep_search: {"pattern": "...", "path": "..."}. Searches for a pattern in files.
- edit_file_diff: {"path": "...", "find": "...", "replace": "..."}. Surgically updates a file. Provide the EXACT 'find' block (from the file, without line numbers) and the 'replace' block.

Example:
Thought: I need to find where a function is defined.
Action: grep_search({"pattern": "func Hello", "path": "pkg/"})
`)

	fmt.Printf("\n--- Multi-Step Search & Edit Integration Test ---\n")

	// 2. Task: Search, Read, then Edit a specific file.
	ctx := context.Background()
	// We'll target tools_test.go which contains 'MockTool' and the comment '// returns the input'
	task := "Search for 'MockTool' in pkg/agent. Read tools_test.go, then update it so the comment '// returns the input' becomes '// returns the raw input'. Use edit_file_diff for the change. Finally, search for 'raw input' to confirm."
	
	fmt.Printf("[TASK]: %s\n", task)

	// Up to 5 steps for this complex navigation task
	for i := 1; i <= 5; i++ {
		t.Logf("\n--- STEP %d ---", i)
		thought, err := a.Step(ctx, task)
		if err != nil {
			t.Fatalf("Step %d failed: %v", i, err)
		}

		// Print the latest turn
		lastIdx := len(a.History) - 1
		if lastIdx >= 1 {
			assistantMsg := a.History[lastIdx-1]
			observationMsg := a.History[lastIdx]
			t.Logf("[ASSISTANT]:\n%s", assistantMsg.Content)
			t.Logf("[OBSERVATION]:\n%s", observationMsg.Content)
		}
		
		if strings.Contains(strings.ToLower(thought), "task complete") || 
		   strings.Contains(strings.ToLower(thought), "final answer") {
			t.Log("Task completion detected.")
			break
		}
		task = "" 
	}
	
	t.Log("\n--- END OF FULL CYCLE ---")

	// 3. Verification: Read tools_test.go back from the REAL file system
	data, _ := os.ReadFile("tools_test.go")
	if !strings.Contains(string(data), "returns the raw input") {
		t.Errorf("FAIL: File was not updated correctly. Content: %s", string(data))
	} else {
		fmt.Printf("SUCCESS: Verified 'raw input' exists in tools_test.go on disk.\n")
	}

	// REVERT: Change it back so we don't pollute the codebase for the next run
	original := strings.Replace(string(data), "returns the raw input", "returns the input", 1)
	os.WriteFile("tools_test.go", []byte(original), 0644)
}

// MockMultiStepLLM allows specifying multiple responses for sequential turns.
type MockMultiStepLLM struct {
	Responses []string
	Turn      int
}

func (m *MockMultiStepLLM) Chat(ctx context.Context, messages []provider.Message) (string, error) {
	if m.Turn >= len(m.Responses) {
		return "Final Answer: I am done.", nil
	}
	resp := m.Responses[m.Turn]
	m.Turn++
	return resp, nil
}

func TestReActMultiStep(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&ShellTool{})

	responses := []string{
		"Thought: I need to check the current user.\nAction: shell(\"whoami\")",
		"Thought: The user is root. Now I'll list files.\nAction: shell(\"ls\")",
	}

	llm := &MockMultiStepLLM{Responses: responses}
	a := New(llm, reg)

	// Step 1: user "Start" -> assistant "whoami" -> user "Observation"
	thought, err := a.Step(context.Background(), "Start")
	if err != nil {
		t.Fatalf("Step 1 failed: %v", err)
	}
	if thought != "I need to check the current user." {
		t.Errorf("Step 1 thought mismatch: %s", thought)
	}
	// History: user(Start), assistant(whoami), user(Observation)
	if len(a.History) != 3 {
		t.Fatalf("Step 1 history length mismatch: %d", len(a.History))
	}

	// Step 2: assistant "ls" -> user "Observation"
	thought, err = a.Step(context.Background(), "")
	if err != nil {
		t.Fatalf("Step 2 failed: %v", err)
	}
	if thought != "The user is root. Now I'll list files." {
		t.Errorf("Step 2 thought mismatch: %s", thought)
	}
	// History: ... user(Observation from whoami), assistant(ls), user(Observation from ls)
	if len(a.History) != 5 {
		t.Fatalf("Step 2 history length mismatch: %d", len(a.History))
	}
}
