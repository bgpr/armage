package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// ReadTool handles surgical file reading with line numbers and ranges.
type ReadTool struct{}

type readArgs struct {
	Path  string `json:"path"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

func (r *ReadTool) Name() string { return "read_file" }

func (r *ReadTool) Description() string {
	return "Reads a file and returns its content with line numbers. Use 'start' and 'end' for line ranges."
}

func (r *ReadTool) Execute(ctx context.Context, args string) (string, error) {
	var a readArgs
	// Try parsing JSON. If it's a simple string, treat it as the path.
	// LLM Action: read_file({"path": "file.txt"}) or read_file("file.txt")
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		// Clean up potential quotes from the LLM if it's not JSON
		a.Path = args
		a.Path = fmt.Sprintf("%v", args) // Coerce to string
	}

	file, err := os.Open(a.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var output string
	scanner := bufio.NewScanner(file)
	lineNum := 1

	for scanner.Scan() {
		// Apply line range filters (if start or end are provided)
		if a.Start > 0 && lineNum < a.Start {
			lineNum++
			continue
		}
		if a.End > 0 && lineNum > a.End {
			break
		}

		output += fmt.Sprintf("%d | %s\n", lineNum, scanner.Text())
		lineNum++

		// Safety break for context protection (1000 lines max)
		if lineNum > 1000 && a.End == 0 {
			output += "\n[Truncated: File too large. Use line ranges for specific sections.]"
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return output, nil
}
