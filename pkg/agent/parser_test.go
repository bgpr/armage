package agent

import (
	"testing"
)

func TestParseReAct_BalancedParens(t *testing.T) {
	input := `Thought: I need to run a subshell.
Action: shell("echo $(date +%Y)")`

	_, calls, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(calls) == 0 || calls[0].Args != `"echo $(date +%Y)"` {
		t.Errorf("Balanced paren parsing failed, got: %v", calls)
	}
}

func TestParseXML_MultiParam(t *testing.T) {
	input := `Thought: I will list files.
<tool_call>
<function=list_dir>
<parameter=path>./pkg</parameter>
<parameter=depth>2</parameter>
</function>
</tool_call>`

	_, calls, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(calls) == 0 || calls[0].Name != "list_dir" {
		t.Fatalf("XML parsing failed, got: %v", calls)
	}

	// Verify both parameters are in the JSON string
	expected := `{"depth":"2","path":"./pkg"}`
	if calls[0].Args != expected {
		t.Errorf("Expected args %s, got %s", expected, calls[0].Args)
	}
}

func TestParse_JSON(t *testing.T) {
	input := `{"thought": "I am thinking in JSON.", "tool_calls": [{"name": "shell", "args": "ls"}]}`
	thought, calls, err := Parse(input)
	if err != nil {
		t.Fatalf("JSON parse failed: %v", err)
	}

	if thought != "I am thinking in JSON." {
		t.Errorf("Thought mismatch: %s", thought)
	}
	if len(calls) == 0 || calls[0].Name != "shell" {
		t.Errorf("Tool call mismatch: %v", calls)
	}
}

func TestParse_FallbackToThought(t *testing.T) {
	input := "I am a direct answer without markers."
	thought, calls, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if thought != input {
		t.Errorf("Expected fallback to full input, got: %s", thought)
	}
	if len(calls) != 0 {
		t.Errorf("Expected 0 calls, got %d", len(calls))
	}
}
