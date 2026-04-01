package provider

import (
	"context"
)

// Usage tracks token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Message represents a single turn in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLM is the interface for different AI providers.
type LLM interface {
	Chat(ctx context.Context, messages []Message) (string, Usage, error)
	Model() string
}
