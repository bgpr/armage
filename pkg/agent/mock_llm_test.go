package agent

import (
	"context"
	"github.com/user/armage/pkg/provider"
)

// MockMultiStepLLM allows specifying multiple responses for sequential turns.
type MockMultiStepLLM struct {
	Responses    []string
	Turn         int
	LastMessages []provider.Message
}

func (m *MockMultiStepLLM) Model() string {
	return "mock-model"
}

func (m *MockMultiStepLLM) Chat(ctx context.Context, messages []provider.Message) (string, provider.Usage, error) {
	m.LastMessages = messages
	if m.Turn >= len(m.Responses) {
		return "Final Answer: I am done.", provider.Usage{TotalTokens: 10}, nil
	}
	resp := m.Responses[m.Turn]
	m.Turn++
	return resp, provider.Usage{TotalTokens: 50}, nil
}
