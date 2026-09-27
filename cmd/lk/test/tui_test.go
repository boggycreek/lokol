// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

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
	if !strings.Contains(view, "[Mode: general]") {
		t.Fatalf("expected view to contain '[Mode: general]', got: %s", view)
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
	if !strings.Contains(view, "[Mode: coding]") {
		t.Errorf("expected view to contain '[Mode: coding]' after switch, got: %s", view)
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

	// Invalid mode handling
	m = m.WithInitialPrompt("/mode invalid_mode")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)

	if !strings.Contains(m.ViewportContent(), "Invalid Mode") {
		t.Errorf("expected invalid mode warning in viewport, got: %s", m.ViewportContent())
	}
}


