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
	Thought  string         `json:"thought"`
	Status   AgentStatus    `json:"status"`
	ToolName string         `json:"tool_name,omitempty"`
	ToolArgs string         `json:"tool_args,omitempty"`
	Usage    provider.Usage `json:"usage"`
}

// Agent is the core orchestrator.
type Agent struct {
	LLM             provider.LLM
	Registry        *Registry
	History         []provider.Message
	RequireApproval bool           // Safety Governor flag
	PendingResult   *StepResult    // Stashed result waiting for approval
	MaxHistory      int            // Maximum number of messages to keep (0 for unlimited)
	TotalUsage      provider.Usage // Cumulative token usage
}

type State struct {
	History         []provider.Message `json:"history"`
	RequireApproval bool               `json:"require_approval"`
	MaxHistory      int                `json:"max_history"`
	TotalUsage      provider.Usage     `json:"total_usage"`
}

func New(llm provider.LLM, registry *Registry) *Agent {
	return &Agent{
		LLM:        llm,
		Registry:   registry,
		History:    []provider.Message{},
		MaxHistory: 20, // Default to a reasonable limit for mobile context
	}
}

// Save persists the agent's history to disk.
func (a *Agent) Save(path string) error {
	state := State{
		History:         a.History,
		RequireApproval: a.RequireApproval,
		MaxHistory:      a.MaxHistory,
		TotalUsage:      a.TotalUsage,
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
	a.MaxHistory = state.MaxHistory
	a.TotalUsage = state.TotalUsage
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
	response, usage, err := a.LLM.Chat(ctx, a.History)
	if err != nil {
		return StepResult{}, fmt.Errorf("LLM error: %w", err)
	}
	a.History = append(a.History, provider.Message{Role: "assistant", Content: response})

	// Track Usage
	a.TotalUsage.PromptTokens += usage.PromptTokens
	a.TotalUsage.CompletionTokens += usage.CompletionTokens
	a.TotalUsage.TotalTokens += usage.TotalTokens

	// 2. Parse Response
	thought, toolName, toolArgs, err := Parse(response)
	if err != nil {
		a.trimHistory() // Still trim even if no tool was called
		return StepResult{Thought: thought, Status: StatusRunning, Usage: usage}, nil
	}

	res := StepResult{
		Thought:  thought,
		ToolName: toolName,
		ToolArgs: toolArgs,
		Status:   StatusRunning,
		Usage:    usage,
	}

	// 3. Handle Tool Execution with Safety Governor
	if toolName != "" {
		if a.RequireApproval {
			res.Status = StatusPending
			a.PendingResult = &res
			// Note: trimHistory will be called after Approval/Execution
			return res, nil
		}
		res, err = a.ExecuteTool(ctx, res)
	}

	a.trimHistory()
	return res, err
}

// trimHistory keeps the history within MaxHistory limit, always preserving the system prompt.
func (a *Agent) trimHistory() {
	if a.MaxHistory <= 0 || len(a.History) <= a.MaxHistory {
		return
	}

	// Identify system prompt
	var systemPrompt *provider.Message
	if len(a.History) > 0 && a.History[0].Role == "system" {
		systemPrompt = &a.History[0]
	}

	// Keep the last N-1 messages (room for system prompt)
	keepCount := a.MaxHistory
	if systemPrompt != nil {
		keepCount--
	}

	startIdx := len(a.History) - keepCount
	newHistory := make([]provider.Message, 0, a.MaxHistory)

	if systemPrompt != nil {
		newHistory = append(newHistory, *systemPrompt)
	}

	newHistory = append(newHistory, a.History[startIdx:]...)
	a.History = newHistory
}

// Approve continues the execution of a pending tool call.
func (a *Agent) Approve(ctx context.Context) (StepResult, error) {
	if a.PendingResult == nil {
		return StepResult{}, fmt.Errorf("no pending action to approve")
	}
	res := *a.PendingResult
	a.PendingResult = nil
	res, err := a.ExecuteTool(ctx, res)
	a.trimHistory()
	return res, err
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
