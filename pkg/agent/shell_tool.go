package agent

import (
	"context"
	"os/exec"
)

// ShellTool allows the agent to execute shell commands.
type ShellTool struct{}

func (s *ShellTool) Name() string {
	return "shell"
}

func (s *ShellTool) Description() string {
	return "Executes a shell command and returns the output. Use with caution."
}

func (s *ShellTool) Execute(ctx context.Context, command string) (string, error) {
	// For Termux/Linux, we run inside 'sh -c'
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// We return the output even on error (it contains stderr)
		return string(output), nil
	}
	
	return string(output), nil
}
