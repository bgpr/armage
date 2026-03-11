package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSymbolsTool(t *testing.T) {
	// 1. Setup a mock Go file
	tmpFile := "test_symbols.go"
	content := `package main

import "fmt"

type User struct {
	Name string
}

func (u *User) GetName() string {
	return u.Name
}

func main() {
	fmt.Println("Hello")
}
`
	os.WriteFile(tmpFile, []byte(content), 0644)
	defer os.Remove(tmpFile)

	tool := &SymbolsTool{}

	// Test: Get symbols from the mock file
	res, err := tool.Execute(context.Background(), `{"path": "test_symbols.go"}`)
	if err != nil {
		t.Fatalf("SymbolsTool failed: %v", err)
	}

	// Expect to find the struct and the functions
	expectedSymbols := []string{
		"type User",
		"func (u *User) GetName",
		"func main",
	}

	for _, s := range expectedSymbols {
		if !strings.Contains(res, s) {
			t.Errorf("Expected to find symbol '%s' in output, but didn't. Output:\n%s", s, res)
		}
	}

	// Check for line numbers
	if !strings.Contains(res, "5 | type User") {
		t.Errorf("Expected line number for 'type User', got:\n%s", res)
	}

	// 2. Test Rust
	rustPath := "test_symbols.rs"
	os.WriteFile(rustPath, []byte("pub fn calculate() {}\nstruct Data {}"), 0644)
	defer os.Remove(rustPath)
	res, _ = tool.Execute(context.Background(), rustPath)
	if !strings.Contains(res, "fn calculate") || !strings.Contains(res, "struct Data") {
		t.Errorf("Rust symbols not found, got:\n%s", res)
	}

	// 3. Test Markdown
	mdPath := "test_symbols.md"
	os.WriteFile(mdPath, []byte("# Main Title\n## Sub Title"), 0644)
	defer os.Remove(mdPath)
	res, _ = tool.Execute(context.Background(), mdPath)
	if !strings.Contains(res, "1 | # Main Title") || !strings.Contains(res, "2 | ## Sub Title") {
		t.Errorf("Markdown symbols not found, got:\n%s", res)
	}

	// 4. Test Shell
	shPath := "test_symbols.sh"
	os.WriteFile(shPath, []byte("setup() {\n  echo hello\n}"), 0644)
	defer os.Remove(shPath)
	res, _ = tool.Execute(context.Background(), shPath)
	if !strings.Contains(res, "1 | setup() {") {
		t.Errorf("Shell function not found, got:\n%s", res)
	}
}
