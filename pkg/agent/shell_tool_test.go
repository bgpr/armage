package agent

import (
	"context"
	"strings"
	"testing"
)

func TestShellTool(t *testing.T) {
	tool := &ShellTool{}

	t.Run("RawString", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), "pwd")
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if !strings.Contains(res, "/") {
			t.Errorf("Unexpected output: %s", res)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), `{"command": "pwd"}`)
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if !strings.Contains(res, "/") {
			t.Errorf("Unexpected output: %s", res)
		}
	})
}
