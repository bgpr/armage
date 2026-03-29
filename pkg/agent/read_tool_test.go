package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestReadTool(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3"
	tmpFile := "test_read.txt"
	os.WriteFile(tmpFile, []byte(content), 0644)
	defer os.Remove(tmpFile)

	tool := &ReadTool{}

	t.Run("RawString", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), "test_read.txt")
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if !strings.Contains(res, "1 | Line 1") {
			t.Errorf("Expected line 1, got: %s", res)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), `{"path": "test_read.txt", "start": 2, "end": 2}`)
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if strings.Contains(res, "Line 1") || !strings.Contains(res, "2 | Line 2") {
			t.Errorf("Range failed, got: %s", res)
		}
	})
}
