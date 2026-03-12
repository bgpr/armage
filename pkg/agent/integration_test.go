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
	reg.Register(&ListDirTool{})
	reg.Register(&SymbolsTool{})
	reg.Register(&ApplyPatchTool{})

	a := New(llm, reg)
	reg.Register(&PinTool{Agent: a})
	reg.Register(&PlanningTool{Agent: a})

	a.RequireApproval = true
	a.AddSystemPrompt(`You are Armage, an expert coding agent for Termux. 
Follow the ReAct pattern strictly: 
Thought: [Your reasoning]
Action: ToolName([JSON Arguments])

Available Tools:
- shell: Executes a shell command and returns the output.
- list_dir: {"path": "...", "depth": 1}. Lists files/directories.
- get_symbols: {"path": "..."}. Lists functions, classes, and types in a file.
- propose_plan: {"plan": "..."}. Documents a strategy in PLAN.md and pins it.
- pin_file: {"path": "..."}. Pins a file to your history permanently.
- read_file: {"path": "...", "start": 1, "end": 10}. Reads a file with line numbers.
- write_file: {"path": "...", "content": "..."}. Writes content to a file atomically.
- grep_search: {"pattern": "...", "path": "..."}. Searches for a pattern in files.
- edit_file_diff: {"path": "...", "find": "...", "replace": "..."}. Surgically updates a file.
- apply_patch: {"path": "...", "patch": "..."}. Applies a standard unified diff (patch).

Example:
Thought: I need a strategy first.
Action: propose_plan({"plan": "1. Research\n2. Implement"})
`)

	fmt.Printf("\n--- Multi-Step Search & Edit Integration Test ---\n")

	// 2. Task: Pin, Search, Read, then Edit a specific file.
	ctx := context.Background()
	task := "Propose a plan in PLAN.md for this task. Pin TODO.md to your context. Explore the project structure with list_dir. Then search for 'MockTool' in pkg/agent. Use get_symbols to map tools_test.go, then update it so the return string 'returns the input' becomes 'returns the raw input'. Use apply_patch for the change. Finally, search for 'raw input' to confirm."
	
	fmt.Printf("[TASK]: %s\n", task)

	// Up to 10 steps for this complex sequence
	currentTask := task
	for i := 1; i <= 10; i++ {
		t.Logf("\n--- STEP %d ---", i)
		res, err := a.Step(ctx, currentTask)
		if err != nil {
			t.Fatalf("Step %d failed: %v", i, err)
		}
		currentTask = "" // Clear task after first turn to allow ReAct loop to continue

		t.Logf("[Usage: %d tokens (Total: %d)]", res.Usage.TotalTokens, a.TotalUsage.TotalTokens)

		// Handle Safety Governor approval automatically in the test
		if res.Status == StatusPending {
			t.Logf("[APPROVAL REQUIRED]: %s(%s)", res.ToolName, res.ToolArgs)
			res, err = a.Approve(ctx)
			if err != nil {
				t.Fatalf("Approval failed: %v", err)
			}
		}

		thought := res.Thought

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
	os.Remove("PLAN.md")
}

// MockMultiStepLLM allows specifying multiple responses for sequential turns.
type MockMultiStepLLM struct {
	Responses []string
	Turn      int
}

func (m *MockMultiStepLLM) Chat(ctx context.Context, messages []provider.Message) (string, provider.Usage, error) {
	if m.Turn >= len(m.Responses) {
		return "Final Answer: I am done.", provider.Usage{TotalTokens: 10}, nil
	}
	resp := m.Responses[m.Turn]
	m.Turn++
	return resp, provider.Usage{TotalTokens: 50}, nil
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
	res, err := a.Step(context.Background(), "Start")
	if err != nil {
		t.Fatalf("Step 1 failed: %v", err)
	}
	if res.Thought != "I need to check the current user." {
		t.Errorf("Step 1 thought mismatch: %s", res.Thought)
	}
	// History: 
	// 1. user(Start)
	// 2. assistant(Thought/Action)
	// 3. user(Observation)
	if len(a.History) != 3 {
		t.Fatalf("Step 1 history length mismatch: %d", len(a.History))
	}

	// Step 2: assistant "ls" -> user "Observation"
	res, err = a.Step(context.Background(), "")
	if err != nil {
		t.Fatalf("Step 2 failed: %v", err)
	}
	if res.Thought != "The user is root. Now I'll list files." {
		t.Errorf("Step 2 thought mismatch: %s", res.Thought)
	}
	// History: 
	// 1-3. (from Step 1)
	// 4. assistant(Thought/Action)
	// 5. user(Observation)
	if len(a.History) != 5 {
		t.Fatalf("Step 2 history length mismatch: %d", len(a.History))
	}
}

func TestIntegrationApproval(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&ShellTool{})

	// LLM wants to run a shell command
	llm := &MockMultiStepLLM{
		Responses: []string{
			"Thought: I will check the user.\nAction: shell(\"whoami\")",
		},
	}

	a := New(llm, reg)
	a.RequireApproval = true

	// 1. Initial Step - Should be Pending
	res, err := a.Step(context.Background(), "Check user")
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	if res.Status != StatusPending {
		t.Errorf("Expected status Pending, got: %v", res.Status)
	}
	if len(a.History) != 2 {
		t.Errorf("Expected history length 2 (User + Assistant), got: %d", len(a.History))
	}

	// 2. Approve Step - Should execute and return Running
	res, err = a.Approve(context.Background())
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	if res.Status != StatusRunning {
		t.Errorf("Expected status Running, got: %v", res.Status)
	}
	if len(a.History) != 3 {
		t.Errorf("Expected history length 3 (User + Assistant + Observation), got: %d", len(a.History))
	}
	
	if !strings.Contains(a.History[2].Content, "Observation:") {
		t.Errorf("Expected observation in history, got: %s", a.History[2].Content)
	}
}

func TestIntegrationAutoRetry(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&ShellTool{})

	// Step 1: Call non-existent tool
	// Step 2: React to error and call shell
	llm := &MockMultiStepLLM{
		Responses: []string{
			"Thought: I will use a broken tool.\nAction: broken_tool(\"data\")",
			"Thought: broken_tool failed. I will use shell instead.\nAction: shell(\"whoami\")",
		},
	}

	a := New(llm, reg)
	ctx := context.Background()

	// Turn 1
	res, err := a.Step(ctx, "Start task")
	if err != nil {
		t.Fatalf("Step 1 failed: %v", err)
	}
	if !strings.Contains(a.History[len(a.History)-1].Content, "Error: Tool 'broken_tool' not found") {
		t.Errorf("Expected error observation, got: %s", a.History[len(a.History)-1].Content)
	}

	// Turn 2: Automatic retry (empty input)
	res, err = a.Step(ctx, "")
	if err != nil {
		t.Fatalf("Step 2 failed: %v", err)
	}

	if res.ToolName != "shell" {
		t.Errorf("Expected retry with 'shell', got: %s", res.ToolName)
	}
}

func TestIntegrationSummarization(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&ShellTool{})

	// Mock LLM that returns a thought and then a summary when asked
	llm := &MockMultiStepLLM{
		Responses: []string{
			"Thought: Step 1.\nAction: shell(\"1\")",
			"Thought: Step 2.\nAction: shell(\"2\")",
			"Thought: Step 3.\nAction: shell(\"3\")",
			"SUMMARY: We performed three steps.",
		},
	}

	a := New(llm, reg)
	a.MaxHistory = 4 // [System, Summary, User, Assistant]
	ctx := context.Background()

	// Fill history
	a.Step(ctx, "task 1")
	a.Step(ctx, "task 2")
	
	// This step should trigger summarization
	a.Step(ctx, "task 3")

	foundSummary := false
	for _, msg := range a.History {
		if strings.Contains(msg.Content, "Conversation Summary:") {
			foundSummary = true
			break
		}
	}

	if !foundSummary {
		t.Errorf("Summarization was not triggered or summary not found in history")
	}
}
