package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestReadTool(t *testing.T) {
	// 1. Setup a test file
	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	tmpFile := "test_read.txt"
	os.WriteFile(tmpFile, []byte(content), 0644)
	defer os.Remove(tmpFile)

	// ReadTool doesn't exist yet - RED PHASE
	tool := &ReadTool{}

	// Test 1: Read entire file (with line numbers)
	res, err := tool.Execute(context.Background(), `{"path": "test_read.txt"}`)
	if err != nil {
		t.Fatalf("ReadTool failed: %v", err)
	}
	if !strings.Contains(res, "1 | Line 1") {
		t.Errorf("Expected line numbers, got: %s", res)
	}

	// Test 2: Read specific range (lines 2-3)
	// Arguments are JSON strings from the LLM
	res, err = tool.Execute(context.Background(), `{"path": "test_read.txt", "start": 2, "end": 3}`)
	if err != nil {
		t.Fatalf("ReadTool range failed: %v", err)
	}
	
	if strings.Contains(res, "Line 1") {
		t.Errorf("Should not contain Line 1, got: %s", res)
	}
	if !strings.Contains(res, "2 | Line 2") || !strings.Contains(res, "3 | Line 3") {
		t.Errorf("Range failed to include lines 2 and 3, got: %s", res)
	}
	if strings.Contains(res, "Line 4") {
		t.Errorf("Should not contain Line 4, got: %s", res)
	}
}
