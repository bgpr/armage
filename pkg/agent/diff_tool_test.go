package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestDiffTool(t *testing.T) {
	tmpFile := "test_diff.txt"
	content := "Line 1\nLine 2\nLine 3"
	os.WriteFile(tmpFile, []byte(content), 0644)
	defer os.Remove(tmpFile)

	tool := &DiffTool{}

	// Test Successful Replace
	res, err := tool.Execute(context.Background(), `{"path": "test_diff.txt", "find": "Line 2", "replace": "Updated 2"}`)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.Contains(res, "Successfully updated") {
		t.Errorf("Unexpected result: %s", res)
	}

	updated, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(updated), "Updated 2") {
		t.Errorf("File not updated: %s", string(updated))
	}

	// Test Not Found
	res, err = tool.Execute(context.Background(), `{"path": "test_diff.txt", "find": "Missing", "replace": "New"}`)
	if err == nil || !strings.Contains(err.Error(), "was not found exactly") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}
