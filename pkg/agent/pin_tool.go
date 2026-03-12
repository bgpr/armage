package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

// PinTool allows the agent to pin a file to its context permanently.
type PinTool struct {
	Agent *Agent
}

type pinArgs struct {
	Path string `json:"path"`
}

func (p *PinTool) Name() string { return "pin_file" }

func (p *PinTool) Description() string {
	return "Pins a file's content to the conversation history permanently. Use this for critical files you need to remember."
}

func (p *PinTool) Execute(ctx context.Context, args string) (string, error) {
	var a pinArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		a.Path = args
	}

	if a.Path == "" {
		return "", fmt.Errorf("path is required for pin_file")
	}

	if p.Agent == nil {
		return "", fmt.Errorf("agent not initialized in PinTool")
	}

	err := p.Agent.PinFile(a.Path)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Successfully pinned %s to context.", a.Path), nil
}
