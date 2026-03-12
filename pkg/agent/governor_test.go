package agent

import (
	"context"
	"strings"
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

func TestAutoRetryOnToolFailure(t *testing.T) {
	reg := NewRegistry()
	// No tools registered, so any action will fail with "Tool not found"
	
	llm := &MockMultiStepLLM{
		Responses: []string{
			"Thought: I will use a non-existent tool.\nAction: ghost_tool(\"boo\")",
			"Thought: Oh, ghost_tool failed. I will try a different approach.\nAction: shell(\"ls\")",
		},
	}

	a := New(llm, reg)
	// We want to see if the agent can react to the ERROR observation automatically
	// Step 1: LLM calls ghost_tool -> Execute fails -> Observation added to history
	res, err := a.Step(context.Background(), "Start")
	if err != nil {
		t.Fatalf("Step 1 failed: %v", err)
	}

	if !strings.Contains(a.History[len(a.History)-1].Content, "Error: Tool 'ghost_tool' not found") {
		t.Errorf("Expected error observation in history, got: %s", a.History[len(a.History)-1].Content)
	}

	// Now, if we call Step again with EMPTY input, it should process the history 
	// (which includes the error) and return the second response (the retry).
	res, err = a.Step(context.Background(), "")
	if err != nil {
		t.Fatalf("Step 2 failed: %v", err)
	}

	if res.ToolName != "shell" {
		t.Errorf("Expected agent to retry with 'shell', got: %s", res.ToolName)
	}
}
