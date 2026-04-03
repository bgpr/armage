package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/armage/pkg/agent"
	"github.com/user/armage/pkg/provider"
)

// UI States
const (
	stateIdle            = "idle"
	stateThinking        = "thinking"
	statePendingApproval = "pending"
	stateHelp            = "help"
	statePaused          = "paused"
	stateSearch          = "search"
	statePlan            = "plan" // New state
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
			Background(lipgloss.Color("#FF5555")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)

	logStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4"))

	timerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#50FA7B"))

	activeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#008080")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 2).
			Background(lipgloss.Color("#1A1B26"))

	approvalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFA500")).
			Padding(0, 1).
			MarginTop(1)

	searchStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#FFD700")).
			Foreground(lipgloss.Color("#000000")).
			Bold(true)
)

type stepMsg agent.StepResult
type errMsg error
type logMsg string

type model struct {
	agent          *agent.Agent
	state          string
	prevState      string
	history        []string
	systemLogs     []string
	viewport       viewport.Model
	logViewport    viewport.Model
	planViewport   viewport.Model // New
	textInput      textinput.Model
	searchInput    textinput.Model
	spinner        spinner.Model
	renderer       *glamour.TermRenderer // New: Cached
	lastTask       string
	statePath      string
	systemPrompt   string
	err            error
	width, height  int
	ready          bool
	showLogs       bool
	showPlan       bool // New
	startTime      time.Time
	elapsed        time.Duration
	scrubberOn     bool
	logChan        chan string
	pendingActions []agent.ToolCall
	autoTurns      int
	focusMode      bool
	inputHistory   []string
	historyIdx     int
}

func newModel(a *agent.Agent, statePath, systemPrompt string) model {
	ti := textinput.New()
	ti.Placeholder = "Enter task..."
	ti.Focus()
	ti.CharLimit = 512

	si := textinput.New()
	si.Placeholder = "Search history..."
	si.CharLimit = 64

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF"))

	logChan := make(chan string, 20)
	
	scrubberOn := false
	if sl, ok := a.LLM.(*provider.ScrubbingLLM); ok {
		scrubberOn = true
		sl.Logger = func(m string) { logChan <- m }
		if or, ok := sl.Inner.(*provider.OpenRouter); ok {
			or.Logger = func(m string) { logChan <- m }
		}
	} else if or, ok := a.LLM.(*provider.OpenRouter); ok {
		or.Logger = func(m string) { logChan <- m }
	}

	return model{
		agent:        a,
		state:        stateIdle,
		textInput:    ti,
		searchInput:  si,
		spinner:      s,
		statePath:    statePath,
		systemPrompt: systemPrompt,
		scrubberOn:   scrubberOn,
		logChan:      logChan,
		historyIdx:   -1,
	}
}

func (m model) renderMarkdown(text string) string {
	if m.renderer == nil {
		return text
	}
	out, _ := m.renderer.Render(text)
	return out
}

func waitForLog(c chan string) tea.Cmd {
	return func() tea.Msg {
		return logMsg(<-c)
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink, 
		m.spinner.Tick,
		waitForLog(m.logChan),
	)
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
		_, err := a.Approve(context.Background())
		if err != nil {
			return errMsg(err)
		}
		res, err := a.Step(context.Background(), "")
		if err != nil {
			return errMsg(err)
		}
		return stepMsg(res)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		siCmd tea.Cmd
		vpCmd tea.Cmd
		lvCmd tea.Cmd
		pvCmd tea.Cmd // Plan viewport
		sCmd  tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.state == stateHelp {
			m.state = m.prevState
			return m, nil
		}

		if m.state == stateSearch {
			switch msg.Type {
			case tea.KeyEsc, tea.KeyCtrlC:
				m.state = stateIdle
				m.searchInput.Blur()
				m.textInput.Focus()
				m.updateViewports(false)
				return m, nil
			case tea.KeyEnter:
				m.state = stateIdle
				m.searchInput.Blur()
				m.textInput.Focus()
				m.updateViewports(false)
				return m, nil
			}
			m.searchInput, siCmd = m.searchInput.Update(msg)
			m.updateViewports(false)
			return m, siCmd
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyCtrlL:
			if !m.showPlan {
				m.showLogs = !m.showLogs
				m.resizeViewports()
			}
			return m, nil

		case tea.KeyF1:
			m.prevState = m.state
			m.state = stateHelp
			return m, nil

		case tea.KeyF2: 
			return m.resetSession()

		case tea.KeyF3: 
			m.focusMode = !m.focusMode
			m.resizeViewports()
			return m, nil

		case tea.KeyF4: // Toggle Plan Mode
			if m.state == statePlan {
				m.state = stateIdle
				m.showPlan = false
			} else {
				m.state = statePlan
				m.showPlan = true
				m.loadPlan()
			}
			m.resizeViewports()
			return m, nil

		case tea.KeyF7, tea.KeyCtrlG:
			m.toggleSafety()
			return m, nil

		case tea.KeyUp:
			if m.state == stateIdle && len(m.inputHistory) > 0 {
				if m.historyIdx < len(m.inputHistory)-1 {
					m.historyIdx++
					m.textInput.SetValue(m.inputHistory[len(m.inputHistory)-1-m.historyIdx])
				}
				return m, nil
			}

		case tea.KeyDown:
			if m.state == stateIdle {
				if m.historyIdx > 0 {
					m.historyIdx--
					m.textInput.SetValue(m.inputHistory[len(m.inputHistory)-1-m.historyIdx])
				} else {
					m.historyIdx = -1
					m.textInput.SetValue("")
				}
				return m, nil
			}

		case tea.KeyEnter:
			if m.state == stateIdle {
				input := m.textInput.Value()
				if input == "" { break }
				if input == "clear" { return m.resetSession() }
				if input == "safety" {
					m.toggleSafety()
					m.textInput.SetValue("")
					return m, nil
				}
				
				m.inputHistory = append(m.inputHistory, input)
				m.historyIdx = -1

				m.lastTask = input
				m.history = append(m.history, lipgloss.NewStyle().Foreground(lipgloss.Color("#55FF55")).Render("> "+input))
				m.textInput.SetValue("")
				m.state = stateThinking
				m.autoTurns = 0
				m.startTime = time.Now()
				m.err = nil 
				return m, runStep(m.agent, input)
			}

			if m.state == statePendingApproval {
				m.history = append(m.history, infoStyle.Render("Approved."))
				m.state = stateThinking
				m.startTime = time.Now()
				m.pendingActions = nil
				m.err = nil
				return m, runApprove(m.agent)
			}

		case tea.KeyRunes:
			if m.state == statePendingApproval && msg.String() == "n" {
				m.history = append(m.history, errorStyle.Render("Cancelled."))
				m.state = stateIdle
				m.pendingActions = nil
				m.updateViewports(true)
				return m, nil
			}
			if msg.String() == "?" && m.state == stateIdle && m.textInput.Value() == "" {
				m.prevState = m.state
				m.state = stateHelp
				return m, nil
			}
			if msg.String() == "/" && m.state == stateIdle && m.textInput.Value() == "" {
				m.state = stateSearch
				m.searchInput.Focus()
				return m, nil
			}
			if (m.state == stateIdle || m.state == statePlan) && m.textInput.Value() == "" {
				switch msg.String() {
				case "j":
					if m.showPlan { m.planViewport.LineDown(1) } else { m.viewport.LineDown(1) }
					return m, nil
				case "k":
					if m.showPlan { m.planViewport.LineUp(1) } else { m.viewport.LineUp(1) }
					return m, nil
				case "g":
					if m.showPlan { m.planViewport.GotoTop() } else { m.viewport.GotoTop() }
					return m, nil
				case "G":
					if m.showPlan { m.planViewport.GotoBottom() } else { m.viewport.GotoBottom() }
					return m, nil
				}
			}
		}

	case logMsg:
		m.systemLogs = append(m.systemLogs, string(msg))
		if len(m.systemLogs) > 50 { m.systemLogs = m.systemLogs[1:] }
		return m, waitForLog(m.logChan)

	case stepMsg:
		m.elapsed = time.Since(m.startTime)
		m.autoTurns++

		if msg.Thought != "" {
			m.history = append(m.history, m.renderMarkdown(msg.Thought))
		}

		for _, tc := range msg.ToolCalls {
			m.history = append(m.history, actionStyle.Render(fmt.Sprintf("⚙️ Action: %s(%s)", tc.Name, tc.Args)))
		}

		if len(m.agent.History) > 0 {
			last := m.agent.History[len(m.agent.History)-1]
			if strings.HasPrefix(last.Content, "Observations:") {
				m.history = append(m.history, m.renderMarkdown(last.Content))
			}
		}

		if m.statePath != "" { m.agent.Save(m.statePath) }

		if strings.Contains(strings.ToLower(msg.Thought), "final answer") {
			m.state = stateIdle
			m.history = append(m.history, titleStyle.Render("DONE")+" "+timerStyle.Render(fmt.Sprintf("(%v)", m.elapsed.Round(time.Millisecond))))
			m.updateViewports(true)
			break
		}

		if msg.Status == agent.StatusPending {
			m.state = statePendingApproval
			m.pendingActions = msg.ToolCalls
			m.updateViewports(true)
			break
		}

		m.state = stateIdle
		m.updateViewports(true)
		break

	case errMsg:
		m.err = msg
		m.history = append(m.history, errorStyle.Render(fmt.Sprintf(" Error: %v ", msg)))
		m.state = stateIdle
		m.updateViewports(true)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewports()

	case spinner.TickMsg:
		m.spinner, sCmd = m.spinner.Update(msg)
		if m.state == stateThinking { m.elapsed = time.Since(m.startTime) }
	}

	m.textInput, tiCmd = m.textInput.Update(msg)
	m.searchInput, siCmd = m.searchInput.Update(msg)
	m.updateViewports(false)
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.planViewport, pvCmd = m.planViewport.Update(msg)
	m.logViewport, lvCmd = m.logViewport.Update(msg)

	return m, tea.Batch(tiCmd, siCmd, vpCmd, pvCmd, lvCmd, sCmd)
}

func (m *model) loadPlan() {
	data, err := os.ReadFile("PLAN.md")
	if err != nil {
		m.planViewport.SetContent(errorStyle.Render("No PLAN.md found."))
		return
	}
	m.planViewport.SetContent(m.renderMarkdown(string(data)))
}

func (m model) resetSession() (tea.Model, tea.Cmd) {
	m.agent.History = []provider.Message{}
	m.agent.TotalUsage = provider.Usage{}
	m.agent.AddSystemPrompt(m.systemPrompt)
	m.history = []string{infoStyle.Render("Full context and cache reset.")}
	m.textInput.SetValue("")
	if m.statePath != "" { os.Remove(m.statePath) }
	os.Remove(".armage_scrub_cache.json")
	m.err = nil
	m.pendingActions = nil
	m.autoTurns = 0
	m.updateViewports(true)
	return m, nil
}

func (m *model) toggleSafety() {
	m.agent.RequireApproval = !m.agent.RequireApproval
	status := "DISABLED"
	if m.agent.RequireApproval { status = "ENABLED" }
	m.systemLogs = append(m.systemLogs, infoStyle.Render("Safety Governor: "+status))
}

func (m *model) resizeViewports() {
	headerHeight := 3
	footerHeight := 8 
	if m.focusMode {
		headerHeight = 0
		footerHeight = 3
	}
	
	mainHeight := m.height - headerHeight - footerHeight
	if mainHeight < 1 { mainHeight = 1 }
	
	if m.showLogs && !m.focusMode && !m.showPlan {
		logHeight := 6
		if mainHeight > 10 {
			mainHeight -= (logHeight + 1)
			m.logViewport = viewport.New(m.width, logHeight)
			m.logViewport.Style = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, false, false).BorderForeground(lipgloss.Color("#44475A"))
		} else {
			m.showLogs = false
		}
	}

	// RESPONSIVE INPUTS
	m.textInput.Width = m.width - 4
	m.searchInput.Width = m.width / 2

	// RECACHE RENDERER
	m.renderer, _ = glamour.NewTermRenderer(
		glamour.WithStandardStyle("notty"),
		glamour.WithWordWrap(m.width - 4),
	)

	if !m.ready {
		m.viewport = viewport.New(m.width, mainHeight)
		m.planViewport = viewport.New(m.width, mainHeight)
		m.ready = true
	} else {
		m.viewport.Width = m.width
		m.viewport.Height = mainHeight
		m.planViewport.Width = m.width
		m.planViewport.Height = mainHeight
	}
	
	if m.showPlan {
		m.loadPlan()
	}
	
	m.updateViewports(true)
}

func (m *model) updateViewports(snapBottom bool) {
	content := strings.Join(m.history, "\n")
	search := m.searchInput.Value()
	if search != "" {
		content = strings.ReplaceAll(content, search, searchStyle.Render(search))
	}
	m.viewport.SetContent(content)
	if snapBottom { m.viewport.GotoBottom() }

	if m.showLogs {
		m.logViewport.SetContent(logStyle.Render("System Logs (Ctrl+L to hide):\n") + strings.Join(m.systemLogs, "\n"))
		m.logViewport.GotoBottom()
	}
}

func (m model) View() string {
	if !m.ready { return "\n  Initializing Armage TUI..." }
	if m.state == stateHelp { return m.helpView() }

	var header string
	if !m.focusMode {
		cwd, _ := os.Getwd()
		label := "HISTORY"
		if m.showPlan { label = "PLAN (F4 to exit)" }
		header = titleStyle.Render(" ARMAGE ") + " [" + label + "]  " + infoStyle.Render(fmt.Sprintf("Total: %d", m.agent.TotalUsage.TotalTokens)) + "  " + logStyle.Render(cwd) + "\n\n"
	}
	
	var status string
	switch m.state {
	case stateThinking:
		status = m.spinner.View() + " Thinking... " + timerStyle.Render(m.elapsed.Round(time.Second).String())
	case statePendingApproval:
		status = lipgloss.NewStyle().Background(lipgloss.Color("#FFA500")).Foreground(lipgloss.Color("#000000")).Render(" APPROVAL REQUIRED ")
	case statePaused:
		status = lipgloss.NewStyle().Background(lipgloss.Color("#7D56F4")).Foreground(lipgloss.Color("#FFFFFF")).Render(" PAUSED ")
	case stateSearch:
		status = searchStyle.Render(" SEARCH ") + " " + m.searchInput.View()
	case statePlan:
		status = activeStyle.Render(" PLAN VIEW ")
	default:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("#55FF55")).Render("●") + " Ready"
	}

	var approvalPanel string
	if m.state == statePendingApproval {
		var tools string
		for i, tc := range m.pendingActions {
			tools += fmt.Sprintf("%d. %s(%s)\n", i+1, actionStyle.Render(tc.Name), tc.Args)
		}
		approvalPanel = approvalStyle.Render(fmt.Sprintf("%s\n%s\n%s", 
			lipgloss.NewStyle().Bold(true).Render("Pending Actions:"),
			tools,
			thoughtStyle.Render("[Enter] Approve  [n] Cancel")))
	}

	var errorBar string
	if m.err != nil {
		errorBar = "\n" + errorStyle.Render(fmt.Sprintf(" Error: %v ", m.err))
	}

	governorStatus := "🛡️ ON"
	if !m.agent.RequireApproval { governorStatus = "🛡️ OFF" }

	footer := fmt.Sprintf("\n%s %s%s\n%s%s", 
		status, m.tokenGauge(), errorBar,
		approvalPanel,
		m.textInput.View())

	if !m.focusMode {
		footer += fmt.Sprintf("\n%s %s %s", 
			activeStyle.Render(m.agent.LLM.Model()),
			infoStyle.Render(governorStatus),
			infoStyle.Render(fmt.Sprintf("SCRUB: %v", m.scrubberOn)))
	}

	mainView := m.viewport.View()
	if m.showPlan {
		mainView = m.planViewport.View()
	}
	
	if m.showLogs && !m.focusMode && !m.showPlan { mainView += "\n" + m.logViewport.View() }

	return fmt.Sprintf("%s%s\n%s", header, mainView, footer)
}

func (m model) helpView() string {
	helpText := `# ARMAGE KEYBINDINGS

- **Enter**      : Submit task / Approve / Continue
- **/**          : Search History
- **Up / Down**  : Cycle Input History
- **j / k**      : Scroll Down / Up (Vim style)
- **g / G**      : Go to Top / Bottom
- **F1 / ?**     : Toggle this help screen
- **F3**         : Toggle Focus Mode (Max Real Estate)
- **F4**         : Toggle Plan View (PLAN.md)
- **F7 / Ctrl+G**: Toggle Safety Governor
- **Ctrl+L**     : Toggle System Logs
- **Ctrl+C**     : Quit Armage

# TEXT COMMANDS
- **clear**      : Full session reset
- **safety**     : Toggle Safety Governor
`
	
	return helpStyle.Render(m.renderMarkdown(helpText))
}

func (m model) tokenGauge() string {
	limit := 10000000 
	used := m.agent.TotalUsage.TotalTokens
	if used == 0 { return "" }
	
	percentage := (float64(used) / float64(limit)) * 100
	if percentage > 100 { percentage = 100 }
	
	width := 20
	filled := int(float64(used) / float64(limit) * float64(width))
	if filled > width { filled = width }
	
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	color := "#50FA7B" 
	if percentage > 90 { color = "#FF5555" } 
	
	return "Usage: " + lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("[" + bar + "]") + fmt.Sprintf(" %.1f%%", percentage)
}
