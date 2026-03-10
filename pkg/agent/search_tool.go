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
		// Fallback for simple "pattern" call
		a.Pattern = args
		a.Path = "."
	}

	if a.Path == "" {
		a.Path = "."
	}

	// 1. Pre-check path existence for better error messages
	if _, err := os.Stat(a.Path); os.IsNotExist(err) {
		return "", fmt.Errorf("search path %s does not exist. The current directory is where the test/agent is running", a.Path)
	}

	// 2. Run grep -rnIE (recursive, line numbers, ignore binary, extended regex)
	cmd := exec.CommandContext(ctx, "grep", "-rnIE", a.Pattern, a.Path)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// grep returns exit code 1 if no matches are found. In our context, this is a valid result.
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return "No matches found.", nil
		}
		// Any other error is a real failure (e.g., path doesn't exist)
		return string(output), fmt.Errorf("grep failed: %w", err)
	}

	return string(output), nil
}
