package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ListDirTool provides a token-efficient, structured directory tree.
type ListDirTool struct{}

type listDirArgs struct {
	Path  string `json:"path"`
	Depth int    `json:"depth"` // 0 for current level, > 0 for nested
}

func (l *ListDirTool) Name() string { return "list_dir" }

func (l *ListDirTool) Description() string {
	return "Lists files and directories in a path. Use 'depth' to see nested items (max 3)."
}

func (l *ListDirTool) Execute(ctx context.Context, args string) (string, error) {
	var a listDirArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		// Fallback for simple path string
		a.Path = args
	}

	if a.Path == "" {
		a.Path = "."
	}

	// Limit depth for context protection
	if a.Depth > 3 {
		a.Depth = 3
	}
	if a.Depth < 0 {
		a.Depth = 0
	}

	output, err := l.walk(a.Path, "", 0, a.Depth)
	if err != nil {
		return "", err
	}

	if output == "" {
		return "Directory is empty or all items were ignored.", nil
	}

	return strings.TrimSuffix(output, "\n"), nil
}

func (l *ListDirTool) walk(path, indent string, currentDepth, maxDepth int) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden and common bulky directories
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
			continue
		}

		if entry.IsDir() {
			builder.WriteString(fmt.Sprintf("%s%s/\n", indent, name))
			if currentDepth < maxDepth {
				subOutput, err := l.walk(filepath.Join(path, name), indent+"  ", currentDepth+1, maxDepth)
				if err != nil {
					return "", err
				}
				builder.WriteString(subOutput)
			}
		} else {
			builder.WriteString(fmt.Sprintf("%s%s\n", indent, name))
		}
	}

	return builder.String(), nil
}
