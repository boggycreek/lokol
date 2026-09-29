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
	"github.com/boggycreek/lokol/liblokol/regulator"
	"github.com/boggycreek/lokol/liblokol/model"
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

	// Loop circuit breaker tracking
	lastSig     string
	repeatCount int
	regulator   *regulator.Regulator
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

	dashLightYOLOActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF5555"))

	dashLightYOLOInactiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4"))

	dashLightChatStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#8BE9FD"))

	dashLightCodeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#50FA7B"))

	dashLightExpertStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF79C6"))
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

// PendingAction returns the currently proposed action waiting for approval, if any.
func (m Model) PendingAction() *agent.Action {
	return m.pendingAct
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
	ta.Placeholder = "Ask lokol to inspect files, write reports, or type /mode, /yolo, /clear..."
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

	workDir := "."
	if session != nil && session.GetWorkDir() != "" {
		workDir = session.GetWorkDir()
	}

	m := Model{
		session:   session,
		hardware:  hw,
		state:     StateIdle,
		viewport:  vp,
		textarea:  ta,
		spinner:   s,
		tokenChan: make(chan string, 100),
		yoloMode:  yoloMode,
		chatLog:   initialText,
		regulator: regulator.New(workDir),
	}
	if session != nil {
		m.regulator.SetSlotStatusProvider(regulator.SlotStatusFunc(func(ctx context.Context) (*regulator.SlotMetrics, error) {
			slot, err := session.GetSlotStatus(ctx)
			if err != nil {
				return nil, err
			}
			return &regulator.SlotMetrics{
				NCtx:          slot.NCtx,
				NPromptTokens: slot.NPromptTokens,
				IsProcessing:  slot.IsProcessing,
			}, nil
		}))
	}
	return m
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
		case tea.KeyCtrlT:
			curr := agent.ModeGeneral
			if m.session != nil {
				curr = m.session.GetMode()
			}
			var nextMode agent.Mode
			switch curr {
			case agent.ModeGeneral:
				nextMode = agent.ModeCoding
			case agent.ModeCoding:
				nextMode = agent.ModeMoE
			default:
				nextMode = agent.ModeGeneral
			}
			if m.session != nil {
				m.session.SetMode(nextMode)
			}
			m.appendLog(fmt.Sprintf("🔄 [Mode Switched] Active persona is now: %s\n\n", nextMode))
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
				if m.verbose && m.pendingAct != nil {
					m.appendLog(m.pendingAct.VerboseDescription() + "\n\n")
				}
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

				// Handle slash commands
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
						// Circular mode toggle: general -> coding -> moe -> general
						curr := agent.ModeGeneral
						if m.session != nil {
							curr = m.session.GetMode()
						}
						var nextMode agent.Mode
						switch curr {
						case agent.ModeGeneral:
							nextMode = agent.ModeCoding
						case agent.ModeCoding:
							nextMode = agent.ModeMoE
						default:
							nextMode = agent.ModeGeneral
						}
						if m.session != nil {
							m.session.SetMode(nextMode)
						}
						m.appendLog(fmt.Sprintf("🔄 [Mode Switched] Active persona is now: %s\n\n", nextMode))
					}
					return m, nil
				} else if input == "/yolo" {
					m.yoloMode = !m.yoloMode
					if m.yoloMode {
						m.appendLog("⚡ [YOLO MODE ENGAGED] Autonomous command execution active.\n\n")
					} else {
						m.appendLog("🛡️ [SAFE MODE ENGAGED] Manual action approval required.\n\n")
					}
					return m, nil
				} else if input == "/clear" {
					if m.session != nil {
						m.session.Reset()
					}
					m.chatLog = ""
					m.stepCount = 0
					m.lastThought = ""
					m.lastTool = ""
					m.appendLog("🧹 [Session Cleared] Chat history and conversation context reset.\n\n")
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
				m.lastSig = ""
				m.repeatCount = 0
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
				if m.verbose && m.pendingAct != nil {
					m.appendLog(m.pendingAct.VerboseDescription() + "\n\n")
				}
				return m, tea.Batch(
					executeAction(m.session, m.pendingAct),
					m.spinner.Tick,
				)
			case "n", "N":
				m.state = StateIdle
				m.consecutiveCommands = 0
				m.lastBadge = ""
				m.lastSig = ""
				m.repeatCount = 0
				m.appendLog("[Action rejected by user]\n\n")
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

				currentSig := act.Name + ":" + strings.TrimSpace(act.Command)
				if currentSig == m.lastSig {
					m.repeatCount++
				} else {
					m.lastSig = currentSig
					m.repeatCount = 1
				}

				if m.repeatCount >= 4 {
					m.state = StateIdle
					m.consecutiveCommands = 0
					m.lastBadge = ""
					m.pendingAct = nil
					m.lastSig = ""
					m.repeatCount = 0
					m.appendLog(fmt.Sprintf("⚠️  [Loop Circuit Breaker] Action %s repeated %d times consecutively without progress. Halting loop to protect KV cache.\n\n", act.Name, 4))
					return m, nil
				}

				if m.verbose && act.CleanThought != "" {
					m.appendLog(agentStyle.Render("lokol: ") + act.CleanThought + "\n")
				}

				if m.regulator == nil {
					workDir := "."
					if m.session != nil && m.session.GetWorkDir() != "" {
						workDir = m.session.GetWorkDir()
					}
					m.regulator = regulator.New(workDir)
					if m.session != nil {
						m.regulator.SetSlotStatusProvider(regulator.SlotStatusFunc(func(ctx context.Context) (*regulator.SlotMetrics, error) {
							slot, err := m.session.GetSlotStatus(ctx)
							if err != nil {
								return nil, err
							}
							return &regulator.SlotMetrics{
								NCtx:          slot.NCtx,
								NPromptTokens: slot.NPromptTokens,
								IsProcessing:  slot.IsProcessing,
							}, nil
						}))
					}
				}

				actionPath := act.TargetSummary()
				if act.Name == "find_files" || act.Name == "search_code" {
					actionPath = regulator.ExtractTagContent(act.Command, "path")
				}

				perm := m.regulator.CheckPermission(context.Background(), regulator.ActionCandidate{
					Name:    act.Name,
					Command: act.Command,
					Path:    actionPath,
				})

				if perm.Status != regulator.StatusAllowed && m.yoloMode {
					// Disengage automatic execution in YOLO mode when a security boundary is tripped
					m.appendLog(fmt.Sprintf("⚠️  [REGULATOR INTERCEPT] Autonomous execution paused: %s\n", perm.Reason))
				} else if m.yoloMode {
					// YOLO Mode: execute immediately without waiting for user approval
					m.state = StateExecutingAction
					if m.verbose {
						m.appendLog(act.VerboseDescription() + "\n\n")
					}
					return m, tea.Batch(
						executeAction(m.session, act),
						m.spinner.Tick,
					)
				}

				m.consecutiveCommands = 0
				m.lastBadge = ""
				m.state = StateWaitingActionApproval
				warningBadge := ""
				if perm.Status != regulator.StatusAllowed {
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
				m.lastSig = ""
				m.repeatCount = 0
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
				m.lastSig = ""
				m.repeatCount = 0
				m.appendLog(agentStyle.Render("lokol: ") + strings.TrimSpace(response) + "\n\n")
				m.stepCount = 0
				m.lastThought = ""
				m.lastTool = ""
			}
			return m, nil
		}

	case ActionExecutedMsg:
		output := string(msg)
		isError := strings.HasPrefix(output, "[Error: ")

		if m.verbose {
			if isError {
				m.appendLog("✗ Execution failed\n\n")
			} else {
				m.appendLog("✓ Executed successfully\n\n")
			}
		} else if m.pendingAct != nil {
			if isError {
				errSummary := output
				if idx := strings.Index(errSummary, "\n"); idx != -1 {
					errSummary = errSummary[:idx]
				}
				errSummary = strings.TrimPrefix(errSummary, "[Error: ")
				errSummary = strings.TrimSuffix(errSummary, "]")
				if len(errSummary) > 60 {
					errSummary = errSummary[:57] + "..."
				}
				badge := fmt.Sprintf("Failed %s (%s) ✗", m.pendingAct.Name, errSummary)
				badgeStr := toolBadgeStyle.Render(badge) + "\n\n"
				m.lastBadge = badgeStr
				m.appendLog(badgeStr)
			} else {
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
		}

		// Inject system intervention if repeating action loop detected
		sessionOutput := output
		if m.repeatCount >= 2 && m.pendingAct != nil {
			if isError {
				if m.pendingAct.Name == "read_window" {
					sessionOutput += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have attempted this exact read %d times consecutively and it failed. DO NOT repeat this action. If you are trying to inspect a directory, use <action name=\"find_files\"><pattern>*</pattern></action> instead.]", m.repeatCount)
				} else if m.pendingAct.Name == "replace_file" {
					sessionOutput += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have attempted this EXACT edit %d times consecutively and it failed. DO NOT repeat this action. Use <action name=\"read_window\"> to inspect lines, or use <action name=\"write_file\"> to rewrite completely.]", m.repeatCount)
				} else {
					sessionOutput += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have attempted this EXACT action %d times consecutively and it failed. DO NOT repeat this action. Change your strategy or call <action name=\"task_finish\">.]", m.repeatCount)
				}
			} else {
				if m.pendingAct.Name == "read_window" {
					sessionOutput += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have read this exact line window %d times consecutively. You now have the contents. Take action to edit the file or proceed with your next step.]", m.repeatCount)
				}
			}
		}

		// Feed tool output back to agent session
		if m.session != nil {
			m.session.AppendActionResult(sessionOutput, nil)
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

func cleanModelName(raw string) string {
	if raw == "" {
		return ""
	}
	base := filepath.Base(raw)
	base = strings.TrimSuffix(base, ".gguf")
	base = strings.TrimSuffix(base, ".bin")
	lower := strings.ToLower(base)
	switch {
	case strings.Contains(lower, "qwen2.5-coder-7b"):
		return "Qwen 2.5 Coder 7B"
	case strings.Contains(lower, "qwen2.5-coder-3b"):
		return "Qwen 2.5 Coder 3B"
	case strings.Contains(lower, "qwen2.5-coder-1.5b"):
		return "Qwen 2.5 Coder 1.5B"
	case strings.Contains(lower, "llama-3.1-8b"):
		return "Meta Llama 3.1 8B"
	case strings.Contains(lower, "llama-3.2-3b"):
		return "Meta Llama 3.2 3B"
	case strings.Contains(lower, "llama-3.2-1b"):
		return "Meta Llama 3.2 1B"
	case strings.Contains(lower, "qwen1.5-moe") || strings.Contains(lower, "qwen-1.5-moe"):
		return "Qwen 1.5 MoE A2.7B"
	default:
		if strings.Contains(raw, " (") {
			parts := strings.Split(raw, " (")
			return parts[0]
		}
		return base
	}
}

func (m Model) currentModelName() string {
	if m.slotStatus != nil && m.slotStatus.ModelName != "" {
		cleaned := cleanModelName(m.slotStatus.ModelName)
		if cleaned != "" {
			return cleaned
		}
	}

	currMode := agent.ModeGeneral
	if m.session != nil {
		currMode = m.session.GetMode()
	}
	hw := m.hardware
	if hw == nil {
		hw = &probe.HardwareProfile{}
	}
	rec := model.SelectOptimalModelForMode(hw, model.Mode(currMode))
	if rec.ModelName != "" {
		return cleanModelName(rec.ModelName)
	}
	return "Meta Llama 3.1 8B"
}

func (m Model) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	gpuInfo := "None (CPU Fallback)"
	if m.hardware != nil && m.hardware.GPUName != "" {
		vramGiB := float64(m.hardware.VRAMBytes) / (1024 * 1024 * 1024)
		gpuInfo = fmt.Sprintf("%s (%.2f GiB)", m.hardware.GPUName, vramGiB)
	}

	modelName := m.currentModelName()
	headerPill := headerStyle.Render(fmt.Sprintf(" ⚡ lokol %s ", version.Version))
	header := headerPill + "  " + hudStyle.Render(fmt.Sprintf("Model: %s | GPU: %s", modelName, gpuInfo))

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
		hints = "[Ready] Enter send · [/mode] Switch · [Ctrl+Y] YOLO · [Ctrl+C] Quit"
		if w >= 100 {
			hints = "[Ready] Enter send · [/mode] Switch · [PgUp/PgDn] Scroll · [Ctrl+Y] YOLO · [Ctrl+C] Quit"
		}
	case StateStreaming:
		hints = "[Thinking] Generating response from local engine... · [Esc] Stop"
	case StateWaitingActionApproval:
		hints = "[Approval Needed] [Y] Approve · [N] Deny · [Ctrl+Y] Auto-approve all"
	case StateExecutingAction:
		hints = "[Executing] Local runner active · [Esc] Stop"
	}

	// Dash-light indicators on the bottom-1 line, right-justified opposite key binding hints
	currMode := agent.ModeGeneral
	if m.session != nil {
		currMode = m.session.GetMode()
	}
	var modeDash string
	switch currMode {
	case agent.ModeCoding:
		modeDash = dashLightCodeStyle.Render("[ CODE ]")
	case agent.ModeMoE:
		modeDash = dashLightExpertStyle.Render("[EXPERT]")
	case agent.ModeGeneral:
		fallthrough
	default:
		modeDash = dashLightChatStyle.Render("[ CHAT ]")
	}

	var yoloDash string
	if m.yoloMode {
		yoloDash = dashLightYOLOActiveStyle.Render("[YOLO]")
	} else {
		yoloDash = dashLightYOLOInactiveStyle.Render("[YOLO]")
	}
	dashLights := modeDash + " " + yoloDash
	dashWidth := ansi.StringWidth(dashLights)

	hintsWidth := ansi.StringWidth(hints)
	gapHints := w - hintsWidth - dashWidth
	if gapHints < 1 {
		avail := w - dashWidth - 1
		if avail > 15 {
			hints = ansi.Truncate(hints, avail, "…")
			hintsWidth = ansi.StringWidth(hints)
			gapHints = w - hintsWidth - dashWidth
		}
		if gapHints < 1 {
			gapHints = 1
		}
	}
	hotkeyLine := bottomKeyBarStyle.Render(hints) + strings.Repeat(" ", gapHints) + dashLights

	// Bottom-most Line: Left = Project path & branch pill, Right = Context token usage
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

	var contextBadge string
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

		ctxStr := fmt.Sprintf("Context: %d/%d (%.1f%%) [%s]", ctxUsed, ctxMax, pct, engineStatus)
		hudCtxStyle := lipgloss.NewStyle().Foreground(ctxFg).Bold(true)
		contextBadge = hudCtxStyle.Render(ctxStr)
	} else {
		contextBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")).Render("Context: Idle")
	}

	leftWidth := ansi.StringWidth(projBadge)
	rightWidth := ansi.StringWidth(contextBadge)
	gapInfo := w - leftWidth - rightWidth
	if gapInfo < 1 {
		avail := w - rightWidth - 1
		if avail > 10 {
			projBadge = bottomProjStyle.Render(ansi.Truncate(projText, avail, "…"))
			leftWidth = ansi.StringWidth(projBadge)
			gapInfo = w - leftWidth - rightWidth
		}
		if gapInfo < 1 {
			gapInfo = 1
		}
	}
	infoLine := projBadge + strings.Repeat(" ", gapInfo) + contextBadge

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
