package agent

import (
	"context"
	"fmt"
)

// LLM is the interface for different AI providers (OpenRouter, Gemini, etc.)
type LLM interface {
	Chat(ctx context.Context, messages []Message) (string, error)
}

// Message represents a single turn in the conversation.
type Message struct {
	Role    string // "user", "assistant", "system"
	Content string
}

// Agent is the core orchestrator.
type Agent struct {
	LLM      LLM
	Registry *Registry
	History  []Message
}

func New(llm LLM, registry *Registry) *Agent {
	return &Agent{
		LLM:      llm,
		Registry: registry,
		History:  []Message{},
	}
}

// AddSystemPrompt sets the initial context.
func (a *Agent) AddSystemPrompt(prompt string) {
	a.History = append(a.History, Message{Role: "system", Content: prompt})
}

// Step performs a single ReAct turn.
func (a *Agent) Step(ctx context.Context, input string) (string, error) {
	if input != "" {
		a.History = append(a.History, Message{Role: "user", Content: input})
	}

	// 1. Get LLM Response
	response, err := a.LLM.Chat(ctx, a.History)
	if err != nil {
		return "", fmt.Errorf("LLM error: %w", err)
	}
	a.History = append(a.History, Message{Role: "assistant", Content: response})

	// 2. Parse Response
	thought, toolName, toolArgs, err := Parse(response)
	if err != nil {
		return thought, nil // Just a thought, no action.
	}

	// 3. Execute Tool if Action exists
	if toolName != "" {
		tool, exists := a.Registry.Get(toolName)
		if !exists {
			observation := fmt.Sprintf("Error: Tool '%s' not found.", toolName)
			a.History = append(a.History, Message{Role: "user", Content: "Observation: " + observation})
			return thought, nil
		}

		observation, err := tool.Execute(ctx, toolArgs)
		if err != nil {
			observation = fmt.Sprintf("Error executing tool: %v", err)
		}
		a.History = append(a.History, Message{Role: "user", Content: "Observation: " + observation})
	}

	return thought, nil
}
