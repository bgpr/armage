package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/user/armage/pkg/provider"
)

type MockLLM struct {
	Response string
}

func (m *MockLLM) Chat(ctx context.Context, messages []provider.Message) (string, provider.Usage, error) {
	return m.Response, provider.Usage{TotalTokens: 100}, nil
}

func TestAgentStep(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&MockTool{}) // Registers 'echo'

	llm := &MockLLM{
		Response: `Thought: I will echo hello.
Action: echo("hello")`,
	}

	a := New(llm, reg)
	res, err := a.Step(context.Background(), "Start")
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	if res.Thought != "I will echo hello." {
		t.Errorf("Expected thought 'I will echo hello.', got: %s", res.Thought)
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
	// Observations are now multi-part: Observations:\nObservation 1 (echo):\n"hello"
	if !strings.Contains(obs, "Observation 1 (echo):") || !strings.Contains(obs, "\"hello\"") {
		t.Errorf("Expected multi-part Observation, got: %s", obs)
	}
}

func TestAgentStepTransient(t *testing.T) {
	reg := NewRegistry()
	llm := &MockLLM{Response: "Thought: I am transient."}
	a := New(llm, reg)
	a.History = []provider.Message{{Role: "user", Content: "Existing"}}

	res, err := a.StepTransient(context.Background(), "Nudge")
	if err != nil {
		t.Fatalf("StepTransient failed: %v", err)
	}

	if res.Thought != "I am transient." {
		t.Errorf("Unexpected thought: %s", res.Thought)
	}

	// Verify history is UNCHANGED
	if len(a.History) != 1 || a.History[0].Content != "Existing" {
		t.Errorf("History was modified by transient step: %v", a.History)
	}
}
