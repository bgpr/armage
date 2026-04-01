package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/armage/pkg/agent"
	"github.com/user/armage/pkg/provider"
)

// UI States
const (
	stateIdle            = "idle"
	stateThinking        = "thinking"
	statePendingApproval = "pending"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AF87FF"))

	thoughtStyle = lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("#AAAAAA"))

	actionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFD700"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555"))
)

type stepMsg agent.StepResult
type errMsg error

type model struct {
	agent          *agent.Agent
	state          string
	history        []string
	viewport       viewport.Model
	textInput      textinput.Model
	spinner        spinner.Model
	lastTask       string
	statePath      string
	systemPrompt   string
	err            error
	width, height  int
	ready          bool
}

func newModel(a *agent.Agent, statePath, systemPrompt string) model {
	ti := textinput.New()
	ti.Placeholder = "Enter a task for Armage..."
	ti.Focus()
	ti.CharLimit = 512
	ti.Width = 80

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return model{
		agent:        a,
		state:        stateIdle,
		textInput:    ti,
		spinner:      s,
		statePath:    statePath,
		systemPrompt: systemPrompt,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

func runStep(a *agent.Agent, input string) tea.Cmd {
	return func() tea.Msg {
		res, err := a.Step(context.Background(), input)
		if err != nil {
			return errMsg(err)
		}
		return stepMsg(res)
	}
}

func runApprove(a *agent.Agent) tea.Cmd {
	return func() tea.Msg {
		res, err := a.Approve(context.Background())
		if err != nil {
			return errMsg(err)
		}
		return stepMsg(res)
	}
}

func runNudge(a *agent.Agent, task string) tea.Cmd {
	return func() tea.Msg {
		instr := fmt.Sprintf("Please continue working on the task: %s\nProvide your next Action or your Final Answer. Remember to use the Action: ToolName() format.", task)
		res, err := a.StepTransient(context.Background(), instr)
		if err != nil {
			return errMsg(err)
		}
		return stepMsg(res)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
		sCmd  tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyEnter:
			if m.state == stateIdle {
				input := m.textInput.Value()
				if input == "" {
					break
				}
				if input == "clear" {
					m.agent.History = []provider.Message{}
					m.agent.TotalUsage = provider.Usage{}
					m.agent.AddSystemPrompt(m.systemPrompt)
					m.history = []string{infoStyle.Render("Full context, state, and cache cleared.")}
					m.textInput.SetValue("")
					if m.statePath != "" {
						os.Remove(m.statePath)
					}
					os.Remove(".armage_scrub_cache.json")
					m.updateViewport()
					return m, nil
				}
				m.lastTask = input
				m.history = append(m.history, lipgloss.NewStyle().Foreground(lipgloss.Color("#55FF55")).Render("> "+input))
				m.textInput.SetValue("")
				m.state = stateThinking
				return m, runStep(m.agent, input)
			}

			if m.state == statePendingApproval {
				m.history = append(m.history, infoStyle.Render("Approved."))
				m.state = stateThinking
				return m, runApprove(m.agent)
			}

		case tea.KeyRunes:
			if m.state == statePendingApproval && msg.String() == "n" {
				m.history = append(m.history, errorStyle.Render("Cancelled."))
				m.state = stateIdle
				return m, nil
			}
		}

	case stepMsg:
		if msg.Thought != "" {
			m.history = append(m.history, thoughtStyle.Render(msg.Thought))
		}

		// Save state for resilience
		if m.statePath != "" {
			m.agent.Save(m.statePath)
		}

		if strings.Contains(strings.ToLower(msg.Thought), "final answer") {
			m.state = stateIdle
			m.history = append(m.history, titleStyle.Render("DONE"))
			break
		}

		if msg.Status == agent.StatusPending {
			m.state = statePendingApproval
			for i, tc := range msg.ToolCalls {
				m.history = append(m.history, actionStyle.Render(fmt.Sprintf("%d. %s(%s)", i+1, tc.Name, tc.Args)))
			}
			break
		}

		if len(msg.ToolCalls) == 0 {
			m.state = stateThinking
			return m, runNudge(m.agent, m.lastTask)
		}

		// Auto-continue to next step if tool was executed (non-pending)
		m.state = stateThinking
		return m, runStep(m.agent, "")

	case errMsg:
		m.err = msg
		m.history = append(m.history, errorStyle.Render(fmt.Sprintf("Error: %v", msg)))
		m.state = stateIdle

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-7)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height-7
		}

	case spinner.TickMsg:
		m.spinner, sCmd = m.spinner.Update(msg)
	}

	m.textInput, tiCmd = m.textInput.Update(msg)
	m.updateViewport()
	m.viewport, vpCmd = m.viewport.Update(msg)

	return m, tea.Batch(tiCmd, vpCmd, sCmd)
}

func (m *model) updateViewport() {
	m.viewport.SetContent(strings.Join(m.history, "\n\n"))
	m.viewport.GotoBottom()
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing Armage TUI..."
	}

	header := titleStyle.Render(" ARMAGE ") + "  " + infoStyle.Render(fmt.Sprintf("Tokens: %d", m.agent.TotalUsage.TotalTokens))
	
	var status string
	switch m.state {
	case stateThinking:
		status = m.spinner.View() + " Thinking..."
	case statePendingApproval:
		status = " APPROVAL REQUIRED [Enter/n] "
	default:
		status = " Ready "
	}

	return fmt.Sprintf("%s\n\n%s\n\n%s %s\n%s", 
		header, 
		m.viewport.View(), 
		lipgloss.NewStyle().Background(lipgloss.Color("#3C3C3C")).Render(status),
		infoStyle.Render(m.agent.LLM.Model()),
		m.textInput.View())
}
