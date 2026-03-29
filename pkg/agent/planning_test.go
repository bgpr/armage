package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPlanningRequirement(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&WriteTool{})
	
	// Mock LLM that correctly proposes a plan first
	llm := &MockMultiStepLLM{
		Responses: []string{
			"Thought: I need a strategy.\nAction: propose_plan({\"plan\": \"1. Fix bug\\n2. Test bug\"})",
		},
	}

	a := New(llm, reg)
	reg.Register(&PlanningTool{Agent: a}) // Register the tool
	a.AddSystemPrompt("You MUST use propose_plan before writing any files.")

	res, err := a.Step(context.Background(), "Refactor everything")
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	if len(res.ToolCalls) == 0 || res.ToolCalls[0].Name != "propose_plan" {
		t.Errorf("Expected propose_plan, got: %+v", res.ToolCalls)
	}

	// Verify PLAN.md was created
	if _, err := os.Stat("PLAN.md"); os.IsNotExist(err) {
		t.Errorf("PLAN.md was not created")
	}
	defer os.Remove("PLAN.md")

	// Verify it was pinned (history should have the system msg for pinning)
	foundPin := false
	for _, msg := range a.History {
		if strings.Contains(msg.Content, "Pinned File: PLAN.md") {
			foundPin = true
			break
		}
	}
	if !foundPin {
		t.Errorf("PLAN.md was not pinned to context")
	}
}
