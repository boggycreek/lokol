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
	"strings"

	"github.com/boggycreek/lokol/liblokol/catalog"
	"github.com/boggycreek/lokol/liblokol/config"
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
	GetPersona() (agentName, operatorName string)
	SetPersona(agentName, operatorName string)
	PruneToolOutputs(preserveRecent int) int
	CompactHistory(summaryLedger string, preserveRecent int)
}

// Session represents a stateful conversational agent session.
// It encapsulates conversation history, prompt hierarchy, tool execution,
// slot lifecycle, and token streaming independently of any UI or presentation layer.
type Session struct {
	Client           *Client
	WorkDir          string
	Mode             Mode
	CodebaseContext  string
	AgentName        string
	OperatorName     string
	History          []Message
	consecutiveReads int
	windowCache      *WindowReadCache
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
		AgentName:       env.AgentName,
		OperatorName:    env.OperatorName,
		windowCache:     NewWindowReadCache(),
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

// GetPersona returns the configured or active agent persona name and operator name.
func (s *Session) GetPersona() (string, string) {
	agentName := s.AgentName
	if agentName == "" {
		agentName = config.DefaultAgentName
	}
	operatorName := s.OperatorName
	if operatorName == "" {
		operatorName = config.DefaultOperatorName
	}
	return agentName, operatorName
}

// SetPersona dynamically changes the persona name and operator name and recalculates the system prompt.
func (s *Session) SetPersona(agentName, operatorName string) {
	if strings.TrimSpace(agentName) != "" {
		s.AgentName = strings.TrimSpace(agentName)
	}
	if strings.TrimSpace(operatorName) != "" {
		s.OperatorName = strings.TrimSpace(operatorName)
	}
	env := DetectHostEnvironment(s.WorkDir)
	if s.AgentName != "" {
		env.AgentName = s.AgentName
	}
	if s.OperatorName != "" {
		env.OperatorName = s.OperatorName
	}
	systemPrompt := BuildSystemPromptForMode(s.GetMode(), env, s.CodebaseContext)

	if len(s.History) > 0 && s.History[0].Role == "system" {
		s.History[0].Content = systemPrompt
	} else {
		s.History = append([]Message{{Role: "system", Content: systemPrompt}}, s.History...)
	}
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
	if s.AgentName != "" {
		env.AgentName = s.AgentName
	}
	if s.OperatorName != "" {
		env.OperatorName = s.OperatorName
	}
	systemPrompt := BuildSystemPromptForMode(s.Mode, env, s.CodebaseContext)

	if len(s.History) > 0 && s.History[0].Role == "system" {
		s.History[0].Content = systemPrompt
	} else {
		s.History = append([]Message{{Role: "system", Content: systemPrompt}}, s.History...)
	}

	// Refresh conversation context with explicit mode switch event (lokol-kih.9)
	desc := ModeDescription(s.Mode)
	event := fmt.Sprintf("[Mode Switched: Active persona is now %s (%s). Active capabilities and guidelines have been refreshed. Re-evaluate ongoing tasks and conversation through the perspective of %s mode.]", s.Mode, desc, s.Mode)
	s.History = append(s.History, Message{Role: "user", Content: event})
}

// AppendUserMessage appends a user message to the session's conversation history,
// dynamically injecting specialized tools matching the user's intent per ADR-0024.
func (s *Session) AppendUserMessage(content string) {
	if catalog.IsConversationalFeedback(content) {
		s.History = append(s.History, Message{Role: "user", Content: content})
		return
	}

	fullContent := content
	if catalog.IsContextMetaQuery(content) {
		agentName, operatorName := s.GetPersona()
		fullContent = content + fmt.Sprintf("\n[Context Introspection: The operator is inquiring about your context window, system prompt, or session state. DO NOT search the filesystem or call find_files. Your active mode is %q (%s), agent name is %q, operator name is %q. There are %d messages in conversation history. Answer directly by describing your active mode, instructions, and conversation state.]", s.GetMode(), ModeDescription(s.GetMode()), agentName, operatorName, len(s.History))
	} else if catalog.IsProjectSummaryQuery(content) {
		fullContent = content + "\n[System Guidance: Ground your answer in actual repository files. Inspect README.md, go.mod, package.json, or primary docs with read_window before synthesizing your project summary.]"
	}

	injected := catalog.DefaultRouter.RouteIntent(context.Background(), content)
	if len(injected) > 0 {
		fullContent = fullContent + catalog.DefaultRouter.FormatInjectedTools(injected)
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
	if act == nil {
		return "", fmt.Errorf("action is nil")
	}

	// Trajectory inspection & short-term cache: suppress identical unchanged read_window calls (lokol-kih.4)
	if act.Name == "read_window" && s.windowCache != nil {
		if input, parseErr := refinery.ParseReadWindowPayload(act.Command); parseErr == nil {
			if redundant, notice, _ := s.windowCache.CheckRedundant(input.Path, input.StartLine, input.EndLine, s.WorkDir); redundant {
				return notice, nil
			}
		}
	}

	if act.Name == "read_window" || act.Name == "read_outline" {
		s.consecutiveReads++
	} else {
		s.consecutiveReads = 0
	}

	out, err := DispatchAction(ctx, act, s.WorkDir)
	if err == nil {
		if act.Name == "read_window" && s.windowCache != nil {
			if input, parseErr := refinery.ParseReadWindowPayload(act.Command); parseErr == nil {
				s.windowCache.RecordRead(input.Path, input.StartLine, input.EndLine, s.WorkDir)
			}
		} else if (act.Name == "replace_file" || act.Name == "write_file") && s.windowCache != nil {
			target := act.TargetSummary()
			s.windowCache.Invalidate(target, s.WorkDir)
		} else if act.Name == "exec_bash" && s.windowCache != nil {
			// External bash execution may modify files; clear cache
			s.windowCache.Clear()
		} else if act.Name == "get_environment" {
			out += "\n\n[Observation: The execution environment details requested by the operator are provided above in full. Synthesize your final response directly from this environment data now. Do NOT execute tangential file reads (e.g. README.md).]"
		}

		if s.consecutiveReads >= 2 {
			out += "\n\n[Guidance: You have inspected multiple files. Please synthesize your findings now and conclude your response, or call task_finish.]"
		}
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

// Reset resets the conversation history back to the initial system prompt for the active mode,
// clearing temporary window caches, resetting read counters, preserving persona and environment
// invariants, and purging the inference engine KV cache slot (lokol-m7s).
func (s *Session) Reset() {
	if s.windowCache != nil {
		s.windowCache.Clear()
	}
	s.consecutiveReads = 0
	if s.Client != nil {
		_ = s.Client.AbortActiveSlots(context.Background())
	}
	env := DetectHostEnvironment(s.WorkDir)
	if s.AgentName != "" {
		env.AgentName = s.AgentName
	}
	if s.OperatorName != "" {
		env.OperatorName = s.OperatorName
	}
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

// PruneToolOutputs triggers micro-compaction on older conversation turns, stripping verbose observation outputs
// while preserving recent turns intact. It returns the number of characters reclaimed.
func (s *Session) PruneToolOutputs(preserveRecent int) int {
	pruned, reclaimed := PruneStaleToolOutputs(s.History, preserveRecent)
	s.History = pruned
	return reclaimed
}

// CompactHistory triggers macro-compaction, compressing older turns into a structured summary ledger
// while preserving the system prompt, initial user objective, and recent turns intact.
func (s *Session) CompactHistory(summaryLedger string, preserveRecent int) {
	s.History = CompactHistory(s.History, summaryLedger, preserveRecent)
}


