package agent

import (
	"os"
	"testing"

	"github.com/user/armage/pkg/provider"
)

func TestStatePersistence(t *testing.T) {
	tmpFile := "test_state.json"
	defer os.Remove(tmpFile)

	// Create an agent with history
	a := New(&MockLLM{}, NewRegistry())
	a.History = []provider.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Thought: Hi!"},
	}

	// 1. Save state - RED PHASE (Save doesn't exist yet)
	err := a.Save(tmpFile)
	if err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// 2. Load into new agent - RED PHASE (Load doesn't exist yet)
	b := New(&MockLLM{}, NewRegistry())
	err = b.Load(tmpFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if len(b.History) != 2 {
		t.Fatalf("Expected 2 messages in history, got: %d", len(b.History))
	}
	if b.History[1].Content != "Thought: Hi!" {
		t.Errorf("History mismatch: %s", b.History[1].Content)
	}
}
