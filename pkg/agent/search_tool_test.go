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
	
	f2 := tmpDir + "/file2.txt"
	os.WriteFile(f2, []byte("This is a test file with the word 'Hello' in it.\n"), 0644)

	tool := &SearchTool{}

	// Test case 1: Pattern found in multiple files
	res, err := tool.Execute(context.Background(), `{"pattern": "Hello", "path": "test_search"}`)
	if err != nil {
		t.Fatalf("SearchTool failed: %v", err)
	}

	if !strings.Contains(res, "file1.go") || !strings.Contains(res, "file2.txt") {
		t.Errorf("Expected results from both files, got: %s", res)
	}
	if !strings.Contains(res, "file1.go:2:") || !strings.Contains(res, "file2.txt:1:") {
		t.Errorf("Expected filename and line numbers, got: %s", res)
	}

	// Test case 2: Pattern not found
	res, err = tool.Execute(context.Background(), `{"pattern": "DoesNotExist", "path": "test_search"}`)
	if err != nil {
		t.Fatalf("SearchTool should not return error on no match: %v", err)
	}
	if res != "No matches found." {
		t.Errorf("Expected 'No matches found.', got: %s", res)
	}
}
