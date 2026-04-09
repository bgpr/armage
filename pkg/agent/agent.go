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

// ToolCall represents a single tool invocation request.
type ToolCall struct {
	Name string `json:"name"`
	Args string `json:"args"`
}

// StepResult contains the outcome of a single turn.
type StepResult struct {
	Thought      string         `json:"thought"`
	Status       AgentStatus    `json:"status"`
	ToolCalls    []ToolCall     `json:"tool_calls,omitempty"`
	Observations string         `json:"observations,omitempty"`
	Usage        provider.Usage `json:"usage"`
}

// Agent is the core orchestrator.
type Agent struct {
	LLM              provider.LLM
	Registry         *Registry
	History          []provider.Message
	RequireApproval  bool           // Safety Governor flag
	PendingResult    *StepResult    // Stashed result waiting for approval
	MaxHistory       int            // Maximum number of messages to keep (0 for unlimited)
	MaxContextTokens int            // Threshold to trigger history summarization
	TotalUsage       provider.Usage // Cumulative token usage
	PinnedFiles      []string       // List of paths pinned to context
}

type State struct {
	History          []provider.Message `json:"history"`
	RequireApproval  bool               `json:"require_approval"`
	MaxHistory       int                `json:"max_history"`
	MaxContextTokens int                `json:"max_context_tokens"`
	TotalUsage       provider.Usage     `json:"total_usage"`
	PinnedFiles      []string           `json:"pinned_files"`
}

func New(llm provider.LLM, registry *Registry) *Agent {
	return &Agent{
		LLM:              llm,
		Registry:         registry,
		History:          []provider.Message{},
		MaxHistory:       20, 
		MaxContextTokens: 0, 
		PinnedFiles:      []string{},
	}
}

// Save persists the agent's history to disk.
func (a *Agent) Save(path string) error {
	state := State{
		History:          a.History,
		RequireApproval:  a.RequireApproval,
		MaxHistory:       a.MaxHistory,
		MaxContextTokens: a.MaxContextTokens,
		TotalUsage:       a.TotalUsage,
		PinnedFiles:      a.PinnedFiles,
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
	a.MaxContextTokens = state.MaxContextTokens
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
	
	for _, p := range a.PinnedFiles {
		if p == path {
			return nil 
		}
	}

	a.PinnedFiles = append(a.PinnedFiles, path)
	msg := provider.Message{Role: "system", Content: content}
	
	if len(a.History) > 0 && a.History[0].Role == "system" && !strings.Contains(a.History[0].Content, "Pinned File:") {
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

// UpdatePinnedFiles refreshes the content of all pinned files in the history in-place.
func (a *Agent) UpdatePinnedFiles() {
	for _, path := range a.PinnedFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue 
		}

		newContent := fmt.Sprintf("Pinned File: %s\n---\n%s\n---", path, string(data))
		
		for i, msg := range a.History {
			if msg.Role == "system" && strings.HasPrefix(msg.Content, fmt.Sprintf("Pinned File: %s", path)) {
				a.History[i].Content = newContent
				break
			}
		}
	}
}

// Step performs a single ReAct turn.
func (a *Agent) Step(ctx context.Context, input string) (StepResult, error) {
	a.UpdatePinnedFiles()

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
	thought, toolCalls, err := Parse(response)
	if err != nil {
		if strings.Contains(err.Error(), "malformed action") {
			nudge := "Error: Your last response contained a malformed Action. Please provide the Action again using valid JSON or the strict 'Action: ToolName(Args)' format."
			fmt.Printf("\n[Agent] Detected malformed action. Nudging LLM for correction...\n")
			return a.Step(ctx, nudge) 
		}
		a.trimHistory(ctx) 
		return StepResult{Thought: thought, Status: StatusRunning, Usage: usage}, nil
	}

	isFinished := strings.Contains(strings.ToLower(thought), "final answer")
	if len(toolCalls) == 0 && !isFinished {
		res := StepResult{
			Thought: thought,
			Status:  StatusRunning, 
			Usage:   usage,
		}
		a.trimHistory(ctx)
		return res, nil
	}

	if len(toolCalls) > 5 {
		toolCalls = toolCalls[:5]
	}

	res := StepResult{
		Thought:   thought,
		ToolCalls: toolCalls,
		Status:    StatusRunning,
		Usage:     usage,
	}

	if len(toolCalls) > 0 {
		if a.RequireApproval {
			res.Status = StatusPending
			a.PendingResult = &res
			return res, nil
		}
		res, err = a.ExecuteTools(ctx, res)
	}

	a.trimHistory(ctx)
	return res, err
}

// StepTransient sends a message to the LLM without adding it to the history.
func (a *Agent) StepTransient(ctx context.Context, instruction string) (StepResult, error) {
	tempHistory := make([]provider.Message, len(a.History))
	copy(tempHistory, a.History)
	tempHistory = append(tempHistory, provider.Message{Role: "user", Content: instruction})

	response, usage, err := a.LLM.Chat(ctx, tempHistory)
	if err != nil {
		return StepResult{}, err
	}

	a.TotalUsage.PromptTokens += usage.PromptTokens
	a.TotalUsage.CompletionTokens += usage.CompletionTokens
	a.TotalUsage.TotalTokens += usage.TotalTokens

	thought, toolCalls, err := Parse(response)
	if err != nil {
		return StepResult{Thought: thought, Usage: usage}, nil
	}

	return StepResult{
		Thought:   thought,
		ToolCalls: toolCalls,
		Status:    StatusRunning,
		Usage:     usage,
	}, nil
}

func (a *Agent) trimHistory(ctx context.Context) {
	limit := a.LLM.ContextWindow()
	if limit <= 0 {
		limit = 4096 
	}
	
	triggerThreshold := int(float64(limit) * 0.75)

	currentChars := 0
	for _, m := range a.History {
		currentChars += len(m.Content)
	}
	estimatedTokens := currentChars / 3 

	overTurnLimit := a.MaxHistory > 0 && len(a.History) > a.MaxHistory
	overTokenLimit := estimatedTokens > triggerThreshold

	if !overTurnLimit && !overTokenLimit {
		return
	}

	prefixCount := 0
	for _, msg := range a.History {
		if msg.Role == "system" {
			prefixCount++
		} else {
			break
		}
	}

	keepCount := 4 
	if keepCount > len(a.History)-prefixCount {
		keepCount = len(a.History) - prefixCount
	}

	startIdx := len(a.History) - keepCount
	if startIdx <= prefixCount {
		return 
	}

	turnsToSummarize := a.History[prefixCount:startIdx]
	summary, err := a.summarize(ctx, turnsToSummarize)
	
	newHistory := make([]provider.Message, 0, a.MaxHistory+prefixCount)
	newHistory = append(newHistory, a.History[:prefixCount]...)
	
	if err == nil && summary != "" {
		summaryMsg := provider.Message{
			Role:    "system",
			Content: fmt.Sprintf("Previous Conversation Summary: %s", summary),
		}
		newHistory = append(newHistory, summaryMsg)
	}
	
	newHistory = append(newHistory, a.History[startIdx:]...)
	a.History = newHistory
}

func (a *Agent) summarize(ctx context.Context, messages []provider.Message) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	prompt := []provider.Message{
		{Role: "system", Content: "Summarize the following conversation turns VERY concisely (max 1 sentence). Focus only on the final outcome. End with 'SUMMARY: [your summary]'."},
	}
	prompt = append(prompt, messages...)

	summary, _, err := a.LLM.Chat(ctx, prompt)
	if err != nil {
		return "", err
	}

	if parts := strings.Split(summary, "SUMMARY:"); len(parts) > 1 {
		return strings.TrimSpace(parts[len(parts)-1]), nil
	}

	return summary, nil
}

// Approve continues the execution of pending tool calls.
func (a *Agent) Approve(ctx context.Context) (StepResult, error) {
	if a.PendingResult == nil {
		return StepResult{}, fmt.Errorf("no pending action to approve")
	}
	res := *a.PendingResult
	a.PendingResult = nil
	res, err := a.ExecuteTools(ctx, res)
	a.trimHistory(ctx)
	return res, err
}

// ExecuteTools performs the actual tool calls and records the observations.
func (a *Agent) ExecuteTools(ctx context.Context, res StepResult) (StepResult, error) {
	var observations []string

	for i, tc := range res.ToolCalls {
		tool, exists := a.Registry.Get(tc.Name)
		var obs string
		if !exists {
			obs = fmt.Sprintf("Error: Tool '%s' not found.", tc.Name)
		} else {
			var err error
			obs, err = tool.Execute(ctx, tc.Args)
			if err != nil {
				obs = fmt.Sprintf("Error executing tool '%s': %v", tc.Name, err)
			}
		}
		observations = append(observations, fmt.Sprintf("Observation %d (%s):\n%s", i+1, tc.Name, obs))
	}

	fullObservation := strings.Join(observations, "\n\n")
	res.Observations = "Observations:\n" + fullObservation
	a.History = append(a.History, provider.Message{Role: "user", Content: res.Observations})

	res.Status = StatusRunning
	return res, nil
}
