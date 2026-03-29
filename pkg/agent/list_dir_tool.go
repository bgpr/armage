package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ListDirTool struct{}

func (t *ListDirTool) Name() string { return "list_dir" }
func (t *ListDirTool) Description() string {
	return "Lists files and directories. Arguments can be a JSON object like {\"path\": \".\", \"depth\": 1} or a raw path string."
}

type listDirArgs struct {
	Path  string `json:"path"`
	Depth int    `json:"depth"`
}

func (t *ListDirTool) Execute(ctx context.Context, args string) (string, error) {
	var a listDirArgs
	// 1. Try to parse as JSON
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		// 2. Fallback: treat as raw path string
		a.Path = strings.Trim(args, "\"'")
		a.Depth = 1
	}

	if a.Path == "" {
		a.Path = "."
	}
	if a.Depth <= 0 {
		a.Depth = 1
	}

	var result strings.Builder
	err := filepath.WalkDir(a.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate current depth
		rel, _ := filepath.Rel(a.Path, path)
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(os.PathSeparator)) + 1
		}

		if depth > a.Depth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Ignore hidden files and common junk
		if strings.HasPrefix(d.Name(), ".") && d.Name() != "." && d.Name() != ".." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		indent := strings.Repeat("  ", depth)
		if d.IsDir() {
			result.WriteString(fmt.Sprintf("%s%s/\n", indent, d.Name()))
		} else {
			result.WriteString(fmt.Sprintf("%s%s\n", indent, d.Name()))
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(result.String()), nil
}
