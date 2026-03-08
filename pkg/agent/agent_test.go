package agent

import (
	"context"
	"testing"
)

type MockLLM struct {
	Response string
}

func (m *MockLLM) Chat(ctx context.Context, messages []Message) (string, error) {
	return m.Response, nil
}

func TestAgentStep(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&MockTool{}) // Registers 'echo'

	llm := &MockLLM{
		Response: `Thought: I will echo hello.
Action: echo("hello")`,
	}

	a := New(llm, reg)
	_, err := a.Step(context.Background(), "Start")
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	// Verify history
	// 1. user: Start
	// 2. assistant: Thought/Action
	// 3. user: Observation
	if len(a.History) != 3 {
		t.Fatalf("Expected 3 messages in history, got: %d", len(a.History))
	}

	obs := a.History[2].Content
	// In MockTool, Execute returns args as-is.
	// If the LLM response has Action: echo("hello"), toolArgs is "\"hello\""
	if obs != "Observation: \"hello\"" {
		t.Errorf("Expected Observation: \"hello\", got: %s", obs)
	}
}
