package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouterChat(t *testing.T) {
	// Setup a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock response body from OpenRouter/OpenAI API
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "Thought: Hello!\nAction: ls({\"path\": \".\"})",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// NewOpenRouter doesn't exist yet - RED PHASE
	// This will cause a build error
	client := NewOpenRouter("test-key", "google/gemini-2.0-flash-001")
	client.BaseURL = server.URL // Override for testing

	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	res, err := client.Chat(context.Background(), messages)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if res != "Thought: Hello!\nAction: ls({\"path\": \".\"})" {
		t.Errorf("Unexpected response: %s", res)
	}
}
