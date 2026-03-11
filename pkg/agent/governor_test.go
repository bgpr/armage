package agent

import (
	"context"
	"testing"
)

func TestGovernorApproval(t *testing.T) {
	reg := NewRegistry()
	// Mock modifying tool
	reg.Register(&ShellTool{})

	// LLM wants to run a shell command
	llm := &MockMultiStepLLM{
		Responses: []string{
			"Thought: I need to delete a file.\nAction: shell(\"rm test.txt\")",
		},
	}

	a := New(llm, reg)
	// Enable safety protocol
	a.RequireApproval = true

	// Step 1: Trigger the tool
	res, err := a.Step(context.Background(), "Delete the file")
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	// Expect StatusPending and Tool info
	if res.Status != StatusPending {
		t.Errorf("Expected status Pending, got: %v", res.Status)
	}
	// The parser returns the raw argument string, including quotes if they were in the LLM response
	expectedArgs := "\"rm test.txt\""
	if res.ToolName != "shell" || res.ToolArgs != expectedArgs {
		t.Errorf("Expected shell tool with args %s, got: %s(%s)", expectedArgs, res.ToolName, res.ToolArgs)
	}

	// Verify tool was NOT executed yet (no Observation in history)
	// History: 
	// 1. User(Delete the file) - added by Step
	// 2. Assistant(Thought/Action) - added by Step
	if len(a.History) != 2 {
		t.Errorf("Expected history length 2, got: %d. Tool might have executed prematurely.", len(a.History))
	}

	// Step 2: Approve and continue
	res, err = a.Approve(context.Background())
	if err != nil {
		t.Fatalf("Approval failed: %v", err)
	}

	if res.Status != StatusRunning {
		t.Errorf("Expected status Running after approval, got: %v", res.Status)
	}
	// Now history should have the observation
	// 3. User(Observation) - added by Approve/ExecuteTool
	if len(a.History) != 3 {
		t.Errorf("Expected history length 3 after approval, got: %d", len(a.History))
	}
}
