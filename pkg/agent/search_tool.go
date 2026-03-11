package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type SearchTool struct{}

type searchArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func (s *SearchTool) Name() string { return "grep_search" }

func (s *SearchTool) Description() string {
	return "Searches for a regex pattern in files within a path. Returns matching lines with line numbers."
}

func (s *SearchTool) Execute(ctx context.Context, args string) (string, error) {
	var a searchArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		a.Pattern = args
		a.Path = "."
	}

	if a.Path == "" {
		a.Path = "."
	}

	if _, err := os.Stat(a.Path); os.IsNotExist(err) {
		return "", fmt.Errorf("search path %s does not exist", a.Path)
	}

	// 1. Try ripgrep (rg) first - Faster and respects .gitignore
	if _, err := exec.LookPath("rg"); err == nil {
		cmd := exec.CommandContext(ctx, "rg", "-n", "--column", "--no-heading", a.Pattern, a.Path)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return string(output), nil
		}
		// If rg returns 1, no matches found
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return "No matches found.", nil
		}
		// Any other error, we fall back to grep
	}

	// 2. Fallback to standard grep
	cmd := exec.CommandContext(ctx, "grep", "-rnIE", a.Pattern, a.Path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return "No matches found.", nil
		}
		return string(output), fmt.Errorf("search failed: %w", err)
	}

	return string(output), nil
}
