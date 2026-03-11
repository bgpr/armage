package agent

import (
	"context"
	"testing"
)

func TestContextTrimming(t *testing.T) {
	reg := NewRegistry()
	llm := &MockLLM{Response: "Thought: I am thinking."}
	a := New(llm, reg)

	// Setup: System prompt + high MaxHistory
	a.AddSystemPrompt("You are Armage.")
	a.MaxHistory = 5 // Keep system + 4 latest messages (2 turns)

	// Step 1: history [system, user1, assistant1]
	a.Step(context.Background(), "user 1")
	if len(a.History) != 3 {
		t.Errorf("Step 1: expected length 3, got %d", len(a.History))
	}

	// Step 2: history [system, user1, assistant1, user2, assistant2] -> length 5
	a.Step(context.Background(), "user 2")
	if len(a.History) != 5 {
		t.Errorf("Step 2: expected length 5, got %d", len(a.History))
	}

	// Step 3: history [system, user1, assistant1, user2, assistant2, user3, assistant3] -> length 7
	// Trimming should kick in and keep [system, user2, assistant2, user3, assistant3] -> length 5
	a.Step(context.Background(), "user 3")
	
	if len(a.History) != 5 {
		t.Errorf("Step 3: expected trimmed length 5, got %d", len(a.History))
	}

	// Verify System prompt is still there
	if a.History[0].Role != "system" {
		t.Errorf("System prompt was lost during trimming")
	}

	// Verify it kept the LATEST turns
	if a.History[len(a.History)-1].Content != "Thought: I am thinking." {
		t.Errorf("Latest turn was not preserved")
	}
}
