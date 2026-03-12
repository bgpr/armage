package agent

import (
	"context"
	"strings"
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

func TestContextSummarization(t *testing.T) {
	reg := NewRegistry()
	// Mock LLM returns a summary when it sees the summarization prompt
	llm := &MockMultiStepLLM{
		Responses: []string{
			"Thought: I am Step 1.\nAction: echo(\"1\")",
			"Thought: I am Step 2.\nAction: echo(\"2\")",
			"Thought: I am Step 3.\nAction: echo(\"3\")",
			"SUMMARY: The conversation so far involved three steps of echoing numbers.",
		},
	}
	a := New(llm, reg)
	a.MaxHistory = 4 // Very tight: [System, Summary, UserLast, AssistantLast]

	a.AddSystemPrompt("You are Armage.")
	
	// Step 1 & 2 will fill it up
	a.Step(context.Background(), "do 1")
	a.Step(context.Background(), "do 2")
	
	// Step 3 should trigger summarization
	// Current history before Step 3 logic: [System, user1, assistant1, user2, assistant2, user3] -> length 6
	a.Step(context.Background(), "do 3")

	// Verify history contains a "Summary" message
	foundSummary := false
	for _, msg := range a.History {
		if strings.Contains(msg.Content, "SUMMARY:") || strings.Contains(msg.Content, "summary of the conversation") {
			foundSummary = true
			break
		}
	}

	if !foundSummary {
		t.Errorf("No summary found in history after trimming. History: %v", a.History)
	}

	// Verify System prompt was preserved
	if a.History[0].Role != "system" {
		t.Errorf("System prompt lost during summarization")
	}
}
