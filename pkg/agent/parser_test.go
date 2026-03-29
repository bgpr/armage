package agent

import (
	"testing"
)

func TestParseReAct(t *testing.T) {
	input := `Thought: I should check the files in the current directory.
Action: ls({"path": "."})`

	thought, calls, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	expectedThought := "I should check the files in the current directory."
	if thought != expectedThought {
		t.Errorf("Expected thought '%s', got: '%s'", expectedThought, thought)
	}
	if len(calls) == 0 || calls[0].Name != "ls" {
		t.Errorf("Expected tool 'ls', got: '%v'", calls)
	}
	if calls[0].Args != `{"path": "."}` {
		t.Errorf("Expected args '{\"path\": \".\"}', got: '%s'", calls[0].Args)
	}
}

func TestParseMultiAction(t *testing.T) {
	input := "Thought: Multiple actions.\nAction: tool1(\"arg1\")\nAction: tool2(\"arg2\")"
	_, calls, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(calls) != 2 {
		t.Errorf("Expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "tool1" || calls[1].Name != "tool2" {
		t.Errorf("Tool name mismatch: %+v", calls)
	}
}
