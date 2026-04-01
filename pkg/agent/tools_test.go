package agent

import (
	"context"
	"testing"
)

func TestToolMeta(t *testing.T) {
	tools := []Tool{
		&ShellTool{},
		&ReadTool{},
		&WriteTool{},
		&SearchTool{},
		&DiffTool{},
		&ListDirTool{},
		&SymbolsTool{},
		&ApplyPatchTool{},
		&PinTool{},
		&PlanningTool{},
	}

	for _, tool := range tools {
		if tool.Name() == "" {
			t.Errorf("Tool %T has empty name", tool)
		}
		if tool.Description() == "" {
			t.Errorf("Tool %T has empty description", tool)
		}
	}
}

// MockTool for testing
type MockTool struct{}
func (t *MockTool) Name() string { return "echo" }
func (t *MockTool) Description() string { return "echos args" }
func (t *MockTool) Execute(ctx context.Context, args string) (string, error) {
	return args, nil
}
