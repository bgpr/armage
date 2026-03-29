package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type ShellTool struct{}

func (t *ShellTool) Name() string        { return "shell" }
func (t *ShellTool) Description() string { return "Executes a shell command and returns the output." }

type shellArgs struct {
	Command string `json:"command"`
}

func (t *ShellTool) Execute(ctx context.Context, args string) (string, error) {
	var a shellArgs
	// 1. Try to parse as JSON (standard for XML/Fallback models)
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		// 2. Fallback: treat as raw command string (standard for ReAct)
		a.Command = args
	}

	command := strings.TrimSpace(a.Command)
	// Strip quotes if they were added by the LLM
	command = strings.Trim(command, "\"'")

	if command == "" {
		return "", fmt.Errorf("empty command")
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("shell error: %w", err)
	}
	return string(out), nil
}
