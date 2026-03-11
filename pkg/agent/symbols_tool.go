package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// SymbolsTool extracts functions, classes, and types from files.
type SymbolsTool struct{}

type symbolsArgs struct {
	Path string `json:"path"`
}

func (s *SymbolsTool) Name() string { return "get_symbols" }

func (s *SymbolsTool) Description() string {
	return "Lists functions, classes, and types in a file. Very efficient for mapping out code."
}

func (s *SymbolsTool) Execute(ctx context.Context, args string) (string, error) {
	var a symbolsArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		a.Path = args
	}

	if a.Path == "" {
		return "", fmt.Errorf("path is required for get_symbols")
	}

	file, err := os.Open(a.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Define common patterns for code symbols
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?([A-Za-z0-9_]*)`),               // Go functions
		regexp.MustCompile(`^type\s+([A-Za-z0-9_]*)\s+(?:struct|interface)`),        // Go types
		regexp.MustCompile(`^def\s+([a-z0-9_]+)\s*\(`),                               // Python functions
		regexp.MustCompile(`^class\s+([A-Z][A-Za-z0-9_]*)\s*[:(]`),                  // Python classes
		regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+([a-z0-9_]+)`),    // JS/TS functions
		regexp.MustCompile(`^(?:pub\s+)?fn\s+([a-z0-9_]+)`),                         // Rust functions
		regexp.MustCompile(`^(?:pub\s+)?(?:struct|enum|trait|impl)\s+([A-Z]\w*)`),    // Rust types
		regexp.MustCompile(`^[a-zA-Z_]\w*\s+[a-zA-Z_]\w*\s*\(`),                      // C/C++ Functions (basic)
		regexp.MustCompile(`^(?:public|private|protected|internal)\s+class\s+(\w+)`), // Java/Kotlin classes
		regexp.MustCompile(`^#+\s+.*`),                                               // Markdown headers
		regexp.MustCompile(`^[a-z0-9_]+\s*\(\)\s*\{`),                                // Shell functions
	}

	var output strings.Builder
	scanner := bufio.NewScanner(file)
	lineNum := 1

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			// Don't skip Markdown headers which start with #
			if !strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "##") {
				lineNum++
				continue
			}
		}

		for _, p := range patterns {
			if p.MatchString(trimmed) {
				output.WriteString(fmt.Sprintf("%d | %s\n", lineNum, trimmed))
				break
			}
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if output.Len() == 0 {
		return "No major symbols found in this file.", nil
	}

	return strings.TrimSuffix(output.String(), "\n"), nil
}
