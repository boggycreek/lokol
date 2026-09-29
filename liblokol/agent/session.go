// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/boggycreek/lokol/liblokol/catalog"
	"github.com/boggycreek/lokol/liblokol/refinery"
	"github.com/boggycreek/lokol/liblokol/regulator"
)

// SessionCore defines the presentation-facing contract of the core agent engine.
// Presentation layers (Bubble Tea TUI, headless CLI, desktop GUIs, web dashboards)
// interact with the core engine exclusively through this contract.
type SessionCore interface {
	StreamTurn(ctx context.Context, tokenChan chan<- string) (string, error)
	ExecuteAction(ctx context.Context, act *Action) (string, error)
	AppendUserMessage(content string)
	AppendAssistantMessage(content string)
	AppendActionResult(output string, err error)
	Abort(ctx context.Context) error
	GetSlotStatus(ctx context.Context) (*SlotStatus, error)
	Reset()
	GetMode() Mode
	SetMode(mode Mode)
	GetWorkDir() string
}

// Session represents a stateful conversational agent session.
// It encapsulates conversation history, prompt hierarchy, tool execution,
// slot lifecycle, and token streaming independently of any UI or presentation layer.
type Session struct {
	Client          *Client
	WorkDir         string
	Mode            Mode
	CodebaseContext string
	History         []Message
	consecutiveReads int
}

var _ SessionCore = (*Session)(nil)

// NewSession creates and initializes a new agent Session for the given working directory.
// By default, it operates in ModeGeneral unless a mode is explicitly set via SetMode
// or created via NewSessionWithMode.
func NewSession(client *Client, workDir string, codebaseCtxOpt ...string) *Session {
	return NewSessionWithMode(client, workDir, ModeGeneral, codebaseCtxOpt...)
}

// NewSessionWithMode creates and initializes a new agent Session for a specified mode.
func NewSessionWithMode(client *Client, workDir string, mode Mode, codebaseCtxOpt ...string) *Session {
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(workDir); err == nil {
		workDir = abs
	} else {
		workDir = filepath.Clean(workDir)
	}

	if mode == "" {
		mode = ModeGeneral
	}

	var codebaseCtx string
	if len(codebaseCtxOpt) > 0 && codebaseCtxOpt[0] != "" {
		codebaseCtx = codebaseCtxOpt[0]
	} else if mode == ModeCoding {
		codebaseCtx = refinery.LoadCodebaseContext(workDir)
	}

	env := DetectHostEnvironment(workDir)
	systemPrompt := BuildSystemPromptForMode(mode, env, codebaseCtx)

	return &Session{
		Client:          client,
		WorkDir:         workDir,
		Mode:            mode,
		CodebaseContext: codebaseCtx,
		History: []Message{
			{Role: "system", Content: systemPrompt},
		},
	}
}

// GetMode returns the current operational mode of the session.
func (s *Session) GetMode() Mode {
	if s.Mode == "" {
		return ModeGeneral
	}
	return s.Mode
}

// SetMode dynamically changes the operational mode of the session and recalculates the system prompt.
func (s *Session) SetMode(mode Mode) {
	if mode == "" {
		mode = ModeGeneral
	}
	s.Mode = mode
	if s.Mode == ModeCoding && s.CodebaseContext == "" {
		s.CodebaseContext = refinery.LoadCodebaseContext(s.WorkDir)
	}
	env := DetectHostEnvironment(s.WorkDir)
	systemPrompt := BuildSystemPromptForMode(s.Mode, env, s.CodebaseContext)

	if len(s.History) > 0 && s.History[0].Role == "system" {
		s.History[0].Content = systemPrompt
	} else {
		s.History = append([]Message{{Role: "system", Content: systemPrompt}}, s.History...)
	}
}

// AppendUserMessage appends a user message to the session's conversation history,
// dynamically injecting specialized tools matching the user's intent per ADR-0024.
func (s *Session) AppendUserMessage(content string) {
	injected := catalog.DefaultRouter.RouteIntent(context.Background(), content)
	fullContent := content
	if len(injected) > 0 {
		fullContent = content + catalog.DefaultRouter.FormatInjectedTools(injected)
	}
	s.History = append(s.History, Message{Role: "user", Content: fullContent})
}

// AppendAssistantMessage appends an assistant message to the session's conversation history.
func (s *Session) AppendAssistantMessage(content string) {
	s.History = append(s.History, Message{Role: "assistant", Content: content})
}

// AppendActionResult appends an action execution result to the conversation history.
func (s *Session) AppendActionResult(output string, err error) {
	var toolResult string
	if err != nil {
		toolResult = fmt.Sprintf("<action_result>\n[Error: %v]\n%s\n</action_result>\n[Observation: The tool failed with the error above. Proceed with your next step.]", err, output)
	} else {
		toolResult = fmt.Sprintf("<action_result>\n%s\n</action_result>\n[Observation: Analyze the tool output above directly to fulfill the user request. Do not thank the user.]", output)
	}
	s.History = append(s.History, Message{Role: "user", Content: toolResult})
}

// ExecuteAction executes a parsed Action using the centralized DispatchAction.
func (s *Session) ExecuteAction(ctx context.Context, act *Action) (string, error) {
	if act != nil && (act.Name == "read_window" || act.Name == "read_outline") {
		s.consecutiveReads++
	} else {
		s.consecutiveReads = 0
	}

	out, err := DispatchAction(ctx, act, s.WorkDir)
	if err == nil && s.consecutiveReads >= 2 {
		out += "\n\n[Guidance: You have inspected multiple files. Please synthesize your findings now and conclude your response, or call task_finish.]"
	}
	return out, err
}

// StreamTurn initiates a model streaming response using the session's history.
func (s *Session) StreamTurn(ctx context.Context, tokenChan chan<- string) (string, error) {
	if s.Client == nil {
		return "", fmt.Errorf("session client is nil")
	}
	return s.Client.StreamResponse(ctx, s.History, tokenChan)
}

// Abort cancels any active slots running on the local inference engine.
func (s *Session) Abort(ctx context.Context) error {
	if s.Client == nil {
		return nil
	}
	return s.Client.AbortActiveSlots(ctx)
}

// GetSlotStatus retrieves the current inference slot status from the engine.
func (s *Session) GetSlotStatus(ctx context.Context) (*SlotStatus, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("session client is nil")
	}
	return s.Client.GetSlotStatus(ctx)
}

// GetSlotMetrics implements regulator.SlotStatusProvider for the active session (ADR 0026).
func (s *Session) GetSlotMetrics(ctx context.Context) (*regulator.SlotMetrics, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("session client is nil")
	}
	return s.Client.GetSlotMetrics(ctx)
}

var _ regulator.SlotStatusProvider = (*Session)(nil)

// Reset resets the conversation history back to the initial system prompt for the active mode.
func (s *Session) Reset() {
	env := DetectHostEnvironment(s.WorkDir)
	s.History = []Message{
		{Role: "system", Content: BuildSystemPromptForMode(s.GetMode(), env, s.CodebaseContext)},
	}
}

// GetWorkDir returns the absolute workspace directory associated with the session.
func (s *Session) GetWorkDir() string {
	if s.WorkDir == "" {
		return "."
	}
	return s.WorkDir
}

