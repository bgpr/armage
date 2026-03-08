package agent

import (
	"context"
	"os"
	"testing"
)

func TestWriteTool(t *testing.T) {
	tmpFile := "test_write.txt"
	defer os.Remove(tmpFile)

	// WriteTool doesn't exist yet - RED PHASE
	tool := &WriteTool{}

	// Test 1: Write new file
	content := "Hello Armage"
	res, err := tool.Execute(context.Background(), `{"path": "test_write.txt", "content": "Hello Armage"}`)
	if err != nil {
		t.Fatalf("WriteTool failed: %v", err)
	}

	if res != "Successfully wrote to test_write.txt" {
		t.Errorf("Unexpected result message: %s", res)
	}

	data, _ := os.ReadFile(tmpFile)
	if string(data) != content {
		t.Errorf("Expected '%s', got: '%s'", content, string(data))
	}
}
