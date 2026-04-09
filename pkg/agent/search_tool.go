package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type SearchTool struct{}

func (t *SearchTool) Name() string        { return "grep_search" }
func (t *SearchTool) Description() string { return "Recursively searches for a pattern in files within a path." }

type searchArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func (t *SearchTool) Execute(ctx context.Context, args string) (string, error) {
	var a searchArgs
	// 1. Try to parse as JSON
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		// 2. Fallback: If it's a simple string, it's ambiguous, but usually it's the pattern
		a.Pattern = strings.Trim(args, "\"'")
		a.Path = "."
	}

	if a.Pattern == "" {
		return "", fmt.Errorf("missing pattern")
	}
	if a.Path == "" {
		a.Path = "."
	}

	// Use grep -rn (recursive, line numbers)
	// We use -E for extended regex support
	cmd := exec.CommandContext(ctx, "grep", "-rnE", "--exclude-dir=.git", a.Pattern, a.Path)
	out, _ := cmd.CombinedOutput()
	
	result := string(out)
	if result == "" {
		return "No matches found.", nil
	}

	return Truncate(result, 5000), nil
}

func (t *SearchTool) Preview(ctx context.Context, args string) (string, error) {
	var a searchArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		a.Pattern = strings.Trim(args, "\"'")
		a.Path = "."
	}
	return fmt.Sprintf("Search for '%s' in %s", a.Pattern, a.Path), nil
}
