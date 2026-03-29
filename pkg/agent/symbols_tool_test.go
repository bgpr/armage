package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSymbolsTool(t *testing.T) {
	tmpFile := "test_symbols.go"
	content := "package main\ntype User struct{}\nfunc (u *User) Hello() {}"
	os.WriteFile(tmpFile, []byte(content), 0644)
	defer os.Remove(tmpFile)

	tool := &SymbolsTool{}

	t.Run("RawString", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), "test_symbols.go")
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if !strings.Contains(res, "type User") {
			t.Errorf("Expected type User, got: %s", res)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), `{"path": "test_symbols.go"}`)
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if !strings.Contains(res, "func (u *User) Hello") {
			t.Errorf("Expected func Hello, got: %s", res)
		}
	})
}
