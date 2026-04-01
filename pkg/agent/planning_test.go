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
	
	llm := &MockMultiStepLLM{
		Responses: []string{
			"Thought: I will propose a plan.\nAction: propose_plan({\"plan\": \"Step 1\"})",
		},
	}

	a := New(llm, reg)
	tool := &PlanningTool{Agent: a}
	reg.Register(tool)

	t.Run("ProposePlan", func(t *testing.T) {
		res, err := a.Step(context.Background(), "Task")
		if err != nil {
			t.Fatalf("Step failed: %v", err)
		}
		if len(res.ToolCalls) == 0 || res.ToolCalls[0].Name != "propose_plan" {
			t.Errorf("Expected propose_plan, got: %+v", res.ToolCalls)
		}
		
		if _, err := os.Stat("PLAN.md"); os.IsNotExist(err) {
			t.Errorf("PLAN.md not created")
		}
		os.Remove("PLAN.md")
	})

	t.Run("EmptyPlan", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), "")
		if err == nil || !strings.Contains(err.Error(), "plan content is required") {
			t.Errorf("Expected 'plan content is required' error, got: %v", err)
		}
	})
}
