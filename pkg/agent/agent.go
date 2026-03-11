package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/user/armage/pkg/provider"
)

type AgentStatus string

const (
	StatusRunning AgentStatus = "running"
	StatusPending AgentStatus = "pending" // Waiting for approval
)

// StepResult contains the outcome of a single turn.
type StepResult struct {
	Thought  string      `json:"thought"`
	Status   AgentStatus `json:"status"`
	ToolName string      `json:"tool_name,omitempty"`
	ToolArgs string      `json:"tool_args,omitempty"`
}

// Agent is the core orchestrator.
type Agent struct {
	LLM             provider.LLM
	Registry        *Registry
	History         []provider.Message
	RequireApproval bool        // Safety Governor flag
	PendingResult   *StepResult // Stashed result waiting for approval
}

type State struct {
	History         []provider.Message `json:"history"`
	RequireApproval bool               `json:"require_approval"`
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
	state := State{
		History:         a.History,
		RequireApproval: a.RequireApproval,
	}
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
	a.RequireApproval = state.RequireApproval
	return nil
}

// AddSystemPrompt sets the initial context.
func (a *Agent) AddSystemPrompt(prompt string) {
	a.History = append(a.History, provider.Message{Role: "system", Content: prompt})
}

// Step performs a single ReAct turn.
func (a *Agent) Step(ctx context.Context, input string) (StepResult, error) {
	if input != "" {
		a.History = append(a.History, provider.Message{Role: "user", Content: input})
	}

	// 1. Get LLM Response
	response, err := a.LLM.Chat(ctx, a.History)
	if err != nil {
		return StepResult{}, fmt.Errorf("LLM error: %w", err)
	}
	a.History = append(a.History, provider.Message{Role: "assistant", Content: response})

	// 2. Parse Response
	thought, toolName, toolArgs, err := Parse(response)
	if err != nil {
		return StepResult{Thought: thought, Status: StatusRunning}, nil
	}

	res := StepResult{
		Thought:  thought,
		ToolName: toolName,
		ToolArgs: toolArgs,
		Status:   StatusRunning,
	}

	// 3. Handle Tool Execution with Safety Governor
	if toolName != "" {
		if a.RequireApproval {
			res.Status = StatusPending
			a.PendingResult = &res
			return res, nil
		}
		return a.ExecuteTool(ctx, res)
	}

	return res, nil
}

// Approve continues the execution of a pending tool call.
func (a *Agent) Approve(ctx context.Context) (StepResult, error) {
	if a.PendingResult == nil {
		return StepResult{}, fmt.Errorf("no pending action to approve")
	}
	res := *a.PendingResult
	a.PendingResult = nil
	return a.ExecuteTool(ctx, res)
}

// ExecuteTool performs the actual tool call and records the observation.
func (a *Agent) ExecuteTool(ctx context.Context, res StepResult) (StepResult, error) {
	tool, exists := a.Registry.Get(res.ToolName)
	if !exists {
		observation := fmt.Sprintf("Error: Tool '%s' not found.", res.ToolName)
		a.History = append(a.History, provider.Message{Role: "user", Content: "Observation: " + observation})
		return res, nil
	}

	observation, err := tool.Execute(ctx, res.ToolArgs)
	if err != nil {
		observation = fmt.Sprintf("Error executing tool: %v", err)
	}
	a.History = append(a.History, provider.Message{Role: "user", Content: "Observation: " + observation})

	// Ensure status is Running for the returned result
	res.Status = StatusRunning
	return res, nil
}
