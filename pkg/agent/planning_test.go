package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestHierarchicalPlanning(t *testing.T) {
	a := New(&MockLLM{}, NewRegistry())
	tool := &PlanningTool{Agent: a}

	defer os.Remove("PLAN.md")

	t.Run("CreatePlan", func(t *testing.T) {
		args := `{"action": "create", "plan": "# Project Plan\n- [ ] Task 1"}`
		_, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		data, _ := os.ReadFile("PLAN.md")
		if !strings.Contains(string(data), "# Project Plan") {
			t.Errorf("PLAN.md content incorrect: %s", string(data))
		}
	})

	t.Run("AppendTask", func(t *testing.T) {
		args := `{"action": "append", "task": "Task 2"}`
		_, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
		data, _ := os.ReadFile("PLAN.md")
		if !strings.Contains(string(data), "- [ ] Task 2") {
			t.Errorf("Task 2 not appended: %s", string(data))
		}
	})

	t.Run("CompleteTask", func(t *testing.T) {
		args := `{"action": "complete", "task": "Task 1"}`
		_, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("Complete failed: %v", err)
		}
		data, _ := os.ReadFile("PLAN.md")
		if !strings.Contains(string(data), "- [x] Task 1") {
			t.Errorf("Task 1 not completed: %s", string(data))
		}
	})

	t.Run("CompleteTaskRobustness", func(t *testing.T) {
		// Reset Task 1 to pending first
		os.WriteFile("PLAN.md", []byte("# Project Plan\n- [ ] Task 1"), 0644)
		
		// Model hallucination: uses 'plan' key instead of 'task'
		args := `{"action": "complete", "plan": "Task 1"}`
		_, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("Complete failed: %v", err)
		}
		data, _ := os.ReadFile("PLAN.md")
		if !strings.Contains(string(data), "- [x] Task 1") {
			t.Errorf("Task 1 robustness completion failed: %s", string(data))
		}
	})
}
