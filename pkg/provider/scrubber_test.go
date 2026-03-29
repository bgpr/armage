package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalScrubber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock response from local llama.cpp
		resp := struct {
			Choices []struct {
				Message Message `json:"message"`
			} `json:"choices"`
		}{
			Choices: []struct {
				Message Message `json:"message"`
			}{
				{Message: Message{Role: "assistant", Content: "Hello [NAME], your key is [KEY]"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	scrubber := &LocalScrubber{BaseURL: server.URL}
	res, err := scrubber.Scrub(context.Background(), "Hello John, your key is sk-123")
	if err != nil {
		t.Fatalf("Scrub failed: %v", err)
	}

	expected := "Hello [NAME], your key is [KEY]"
	if res != expected {
		t.Errorf("Expected '%s', got '%s'", expected, res)
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

func TestScrubbingLLM(t *testing.T) {
	inner := &MockLLM{}
	scrubber := &MockScrubber{}
	sllm := NewScrubbingLLM(inner, scrubber)

	messages := []Message{
		{Role: "system", Content: "You are Armage..."},
		{Role: "user", Content: "My secret is secret"},
	}

	_, _, err := sllm.Chat(context.Background(), messages)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if inner.LastMessages[1].Content != "My [REDACTED] is [REDACTED]" {
		t.Errorf("Scrubbing failed, got: %s", inner.LastMessages[1].Content)
	}

	// Test Caching
	_, _, _ = sllm.Chat(context.Background(), messages)
	if scrubber.Calls != 1 {
		t.Errorf("Caching failed, expected 1 call to scrubber, got %d", scrubber.Calls)
	}
}
