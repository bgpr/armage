package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLocalScrubber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Choices []struct {
				Message Message `json:"message"`
			} `json:"choices"`
		}{
			Choices: []struct {
				Message Message `json:"message"`
			}{
				{Message: Message{Role: "assistant", Content: "<safe_text>Hello [NAME]</safe_text>"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	scrubber := &LocalScrubber{BaseURL: server.URL}
	res, err := scrubber.Scrub(context.Background(), "Hello John")
	if err != nil {
		t.Fatalf("Scrub failed: %v", err)
	}

	if res != "Hello [NAME]" {
		t.Errorf("Expected 'Hello [NAME]', got '%s'", res)
	}
}

type MockLLM struct {
	LastMessages []Message
}

func (m *MockLLM) Chat(ctx context.Context, messages []Message) (string, Usage, error) {
	m.LastMessages = messages
	return "OK", Usage{}, nil
}

type MockScrubber struct {
	Calls int
}

func (m *MockScrubber) Scrub(ctx context.Context, text string) (string, error) {
	m.Calls++
	return strings.ReplaceAll(text, "secret", "[REDACTED]"), nil
}

func TestScrubbingLLM_Bypass(t *testing.T) {
	inner := &MockLLM{}
	scrubber := &MockScrubber{}
	sllm := NewScrubbingLLM(inner, scrubber, "")

	messages := []Message{
		{Role: "user", Content: "Observations:\n1 | func Test()"},
		{Role: "assistant", Content: "Thought: I see secret"},
		{Role: "user", Content: "Short"},
	}

	_, _, err := sllm.Chat(context.Background(), messages)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	// None of these should have called the scrubber
	if scrubber.Calls != 0 {
		t.Errorf("Expected 0 scrubber calls due to bypasses, got %d", scrubber.Calls)
	}
}

func TestScrubbingLLM_Parallel(t *testing.T) {
	inner := &MockLLM{}
	scrubber := &MockScrubber{}
	sllm := NewScrubbingLLM(inner, scrubber, "")

	messages := []Message{
		{Role: "user", Content: "This is a secret message 1"},
		{Role: "user", Content: "This is a secret message 2"},
	}

	_, _, err := sllm.Chat(context.Background(), messages)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if scrubber.Calls != 2 {
		t.Errorf("Expected 2 scrubber calls, got %d", scrubber.Calls)
	}
}

func TestScrubbingLLM_Persistence(t *testing.T) {
	cachePath := "test_scrub_cache.json"
	defer os.Remove(cachePath)

	inner := &MockLLM{}
	scrubber := &MockScrubber{}
	
	// Turn 1: Scrub and Save
	sllm1 := NewScrubbingLLM(inner, scrubber, cachePath)
	_, _, _ = sllm1.Chat(context.Background(), []Message{{Role: "user", Content: "This is a long message that contains a secret."}})
	if scrubber.Calls != 1 {
		t.Errorf("Expected 1 call, got %d", scrubber.Calls)
	}

	// Turn 2: Load and Verify cache hit
	scrubber.Calls = 0
	sllm2 := NewScrubbingLLM(inner, scrubber, cachePath)
	_, _, _ = sllm2.Chat(context.Background(), []Message{{Role: "user", Content: "This is a long message that contains a secret."}})
	if scrubber.Calls != 0 {
		t.Errorf("Expected cache hit (0 calls), got %d", scrubber.Calls)
	}

}
