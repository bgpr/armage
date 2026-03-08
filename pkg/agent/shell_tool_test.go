package agent

import (
	"context"
	"testing"
)

func TestShellTool(t *testing.T) {
	// ShellTool doesn't exist yet - RED PHASE
	tool := &ShellTool{}

	// Test a simple command
	res, err := tool.Execute(context.Background(), "echo 'hello world'")
	if err != nil {
		t.Fatalf("ShellTool execution failed: %v", err)
	}

	if res != "hello world\n" {
		t.Errorf("Expected 'hello world\n', got: '%s'", res)
	}
}
