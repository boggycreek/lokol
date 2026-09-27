// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/probe"
	"github.com/boggycreek/lokol/liblokol/version"
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
	stepCount   int
	lastThought string
	lastTool    string
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
	m := NewWithSession(session, nil, yoloMode)
	m.verbose = verbose
	return m
}

// WithInitialPrompt pre-populates the prompt into the input area.
func (m Model) WithInitialPrompt(prompt string) Model {
	m.textarea.SetValue(prompt)
	return m
}

// NewWithSession creates and initializes the TUI model with an existing agent SessionCore.
func NewWithSession(session agent.SessionCore, hw *probe.HardwareProfile, yoloMode bool) Model {
	ta := textarea.New()
	ta.Placeholder = "Ask lokol to inspect files, write reports, or type /mode..."
	ta.Focus()
	ta.Prompt = "│ "
	ta.CharLimit = 1000
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

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
		headerHeight := 2
		inputHeight := 5
		spacing := 5
		vpHeight := msg.Height - headerHeight - inputHeight - spacing
		if vpHeight < 5 {
			vpHeight = 5
		}
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = vpHeight
		m.textarea.SetWidth(msg.Width - 4)
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
				return m, executeAction(m.session, m.pendingAct)
			}

			if m.state == StateIdle {
				input := strings.TrimSpace(m.textarea.Value())
				if input == "" {
					return m, nil
				}
				m.textarea.Reset()

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
				m.currentResp = ""
				m.tokenChan = make(chan string, 100)

				streamCtx, cancel := context.WithCancel(context.Background())
				m.streamCancel = cancel

				return m, tea.Batch(
					startStream(streamCtx, m.session, m.tokenChan),
					waitForToken(m.tokenChan),
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
				return m, executeAction(m.session, m.pendingAct)
			case "n", "N":
				m.state = StateIdle
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

				if m.yoloMode {
					// YOLO Mode: execute immediately without waiting for user approval
					m.state = StateExecutingAction
					if m.verbose {
						m.appendLog(act.VerboseDescription() + "\n")
					}
					return m, executeAction(m.session, act)
				}

				m.state = StateWaitingActionApproval
				box := actionBoxStyle.Render(fmt.Sprintf(
					"PROPOSED ACTION: %s\nPayload:\n%s\n\nPress [Enter] or [Y] to approve, [N] to reject | [Ctrl+Y] Auto-approve all",
					act.Name, act.Command,
				))
				m.appendLog("\n" + box + "\n")
			} else if act != nil && act.Name == "task_finish" {
				m.state = StateIdle
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
	currentMode := "general"
	if m.session != nil {
		currentMode = string(m.session.GetMode())
	}
	modeBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8BE9FD")).Render(fmt.Sprintf(" [Mode: %s]", currentMode))
	header += modeBadge
	if m.yoloMode {
		yoloBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render(" [YOLO ACTIVE]")
		header += yoloBadge
	}
	if m.verbose {
		verbBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9")).Render(" [VERBOSE]")
		header += verbBadge
	}

	// Real-time Context / KV Cache HUD
	if m.slotStatus != nil && m.slotStatus.NCtx > 0 {
		ctxUsed := m.slotStatus.NPromptTokens
		ctxMax := m.slotStatus.NCtx
		pct := (float64(ctxUsed) / float64(ctxMax)) * 100

		// Engine / Memory Status
		engineStatus := "Pure VRAM"
		if pct > 90.0 {
			engineStatus = "High Pressure"
		}

		hudCtxStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Bold(true)
		if pct > 75.0 {
			hudCtxStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C")).Bold(true)
		}
		if pct > 90.0 {
			hudCtxStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true)
		}

		ctxStr := fmt.Sprintf(" | Context: %d/%d (%.1f%%) [%s]", ctxUsed, ctxMax, pct, engineStatus)
		header += hudCtxStyle.Render(ctxStr)
	}

	var statusLine string
	switch m.state {
	case StateIdle:
		statusLine = "[Ready] Enter send | [/mode <mode>] Switch mode | [PgUp/PgDn] Scroll | [Ctrl+Y] YOLO | [Ctrl+C] Quit"
	case StateStreaming:
		if m.stepCount > 0 {
			toolHint := ""
			if m.lastTool != "" {
				toolHint = fmt.Sprintf(" (after %s)", m.lastTool)
			}
			statusLine = fmt.Sprintf("⚡ [Step %d%s] Reasoning and deciding next action... | [Ctrl+C] Stop", m.stepCount+1, toolHint)
		} else {
			statusLine = "[Thinking] Generating response from local engine... | [Ctrl+C] Stop"
		}
	case StateWaitingActionApproval:
		statusLine = "[Approval Needed] Review proposed action above. Press [Y] Approve, [N] Deny | [Ctrl+Y] Auto-approve all"
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
		statusLine = fmt.Sprintf("⚡ [Step %d] Executing %s locally... | [Ctrl+C] Stop", m.stepCount, toolDesc)
	}

	return fmt.Sprintf("%s\n\n%s\n\n%s\n%s",
		header,
		m.viewport.View(),
		statusLine,
		m.textarea.View(),
	)
}

// ViewportContent returns the raw wrapped text content currently held in the viewport.
func (m Model) ViewportContent() string {
	return wrapContent(m.chatLog, m.viewport.Width)
}
