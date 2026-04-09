package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type DiffTool struct{}

type diffArgs struct {
	Path    string `json:"path"`
	Find    string `json:"find"`
	Replace string `json:"replace"`
}

func (d *DiffTool) Name() string { return "edit_file_diff" }

func (d *DiffTool) Description() string {
	return "Surgically updates a file. Provide the EXACT 'find' block (from the file, without line numbers) and the 'replace' block."
}

func (d *DiffTool) Execute(ctx context.Context, args string) (string, error) {
	var a diffArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("invalid JSON for diff: %w", err)
	}

	if a.Path == "" || a.Find == "" {
		return "", fmt.Errorf("path and find blocks are required")
	}

	content, err := os.ReadFile(a.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if !strings.Contains(string(content), a.Find) {
		return "", fmt.Errorf("the 'find' block was not found exactly as provided in %s. Check whitespace and line numbers", a.Path)
	}

	newContent := strings.Replace(string(content), a.Find, a.Replace, 1)
	
	tmpPath := a.Path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(newContent), 0644); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, a.Path); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	return fmt.Sprintf("Successfully updated %s", a.Path), nil
}

func (d *DiffTool) Preview(ctx context.Context, args string) (string, error) {
	var a diffArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", err
	}

	content, err := os.ReadFile(a.Path)
	if err != nil {
		return "", err
	}

	if !strings.Contains(string(content), a.Find) {
		return fmt.Sprintf("Diff (PREVIEW ERROR): 'find' block not found in %s", a.Path), nil
	}

	newContent := strings.Replace(string(content), a.Find, a.Replace, 1)
	
	tmpPath := a.Path + ".diff.tmp"
	if err := os.WriteFile(tmpPath, []byte(newContent), 0644); err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)

	cmd := exec.CommandContext(ctx, "diff", "-u", a.Path, tmpPath)
	out, _ := cmd.Output()
	
	if len(out) == 0 {
		return "No changes.", nil
	}

	return string(out), nil
}
