package agent

import (
	"testing"
)

func TestParseReAct(t *testing.T) {
	input := `Thought: I should check the files in the current directory.
Action: ls({"path": "."})`

	thought, tool, args, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	expectedThought := "I should check the files in the current directory."
	if thought != expectedThought {
		t.Errorf("Expected thought '%s', got: '%s'", expectedThought, thought)
	}
	if tool != "ls" {
		t.Errorf("Expected tool 'ls', got: '%s'", tool)
	}
	if args != `{"path": "."}` {
		t.Errorf("Expected args '{\"path\": \".\"}', got: '%s'", args)
	}
}
