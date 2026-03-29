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

type SymbolsTool struct{}

func (t *SymbolsTool) Name() string        { return "get_symbols" }
func (t *SymbolsTool) Description() string { return "Lists functions, classes, and types in a file." }

type symbolsArgs struct {
	Path string `json:"path"`
}

func (t *SymbolsTool) Execute(ctx context.Context, args string) (string, error) {
	var a symbolsArgs
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

	// Regex for Go, Python, and some JS/TS symbols
	symbolPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^func\s+(?:\(\w+\s+\*?\w+\)\s+)?(\w+)`), // Go functions
		regexp.MustCompile(`^type\s+(\w+)\s+(?:struct|interface)`), // Go types
		regexp.MustCompile(`^def\s+(\w+)\(`),                       // Python functions
		regexp.MustCompile(`^class\s+(\w+)`),                       // Python/JS classes
		regexp.MustCompile(`^(?:export\s+)?function\s+(\w+)`),      // JS functions
	}

	var symbols []string
	scanner := bufio.NewScanner(file)
	lineNum := 1
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		for _, re := range symbolPatterns {
			if match := re.FindStringSubmatch(line); len(match) > 1 {
				symbols = append(symbols, fmt.Sprintf("%d | %s", lineNum, line))
				break
			}
		}
		lineNum++
	}

	if len(symbols) == 0 {
		return "No symbols found or file type not supported for symbol extraction.", nil
	}

	return strings.Join(symbols, "\n"), nil
}
