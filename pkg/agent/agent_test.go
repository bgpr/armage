package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/user/armage/pkg/provider"
)

type MockLLM struct {
	Response string
}

func (m *MockLLM) Model() string {
	return "mock-model"
}

func (m *MockLLM) Chat(ctx context.Context, messages []provider.Message) (string, provider.Usage, error) {
	if len(messages) > 0 && messages[0].Role == "system" && strings.Contains(messages[0].Content, "Summarize") {
		return "SUMMARY: All good", provider.Usage{TotalTokens: 10}, nil
	}
	return m.Response, provider.Usage{TotalTokens: 100}, nil
}

func TestAgentStep(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&MockTool{})

	llm := &MockLLM{Response: "Thought: echo\nAction: echo(\"hello\")"}
	a := New(llm, reg)
	res, err := a.Step(context.Background(), "Start")
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	if res.Thought != "echo" {
		t.Errorf("Unexpected thought: %s", res.Thought)
	}

	if len(a.History) != 3 {
		t.Fatalf("Expected 3 messages, got: %d", len(a.History))
	}
	
	obs := a.History[2].Content
	if !strings.Contains(obs, "Observation 1 (echo):") {
		t.Errorf("Unexpected observation: %s", obs)
	}
}

func TestAgent_SelfCorrection(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&ShellTool{})
	
	// Turn 0: Malformed JSON (Nemotron style)
	// Turn 1: Fixed ReAct format after nudge
	llm := &MockMultiStepLLM{
		Responses: []string{
			`{"Action": "shell({\"command\": \"ls\"})}`, 
			"Thought: Fixing format.\nAction: shell({\"command\": \"ls\"})",
		},
	}
	
	a := New(llm, reg)
	res, err := a.Step(context.Background(), "Run ls")
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Name != "shell" {
		t.Errorf("Expected shell tool call after self-correction, got: %v", res.ToolCalls)
	}
}

func TestAgentStepTransient(t *testing.T) {
	reg := NewRegistry()
	llm := &MockLLM{Response: "Thought: I am transient."}
	a := New(llm, reg)
	a.History = []provider.Message{{Role: "user", Content: "Existing"}}

	res, err := a.StepTransient(context.Background(), "Nudge")
	if err != nil {
		t.Fatalf("StepTransient failed: %v", err)
	}

	if res.Thought != "I am transient." {
		t.Errorf("Unexpected thought: %s", res.Thought)
	}

	if len(a.History) != 1 || a.History[0].Content != "Existing" {
		t.Errorf("History was modified by transient step: %v", a.History)
	}
}

func TestAgentStep_Approval(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&MockTool{})
	llm := &MockLLM{Response: "Action: echo(\"approve\")"}
	a := New(llm, reg)
	a.RequireApproval = true

	res, _ := a.Step(context.Background(), "Task")
	if res.Status != StatusPending {
		t.Errorf("Expected Pending status, got %v", res.Status)
	}

	res, _ = a.Approve(context.Background())
	if res.Status != StatusRunning {
		t.Errorf("Expected Running status after approval, got %v", res.Status)
	}
}

func TestAgentStep_Stuck(t *testing.T) {
	reg := NewRegistry()
	llm := &MockLLM{Response: "I am just thinking without tags"}
	a := New(llm, reg)

	res, _ := a.Step(context.Background(), "Task")
	if res.Thought != "I am just thinking without tags" {
		t.Errorf("Fallback failed, got: %s", res.Thought)
	}
}

func TestAgentStep_MultiAction(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&MockTool{})
	llm := &MockLLM{Response: "Action: echo(\"1\")\nAction: echo(\"2\")"}
	a := New(llm, reg)

	res, _ := a.Step(context.Background(), "Task")
	if len(res.ToolCalls) != 2 {
		t.Errorf("Expected 2 tool calls, got %d", len(res.ToolCalls))
	}
}

func TestAgentPersistence(t *testing.T) {
	reg := NewRegistry()
	llm := &MockLLM{}
	a := New(llm, reg)
	a.History = []provider.Message{{Role: "user", Content: "Persistent"}}
	
	tmpFile := "test_state.json"
	defer os.Remove(tmpFile)
	if err := a.Save(tmpFile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	a2 := New(llm, reg)
	if err := a2.Load(tmpFile); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(a2.History) != 1 {
		t.Errorf("Load failed, history length: %d", len(a2.History))
	}
}

func TestAgentPinFile(t *testing.T) {
	reg := NewRegistry()
	llm := &MockLLM{}
	a := New(llm, reg)
	
	tmpFile := "test_pin.txt"
	os.WriteFile(tmpFile, []byte("Content"), 0644)
	defer os.Remove(tmpFile)

	if err := a.PinFile(tmpFile); err != nil {
		t.Fatalf("PinFile failed: %v", err)
	}
	if len(a.PinnedFiles) != 1 {
		t.Errorf("Pin failed, files: %v", a.PinnedFiles)
	}
}

func TestAgentToolError(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&ErrorTool{})
	llm := &MockLLM{Response: "Action: error_tool()"}
	a := New(llm, reg)

	a.Step(context.Background(), "Task")
	lastMsg := a.History[len(a.History)-1].Content
	if !strings.Contains(lastMsg, "Error executing tool") {
		t.Errorf("Expected tool error message, got: %s", lastMsg)
	}
}

func TestAgentTrimHistory(t *testing.T) {
	reg := NewRegistry()
	llm := &MockLLM{Response: "Thought: Doing stuff"}
	a := New(llm, reg)
	a.MaxHistory = 5
	a.AddSystemPrompt("You are Armage")

	for i := 0; i < 10; i++ {
		a.History = append(a.History, provider.Message{Role: "user", Content: "Task"})
		a.History = append(a.History, provider.Message{Role: "assistant", Content: "Thought: Doing it"})
	}

	a.trimHistory(context.Background())

	if len(a.History) > a.MaxHistory {
		t.Errorf("History not trimmed, got %d, max %d", len(a.History), a.MaxHistory)
	}
}

type ErrorTool struct{}
func (t *ErrorTool) Name() string { return "error_tool" }
func (t *ErrorTool) Description() string { return "failing tool" }
func (t *ErrorTool) Execute(ctx context.Context, args string) (string, error) {
	return "", os.ErrPermission
}
func (t *ErrorTool) Preview(ctx context.Context, args string) (string, error) {
	return "Preview Error", nil
}
