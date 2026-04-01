package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestWriteTool(t *testing.T) {
	tool := &WriteTool{}
	tmpFile := "test_write.txt"
	defer os.Remove(tmpFile)

	t.Run("NewFile", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), `{"path": "test_write.txt", "content": "Hello Write"}`)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if !strings.Contains(res, "Successfully wrote") {
			t.Errorf("Unexpected result: %s", res)
		}
		
		content, _ := os.ReadFile(tmpFile)
		if string(content) != "Hello Write" {
			t.Errorf("Content mismatch: %s", string(content))
		}
	})

	t.Run("InvalidPath", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), `{"path": "/proc/invalid", "content": "fail"}`)
		if err == nil {
			t.Errorf("Expected error for invalid path")
		}
	})
}
