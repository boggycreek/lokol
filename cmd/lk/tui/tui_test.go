// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boggycreek/lokol/cmd/lk/tui"
	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/probe"
	tea "github.com/charmbracelet/bubbletea"
)

// TestTUI_Presentation_SmokeAndHUD simulates TUI initialization, window resize, and hardware HUD rendering.
func TestTUI_Presentation_SmokeAndHUD(t *testing.T) {
	hw := &probe.HardwareProfile{
		OS:        "linux",
		Arch:      "amd64",
		GPUName:   "NVIDIA GeForce RTX 3060",
		VRAMBytes: 12 * 1024 * 1024 * 1024,
	}
	mock := NewMockSession()
	m := tui.NewWithSession(mock, hw, false)

	// 1. Simulate Window Resize
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = newModel.(tui.Model)

	// 2. Initial View render test (verify no panic and HUD rendering)
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "lokol") {
		t.Errorf("expected view to contain 'lokol', got %s", viewOutput)
	}
	if !strings.Contains(viewOutput, "RTX 3060") {
		t.Errorf("expected view to contain 'RTX 3060', got %s", viewOutput)
	}
	if !strings.Contains(viewOutput, "[Ready]") {
		t.Errorf("expected view to contain '[Ready]', got %s", viewOutput)
	}

	// 3. Simulate keystrokes
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	m = newModel.(tui.Model)

	viewOutput = m.View()
	if viewOutput == "" {
		t.Errorf("view output was empty")
	}
}

// TestTUI_Presentation_ViewportLineWrapping verifies that long lines exceeding the viewport width are wrapped.
func TestTUI_Presentation_ViewportLineWrapping(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false)

	// Set a narrow window: Width 40 -> viewport width 36
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	m = newModel.(tui.Model)

	// Send a long user prompt exceeding 36 characters without newlines
	longInput := "This is a very long agent prompt designed to test whether the viewport wraps lines properly or truncates them horizontally at the boundary."
	for _, r := range longInput {
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = newModel.(tui.Model)
	}
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(tui.Model)

	// Verify the mock session received the user message
	if len(mock.UserMessages) != 1 || mock.UserMessages[0] != longInput {
		t.Fatalf("expected user message in mock session, got: %+v", mock.UserMessages)
	}

	content := m.ViewportContent()
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if len(line) > 40 {
			t.Errorf("line %d exceeds viewport boundary: %q (len %d)", i, line, len(line))
		}
	}

	// Verify that the long input was wrapped into multiple lines
	foundSnippet := false
	for _, line := range lines {
		if strings.Contains(line, "horizontally") {
			foundSnippet = true
			break
		}
	}
	if !foundSnippet {
		t.Errorf("expected wrapped content to contain snippet 'horizontally'")
	}
}

// TestTUI_Presentation_InternalizedAndCleanFinalPresentation verifies that in default mode,
// intermediate reasoning and raw tool outputs are suppressed, and the final solution is cleanly displayed.
func TestTUI_Presentation_InternalizedAndCleanFinalPresentation(t *testing.T) {
	mock := NewMockSession()
	hw := &probe.HardwareProfile{OS: "linux", Arch: "amd64", CPUCores: 4}
	m := tui.NewWithSession(mock, hw, true) // YOLO mode active

	// Step 0: User submits prompt
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fix bug")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	if m.State() != tui.StateStreaming {
		t.Fatalf("expected stateStreaming after prompt submission, got %v", m.State())
	}

	// Step 1: Model generates an intermediate action with clean thought
	actionTurn1 := "I will examine the directory to find where the bug is located.\n<action name=\"read_outline\">\n<path>.</path>\n</action>"
	newM, cmd := m.Update(tui.StreamDoneMsg(actionTurn1))
	m = newM.(tui.Model)

	// Intermediate thought and tool execution banners MUST NOT be in the chat viewport in default mode
	content := m.ViewportContent()
	if strings.Contains(content, "I will examine the directory") {
		t.Fatalf("expected intermediate thought to be internalized, but found in viewport: %q", content)
	}
	if strings.Contains(content, "⚡ Reading Outline") {
		t.Fatalf("expected tool execution banner to be internalized in YOLO mode, but found: %q", content)
	}
	if m.StepCount() != 1 {
		t.Fatalf("expected stepCount 1, got %d", m.StepCount())
	}
	if cmd == nil {
		t.Fatal("expected executeAction cmd to be returned in YOLO mode")
	}

	// Status line should reflect the active step
	viewStr := m.View()
	if !strings.Contains(viewStr, "Step 1") {
		t.Fatalf("expected status line to show Step 1, got view:\n%s", viewStr)
	}

	// Step 2: Action completes
	newM, _ = m.Update(tui.ActionExecutedMsg("main.go\ncalc.go"))
	m = newM.(tui.Model)

	// Tool completion message must NOT pollute the chat viewport
	content = m.ViewportContent()
	if strings.Contains(content, "Executed successfully") {
		t.Fatalf("expected 'Executed successfully' to be internalized, but found in viewport: %q", content)
	}
	if m.State() != tui.StateStreaming {
		t.Fatalf("expected StateStreaming after action execution, got %v", m.State())
	}

	// Step 3: Model completes task with task_finish
	finishTurn := "All tests pass. The issue is resolved.\n<action name=\"task_finish\">\nCorrected off-by-one error in calc.go.\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(finishTurn))
	m = newM.(tui.Model)

	if m.State() != tui.StateIdle {
		t.Fatalf("expected StateIdle after task_finish, got %v", m.State())
	}
	if m.StepCount() != 0 {
		t.Fatalf("expected stepCount reset to 0, got %d", m.StepCount())
	}

	// The chat viewport MUST cleanly present the final answer and completion banner!
	finalContent := m.ViewportContent()
	if !strings.Contains(finalContent, "Corrected off-by-one error in calc.go.") {
		t.Fatalf("expected final answer in chat log, got: %q", finalContent)
	}
	if !strings.Contains(finalContent, "✅ Task Complete") {
		t.Fatalf("expected completion banner '✅ Task Complete', got: %q", finalContent)
	}
	if !strings.Contains(finalContent, "Resolved in 1 autonomous step") {
		t.Fatalf("expected step summary in completion banner, got: %q", finalContent)
	}

	// View status line should return to [Ready]
	if !strings.Contains(m.View(), "[Ready]") {
		t.Fatalf("expected [Ready] in status line after finish, got view:\n%s", m.View())
	}
}

// TestTUI_Presentation_VerboseToggle verifies verbose mode displaying intermediate streams and tool banners.
func TestTUI_Presentation_VerboseToggle(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, true)

	// Toggle with Ctrl+V
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	m = newM.(tui.Model)
	if !strings.Contains(m.ViewportContent(), "VERBOSE ON") {
		t.Fatalf("expected verbose ON message in viewport, got: %q", m.ViewportContent())
	}

	// In verbose mode, intermediate actions ARE logged to the viewport
	// Start stream first so state is StateStreaming
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("investigate")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	actionTurn := "Investigating files.\n<action name=\"read_outline\">\n<path>.</path>\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(actionTurn))
	m = newM.(tui.Model)

	content := m.ViewportContent()
	if !strings.Contains(content, "Investigating files.") {
		t.Fatalf("expected intermediate thought in viewport when verbose=true, got: %q", content)
	}
	if !strings.Contains(content, "⚡ Reading Outline: .") {
		t.Fatalf("expected tool execution banner in viewport when verbose=true, got: %q", content)
	}
}

// TestTUI_Presentation_ActionApprovalFlow verifies interactive action confirmation workflow in safe mode.
func TestTUI_Presentation_ActionApprovalFlow(t *testing.T) {
	mock := NewMockSession()
	mock.ExecuteActionFunc = func(ctx context.Context, act *agent.Action) (string, error) {
		return "branch main", nil
	}

	m := tui.NewWithSession(mock, nil, false) // Safe mode (YOLO = false)

	// Step 0: User inputs message to enter streaming state
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("check status")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	// Step 1: Model proposes bash action
	actionTurn := "I will check git status.\n<action name=\"exec_bash\">\ngit status\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(actionTurn))
	m = newM.(tui.Model)

	if m.State() != tui.StateWaitingActionApproval {
		t.Fatalf("expected StateWaitingActionApproval, got %v", m.State())
	}

	// Viewport must present the proposed action box
	content := m.ViewportContent()
	if !strings.Contains(content, "PROPOSED ACTION: exec_bash") || !strings.Contains(content, "git status") {
		t.Fatalf("expected action approval box in viewport, got: %q", content)
	}

	// Status line must solicit approval
	viewStr := m.View()
	if !strings.Contains(viewStr, "[Approval Needed]") {
		t.Fatalf("expected [Approval Needed] in status bar, got: %s", viewStr)
	}

	// User approves with 'y'
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = newM.(tui.Model)

	if m.State() != tui.StateExecutingAction {
		t.Fatalf("expected StateExecutingAction after approval, got %v", m.State())
	}
	if cmd == nil {
		t.Fatal("expected executeAction cmd to be returned upon approval")
	}

	// Status line should indicate execution
	viewStr = m.View()
	if !strings.Contains(viewStr, "Executing exec_bash: git status") {
		t.Fatalf("expected status line to show active execution, got: %s", viewStr)
	}
}

// TestTUI_Presentation_ParameterlessActionFormatting verifies parameterless tools display (no arguments)
// and format cleanly without tautological badges like 'Ran get_environment environment'.
func TestTUI_Presentation_ParameterlessActionFormatting(t *testing.T) {
	mock := NewMockSession()
	mock.ExecuteActionFunc = func(ctx context.Context, act *agent.Action) (string, error) {
		return "OS: linux\nArch: amd64", nil
	}

	m := tui.NewWithSession(mock, nil, false) // Safe mode

	// Step 0: User inputs message
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("check environment")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	// Step 1: Model proposes get_environment action without command arguments
	actionTurn := "I will check the environment.\n<action name=\"get_environment\">\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(actionTurn))
	m = newM.(tui.Model)

	if m.State() != tui.StateWaitingActionApproval {
		t.Fatalf("expected StateWaitingActionApproval, got %v", m.State())
	}

	// Viewport must present the proposed action box with (no arguments)
	content := m.ViewportContent()
	if !strings.Contains(content, "PROPOSED ACTION: get_environment") {
		t.Fatalf("expected PROPOSED ACTION: get_environment, got: %q", content)
	}
	if !strings.Contains(content, "Payload:") || !strings.Contains(content, "(no arguments)") {
		t.Fatalf("expected 'Payload:' and '(no arguments)' payload placeholder, got: %q", content)
	}

	// User approves with 'y'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = newM.(tui.Model)

	// Status line should indicate execution cleanly without tautological target: "Executing get_environment locally..."
	viewStr := m.View()
	if strings.Contains(viewStr, "get_environment: environment") {
		t.Fatalf("expected non-tautological execution line, got: %s", viewStr)
	}
	if !strings.Contains(viewStr, "Executing get_environment locally...") {
		t.Fatalf("expected 'Executing get_environment locally...', got: %s", viewStr)
	}
}

// TestTUI_Presentation_ActionRejectionFlow verifies interactive rejection of proposed actions.
func TestTUI_Presentation_ActionRejectionFlow(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false) // Safe mode

	// Step 0: User inputs message
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("cleanup")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	actionTurn := "I will delete files.\n<action name=\"exec_bash\">\nrm -rf *\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(actionTurn))
	m = newM.(tui.Model)

	if m.State() != tui.StateWaitingActionApproval {
		t.Fatalf("expected StateWaitingActionApproval, got %v", m.State())
	}

	// User rejects with 'n'
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = newM.(tui.Model)

	if m.State() != tui.StateIdle {
		t.Fatalf("expected StateIdle after rejection, got %v", m.State())
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd after rejection, got non-nil")
	}

	// Viewport should record rejection
	content := m.ViewportContent()
	if !strings.Contains(content, "[Action rejected by user]") {
		t.Fatalf("expected rejection note in viewport, got: %q", content)
	}

	// Core session should have received rejection message
	if len(mock.UserMessages) < 2 || !strings.Contains(mock.UserMessages[1], "User rejected") {
		t.Fatalf("expected rejection message recorded in mock session, got: %+v", mock.UserMessages)
	}
}

// TestTUI_Presentation_HUDContextAndPressure verifies real-time context token usage and VRAM pressure alerts.
func TestTUI_Presentation_HUDContextAndPressure(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false)

	// 1. Normal context usage (e.g. 500 / 2048 = 24.4% -> Pure VRAM)
	newM, _ := m.Update(tui.SlotTickMsg(&agent.SlotStatus{
		NCtx:          2048,
		NPromptTokens: 500,
	}))
	m = newM.(tui.Model)

	viewStr := m.View()
	if !strings.Contains(viewStr, "Context: 500/2048 (24.4%) [Pure VRAM]") {
		t.Errorf("expected normal HUD display, got: %s", viewStr)
	}

	// 2. High memory pressure (>90% -> High Pressure)
	newM, _ = m.Update(tui.SlotTickMsg(&agent.SlotStatus{
		NCtx:          2048,
		NPromptTokens: 1950,
	}))
	m = newM.(tui.Model)

	viewStr = m.View()
	if !strings.Contains(viewStr, "Context: 1950/2048 (95.2%) [High Pressure]") {
		t.Errorf("expected high pressure HUD display, got: %s", viewStr)
	}
}

// TestTUI_Presentation_Cancellation verifies cancelling inference and aborting slots with Esc and Ctrl+C.
func TestTUI_Presentation_Cancellation(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false)

	// Step 0: Enter streaming state
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("long query")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	if m.State() != tui.StateStreaming {
		t.Fatalf("expected StateStreaming, got %v", m.State())
	}

	// Press Esc while streaming
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(tui.Model)

	if m.State() != tui.StateIdle {
		t.Fatalf("expected StateIdle after Esc, got %v", m.State())
	}
	if m.StepCount() != 0 {
		t.Fatalf("expected stepCount reset to 0, got %d", m.StepCount())
	}
	if !strings.Contains(m.ViewportContent(), "Stream aborted (Esc)") {
		t.Fatalf("expected abort banner in viewport, got: %q", m.ViewportContent())
	}
}

func TestTUIInterruptionAndSlotRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/slots") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":0,"n_ctx":4096,"n_prompt_tokens":0,"is_processing":false}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := agent.NewClient(server.URL)
	hw := &probe.HardwareProfile{OS: "linux", Arch: "amd64", CPUCores: 4}
	m := tui.New(client, hw, false)

	// Test Ctrl+C while idle triggers tea.Quit
	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_ = updatedModel
	if cmd == nil {
		t.Errorf("expected quit command on Ctrl+C when idle")
	}

	// Test Esc while idle does nothing (does not quit)
	_, escCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if escCmd != nil {
		t.Errorf("expected nil command on Esc when idle, got %v", escCmd)
	}
}

func TestTUI_Presentation_ModeSwitching(t *testing.T) {
	mock := NewMockSession()
	mock.CurrentMode = agent.ModeGeneral

	m := tui.NewModel(mock, false, false)

	// Check default mode in View
	view := m.View()
	if !strings.Contains(view, "[ CHAT ]") {
		t.Fatalf("expected view to contain '[ CHAT ]', got: %s", view)
	}

	// Switch to coding mode via slash command
	m = m.WithInitialPrompt("/mode coding")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("expected nil cmd from slash command, got: %v", cmd)
	}
	m = updated.(tui.Model)

	if mock.CurrentMode != agent.ModeCoding {
		t.Errorf("expected mock session mode to be 'coding', got: %s", mock.CurrentMode)
	}

	view = m.View()
	if !strings.Contains(view, "[ CODE ]") {
		t.Errorf("expected view to contain '[ CODE ]' after switch, got: %s", view)
	}
	if !strings.Contains(m.ViewportContent(), "Active persona is now: coding") {
		t.Errorf("expected log to announce mode switch, got: %s", m.ViewportContent())
	}

	// Switch to moe mode via slash command
	m = m.WithInitialPrompt("/mode moe")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	if mock.CurrentMode != agent.ModeMoE {
		t.Errorf("expected mock session mode to be 'moe', got: %s", mock.CurrentMode)
	}

	view = m.View()
	if !strings.Contains(view, "[EXPERT]") {
		t.Errorf("expected view to contain '[EXPERT]' after switch, got: %s", view)
	}

	// Circular toggle: /mode with no argument should cycle moe -> general
	m = m.WithInitialPrompt("/mode")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	if mock.CurrentMode != agent.ModeGeneral {
		t.Errorf("expected mock session mode to cycle back to 'general', got: %s", mock.CurrentMode)
	}
	view = m.View()
	if !strings.Contains(view, "[ CHAT ]") {
		t.Errorf("expected view to contain '[ CHAT ]' after circular toggle, got: %s", view)
	}

	// Invalid mode handling
	m = m.WithInitialPrompt("/mode invalid_mode")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	if !strings.Contains(m.ViewportContent(), "Invalid Mode") {
		t.Errorf("expected invalid mode warning in viewport, got: %s", m.ViewportContent())
	}
}

func TestTUI_Presentation_RegulatorSecurityWarning(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false) // Safe mode

	// Start stream to enter StateStreaming
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("edit system file")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	// Propose out-of-bounds action
	oobActionTurn := "Writing outside.\n<action name=\"write_file\">\n<path>/etc/passwd</path>\n<content>root:x:0:0::/root:/bin/bash</content>\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(oobActionTurn))
	m = newM.(tui.Model)

	if m.State() != tui.StateWaitingActionApproval {
		t.Fatalf("expected StateWaitingActionApproval, got %v", m.State())
	}

	content := m.ViewportContent()
	if !strings.Contains(content, "SECURITY WARNING") {
		t.Fatalf("expected SECURITY WARNING in viewport, got: %s", content)
	}
	if !strings.Contains(content, "Filesystem boundary violation") {
		t.Fatalf("expected boundary violation explanation in viewport, got: %s", content)
	}
}

func TestTUI_Presentation_RegulatorYOLOIntercept(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, true) // YOLO mode engaged

	// Start stream
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("format drive")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	// Propose destructive action in YOLO mode
	dangerousTurn := "Formatting drive.\n<action name=\"exec_bash\">\nrm -rf /\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(dangerousTurn))
	m = newM.(tui.Model)

	// Must NOT execute automatically; must pause and solicit approval
	if m.State() != tui.StateWaitingActionApproval {
		t.Fatalf("expected YOLO mode to pause and enter StateWaitingActionApproval on dangerous command, got %v", m.State())
	}

	content := m.ViewportContent()
	if !strings.Contains(content, "REGULATOR INTERCEPT") {
		t.Fatalf("expected REGULATOR INTERCEPT in viewport log, got: %s", content)
	}
	if !strings.Contains(content, "SECURITY WARNING") {
		t.Fatalf("expected SECURITY WARNING in viewport approval box, got: %s", content)
	}
}

// TestTUI_Presentation_Ergonomics_JunieLayoutAndToolFolding verifies Junie-inspired ergonomics:
// 1. Dual status bar hierarchy (top hardware/mode, dense bottom status bar with git branch, engine, context HUD)
// 2. Active animated spinner line above framed textarea during inference/action turns
// 3. Compact tool badges folding consecutive commands into "Ran N commands ▸"
func TestTUI_Presentation_Ergonomics_JunieLayoutAndToolFolding(t *testing.T) {
	mock := NewMockSession()
	hw := &probe.HardwareProfile{
		OS:        "linux",
		Arch:      "amd64",
		GPUName:   "NVIDIA GeForce RTX 3060",
		VRAMBytes: 12 * 1024 * 1024 * 1024,
	}
	m := tui.NewWithSession(mock, hw, true) // YOLO mode engaged

	// Context status tick
	newM, _ := m.Update(tui.SlotTickMsg(&agent.SlotStatus{
		NCtx:          2048,
		NPromptTokens: 500,
	}))
	m = newM.(tui.Model)

	// Verify layout when idle:
	// Top bar: hardware and model info
	// Line 1 below lower rule: hotkey hints on left, dash-lights ([ CHAT ] [YOLO]) on right
	// Line 2 (bottom): project/branch on left, context usage on right
	idleView := m.View()
	if !strings.Contains(idleView, "Context: 500/2048 (24.4%) [Pure VRAM]") {
		t.Errorf("expected bottom status bar to contain context token HUD, got: %s", idleView)
	}
	if !strings.Contains(idleView, "> ") {
		t.Errorf("expected prompt to have '> ' prompt icon, got: %s", idleView)
	}
	if !strings.Contains(idleView, "[ CHAT ]") {
		t.Errorf("expected dash-light to contain '[ CHAT ]', got: %s", idleView)
	}
	if !strings.Contains(idleView, "[YOLO]") {
		t.Errorf("expected dash-light to contain '[YOLO]', got: %s", idleView)
	}

	hotkeyIdx := strings.Index(idleView, "[Ready] Enter send")
	ctxIdx := strings.Index(idleView, "Context: 500/2048")
	if hotkeyIdx == -1 {
		t.Errorf("expected hotkey menu in view, got: %s", idleView)
	}
	if ctxIdx == -1 {
		t.Errorf("expected context usage in view, got: %s", idleView)
	}
	if hotkeyIdx > ctxIdx {
		t.Errorf("expected hotkey menu directly below input, before bottom context line")
	}

	// Step 0: User prompt -> enter StateStreaming
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("check codebase")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	// Verify active spinner line above prompt during initial thinking
	streamingView := m.View()
	if !strings.Contains(streamingView, "Thinking...") || !strings.Contains(streamingView, "esc to stop") {
		t.Errorf("expected active spinner line with interruption hint above prompt, got: %s", streamingView)
	}

	// Tool Turn 1: exec_bash git status
	turn1 := "Checking status.\n<action name=\"exec_bash\">\ngit status\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(turn1))
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("On branch main\nnothing to commit"))
	m = newM.(tui.Model)

	content := m.ViewportContent()
	if !strings.Contains(content, "Ran git status ▸") {
		t.Errorf("expected compact single tool badge 'Ran git status ▸', got: %s", content)
	}

	// Tool Turn 2: exec_bash git diff
	turn2 := "Checking diff.\n<action name=\"exec_bash\">\ngit diff\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(turn2))
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg(""))
	m = newM.(tui.Model)

	content = m.ViewportContent()
	if !strings.Contains(content, "Ran git status ▸") || !strings.Contains(content, "Ran git diff ▸") {
		t.Errorf("expected live streaming operations to display individual action digest lines, got: %s", content)
	}

	// Tool Turn 3: exec_bash go test
	turn3 := "Running tests.\n<action name=\"exec_bash\">\ngo test ./...\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(turn3))
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("PASS"))
	m = newM.(tui.Model)

	content = m.ViewportContent()
	if !strings.Contains(content, "Ran go test ./... ▸") {
		t.Errorf("expected live stream to contain 3rd command 'Ran go test ./... ▸', got: %s", content)
	}

	// Task finish - collapses live stream into single-line aggregated summary
	turnDone := "All checks passed.\n<action name=\"task_finish\">\nCodebase is in good health.\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(turnDone))
	m = newM.(tui.Model)

	finalContent := m.ViewportContent()
	if !strings.Contains(finalContent, "Ran 3 commands ▾") {
		t.Errorf("expected collapsed turn summary 'Ran 3 commands ▾' in final viewport, got: %s", finalContent)
	}
	if !strings.Contains(finalContent, "Codebase is in good health.") {
		t.Errorf("expected final finish answer, got: %s", finalContent)
	}
	if !strings.Contains(finalContent, "✅ Task Complete • Resolved in 3 autonomous steps") {
		t.Errorf("expected 3 autonomous steps in complete banner, got: %s", finalContent)
	}

	// Toggle expansion with Ctrl+O
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = newM.(tui.Model)

	expandedContent := m.ViewportContent()
	if !strings.Contains(expandedContent, "Ran 3 commands ▴") {
		t.Errorf("expected expanded turn summary 'Ran 3 commands ▴', got: %s", expandedContent)
	}
	if !strings.Contains(expandedContent, "Ran git status ▸") || !strings.Contains(expandedContent, "Ran git diff ▸") {
		t.Errorf("expected expanded view to reveal detailed action history, got: %s", expandedContent)
	}

	// Toggle collapse back with Ctrl+O
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = newM.(tui.Model)

	recollapsedContent := m.ViewportContent()
	if !strings.Contains(recollapsedContent, "Ran 3 commands ▾") {
		t.Errorf("expected re-collapsed turn summary 'Ran 3 commands ▾', got: %s", recollapsedContent)
	}
}

// TestTUI_Presentation_NewModelAutoDetectsHardware verifies that NewModel automatically probes host hardware.
func TestTUI_Presentation_NewModelAutoDetectsHardware(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewModel(mock, false, false)
	view := m.View()

	hw, _ := probe.Detect()
	if hw != nil && hw.GPUName != "" {
		if !strings.Contains(view, hw.GPUName) {
			t.Errorf("expected view to contain detected GPU %q, got: %s", hw.GPUName, view)
		}
	}
}

// TestTUI_Presentation_FullWindowHeightUtilization verifies that the TUI utilizes the full terminal window height
// so that the bottom status line renders at the exact bottom line without blank line padding.
func TestTUI_Presentation_FullWindowHeightUtilization(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false)

	// Simulate window resize to 80x24 (standard terminal height)
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = newModel.(tui.Model)

	viewOutput := m.View()
	lines := strings.Split(viewOutput, "\n")
	if len(lines) != 24 {
		t.Errorf("expected view output to have exactly 24 lines, got %d", len(lines))
	}
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "~ ") {
		t.Errorf("expected last line of terminal to contain project info, got: %q", lastLine)
	}
	secondLastLine := lines[len(lines)-2]
	if !strings.Contains(secondLastLine, "[Ready] Enter send") {
		t.Errorf("expected second to last line of terminal to contain hotkey menu, got: %q", secondLastLine)
	}
}

// TestTUI_Presentation_LoopCircuitBreakerAndFailureBadges verifies that the TUI halts repeating action loops
// and presents informative failure badges and system interventions.
func TestTUI_Presentation_LoopCircuitBreakerAndFailureBadges(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false)
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = newModel.(tui.Model)

	// User submits initial prompt
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("inspect workspace")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	repeatActionXML := "I will inspect the workspace.\n<action name=\"read_window\">\n<path>.</path>\n<start>1</start>\n<end>50</end>\n</action>"

	// Turn 1: Propose read_window on directory .
	newM, _ = m.Update(tui.StreamDoneMsg(repeatActionXML))
	m = newM.(tui.Model)

	// User approves (or executes)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = newM.(tui.Model)

	// Action returns directory error
	newM, _ = m.Update(tui.ActionExecutedMsg("[Error: current directory \".\" is a directory, not a file. To discover files in this workspace, use <action name=\"find_files\"><pattern>*</pattern></action>.]\n"))
	m = newM.(tui.Model)

	content := m.ViewportContent()
	if !strings.Contains(content, "Failed read_window") {
		t.Errorf("expected failure badge 'Failed read_window', got: %s", content)
	}
	if !strings.Contains(content, "✗") {
		t.Errorf("expected failure badge marker ✗, got: %s", content)
	}

	// Turn 2: Repeat exact same action
	newM, _ = m.Update(tui.StreamDoneMsg(repeatActionXML))
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("[Error: current directory \".\" is a directory, not a file.]\n"))
	m = newM.(tui.Model)

	// Verify mock session received loop intervention
	var receivedIntervention bool
	for _, res := range mock.ActionResults {
		if strings.Contains(res, "SYSTEM INTERVENTION: Loop detected") {
			receivedIntervention = true
			break
		}
	}
	if !receivedIntervention {
		t.Errorf("expected system intervention to be fed to session ActionResults on turn 2 loop, got: %v", mock.ActionResults)
	}

	// Turn 3: Repeat exact same action
	newM, _ = m.Update(tui.StreamDoneMsg(repeatActionXML))
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("[Error: current directory \".\" is a directory, not a file.]\n"))
	m = newM.(tui.Model)

	// Turn 4: Repeat 4th time -> Circuit breaker should trip immediately and halt
	newM, _ = m.Update(tui.StreamDoneMsg(repeatActionXML))
	m = newM.(tui.Model)

	if m.State() != tui.StateIdle {
		t.Errorf("expected model state to halt and be StateIdle, got %v", m.State())
	}
	circuitBreakerContent := m.ViewportContent()
	if !strings.Contains(circuitBreakerContent, "Loop Circuit Breaker") {
		t.Errorf("expected circuit breaker alert in viewport, got: %s", circuitBreakerContent)
	}
	if !strings.Contains(circuitBreakerContent, "repeated 4 times") {
		t.Errorf("expected 4 times repeat explanation in viewport, got: %s", circuitBreakerContent)
	}
}

func TestTUI_Presentation_SlashCommands_YoloAndClear(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = newM.(tui.Model)

	// 1. Test /yolo toggle ON
	m = m.WithInitialPrompt("/yolo")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("expected nil cmd from slash command, got: %v", cmd)
	}
	m = updated.(tui.Model)

	if !strings.Contains(m.ViewportContent(), "YOLO MODE ENGAGED") {
		t.Errorf("expected YOLO MODE ENGAGED in viewport, got: %s", m.ViewportContent())
	}
	if !strings.Contains(m.View(), "[YOLO]") {
		t.Errorf("expected [YOLO] active indicator in view, got: %s", m.View())
	}

	// 2. Test /yolo toggle OFF
	m = m.WithInitialPrompt("/yolo")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	if !strings.Contains(m.ViewportContent(), "SAFE MODE ENGAGED") {
		t.Errorf("expected SAFE MODE ENGAGED in viewport, got: %s", m.ViewportContent())
	}

	// 3. Test /clear command
	mock.AppendUserMessage("User message before clear")
	mock.AppendAssistantMessage("Assistant message before clear")
	if len(mock.UserMessages) != 1 {
		t.Fatalf("expected 1 user message in mock before clear")
	}

	m = m.WithInitialPrompt("/clear")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	if len(mock.UserMessages) != 0 {
		t.Errorf("expected mock user messages to be cleared, got: %v", mock.UserMessages)
	}
	if !strings.Contains(m.ViewportContent(), "Session Cleared") {
		t.Errorf("expected Session Cleared message in viewport, got: %s", m.ViewportContent())
	}
	if strings.Contains(m.ViewportContent(), "User message before clear") {
		t.Errorf("expected old log to be purged from viewport after /clear")
	}
}

// TestTUI_Presentation_ConfigurablePersona verifies custom agent persona and operator names.
func TestTUI_Presentation_ConfigurablePersona(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false, "Aria", "Alice")

	if m.AgentName() != "Aria" {
		t.Errorf("expected agent name Aria, got %s", m.AgentName())
	}
	if m.OperatorName() != "Alice" {
		t.Errorf("expected operator name Alice, got %s", m.OperatorName())
	}

	content := m.ViewportContent()
	if !strings.Contains(content, "Welcome to Aria") {
		t.Errorf("expected Welcome to Aria in initial greeting, got: %s", content)
	}

	// Submit user input
	m = m.WithInitialPrompt("hello there")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	content = m.ViewportContent()
	if !strings.Contains(content, "Alice: hello there") {
		t.Errorf("expected 'Alice: hello there' in chat log, got: %s", content)
	}

	// Stream done response
	updated, _ = m.Update(tui.StreamDoneMsg("Hello Alice, I am ready."))
	m = updated.(tui.Model)

	content = m.ViewportContent()
	if !strings.Contains(content, "Aria: Hello Alice") {
		t.Errorf("expected 'Aria: Hello Alice' in chat log, got: %s", content)
	}
}

// TestTUI_Presentation_PersonaSlashCommands verifies /name and /operator runtime switching.
func TestTUI_Presentation_PersonaSlashCommands(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false)

	// Test /name Nova
	m = m.WithInitialPrompt("/name Nova")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	if m.AgentName() != "Nova" {
		t.Errorf("expected agent name Nova after /name, got %s", m.AgentName())
	}
	if !strings.Contains(m.ViewportContent(), "Persona Updated") || !strings.Contains(m.ViewportContent(), "Nova") {
		t.Errorf("expected Persona Updated confirmation in viewport, got: %s", m.ViewportContent())
	}

	// Test /operator Bob
	m = m.WithInitialPrompt("/operator Bob")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	if m.OperatorName() != "Bob" {
		t.Errorf("expected operator name Bob after /operator, got %s", m.OperatorName())
	}
	if !strings.Contains(m.ViewportContent(), "Operator Updated") || !strings.Contains(m.ViewportContent(), "Bob") {
		t.Errorf("expected Operator Updated confirmation in viewport, got: %s", m.ViewportContent())
	}
}

// TestTUI_Presentation_ActionRejectionPersistsToRegulator verifies that rejected actions are tracked by the regulator.
func TestTUI_Presentation_ActionRejectionPersistsToRegulator(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false)

	// Step 0: User inputs message
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("check files")})
	m = updated.(tui.Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	// Propose action reading README.md
	actionTurn := "I will inspect lines.\n<action name=\"read_window\">\n<path>README.md</path>\n<start_line>101</start_line>\n<end_line>200</end_line>\n</action>"
	updated, _ = m.Update(tui.StreamDoneMsg(actionTurn))
	m = updated.(tui.Model)

	if m.State() != tui.StateWaitingActionApproval {
		t.Fatalf("expected StateWaitingActionApproval, got %v", m.State())
	}

	// Reject with 'n'
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(tui.Model)

	if m.State() != tui.StateIdle {
		t.Fatalf("expected StateIdle, got %v", m.State())
	}

	// User enters second turn prompt
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tell me more")})
	m = updated.(tui.Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	// Attempt to propose another action targeting the rejected README.md
	actionTurn2 := "I will try reading earlier lines.\n<action name=\"read_window\">\n<path>README.md</path>\n<start_line>1</start_line>\n<end_line>100</end_line>\n</action>"
	updated, _ = m.Update(tui.StreamDoneMsg(actionTurn2))
	m = updated.(tui.Model)

	// Since it's rejected in regulator, it should display security warning
	content := m.ViewportContent()
	if !strings.Contains(content, "SECURITY WARNING") || !strings.Contains(content, "rejected by the operator") {
		t.Errorf("expected security warning indicating operator rejection in viewport, got: %s", content)
	}
}

// TestTUI_Presentation_VerbatimLoopIntervention verifies that repeating responses verbatim across turns triggers an intervention.
func TestTUI_Presentation_VerbatimLoopIntervention(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, false)

	// Turn 1: User prompt & model response
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("What can you tell me about the current project?")})
	m = updated.(tui.Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	turn1Response := "Lokol is an autonomous local-first agent engine for developer tools."
	updated, _ = m.Update(tui.StreamDoneMsg(turn1Response))
	m = updated.(tui.Model)

	if !strings.Contains(m.ViewportContent(), turn1Response) {
		t.Fatalf("expected turn 1 response in viewport")
	}

	// Turn 2: User asks follow-up & model produces exact same response verbatim
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Can you summarize that the project does?")})
	m = updated.(tui.Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	updated, _ = m.Update(tui.StreamDoneMsg(turn1Response))
	m = updated.(tui.Model)

	content := m.ViewportContent()
	if !strings.Contains(content, "Loop Intervention") || !strings.Contains(content, "Verbatim response detected across turns") {
		t.Errorf("expected loop intervention warning for verbatim repetition across turns, got: %s", content)
	}
}

// TestTUI_LiveActionDigest_JunieErgonomicsAndMultiCategorySummary verifies Junie-style ergonomics (lokol-zw4):
// 1. Live streaming displays single-line summaries per operation as they execute.
// 2. Post-response collapses into a multi-category summary categorizing operations by type and failure.
// 3. Hotkey (Ctrl+O) toggles between collapsed (▾) and expanded (▴) states.
func TestTUI_LiveActionDigest_JunieErgonomicsAndMultiCategorySummary(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, true) // YOLO mode engaged

	// Submit prompt
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("audit workspace")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	// Step 1: exec_bash git diff
	act1 := "<action name=\"exec_bash\">\ngit diff main...feat/regulator-pipeline\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(act1))
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("diff --git a/foo b/foo"))
	m = newM.(tui.Model)

	// Step 2: exec_bash git status
	act2 := "<action name=\"exec_bash\">\ngit status\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(act2))
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("clean"))
	m = newM.(tui.Model)

	// Step 3: exec_bash failed command
	act3 := "<action name=\"exec_bash\">\ngit diff --invalid-flag\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(act3))
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("[Error: unknown flag --invalid-flag]\n"))
	m = newM.(tui.Model)

	// Step 4: search_code
	act4 := "<action name=\"search_code\">\n<pattern>func.*TargetSummary</pattern>\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(act4))
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("liblokol/agent/tools.go:375: func (a *Action) TargetSummary() string"))
	m = newM.(tui.Model)

	// Step 5: read_window with range
	act5 := "<action name=\"read_window\">\n<path>liblokol/agent/tools.go</path>\n<start>360</start>\n<end>461</end>\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(act5))
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("package agent\n..."))
	m = newM.(tui.Model)

	// Step 6: get_environment (action)
	act6 := "<action name=\"get_environment\">\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(act6))
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("{\"os\":\"linux\"}"))
	m = newM.(tui.Model)

	// Assert live action stream display before turn finishes
	liveContent := m.ViewportContent()
	expectedLive := []string{
		"Ran git diff main...feat/regulator-pipeline ▸",
		"Ran git status ▸",
		"Failed to run git diff --invalid-flag ▸",
		"Searched 'func.*TargetSummary' ▸",
		"Read liblokol/agent/tools.go [360-461]",
		"Ran get_environment ▸",
	}
	for _, exp := range expectedLive {
		if !strings.Contains(liveContent, exp) {
			t.Errorf("expected live action stream to contain %q, got:\n%s", exp, liveContent)
		}
	}

	// Turn completion via task_finish
	turnFinish := "<action name=\"task_finish\">\nAudit complete. All inspections verified.\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(turnFinish))
	m = newM.(tui.Model)

	completedContent := m.ViewportContent()
	// Multi-category summary: 2 commands, 1 action, 1 search, 1 file explored, 1 command failed
	expectedSummary := "Ran 2 commands, ran 1 action, 1 search, explored 1 file, 1 command failed ▾"
	if !strings.Contains(completedContent, expectedSummary) {
		t.Errorf("expected collapsed multi-category summary %q, got:\n%s", expectedSummary, completedContent)
	}

	// Detailed live lines should be collapsed in default post-response view
	if strings.Contains(completedContent, "Ran git diff main...feat/regulator-pipeline ▸") {
		t.Errorf("expected individual live lines to be collapsed on turn completion, got:\n%s", completedContent)
	}

	// Hotkey expansion with Ctrl+O
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = newM.(tui.Model)

	expandedContent := m.ViewportContent()
	expectedExpandedHeader := "Ran 2 commands, ran 1 action, 1 search, explored 1 file, 1 command failed ▴"
	if !strings.Contains(expandedContent, expectedExpandedHeader) {
		t.Errorf("expected expanded summary header %q, got:\n%s", expectedExpandedHeader, expandedContent)
	}
	for _, exp := range expectedLive {
		if !strings.Contains(expandedContent, exp) {
			t.Errorf("expected expanded view to reveal item %q, got:\n%s", exp, expandedContent)
		}
	}

	// Hotkey collapse with Ctrl+O
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = newM.(tui.Model)

	recollapsedContent := m.ViewportContent()
	if !strings.Contains(recollapsedContent, expectedSummary) {
		t.Errorf("expected re-collapsed summary %q, got:\n%s", expectedSummary, recollapsedContent)
	}
	if strings.Contains(recollapsedContent, "Ran git diff main...feat/regulator-pipeline ▸") {
		t.Errorf("expected individual live lines hidden again upon re-collapse")
	}
}

// TestTUI_LiveActionDigest_SlashCommandsAndTabToggle verifies that /actions, /expand, and Tab toggle turn summaries.
func TestTUI_LiveActionDigest_SlashCommandsAndTabToggle(t *testing.T) {
	mock := NewMockSession()
	m := tui.NewWithSession(mock, nil, true)

	// Turn with a command
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("status check")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	act := "<action name=\"exec_bash\">\ngit status\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(act))
	m = newM.(tui.Model)
	newM, _ = m.Update(tui.ActionExecutedMsg("clean"))
	m = newM.(tui.Model)

	finish := "<action name=\"task_finish\">\nDone.\n</action>"
	newM, _ = m.Update(tui.StreamDoneMsg(finish))
	m = newM.(tui.Model)

	if !strings.Contains(m.ViewportContent(), "Ran 1 command ▾") {
		t.Fatalf("expected collapsed summary 'Ran 1 command ▾'")
	}

	// 1. Toggle via /actions
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/actions")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	if !strings.Contains(m.ViewportContent(), "Ran 1 command ▴") {
		t.Errorf("expected /actions to expand summary to 'Ran 1 command ▴', got: %s", m.ViewportContent())
	}

	// 2. Toggle back via /expand
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/expand")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	if !strings.Contains(m.ViewportContent(), "Ran 1 command ▾") {
		t.Errorf("expected /expand to collapse summary back to 'Ran 1 command ▾', got: %s", m.ViewportContent())
	}

	// 3. Toggle via Tab key (when textarea is empty)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newM.(tui.Model)

	if !strings.Contains(m.ViewportContent(), "Ran 1 command ▴") {
		t.Errorf("expected Tab to expand summary to 'Ran 1 command ▴', got: %s", m.ViewportContent())
	}

	// 4. /clear resets turn summaries
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/clear")})
	m = newM.(tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	if len(m.TurnDigests()) != 0 {
		t.Errorf("expected TurnDigests to be empty after /clear, got: %d", len(m.TurnDigests()))
	}
}







