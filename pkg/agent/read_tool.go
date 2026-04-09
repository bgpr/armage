package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ReadTool struct{}

func (t *ReadTool) Name() string        { return "read_file" }
func (t *ReadTool) Description() string { return "Reads a file with line numbers. Supports 'start' and 'end' lines." }

type readArgs struct {
	Path  string `json:"path"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

func (t *ReadTool) Execute(ctx context.Context, args string) (string, error) {
	var a readArgs
	// 1. Try to parse as JSON
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		// 2. Fallback: treat as raw path string
		a.Path = strings.Trim(args, "\"'")
	}

	if a.Path == "" {
		return "", fmt.Errorf("missing path")
	}

	file, err := os.Open(a.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 1
	for scanner.Scan() {
		if (a.Start == 0 || lineNum >= a.Start) && (a.End == 0 || lineNum <= a.End) {
			lines = append(lines, fmt.Sprintf("%d | %s", lineNum, scanner.Text()))
		}
		lineNum++
		if a.End > 0 && lineNum > a.End {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return Truncate(strings.Join(lines, "\n"), 5000), nil
}

func (t *ReadTool) Preview(ctx context.Context, args string) (string, error) {
	var a readArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		a.Path = strings.Trim(args, "\"'")
	}
	return fmt.Sprintf("Read file: %s", a.Path), nil
}
