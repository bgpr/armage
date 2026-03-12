package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	res, usage, err := client.Chat(context.Background(), messages)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if res != "Thought: Hello!\nAction: ls({\"path\": \".\"})" {
		t.Errorf("Unexpected response: %s", res)
	}
	_ = usage
}

func TestOpenRouterFallback(t *testing.T) {
	tries := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tries++
		var reqBody openRouterRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		if tries == 1 {
			// First try: simulate "system role not supported" error
			w.WriteHeader(http.StatusBadRequest)
			resp := map[string]interface{}{
				"error": map[string]string{
					"message": "Developer instruction is not enabled for this model",
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Second try: verify role was changed to user
		if reqBody.Messages[0].Role != "user" || !strings.Contains(reqBody.Messages[0].Content, "System Instructions:") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{"content": "Fallback worked"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenRouter("test-key", "test-model")
	client.BaseURL = server.URL

	messages := []Message{
		{Role: "system", Content: "Be a helper"},
	}

	res, _, err := client.Chat(context.Background(), messages)
	if err != nil {
		t.Fatalf("Fallback failed: %v", err)
	}

	if res != "Fallback worked" {
		t.Errorf("Expected 'Fallback worked', got: %s", res)
	}
	if tries != 2 {
		t.Errorf("Expected 2 tries, got: %d", tries)
	}
}
