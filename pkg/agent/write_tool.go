package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

	// 1. Explicit check for directory existence
	dir := filepath.Dir(a.Path)
	if dir != "." {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			// Directory doesn't exist, create it safely
			if err := os.MkdirAll(dir, 0755); err != nil {
				return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}

	// 2. Atomic write: Write to .tmp first to prevent corruption
	tmpPath := a.Path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(a.Content), 0644); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	// 3. Move .tmp to target (Atomic Move)
	if err := os.Rename(tmpPath, a.Path); err != nil {
		// Cleanup tmp on failure
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to finalize write (atomic move failed): %w", err)
	}

	return fmt.Sprintf("Successfully wrote to %s", a.Path), nil
}
