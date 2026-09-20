package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/boggycreek/quik/pkg/agent"
	"github.com/boggycreek/quik/pkg/probe"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type state int

const (
	stateIdle state = iota
	stateStreaming
	stateWaitingActionApproval
	stateExecutingAction
)

type tokenMsg string
type streamDoneMsg string
type actionExecutedMsg string
type errMsg error

// Model is the Bubble Tea application state.
// Bubble Tea requires Model to be passed by value in Update/View,
// so strings.Builder must NOT be embedded directly as a value field.
// We use simple string variables for immutable, safe value copying.
type Model struct {
	client      *agent.Client
	hardware    *probe.HardwareProfile
	state       state
	viewport    viewport.Model
	textarea    textarea.Model
	history     []agent.Message
	chatLog     string
	pendingAct  *agent.Action
	currentResp string
	tokenChan   chan string
	yoloMode    bool
	width       int
	height      int
	err         error
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

// New creates and initializes the TUI model.
func New(client *agent.Client, hw *probe.HardwareProfile, yoloMode bool) Model {
	ta := textarea.New()
	ta.Placeholder = "Ask quik to inspect code, run tests, or refactor files..."
	ta.Focus()
	ta.Prompt = "│ "
	ta.CharLimit = 1000
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	vp := viewport.New(80, 20)
	initialText := "⚡ Welcome to quik. High-performance, local-first autonomous coding engine.\nType your request below and press Enter to begin.\n\n"
	if yoloMode {
		initialText += "⚡ [YOLO MODE ENGAGED] Autonomous command execution without confirmation.\n\n"
	}
	vp.SetContent(initialText)

	m := Model{
		client:    client,
		hardware:  hw,
		state:     stateIdle,
		viewport:  vp,
		textarea:  ta,
		tokenChan: make(chan string, 100),
		yoloMode:  yoloMode,
		history: []agent.Message{
			{Role: "system", Content: agent.SystemPrompt},
		},
		chatLog: initialText,
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func waitForToken(tokenChan chan string) tea.Cmd {
	return func() tea.Msg {
		token, ok := <-tokenChan
		if !ok {
			return streamDoneMsg("")
		}
		return tokenMsg(token)
	}
}

func startStream(client *agent.Client, history []agent.Message, tokenChan chan string) tea.Cmd {
	return func() tea.Msg {
		fullContent, err := client.StreamResponse(context.Background(), history, tokenChan)
		close(tokenChan)
		if err != nil {
			return errMsg(err)
		}
		return streamDoneMsg(fullContent)
	}
}

func executeAction(act *agent.Action) tea.Cmd {
	return func() tea.Msg {
		if act == nil {
			return actionExecutedMsg("")
		}
		var out string
		var err error

		switch act.Name {
		case "exec_bash":
			out, err = agent.ExecuteBash(context.Background(), act.Command)
		case "replace_file":
			out, err = agent.ExecuteReplaceFile(context.Background(), act.Command)
		case "write_file":
			out, err = agent.ExecuteWriteFile(context.Background(), act.Command)
		default:
			return actionExecutedMsg(fmt.Sprintf("[Unknown action: %s]", act.Name))
		}

		if err != nil {
			return actionExecutedMsg(fmt.Sprintf("[Error: %v]\n%s", err, out))
		}
		return actionExecutedMsg(out)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var vpCmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 2
		inputHeight := 5
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - headerHeight - inputHeight - 2
		m.textarea.SetWidth(msg.Width - 4)

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyCtrlY:
			m.yoloMode = !m.yoloMode
			if m.yoloMode {
				m.appendLog("⚡ [YOLO MODE ENGAGED] Autonomous command execution active.\n")
			} else {
				m.appendLog("🛡️ [SAFE MODE ENGAGED] Manual action approval required.\n")
			}
			return m, nil
		case tea.KeyEnter:
			if m.state == stateWaitingActionApproval {
				// Approve action
				m.state = stateExecutingAction
				m.appendLog(outputBoxStyle.Render("⚡ Executing command: " + m.pendingAct.Command))
				return m, executeAction(m.pendingAct)
			}

			if m.state == stateIdle {
				input := strings.TrimSpace(m.textarea.Value())
				if input == "" {
					return m, nil
				}
				m.textarea.Reset()
				m.appendLog(userStyle.Render("User: ") + input + "\n")
				m.history = append(m.history, agent.Message{Role: "user", Content: input})
				m.state = stateStreaming
				m.currentResp = ""
				m.tokenChan = make(chan string, 100)

				return m, tea.Batch(
					startStream(m.client, m.history, m.tokenChan),
					waitForToken(m.tokenChan),
				)
			}
		}

		if m.state == stateWaitingActionApproval {
			switch msg.String() {
			case "y", "Y":
				m.state = stateExecutingAction
				m.appendLog(outputBoxStyle.Render("⚡ Executing approved command: " + m.pendingAct.Command))
				return m, executeAction(m.pendingAct)
			case "n", "N":
				m.state = stateIdle
				m.appendLog("[Action rejected by user]\n")
				m.history = append(m.history, agent.Message{
					Role:    "user",
					Content: "User rejected the action proposal. Please decide on an alternative or ask for clarification.",
				})
				m.pendingAct = nil
				return m, nil
			}
		}

	case tokenMsg:
		if m.state == stateStreaming {
			m.currentResp += string(msg)
			// Filter out action XML from the live stream display
			displayStr := m.currentResp
			if idx := strings.Index(displayStr, "<action"); idx != -1 {
				displayStr = strings.TrimSpace(displayStr[:idx])
			}
			if displayStr != "" {
				m.viewport.SetContent(m.chatLog + agentStyle.Render("quik: ") + displayStr)
			}
			m.viewport.GotoBottom()
			return m, waitForToken(m.tokenChan)
		}

	case streamDoneMsg:
		if m.state == stateStreaming {
			response := m.currentResp
			m.history = append(m.history, agent.Message{Role: "assistant", Content: response})

			// Check for actions
			act := agent.ParseAction(response)
			if act != nil && (act.Name == "exec_bash" || act.Name == "replace_file" || act.Name == "write_file") {
				m.pendingAct = act
				if act.CleanThought != "" {
					m.appendLog(agentStyle.Render("quik: ") + act.CleanThought + "\n")
				}
				if m.yoloMode {
					// YOLO Mode: execute immediately without waiting for user approval
					m.state = stateExecutingAction
					switch act.Name {
					case "exec_bash":
						m.appendLog("⚡ Executing: " + act.Command + "\n")
					case "replace_file":
						input, _ := agent.ParseReplaceFileInput(act.Command)
						p := "file"
						if input != nil {
							p = input.Path
						}
						m.appendLog("⚡ Editing: " + p + "\n")
					case "write_file":
						input, _ := agent.ParseWriteFileInput(act.Command)
						p := "file"
						if input != nil {
							p = input.Path
						}
						m.appendLog("⚡ Writing: " + p + "\n")
					}
					return m, executeAction(act)
				}

				m.state = stateWaitingActionApproval
				box := actionBoxStyle.Render(fmt.Sprintf(
					"PROPOSED ACTION: %s\nPayload:\n%s\n\nPress [Enter] or [Y] to approve, [N] to reject",
					act.Name, act.Command,
				))
				m.appendLog("\n" + box + "\n")
			} else if act != nil && act.Name == "task_finish" {
				m.state = stateIdle
				if act.CleanThought != "" {
					m.appendLog(agentStyle.Render("quik: ") + act.CleanThought + "\n")
				}
				m.appendLog("✅ Complete: " + act.Command + "\n")
			} else {
				m.state = stateIdle
				m.appendLog(agentStyle.Render("quik: ") + response + "\n")
			}
			return m, nil
		}

	case actionExecutedMsg:
		output := string(msg)
		m.appendLog("✓ Executed successfully\n")

		// Feed tool output back to agent history
		toolResult := fmt.Sprintf("<action_result>\n%s\n</action_result>", output)
		m.history = append(m.history, agent.Message{Role: "user", Content: toolResult})

		// Trigger next agent iteration
		m.state = stateStreaming
		m.currentResp = ""
		m.tokenChan = make(chan string, 100)
		return m, tea.Batch(
			startStream(m.client, m.history, m.tokenChan),
			waitForToken(m.tokenChan),
		)

	case errMsg:
		m.err = msg
		m.state = stateIdle
		m.appendLog(fmt.Sprintf("[Error: %v]\n", msg))
	}

	if m.state == stateIdle {
		var taCmd tea.Cmd
		m.textarea, taCmd = m.textarea.Update(msg)
		cmds = append(cmds, taCmd)
	}

	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) appendLog(text string) {
	m.chatLog += text
	m.viewport.SetContent(m.chatLog)
	m.viewport.GotoBottom()
}

func (m Model) View() string {
	gpuInfo := "CPU Only"
	if m.hardware != nil && m.hardware.GPUName != "" {
		gpuInfo = fmt.Sprintf("%s (%s)", m.hardware.GPUName, m.hardware.HumanVRAM())
	}

	header := headerStyle.Render(" ⚡ quik v0.1.0 ") + "  " +
		hudStyle.Render(fmt.Sprintf("GPU: %s", gpuInfo))
	if m.yoloMode {
		yoloBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render(" [YOLO ACTIVE]")
		header += yoloBadge
	}

	var statusLine string
	switch m.state {
	case stateIdle:
		statusLine = "[Ready] Press Enter to send | [Ctrl+Y] Toggle YOLO | [Ctrl+C] Quit"
	case stateStreaming:
		statusLine = "[Generating] Receiving tokens from local engine..."
	case stateWaitingActionApproval:
		statusLine = "[Approval Needed] Review command above. Press [Y] to Approve, [N] to Deny | [Ctrl+Y] Auto-approve all"
	case stateExecutingAction:
		statusLine = "[Executing] Running bash tool locally..."
	}

	return fmt.Sprintf("%s\n\n%s\n\n%s\n%s",
		header,
		m.viewport.View(),
		statusLine,
		m.textarea.View(),
	)
}
