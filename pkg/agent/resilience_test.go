package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/user/armage/pkg/provider"
)

// TestParser_SOTA_Resilience verifies the prefix-agnostic and inference logic.
func TestParser_SOTA_Resilience(t *testing.T) {
	t.Run("PrefixAgnosticJSON", func(t *testing.T) {
		input := `{ "command": "ls -la" }` // No "Action:" or "Thought:"
		_, calls, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if len(calls) == 0 || calls[0].Name != "shell" {
			t.Errorf("Failed to infer 'shell' from 'command' key, got: %v", calls)
		}
	})

	t.Run("Inference_ListDir", func(t *testing.T) {
		input := `{ "path": "./pkg", "depth": 2 }`
		_, calls, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if len(calls) == 0 || calls[0].Name != "list_dir" {
			t.Errorf("Failed to infer 'list_dir' from 'path' key, got: %v", calls)
		}
	})

	t.Run("Inference_Grep", func(t *testing.T) {
		input := `{ "path": ".", "pattern": "TODO" }`
		_, calls, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if len(calls) == 0 || calls[0].Name != "grep_search" {
			t.Errorf("Failed to infer 'grep_search' from 'pattern' key, got: %v", calls)
		}
	})

	t.Run("Inference_Write", func(t *testing.T) {
		input := `{ "path": "test.txt", "content": "hello" }`
		_, calls, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if len(calls) == 0 || calls[0].Name != "write_file" {
			t.Errorf("Failed to infer 'write_file' from 'content' key, got: %v", calls)
		}
	})
}

// TestAgent_DynamicTrimming verifies that the agent respects the LLM's context window.
func TestAgent_DynamicTrimming(t *testing.T) {
	reg := NewRegistry()
	// Mock LLM with a tiny 100-token window
	llm := &MockSOTA_LLM{
		Limit: 100,
	}
	a := New(llm, reg)
	
	// Add a message that definitely exceeds 75 tokens
	// 1000 repetitions of a string will definitely be > 300 chars (75*4)
	bigMessage := strings.Repeat("Force Trim Context Now! ", 1000) 
	// Use Step to trigger natural trimming logic
	a.History = append(a.History, provider.Message{Role: "system", Content: "Init"})
	_, err := a.Step(context.Background(), bigMessage)
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	
	// History should have been trimmed during Step
	if len(a.History) >= 4 { // Prefix + Summary + BigMsg + Response
		// Should have been summarized
		hasSummary := false
		for _, m := range a.History {
			if strings.Contains(m.Content, "Previous Conversation Summary") {
				hasSummary = true
				break
			}
		}
		if !hasSummary {
			t.Error("History should have been summarized due to token limit")
		}
	}
}

// TestTool_Truncation verifies that discovery tools cap their output.
func TestTool_Truncation(t *testing.T) {
	longOutput := strings.Repeat("A", 10000)
	truncated := Truncate(longOutput, 5000)
	
	if len(truncated) > 5500 { // 5000 + buffer for the "(truncated)" message
		t.Errorf("Truncation failed, length: %d", len(truncated))
	}
	if !strings.Contains(truncated, "truncated") {
		t.Error("Truncated output missing warning message")
	}
}

// MockSOTA_LLM implements the full LLM interface for testing resilience.
type MockSOTA_LLM struct {
	Limit int
}
func (m *MockSOTA_LLM) Model() string { return "mock" }
func (m *MockSOTA_LLM) ContextWindow() int { return m.Limit }
func (m *MockSOTA_LLM) Chat(ctx context.Context, msgs []provider.Message) (string, provider.Usage, error) {
	return "SUMMARY: Achieved goal.", provider.Usage{TotalTokens: 10}, nil
}
