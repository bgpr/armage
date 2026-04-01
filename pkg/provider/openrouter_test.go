package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouterChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "Thought: Hello!",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenRouter("test-key", "test-model")
	client.BaseURL = server.URL

	messages := []Message{{Role: "user", Content: "Hello"}}
	res, _, err := client.Chat(context.Background(), messages)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if res != "Thought: Hello!" {
		t.Errorf("Unexpected response: %s", res)
	}
}

func TestOpenRouterBackoff(t *testing.T) {
	tries := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tries++
		if tries < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "Success after retries"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenRouter("test-key", "test-model")
	client.BaseURL = server.URL

	_, _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err != nil {
		t.Fatalf("Backoff failed: %v", err)
	}

	if tries != 3 {
		t.Errorf("Expected 3 tries, got %d", tries)
	}
}

func TestOpenRouterRotation(t *testing.T) {
	tries := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Model string `json:"model"` }
		json.NewDecoder(r.Body).Decode(&req)
		tries[req.Model]++

		if req.Model == "model-1" {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "Success on model-2"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenRouter("test-key", "model-1")
	client.BaseURL = server.URL
	client.FallbackModels = []string{"model-1", "model-2"}

	_, _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err != nil {
		t.Fatalf("Rotation failed: %v", err)
	}

	if tries["model-1"] < 3 {
		t.Errorf("Expected at least 3 tries on model-1 before rotating, got %d", tries["model-1"])
	}
	if tries["model-2"] != 1 {
		t.Errorf("Expected 1 try on model-2, got %d", tries["model-2"])
	}
}

func TestFetchFreeModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "free-model:free",
					"pricing": map[string]string{"prompt": "0", "completion": "0", "request": "0"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenRouter("test-key", "test-model")
	client.ModelsURL = server.URL

	free, err := client.FetchFreeModels(context.Background())
	if err != nil {
		t.Fatalf("FetchFreeModels failed: %v", err)
	}

	if len(free) != 1 || free[0] != "free-model:free" {
		t.Errorf("Expected free-model:free, got %v", free)
	}
}

func TestOpenRouter403Rotation(t *testing.T) {
	tries := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Model string `json:"model"` }
		json.NewDecoder(r.Body).Decode(&req)
		tries[req.Model]++

		if req.Model == "model-1" {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "Success on model-2 after 403"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenRouter("test-key", "model-1")
	client.BaseURL = server.URL
	client.FallbackModels = []string{"model-1", "model-2"}

	_, _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err != nil {
		t.Fatalf("Rotation on 403 failed: %v", err)
	}

	if tries["model-1"] < 3 {
		t.Errorf("Expected retries on model-1 before rotating, got %d", tries["model-1"])
	}
}

func TestOpenRouterErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewOpenRouter("test-key", "test-model")
	client.BaseURL = server.URL

	_, _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "Hi"}})
	if err == nil {
		t.Errorf("Expected error on 500 with non-json body")
	}
}
