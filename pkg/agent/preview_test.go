package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestWriteToolPreview(t *testing.T) {
	tool := &WriteTool{}
	path := "test_preview.txt"
	os.WriteFile(path, []byte("line 1\nline 2\n"), 0644)
	defer os.Remove(path)

	t.Run("GenerateDiff", func(t *testing.T) {
		args := `{"path": "test_preview.txt", "content": "line 1\nline 2 modified\n"}`
		preview, err := tool.Preview(context.Background(), args)
		if err != nil {
			t.Fatalf("Preview failed: %v", err)
		}

		if !strings.Contains(preview, "-line 2") {
			t.Errorf("Expected diff to contain removal of line 2, got: %s", preview)
		}
		if !strings.Contains(preview, "+line 2 modified") {
			t.Errorf("Expected diff to contain addition of modified line 2, got: %s", preview)
		}
	})
}
