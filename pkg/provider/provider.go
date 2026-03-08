package provider

import (
	"context"
)

// Message represents a single turn in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLM is the interface for different AI providers.
type LLM interface {
	Chat(ctx context.Context, messages []Message) (string, error)
}
