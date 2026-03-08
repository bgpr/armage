package agent

import (
	"context"
	"testing"
)

type MockTool struct{}

func (m *MockTool) Name() string        { return "echo" }
func (m *MockTool) Description() string { return "returns the input" }
func (m *MockTool) Execute(ctx context.Context, args string) (string, error) {
	return args, nil
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	mock := &MockTool{}
	r.Register(mock)

	tool, exists := r.Get("echo")
	if !exists {
		t.Fatal("Tool 'echo' should exist")
	}

	res, _ := tool.Execute(context.Background(), "hello")
	if res != "hello" {
		t.Errorf("Expected 'hello', got: %s", res)
	}
}
