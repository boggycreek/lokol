// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/guardrail"
	"github.com/boggycreek/lokol/liblokol/probe"
	"github.com/boggycreek/lokol/liblokol/version"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// State represents the current lifecycle state of the TUI.
type State int

const (
	StateIdle State = iota
	StateStreaming
	StateWaitingActionApproval
	StateExecutingAction
)

type TokenMsg string
type StreamDoneMsg string
type ActionExecutedMsg string
type ErrMsg error
type SlotTickMsg *agent.SlotStatus

// Model is the Bubble Tea application state.
// Bubble Tea requires Model to be passed by value in Update/View,
// so strings.Builder must NOT be embedded directly as a value field.
// We use simple string variables for immutable, safe value copying.
type Model struct {
	session      agent.SessionCore
	hardware     *probe.HardwareProfile
	state        State
	viewport     viewport.Model
	textarea     textarea.Model
	spinner      spinner.Model
	chatLog      string
	pendingAct   *agent.Action
	currentResp  string
	tokenChan    chan string
	streamCancel context.CancelFunc
	yoloMode     bool
	verbose      bool
	width        int
	height       int
	slotStatus   *agent.SlotStatus
	err          error

	// Internalized activity & step tracking
	stepCount           int
	lastThought         string
	lastTool            string
	consecutiveCommands int
	lastBadge           string
}

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#5A56E0")).
			Padding(0, 1)

	hudStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	userStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#04B575"))

	agentStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF79C6"))

	completeBannerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#50FA7B"))

	actionBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#F1FA8C")).
			Padding(0, 1).
			Foreground(lipgloss.Color("#F8F8F2"))

	outputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#6272A4")).
			Padding(0, 1).
			Foreground(lipgloss.Color("#8BE9FD"))

	toolBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4")).
			Italic(true)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#BD93F9")).
			Bold(true)

	spinnerHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4"))

	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#44475A")).
			Padding(0, 1)

	inputBoxFocusedStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	ruleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#44475A"))

	bottomProjStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#5A56E0")).
			Padding(0, 1)

	bottomModeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#8BE9FD")).
			Background(lipgloss.Color("#282A36")).
			Padding(0, 1)

	bottomKeyBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#282A36")).
			Foreground(lipgloss.Color("#F8F8F2")).
			Padding(0, 1)
)

// SetVerbose toggles verbose display of intermediate inferences and tool payloads.
func (m *Model) SetVerbose(v bool) {
	m.verbose = v
}

// Session returns the underlying agent SessionCore.
func (m Model) Session() agent.SessionCore {
	return m.session
}

// State returns the current interactive state of the TUI model.
func (m Model) State() State {
	return m.state
}

// StepCount returns the current autonomous step count.
func (m Model) StepCount() int {
	return m.stepCount
}

// New creates and initializes the TUI model.
func New(client *agent.Client, hw *probe.HardwareProfile, yoloMode bool, workDirOpt ...string) Model {
	workDir := ""
	if len(workDirOpt) > 0 {
		workDir = workDirOpt[0]
	}
	session := agent.NewSession(client, workDir)
	return NewWithSession(session, hw, yoloMode)
}

// NewModel creates a TUI model with the given session, yoloMode, and verbose setting.
func NewModel(session agent.SessionCore, yoloMode, verbose bool) Model {
	hw, _ := probe.Detect()
	m := NewWithSession(session, hw, yoloMode)
	m.verbose = verbose
	return m
}

// WithInitialPrompt pre-populates the prompt into the input area.
func (m Model) WithInitialPrompt(prompt string) Model {
	m.textarea.SetValue(prompt)
	lines := m.textarea.LineCount()
	if lines < 1 {
		lines = 1
	}
	if lines > 8 {
		lines = 8
	}
	m.textarea.SetHeight(lines)
	m.updateViewportDimensions()
	return m
}

func (m *Model) updateViewportDimensions() {
	height := m.height
	if height <= 0 {
		height = 24
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	inputHeight := m.textarea.Height()
	if inputHeight < 1 {
		inputHeight = 1
	}
	if inputHeight > 8 {
		inputHeight = 8
	}
	vpHeight := height - inputHeight - 6
	if vpHeight < 5 {
		vpHeight = 5
	}
	m.viewport.Height = vpHeight
	vpWidth := width - 4
	if vpWidth < 20 {
		vpWidth = 20
	}
	m.viewport.Width = vpWidth
}

// NewWithSession creates and initializes the TUI model with an existing agent SessionCore.
func NewWithSession(session agent.SessionCore, hw *probe.HardwareProfile, yoloMode bool) Model {
	ta := textarea.New()
	ta.Placeholder = "Ask lokol to inspect files, write reports, or type /mode..."
	ta.Focus()
	ta.Prompt = "> "
	ta.CharLimit = 1000
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#BD93F9"))

	vp := viewport.New(80, 20)
	initialText := "⚡ Welcome to lokol. Local-first autonomous AI agent.\nType your request below and press Enter to begin.\n\n"
	if yoloMode {
		initialText += "⚡ [YOLO MODE ENGAGED] Autonomous command execution without confirmation.\n\n"
	}
	vp.SetContent(wrapContent(initialText, 76))

	return Model{
		session:   session,
		hardware:  hw,
		state:     StateIdle,
		viewport:  vp,
		textarea:  ta,
		spinner:   s,
		tokenChan: make(chan string, 100),
		yoloMode:  yoloMode,
		chatLog:   initialText,
	}
}

func pollSlotStatus(session agent.SessionCore) tea.Cmd {
	return func() tea.Msg {
		if session == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		slot, err := session.GetSlotStatus(ctx)
		if err != nil {
			return nil
		}
		return SlotTickMsg(slot)
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
		pollSlotStatus(m.session),
	)
}

func waitForToken(tokenChan chan string) tea.Cmd {
	return func() tea.Msg {
		token, ok := <-tokenChan
		if !ok {
			return StreamDoneMsg("")
		}
		return TokenMsg(token)
	}
}

func startStream(ctx context.Context, session agent.SessionCore, tokenChan chan string) tea.Cmd {
	return func() tea.Msg {
		if session == nil {
			close(tokenChan)
			return ErrMsg(fmt.Errorf("session is nil"))
		}
		fullContent, err := session.StreamTurn(ctx, tokenChan)
		close(tokenChan)
		if err != nil {
			return ErrMsg(err)
		}
		return StreamDoneMsg(fullContent)
	}
}

func executeAction(session agent.SessionCore, act *agent.Action) tea.Cmd {
	return func() tea.Msg {
		if act == nil {
			return ActionExecutedMsg("")
		}
		if session == nil {
			return ActionExecutedMsg("[Error: session is nil]")
		}
		out, err := session.ExecuteAction(context.Background(), act)
		if err != nil {
			return ActionExecutedMsg(fmt.Sprintf("[Error: %v]\n%s", err, out))
		}
		return ActionExecutedMsg(out)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var vpCmd tea.Cmd

	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case SlotTickMsg:
		if msg != nil {
			m.slotStatus = msg
		}
		// Schedule next poll in 3 seconds
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			if m.session == nil {
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			slot, err := m.session.GetSlotStatus(ctx)
			if err != nil {
				return nil
			}
			return SlotTickMsg(slot)
		})

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		taWidth := msg.Width - 4
		if taWidth < 20 {
			taWidth = 20
		}
		m.textarea.SetWidth(taWidth)
		m.updateViewportDimensions()
		m.viewport.SetContent(wrapContent(m.chatLog, m.viewport.Width))
		m.viewport.GotoBottom()

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.state == StateStreaming || m.state == StateExecutingAction {
				if m.streamCancel != nil {
					m.streamCancel()
					m.streamCancel = nil
				}
				m.state = StateIdle
				m.stepCount = 0
				m.lastThought = ""
				m.lastTool = ""
				m.consecutiveCommands = 0
				m.lastBadge = ""
				m.appendLog("\n[Stream cancelled by user (Ctrl+C). Slot released.]\n")
				return m, nil
			}
			if m.session != nil {
				abortCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				_ = m.session.Abort(abortCtx)
				cancel()
			}
			return m, tea.Quit
		case tea.KeyEsc:
			if m.state == StateStreaming || m.state == StateExecutingAction {
				if m.streamCancel != nil {
					m.streamCancel()
					m.streamCancel = nil
				}
				m.state = StateIdle
				m.stepCount = 0
				m.lastThought = ""
				m.lastTool = ""
				m.consecutiveCommands = 0
				m.lastBadge = ""
				m.appendLog("\n[Stream aborted (Esc). Slot released.]\n")
				return m, nil
			}
			return m, nil
		case tea.KeyCtrlY:
			m.yoloMode = !m.yoloMode
			if m.yoloMode {
				m.appendLog("⚡ [YOLO MODE ENGAGED] Autonomous command execution active.\n")
			} else {
				m.appendLog("🛡️ [SAFE MODE ENGAGED] Manual action approval required.\n")
			}
			return m, nil
		case tea.KeyCtrlV:
			m.verbose = !m.verbose
			if m.verbose {
				m.appendLog("🔍 [VERBOSE ON] Showing intermediate thought streams and tool payloads.\n\n")
			} else {
				m.appendLog("✨ [VERBOSE OFF] Intermediary inferences and tool activity internalized.\n\n")
			}
			return m, nil
		case tea.KeyPgUp:
			m.viewport.LineUp(5)
			return m, nil
		case tea.KeyPgDown:
			m.viewport.LineDown(5)
			return m, nil
		case tea.KeyEnter:
			if m.state == StateWaitingActionApproval {
				// Approve action
				m.state = StateExecutingAction
				m.appendLog(outputBoxStyle.Render("⚡ Executing command: " + m.pendingAct.Command))
				return m, tea.Batch(
					executeAction(m.session, m.pendingAct),
					m.spinner.Tick,
				)
			}

			if m.state == StateIdle {
				input := strings.TrimSpace(m.textarea.Value())
				if input == "" {
					return m, nil
				}
				m.textarea.Reset()
				m.textarea.SetHeight(1)
				m.updateViewportDimensions()

				// Handle /mode slash command
				if strings.HasPrefix(input, "/mode") {
					parts := strings.Fields(input)
					if len(parts) >= 2 {
						targetMode, err := agent.ParseMode(parts[1])
						if err != nil {
							m.appendLog(fmt.Sprintf("⚠️ [Invalid Mode] %v\n\n", err))
						} else if m.session != nil {
							m.session.SetMode(targetMode)
							m.appendLog(fmt.Sprintf("🔄 [Mode Switched] Active persona is now: %s\n\n", targetMode))
						}
					} else {
						curr := "general"
						if m.session != nil {
							curr = string(m.session.GetMode())
						}
						m.appendLog(fmt.Sprintf("ℹ️ Current mode: %s. Use '/mode general', '/mode coding', or '/mode moe'.\n\n", curr))
					}
					return m, nil
				}

				m.appendLog(userStyle.Render("User: ") + input + "\n\n")
				if m.session != nil {
					m.session.AppendUserMessage(input)
				}
				m.state = StateStreaming
				m.stepCount = 0
				m.lastThought = ""
				m.lastTool = ""
				m.consecutiveCommands = 0
				m.lastBadge = ""
				m.currentResp = ""
				m.tokenChan = make(chan string, 100)

				streamCtx, cancel := context.WithCancel(context.Background())
				m.streamCancel = cancel

				return m, tea.Batch(
					startStream(streamCtx, m.session, m.tokenChan),
					waitForToken(m.tokenChan),
					m.spinner.Tick,
				)
			}
		}

		// Allow Alt+Up/Down or Ctrl+Up/Down for scrolling when typing in textarea
		if msg.Type == tea.KeyUp && (msg.Alt || m.state != StateIdle) {
			m.viewport.LineUp(2)
			return m, nil
		}
		if msg.Type == tea.KeyDown && (msg.Alt || m.state != StateIdle) {
			m.viewport.LineDown(2)
			return m, nil
		}

		if m.state == StateWaitingActionApproval {
			switch msg.String() {
			case "y", "Y":
				m.state = StateExecutingAction
				m.appendLog(outputBoxStyle.Render("⚡ Executing approved command: " + m.pendingAct.Command))
				return m, tea.Batch(
					executeAction(m.session, m.pendingAct),
					m.spinner.Tick,
				)
			case "n", "N":
				m.state = StateIdle
				m.consecutiveCommands = 0
				m.lastBadge = ""
				m.appendLog("[Action rejected by user]\n")
				if m.session != nil {
					m.session.AppendUserMessage("User rejected the action proposal. Please decide on an alternative or ask for clarification.")
				}
				m.pendingAct = nil
				return m, nil
			}
		}

	case TokenMsg:
		if m.state == StateStreaming {
			m.currentResp += string(msg)
			if m.verbose {
				// Filter out action XML from the live stream display in verbose mode
				displayStr := m.currentResp
				if idx := strings.Index(displayStr, "<action"); idx != -1 {
					displayStr = strings.TrimSpace(displayStr[:idx])
				}
				if displayStr != "" {
					m.viewport.SetContent(wrapContent(m.chatLog+agentStyle.Render("lokol: ")+displayStr, m.viewport.Width))
				}
				m.viewport.GotoBottom()
			}
			return m, waitForToken(m.tokenChan)
		}

	case StreamDoneMsg:
		m.streamCancel = nil
		if m.state == StateStreaming {
			response := string(msg)
			if response == "" {
				response = m.currentResp
			}
			if m.session != nil {
				m.session.AppendAssistantMessage(response)
			}

			// Check for actions
			act := agent.ParseAction(response)
			if act != nil && act.Name != "task_finish" {
				m.pendingAct = act
				m.stepCount++
				m.lastThought = act.CleanThought
				m.lastTool = act.Name

				if m.verbose && act.CleanThought != "" {
					m.appendLog(agentStyle.Render("lokol: ") + act.CleanThought + "\n")
				}

				workDir := "."
				if m.session != nil && m.session.GetWorkDir() != "" {
					workDir = m.session.GetWorkDir()
				}
				guard := guardrail.New(workDir)
				perm := guard.CheckPermission(context.Background(), guardrail.ActionCandidate{
					Name:    act.Name,
					Command: act.Command,
					Path:    act.TargetSummary(),
				})

				if perm.Status != guardrail.StatusAllowed && m.yoloMode {
					// Disengage automatic execution in YOLO mode when a security boundary is tripped
					m.appendLog(fmt.Sprintf("⚠️  [GUARDRAIL INTERCEPT] Autonomous execution paused: %s\n", perm.Reason))
				} else if m.yoloMode {
					// YOLO Mode: execute immediately without waiting for user approval
					m.state = StateExecutingAction
					if m.verbose {
						m.appendLog(act.VerboseDescription() + "\n")
					}
					return m, tea.Batch(
						executeAction(m.session, act),
						m.spinner.Tick,
					)
				}

				m.state = StateWaitingActionApproval
				warningBadge := ""
				if perm.Status != guardrail.StatusAllowed {
					warningBadge = fmt.Sprintf("⚠️  SECURITY WARNING: %s\n\n", perm.Reason)
				}
				box := actionBoxStyle.Render(fmt.Sprintf(
					"%sPROPOSED ACTION: %s\nPayload:\n%s\n\nPress [Enter] or [Y] to approve, [N] to reject | [Ctrl+Y] Auto-approve all",
					warningBadge, act.Name, act.Command,
				))
				m.appendLog("\n" + box + "\n")
			} else if act != nil && act.Name == "task_finish" {
				m.state = StateIdle
				m.consecutiveCommands = 0
				m.lastBadge = ""
				finalText := strings.TrimSpace(act.Command)
				if act.CleanThought != "" {
					if finalText != "" && !strings.Contains(act.CleanThought, finalText) {
						finalText = act.CleanThought + "\n\n" + finalText
					} else if finalText == "" {
						finalText = act.CleanThought
					}
				}
				if finalText == "" {
					finalText = "Task completed successfully."
				}

				stepSuffix := ""
				if m.stepCount > 0 {
					plural := ""
					if m.stepCount > 1 {
						plural = "s"
					}
					stepSuffix = fmt.Sprintf(" • Resolved in %d autonomous step%s", m.stepCount, plural)
				}
				banner := completeBannerStyle.Render(fmt.Sprintf("✅ Task Complete%s", stepSuffix))

				m.appendLog(agentStyle.Render("lokol: ") + finalText + "\n\n" + banner + "\n\n")
				m.stepCount = 0
				m.lastThought = ""
				m.lastTool = ""
			} else {
				m.state = StateIdle
				m.consecutiveCommands = 0
				m.lastBadge = ""
				m.appendLog(agentStyle.Render("lokol: ") + strings.TrimSpace(response) + "\n\n")
				m.stepCount = 0
				m.lastThought = ""
				m.lastTool = ""
			}
			return m, nil
		}

	case ActionExecutedMsg:
		output := string(msg)
		if m.verbose {
			m.appendLog("✓ Executed successfully\n")
		} else if m.pendingAct != nil {
			m.consecutiveCommands++
			if m.consecutiveCommands == 1 {
				badge := formatCompactAction(m.pendingAct)
				badgeStr := toolBadgeStyle.Render(badge) + "\n\n"
				m.lastBadge = badgeStr
				m.appendLog(badgeStr)
			} else {
				newBadge := fmt.Sprintf("Ran %d commands ▸", m.consecutiveCommands)
				newBadgeStr := toolBadgeStyle.Render(newBadge) + "\n\n"
				if m.lastBadge != "" && strings.HasSuffix(m.chatLog, m.lastBadge) {
					m.chatLog = strings.TrimSuffix(m.chatLog, m.lastBadge) + newBadgeStr
					m.lastBadge = newBadgeStr
					w := m.viewport.Width
					if w <= 0 {
						w = 76
					}
					m.viewport.SetContent(wrapContent(m.chatLog, w))
					m.viewport.GotoBottom()
				} else {
					m.lastBadge = newBadgeStr
					m.appendLog(newBadgeStr)
				}
			}
		}

		// Feed tool output back to agent session
		if m.session != nil {
			m.session.AppendActionResult(output, nil)
		}

		// Trigger next agent iteration
		m.state = StateStreaming
		m.currentResp = ""
		m.tokenChan = make(chan string, 100)

		streamCtx, cancel := context.WithCancel(context.Background())
		m.streamCancel = cancel

		return m, tea.Batch(
			startStream(streamCtx, m.session, m.tokenChan),
			waitForToken(m.tokenChan),
			m.spinner.Tick,
		)

	case ErrMsg:
		m.err = msg
		m.state = StateIdle
		m.streamCancel = nil
		if !errors.Is(msg, context.Canceled) && !strings.Contains(msg.Error(), "context canceled") {
			m.appendLog(fmt.Sprintf("[Error: %v]\n", msg))
		}
	}

	if m.state == StateIdle {
		var taCmd tea.Cmd
		m.textarea, taCmd = m.textarea.Update(msg)
		cmds = append(cmds, taCmd)

		lines := m.textarea.LineCount()
		if lines < 1 {
			lines = 1
		}
		if lines > 8 {
			lines = 8
		}
		if m.textarea.Height() != lines {
			m.textarea.SetHeight(lines)
			m.updateViewportDimensions()
		}
	}

	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

func wrapContent(content string, width int) string {
	if width <= 0 {
		return content
	}
	return ansi.Wrap(content, width, "")
}

func (m *Model) appendLog(text string) {
	m.chatLog += text
	w := m.viewport.Width
	if w <= 0 {
		w = 76
	}
	m.viewport.SetContent(wrapContent(m.chatLog, w))
	m.viewport.GotoBottom()
}

func (m Model) View() string {
	gpuInfo := "CPU Only"
	if m.hardware != nil && m.hardware.GPUName != "" {
		gpuInfo = fmt.Sprintf("%s (%s)", m.hardware.GPUName, m.hardware.HumanVRAM())
	}

	header := headerStyle.Render(fmt.Sprintf(" ⚡ lokol %s ", version.Version)) + "  " +
		hudStyle.Render(fmt.Sprintf("GPU: %s", gpuInfo))

	// Real-time Context / KV Cache HUD in top status bar following GPU info
	if m.slotStatus != nil && m.slotStatus.NCtx > 0 {
		ctxUsed := m.slotStatus.NPromptTokens
		ctxMax := m.slotStatus.NCtx
		pct := (float64(ctxUsed) / float64(ctxMax)) * 100

		engineStatus := "Pure VRAM"
		ctxFg := lipgloss.Color("#50FA7B")
		if pct > 75.0 {
			ctxFg = lipgloss.Color("#FFB86C")
		}
		if pct > 90.0 {
			engineStatus = "High Pressure"
			ctxFg = lipgloss.Color("#FF5555")
		}

		ctxStr := fmt.Sprintf(" | Context: %d/%d (%.1f%%) [%s]", ctxUsed, ctxMax, pct, engineStatus)
		hudCtxStyle := lipgloss.NewStyle().Foreground(ctxFg).Bold(true)
		header += hudCtxStyle.Render(ctxStr)
	}

	if m.yoloMode {
		yoloBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render(" [YOLO ACTIVE]")
		header += yoloBadge
	}
	if m.verbose {
		verbBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")).Render(" [VERBOSE]")
		header += verbBadge
	}

	w := m.width
	if w <= 0 {
		w = 80
	}

	// Upper separator rule between chat viewport and prompt input (integrates spinner/interruption when active)
	var upperRule string
	switch m.state {
	case StateStreaming:
		toolHint := ""
		if m.stepCount > 0 && m.lastTool != "" {
			toolHint = fmt.Sprintf(" (after %s)", m.lastTool)
		}
		statusText := "Thinking..."
		if m.stepCount > 0 {
			statusText = fmt.Sprintf("[Step %d%s] Reasoning and deciding next action...", m.stepCount+1, toolHint)
		}
		left := fmt.Sprintf("── %s %s ", m.spinner.View(), statusText)
		right := " esc to stop ──"
		leftW := ansi.StringWidth(left)
		rightW := ansi.StringWidth(right)
		gap := w - leftW - rightW
		if gap < 1 {
			gap = 1
		}
		upperRule = spinnerStyle.Render(left) + ruleStyle.Render(strings.Repeat("─", gap)) + spinnerHintStyle.Render(right)

	case StateWaitingActionApproval:
		left := "── ⏸ [Approval Needed] Review proposed action above "
		right := " [Y] Approve · [N] Deny · [Ctrl+Y] Auto ──"
		leftW := ansi.StringWidth(left)
		rightW := ansi.StringWidth(right)
		gap := w - leftW - rightW
		if gap < 1 {
			gap = 1
		}
		approvalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F1FA8C")).Bold(true)
		upperRule = approvalStyle.Render(left) + ruleStyle.Render(strings.Repeat("─", gap)) + spinnerHintStyle.Render(right)

	case StateExecutingAction:
		toolDesc := "tool"
		if m.pendingAct != nil {
			target := m.pendingAct.TargetSummary()
			if target != "" {
				toolDesc = fmt.Sprintf("%s: %s", m.pendingAct.Name, target)
			} else {
				toolDesc = m.pendingAct.Name
			}
		}
		left := fmt.Sprintf("── %s [Step %d] Executing %s locally... ", m.spinner.View(), m.stepCount, toolDesc)
		right := " esc to stop ──"
		leftW := ansi.StringWidth(left)
		rightW := ansi.StringWidth(right)
		gap := w - leftW - rightW
		if gap < 1 {
			gap = 1
		}
		execStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Bold(true)
		upperRule = execStyle.Render(left) + ruleStyle.Render(strings.Repeat("─", gap)) + spinnerHintStyle.Render(right)

	default:
		upperRule = ruleStyle.Render(strings.Repeat("─", w))
	}

	// Lower separator rule between prompt input and status lines
	lowerRule := ruleStyle.Render(strings.Repeat("─", w))

	// Hotkey options and active state hints directly below lower rule
	var hints string
	switch m.state {
	case StateIdle:
		hints = "[Ready] Enter send · [/mode] Switch · [PgUp/PgDn] Scroll · [Ctrl+Y] YOLO · [Ctrl+C] Quit"
	case StateStreaming:
		hints = "[Thinking] Generating response from local engine... · [Esc] Stop"
	case StateWaitingActionApproval:
		hints = "[Approval Needed] [Y] Approve · [N] Deny · [Ctrl+Y] Auto-approve all"
	case StateExecutingAction:
		hints = "[Executing] Local runner active · [Esc] Stop"
	}
	hotkeyLine := bottomKeyBarStyle.Render(hints)

	// Bottom-most Line: Left = Project path & branch pill, Right = Right-justified Active Mode
	workDir := "."
	if m.session != nil && m.session.GetWorkDir() != "" {
		workDir = m.session.GetWorkDir()
	}
	projectName := "workspace"
	if abs, err := filepath.Abs(workDir); err == nil {
		base := filepath.Base(abs)
		if base != "/" && base != "." {
			projectName = base
		}
	}
	branch := getGitBranch(workDir)
	projText := fmt.Sprintf("~ %s", projectName)
	if branch != "" {
		projText = fmt.Sprintf("~ %s (%s)", projectName, branch)
	}
	projBadge := bottomProjStyle.Render(projText)

	currMode := "general"
	if m.session != nil {
		currMode = string(m.session.GetMode())
	}
	modeBadge := bottomModeStyle.Render(fmt.Sprintf("[Mode: %s]", currMode))

	leftWidth := ansi.StringWidth(projBadge)
	rightWidth := ansi.StringWidth(modeBadge)
	gap := w - leftWidth - rightWidth
	if gap < 1 {
		gap = 1
	}
	infoLine := projBadge + strings.Repeat(" ", gap) + modeBadge

	return fmt.Sprintf("%s\n\n%s\n%s\n%s\n%s\n%s\n%s",
		header,
		m.viewport.View(),
		upperRule,
		m.textarea.View(),
		lowerRule,
		hotkeyLine,
		infoLine,
	)
}

// ViewportContent returns the raw wrapped text content currently held in the viewport.
func (m Model) ViewportContent() string {
	return wrapContent(m.chatLog, m.viewport.Width)
}

func getGitBranch(workDir string) string {
	if workDir == "" {
		workDir = "."
	}
	gitPath := filepath.Join(workDir, ".git")
	fi, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	headPath := filepath.Join(gitPath, "HEAD")
	if !fi.IsDir() {
		data, err := os.ReadFile(gitPath)
		if err == nil {
			line := strings.TrimSpace(string(data))
			if strings.HasPrefix(line, "gitdir: ") {
				gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
				if !filepath.IsAbs(gitDir) {
					gitDir = filepath.Join(workDir, gitDir)
				}
				headPath = filepath.Join(gitDir, "HEAD")
			}
		}
	}
	headBytes, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	headContent := strings.TrimSpace(string(headBytes))
	if strings.HasPrefix(headContent, "ref: refs/heads/") {
		return strings.TrimPrefix(headContent, "ref: refs/heads/")
	}
	if len(headContent) >= 7 {
		return headContent[:7]
	}
	return headContent
}

func formatCompactAction(act *agent.Action) string {
	if act == nil {
		return "Ran command ▸"
	}
	if act.Name == "exec_bash" {
		cmdStr := strings.TrimSpace(act.Command)
		if idx := strings.Index(cmdStr, "\n"); idx != -1 {
			cmdStr = strings.TrimSpace(cmdStr[:idx])
		}
		if len(cmdStr) > 40 {
			cmdStr = cmdStr[:37] + "..."
		}
		if cmdStr != "" {
			return fmt.Sprintf("Ran %s ▸", cmdStr)
		}
		return "Ran command ▸"
	}
	target := act.TargetSummary()
	if target != "" {
		if len(target) > 30 {
			target = target[:27] + "..."
		}
		return fmt.Sprintf("Ran %s %s ▸", act.Name, target)
	}
	return fmt.Sprintf("Ran %s ▸", act.Name)
}
