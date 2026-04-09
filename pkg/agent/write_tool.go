package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type WriteTool struct{}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (w *WriteTool) Name() string { return "write_file" }

func (w *WriteTool) Description() string {
	return "Writes content to a file. This is an atomic operation. Only creates directories if they don't exist."
}

func (w *WriteTool) Execute(ctx context.Context, args string) (string, error) {
	var a writeArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("invalid JSON arguments for write_file: %w", err)
	}

	if a.Path == "" {
		return "", fmt.Errorf("path is required for write_file")
	}

	dir := filepath.Dir(a.Path)
	if dir != "." {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}

	tmpPath := a.Path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(a.Content), 0644); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, a.Path); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to finalize write (atomic move failed): %w", err)
	}

	return fmt.Sprintf("Successfully wrote to %s", a.Path), nil
}

func (w *WriteTool) Preview(ctx context.Context, args string) (string, error) {
	var a writeArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", err
	}

	// 1. If file doesn't exist, the preview is just the full content
	if _, err := os.Stat(a.Path); os.IsNotExist(err) {
		return fmt.Sprintf("New file: %s\n---\n%s", a.Path, a.Content), nil
	}

	// 2. Use 'diff' command for existing files
	tmpPath := a.Path + ".new.tmp"
	if err := os.WriteFile(tmpPath, []byte(a.Content), 0644); err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)

	cmd := exec.CommandContext(ctx, "diff", "-u", a.Path, tmpPath)
	out, _ := cmd.Output() // diff returns non-zero if files differ, ignore err
	
	if len(out) == 0 {
		return "No changes.", nil
	}

	return string(out), nil
}
