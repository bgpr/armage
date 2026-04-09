package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// PlanningTool manages the hierarchical task list in PLAN.md.
type PlanningTool struct {
	Agent *Agent
}

type planningArgs struct {
	Action string `json:"action"` // "create", "append", "complete"
	Plan   string `json:"plan,omitempty"`
	Task   string `json:"task,omitempty"`
}

func (p *PlanningTool) Name() string { return "propose_plan" }

func (p *PlanningTool) Description() string {
	return `Manages the project strategy in PLAN.md.
Actions:
- create: {"action": "create", "plan": "Full markdown plan"}. Creates/overwrites PLAN.md.
- append: {"action": "append", "task": "New sub-task"}. Adds a task to the end.
- complete: {"action": "complete", "task": "Task description"}. Marks a task as [x].`
}

func (p *PlanningTool) Execute(ctx context.Context, args string) (string, error) {
	var a planningArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		// Fallback for simple string input (legacy support)
		a.Action = "create"
		a.Plan = args
	}

	// Agent-Proofing: Many models hallucinate 'plan' instead of 'task' for completions
	if a.Action == "complete" && a.Task == "" && a.Plan != "" {
		a.Task = a.Plan
	}

	switch a.Action {
	case "create":
		return p.createPlan(a.Plan)
	case "append":
		return p.appendTask(a.Task)
	case "complete":
		return p.completeTask(a.Task)
	default:
		// If action is empty but plan is not, assume create (legacy)
		if a.Action == "" && a.Plan != "" {
			return p.createPlan(a.Plan)
		}
		return "", fmt.Errorf("unknown action: %s", a.Action)
	}
}

func (p *PlanningTool) createPlan(content string) (string, error) {
	if content == "" {
		return "", fmt.Errorf("plan content is required for 'create'")
	}
	err := os.WriteFile("PLAN.md", []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write PLAN.md: %w", err)
	}
	if p.Agent != nil {
		p.Agent.PinFile("PLAN.md")
	}
	return "PLAN.md created and pinned.", nil
}

func (p *PlanningTool) appendTask(task string) (string, error) {
	if task == "" {
		return "", fmt.Errorf("task content is required for 'append'")
	}
	f, err := os.OpenFile("PLAN.md", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return "", err
	}
	defer f.Close()

	formattedTask := fmt.Sprintf("\n- [ ] %s", task)
	if _, err := f.WriteString(formattedTask); err != nil {
		return "", err
	}
	return fmt.Sprintf("Task appended: %s", task), nil
}

func (p *PlanningTool) completeTask(task string) (string, error) {
	if task == "" {
		return "", fmt.Errorf("task description is required for 'complete'")
	}
	data, err := os.ReadFile("PLAN.md")
	if err != nil {
		return "", err
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	found := false
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), strings.ToLower(task)) && strings.Contains(line, "[ ]") {
			lines[i] = strings.Replace(line, "[ ]", "[x]", 1)
			found = true
			break
		}
	}

	if !found {
		return "", fmt.Errorf("task '%s' not found with '[ ]' status in PLAN.md", task)
	}

	newContent := strings.Join(lines, "\n")
	err = os.WriteFile("PLAN.md", []byte(newContent), 0644)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Task completed: %s", task), nil
}

func (p *PlanningTool) Preview(ctx context.Context, args string) (string, error) {
	var a planningArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		a.Action = "create"
		a.Plan = args
	}
	if a.Action == "complete" && a.Task == "" && a.Plan != "" {
		a.Task = a.Plan
	}
	return fmt.Sprintf("Update PLAN.md (Action: %s)", a.Action), nil
}
