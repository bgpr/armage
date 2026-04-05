package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
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

func TestTUI_Update_ToggleLogs(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.width = 100
	m.height = 40
	m.ready = true

	// Toggle (Ctrl+L)
	msg := tea.KeyMsg{Type: tea.KeyCtrlL}
	newModel, _ := m.Update(msg)
	m = newModel.(model)

	if !m.showLogs {
		t.Error("expected showLogs to be true after Ctrl+L")
	}
}

func TestTUI_Update_ErrMsg(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.ready = true

	err := fmt.Errorf("network failure")
	newModel, _ := m.Update(errMsg(err))
	m = newModel.(model)

	if m.err == nil || m.err.Error() != "network failure" {
		t.Errorf("expected error to be stored, got %v", m.err)
	}

	view := m.View()
	if !strings.Contains(view, "network failure") {
		t.Error("expected view to contain the error message")
	}
}

func TestTUI_GenericProgressGuard(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.ready = true

	// 1. STALL: Same Tool + Same Args
	msg := stepMsg{
		ToolCalls: []agent.ToolCall{{Name: "shell", Args: `{"command": "ls"}`}},
	}
	newModel, _ := m.Update(msg)
	m = newModel.(model)
	
	newModel, _ = m.Update(msg) // Second time
	m = newModel.(model)

	if m.stallCount != 0 { // Should reset to 0 after nudge
		t.Errorf("expected stallCount reset to 0 after nudge, got %d", m.stallCount)
	}

	// 2. Verify nudge was added to history
	lastMsg := m.agent.History[len(m.agent.History)-1]
	if !strings.Contains(lastMsg.Content, "stuck or repeating") {
		t.Error("expected nudge message in agent history")
	}
}

func TestTUI_Update_Reset(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	a.TotalUsage.TotalTokens = 500
	m := newModel(a, "", "system prompt")
	m.ready = true

	// Trigger Reset (F2)
	msg := tea.KeyMsg{Type: tea.KeyF2}
	newModel, _ := m.Update(msg)
	m = newModel.(model)

	if m.agent.TotalUsage.TotalTokens != 0 {
		t.Errorf("expected usage to be reset, got %d", m.agent.TotalUsage.TotalTokens)
	}
}

func TestTUI_PlanParsing(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")

	content := `
# PLAN
- [x] Task 1
- [ ] Task 2
- [x] Task 3
- some text
- [ ] Task 4
`
	err := os.WriteFile("PLAN.md", []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write mock PLAN.md: %v", err)
	}
	defer os.Remove("PLAN.md")

	m.refreshPlanStats()

	if m.planTotal != 4 {
		t.Errorf("expected 4 total tasks, got %d", m.planTotal)
	}
	if m.planDone != 2 {
		t.Errorf("expected 2 completed tasks, got %d", m.planDone)
	}
}

func TestTUI_Resilience_SmallScreen(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	
	msg := tea.WindowSizeMsg{Width: 1, Height: 1}
	newModel, _ := m.Update(msg)
	m = newModel.(model)

	if m.viewport.Height < 1 {
		t.Errorf("expected minimum viewport height of 1, got %d", m.viewport.Height)
	}
}

func TestTUI_NavigationKeys(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.width = 100
	m.height = 20
	m.ready = true
	m.viewport = viewport.New(100, 10) // Initialize correctly
	
	content := ""
	for i := 0; i < 100; i++ {
		content += "line\n"
	}
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
	initialOffset := m.viewport.YOffset

	// Navigation keys should work if viewport is focused
	m.focusPort = focusViewport
	msg := tea.KeyMsg{Type: tea.KeyPgUp}
	newModel, _ := m.Update(msg)
	m = newModel.(model)

	if m.viewport.YOffset >= initialOffset {
		t.Errorf("expected YOffset to decrease after PgUp, got %d (initial %d)", m.viewport.YOffset, initialOffset)
	}
}

func TestTUI_Update_FocusCycling(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.ready = true

	if m.focusPort != focusInput {
		t.Errorf("expected initial focus to be %s, got %s", focusInput, m.focusPort)
	}

	msg := tea.KeyMsg{Type: tea.KeyTab}
	newModel, _ := m.Update(msg)
	m = newModel.(model)

	if m.focusPort != focusViewport {
		t.Errorf("expected focus to be %s after Tab, got %s", focusViewport, m.focusPort)
	}
}

func TestTUI_Navigation_WithFocus(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.width = 100
	m.height = 20
	m.ready = true
	m.viewport = viewport.New(100, 10)
	m.viewport.SetContent("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n")
	m.viewport.GotoBottom()
	initialOffset := m.viewport.YOffset

	// 1. When focused on INPUT, Up arrow should NOT scroll
	m.focusPort = focusInput
	msg := tea.KeyMsg{Type: tea.KeyUp}
	newModel, _ := m.Update(msg)
	m = newModel.(model)
	if m.viewport.YOffset != initialOffset {
		t.Error("viewport scrolled while input was focused")
	}

	// 2. When focused on VIEWPORT, Up arrow SHOULD scroll
	m.focusPort = focusViewport
	newModel, _ = m.Update(msg)
	m = newModel.(model)
	if m.viewport.YOffset >= initialOffset && initialOffset > 0 {
		t.Errorf("viewport failed to scroll while focused (offset %d)", m.viewport.YOffset)
	}
}

func TestTUI_VimKeys_WithFocus(t *testing.T) {
	a := agent.New(&MockLLM{}, agent.NewRegistry())
	m := newModel(a, "", "system prompt")
	m.width = 100
	m.height = 20
	m.ready = true
	m.viewport = viewport.New(100, 10)
	m.viewport.SetContent("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n")
	m.viewport.GotoBottom()
	initialOffset := m.viewport.YOffset

	// 1. When focused on INPUT, 'j' should NOT scroll
	m.focusPort = focusInput
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	newModel, _ := m.Update(msg)
	m = newModel.(model)
	if m.viewport.YOffset != initialOffset {
		t.Error("viewport scrolled with Vim key while input was focused")
	}

	// 2. When focused on VIEWPORT, 'k' SHOULD scroll up
	m.focusPort = focusViewport
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}
	newModel, _ = m.Update(msg)
	m = newModel.(model)
	if m.viewport.YOffset >= initialOffset && initialOffset > 0 {
		t.Error("viewport failed to scroll with Vim key while focused")
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
