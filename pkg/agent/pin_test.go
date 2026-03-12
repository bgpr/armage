package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestContextPinning(t *testing.T) {
	// 1. Setup mock file to pin
	pinnedFile := "pinned.txt"
	os.WriteFile(pinnedFile, []byte("CRITICAL DATA"), 0644)
	defer os.Remove(pinnedFile)

	reg := NewRegistry()
	llm := &MockLLM{Response: "Thought: Thinking."}
	a := New(llm, reg)

	a.AddSystemPrompt("You are Armage.")
	a.MaxHistory = 5 // Keep system + 4 latest

	// 2. Pin the file
	err := a.PinFile(pinnedFile)
	if err != nil {
		t.Fatalf("Failed to pin file: %v", err)
	}

	// History should now have: [System, PinnedFileMsg]
	if len(a.History) != 2 || !strings.Contains(a.History[1].Content, "CRITICAL DATA") {
		t.Errorf("Expected pinned content in history, got: %v", a.History)
	}

	// 3. Trigger trimming by adding many turns
	// Step 1: [System, Pin, user1, assistant1] -> 4
	a.Step(context.Background(), "user 1")
	// Step 2: [System, Pin, user1, assistant1, user2, assistant2] -> 6 (Trigger!)
	a.Step(context.Background(), "user 2")

	// After Step 2, history is 6 messages. Max is 5.
	// Expected kept: [System, Pinned, assistant1, user2, assistant2] (or similar latest turns)
	// Crucially, Pinned MUST stay.
	
	if len(a.History) != 5 {
		t.Errorf("Expected history length 5, got %d", len(a.History))
	}

	// Verify System and Pin are still at the front
	if a.History[0].Role != "system" {
		t.Errorf("System prompt lost")
	}
	if !strings.Contains(a.History[1].Content, "pinned.txt") {
		t.Errorf("Pinned file lost during trimming. History: %v", a.History)
	}
}
