package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
	PinnedFiles     []string       // List of paths pinned to context
}

type State struct {
	History         []provider.Message `json:"history"`
	RequireApproval bool               `json:"require_approval"`
	MaxHistory      int                `json:"max_history"`
	TotalUsage      provider.Usage     `json:"total_usage"`
	PinnedFiles     []string           `json:"pinned_files"`
}

func New(llm provider.LLM, registry *Registry) *Agent {
	return &Agent{
		LLM:         llm,
		Registry:    registry,
		History:     []provider.Message{},
		MaxHistory:  20, // Default to a reasonable limit for mobile context
		PinnedFiles: []string{},
	}
}

// Save persists the agent's history to disk.
func (a *Agent) Save(path string) error {
	state := State{
		History:         a.History,
		RequireApproval: a.RequireApproval,
		MaxHistory:      a.MaxHistory,
		TotalUsage:      a.TotalUsage,
		PinnedFiles:     a.PinnedFiles,
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
	a.PinnedFiles = state.PinnedFiles
	return nil
}

// AddSystemPrompt sets the initial context.
func (a *Agent) AddSystemPrompt(prompt string) {
	a.History = append(a.History, provider.Message{Role: "system", Content: prompt})
}

// PinFile adds a file's content permanently to the history (protected from trimming).
func (a *Agent) PinFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := fmt.Sprintf("Pinned File: %s\n---\n%s\n---", path, string(data))
	
	// Check if already pinned to avoid duplicates
	for _, p := range a.PinnedFiles {
		if p == path {
			return nil 
		}
	}

	a.PinnedFiles = append(a.PinnedFiles, path)
	// Add to history right after the system prompt or at the front
	msg := provider.Message{Role: "system", Content: content}
	
	if len(a.History) > 0 && a.History[0].Role == "system" && !strings.Contains(a.History[0].Content, "Pinned File:") {
		// Insert after actual system prompt
		newHistory := make([]provider.Message, 0, len(a.History)+1)
		newHistory = append(newHistory, a.History[0])
		newHistory = append(newHistory, msg)
		newHistory = append(newHistory, a.History[1:]...)
		a.History = newHistory
	} else {
		a.History = append([]provider.Message{msg}, a.History...)
	}

	return nil
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
		a.trimHistory(ctx) // Still trim even if no tool was called
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

	a.trimHistory(ctx)
	return res, err
}

// trimHistory keeps the history within MaxHistory limit, preserving system prompt AND pinned files.
// It summarizes deleted turns to maintain context.
func (a *Agent) trimHistory(ctx context.Context) {
	if a.MaxHistory <= 0 || len(a.History) <= a.MaxHistory {
		return
	}

	// 1. Summarize the turns that are about to be deleted
	// Identify protected prefix (System prompt + all Pinned files)
	prefixCount := 0
	for _, msg := range a.History {
		if msg.Role == "system" {
			prefixCount++
		} else {
			break
		}
	}

	keepCount := a.MaxHistory - (prefixCount + 1) // +1 for the Summary message
	if keepCount < 2 {
		keepCount = 2
	}

	startIdx := len(a.History) - keepCount
	if startIdx <= prefixCount {
		return 
	}

	// turnsToSummarize are from prefixCount to startIdx
	turnsToSummarize := a.History[prefixCount:startIdx]
	summary, err := a.summarize(ctx, turnsToSummarize)
	if err == nil {
		summaryMsg := provider.Message{
			Role:    "system",
			Content: fmt.Sprintf("Previous Conversation Summary: %s", summary),
		}
		
		newHistory := make([]provider.Message, 0, a.MaxHistory)
		newHistory = append(newHistory, a.History[:prefixCount]...)
		newHistory = append(newHistory, summaryMsg)
		newHistory = append(newHistory, a.History[startIdx:]...)
		a.History = newHistory
	} else {
		// Fallback to simple sliding window if summarization fails
		newHistory := make([]provider.Message, 0, a.MaxHistory)
		newHistory = append(newHistory, a.History[:prefixCount]...)
		newHistory = append(newHistory, a.History[startIdx:]...)
		a.History = newHistory
	}
}

func (a *Agent) summarize(ctx context.Context, messages []provider.Message) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	prompt := []provider.Message{
		{Role: "system", Content: "Summarize the following conversation turns concisely, focusing on what was achieved and what the current state is. End your response with 'SUMMARY: [your summary]'."},
	}
	prompt = append(prompt, messages...)

	summary, _, err := a.LLM.Chat(ctx, prompt)
	if err != nil {
		return "", err
	}

	// Extract everything after SUMMARY: if present
	if parts := strings.Split(summary, "SUMMARY:"); len(parts) > 1 {
		return strings.TrimSpace(parts[len(parts)-1]), nil
	}

	return summary, nil
}

// Approve continues the execution of a pending tool call.
func (a *Agent) Approve(ctx context.Context) (StepResult, error) {
	if a.PendingResult == nil {
		return StepResult{}, fmt.Errorf("no pending action to approve")
	}
	res := *a.PendingResult
	a.PendingResult = nil
	res, err := a.ExecuteTool(ctx, res)
	a.trimHistory(ctx)
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
