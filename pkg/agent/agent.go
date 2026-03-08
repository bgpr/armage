package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/user/armage/pkg/provider"
)

// Agent is the core orchestrator.
type Agent struct {
	LLM      provider.LLM
	Registry *Registry
	History  []provider.Message
}

type State struct {
	History []provider.Message `json:"history"`
}

func New(llm provider.LLM, registry *Registry) *Agent {
	return &Agent{
		LLM:      llm,
		Registry: registry,
		History:  []provider.Message{},
	}
}

// Save persists the agent's history to disk.
func (a *Agent) Save(path string) error {
	state := State{History: a.History}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load restores the agent's history from disk.
func (a *Agent) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	a.History = state.History
	return nil
}

// AddSystemPrompt sets the initial context.
func (a *Agent) AddSystemPrompt(prompt string) {
	a.History = append(a.History, provider.Message{Role: "system", Content: prompt})
}

// Step performs a single ReAct turn.
func (a *Agent) Step(ctx context.Context, input string) (string, error) {
	if input != "" {
		a.History = append(a.History, provider.Message{Role: "user", Content: input})
	}

	// 1. Get LLM Response
	response, err := a.LLM.Chat(ctx, a.History)
	if err != nil {
		return "", fmt.Errorf("LLM error: %w", err)
	}
	a.History = append(a.History, provider.Message{Role: "assistant", Content: response})

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
			a.History = append(a.History, provider.Message{Role: "user", Content: "Observation: " + observation})
			return thought, nil
		}

		observation, err := tool.Execute(ctx, toolArgs)
		if err != nil {
			observation = fmt.Sprintf("Error executing tool: %v", err)
		}
		a.History = append(a.History, provider.Message{Role: "user", Content: "Observation: " + observation})
	}

	return thought, nil
}
