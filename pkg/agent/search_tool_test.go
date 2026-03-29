package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSearchTool(t *testing.T) {
	tmpDir := "test_search"
	os.Mkdir(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	f1 := tmpDir + "/file1.go"
	os.WriteFile(f1, []byte("package main\nfunc Hello() {}\n"), 0644)

	tool := &SearchTool{}

	t.Run("RawString", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), "Hello")
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		// Should find it in file1.go by searching from current dir (or where test runs)
		if !strings.Contains(res, "file1.go") {
			t.Errorf("Expected result from file1.go, got: %s", res)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), `{"pattern": "Hello", "path": "test_search"}`)
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if !strings.Contains(res, "file1.go:2:") {
			t.Errorf("Expected filename and line number, got: %s", res)
		}
	})
}
