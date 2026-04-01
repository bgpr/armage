package agent

import (
	"context"
	"os"
	"testing"
)

func TestPinTool(t *testing.T) {
	tmpFile := "test_pin_tool.txt"
	content := "Secret context"
	os.WriteFile(tmpFile, []byte(content), 0644)
	defer os.Remove(tmpFile)

	llm := &MockLLM{}
	reg := NewRegistry()
	a := New(llm, reg)
	
	tool := &PinTool{Agent: a}

	t.Run("JSON", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), `{"path": "test_pin_tool.txt"}`)
		if err != nil {
			t.Fatalf("Pin failed: %v", err)
		}
		if len(a.PinnedFiles) != 1 || a.PinnedFiles[0] != "test_pin_tool.txt" {
			t.Errorf("File not pinned: %v", a.PinnedFiles)
		}
	})

	t.Run("RawString", func(t *testing.T) {
		// Reset
		a.PinnedFiles = []string{}
		_, err := tool.Execute(context.Background(), "test_pin_tool.txt")
		if err != nil {
			t.Fatalf("Pin failed: %v", err)
		}
		if len(a.PinnedFiles) != 1 {
			t.Errorf("File not pinned via raw string")
		}
	})
}
