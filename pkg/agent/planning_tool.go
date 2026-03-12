package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// PlanningTool forces the agent to document its strategy before acting.
type PlanningTool struct {
	Agent *Agent
}

type planningArgs struct {
	Plan string `json:"plan"`
}

func (p *PlanningTool) Name() string { return "propose_plan" }

func (p *PlanningTool) Description() string {
	return "Documents a step-by-step strategy in PLAN.md and pins it to context. Use this for complex tasks before editing files."
}

func (p *PlanningTool) Execute(ctx context.Context, args string) (string, error) {
	var a planningArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		a.Plan = args
	}

	if a.Plan == "" {
		return "", fmt.Errorf("plan content is required")
	}

	// 1. Write to PLAN.md
	err := os.WriteFile("PLAN.md", []byte(a.Plan), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write PLAN.md: %w", err)
	}

	// 2. Automatically pin it if agent is available
	if p.Agent != nil {
		err = p.Agent.PinFile("PLAN.md")
		if err != nil {
			return "", fmt.Errorf("plan saved but failed to pin: %w", err)
		}
	}

	return "Strategy documented in PLAN.md and pinned to context. You may now proceed with the implementation.", nil
}
