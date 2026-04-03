package main

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/user/armage/pkg/agent"
	"github.com/user/armage/pkg/provider"
)

// MockLLM for TUI testing
type MockLLM struct{}

func (m *MockLLM) Chat(ctx context.Context, msgs []provider.Message) (string, provider.Usage, error) {
	return "Thought: Testing.", provider.Usage{TotalTokens: 100}, nil
}
func (m *MockLLM) Model() string { return "test-model" }

func TestTUI_Init(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil command")
	}
}

func TestTUI_Update_FocusMode(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.width = 100
	m.height = 40
	m.ready = true

	// Toggle Focus Mode (F3)
	msg := tea.KeyMsg{Type: tea.KeyF3}
	newModel, _ := m.Update(msg)
	m = newModel.(model)

	if !m.focusMode {
		t.Error("expected focusMode to be true after F3")
	}
}

func TestTUI_Update_PlanMode(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.width = 100
	m.height = 40
	m.ready = true

	// Toggle Plan Mode (F4)
	msg := tea.KeyMsg{Type: tea.KeyF4}
	newModel, _ := m.Update(msg)
	m = newModel.(model)

	if m.state != statePlan {
		t.Errorf("expected state %s, got %s", statePlan, m.state)
	}
	if !m.showPlan {
		t.Error("expected showPlan to be true")
	}
}

func TestTUI_Update_StepMsg_Approval(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.width = 100
	m.height = 40
	m.ready = true

	// Simulate a step result with tool calls
	msg := stepMsg{
		Thought: "I need to read a file.",
		ToolCalls: []agent.ToolCall{
			{Name: "read_file", Args: `{"path": "main.go"}`},
		},
		Status: agent.StatusPending,
	}

	newModel, _ := m.Update(msg)
	m = newModel.(model)

	if m.state != statePendingApproval {
		t.Errorf("expected state %s, got %s", statePendingApproval, m.state)
	}
	if len(m.pendingActions) != 1 {
		t.Errorf("expected 1 pending action, got %d", len(m.pendingActions))
	}
}

func TestTUI_View_Ready(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.width = 100
	m.height = 40
	m.ready = true

	view := m.View()
	if !strings.Contains(view, "Ready") {
		t.Error("expected view to contain 'Ready'")
	}
	if !strings.Contains(view, "ARMAGE") {
		t.Error("expected view to contain 'ARMAGE'")
	}
}
