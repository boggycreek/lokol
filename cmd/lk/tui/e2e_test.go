// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tui_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/boggycreek/lokol/cmd/lk/tui"
	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/probe"
	tea "github.com/charmbracelet/bubbletea"
)

// mockInferenceServer spins up an in-memory HTTP server emulating llama-server's
// SSE completions, slot status polling, and slot release endpoints.
type mockInferenceServer struct {
	server       *httptest.Server
	mu           sync.Mutex
	responses    []string
	requestCount int
	slotReleases int
}

func newMockInferenceServer(responses ...string) *mockInferenceServer {
	m := &mockInferenceServer{
		responses: responses,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/chat/completions"):
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)

			resp := ""
			if m.requestCount < len(m.responses) {
				resp = m.responses[m.requestCount]
			} else {
				resp = "Task complete.\n<action name=\"task_finish\">Done</action>"
			}
			m.requestCount++

			// Split into chunks if there is thought text and action XML to emulate streaming
			if strings.Contains(resp, "<action") {
				parts := strings.SplitN(resp, "<action", 2)
				if parts[0] != "" {
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", parts[0])
					if ok {
						flusher.Flush()
					}
				}
				actionPart := "<action" + parts[1]
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", actionPart)
				if ok {
					flusher.Flush()
				}
			} else {
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", resp)
				if ok {
					flusher.Flush()
				}
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")

		case strings.HasPrefix(r.URL.Path, "/slots"):
			if strings.Contains(r.URL.RawQuery, "action=release") {
				m.slotReleases++
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, `{"status":"ok"}`)
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `[{"id":0,"n_ctx":4096,"n_past":250}]`)

		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	m.server = httptest.NewServer(handler)
	return m
}

func (m *mockInferenceServer) Close() {
	if m.server != nil {
		m.server.Close()
	}
}

func (m *mockInferenceServer) URL() string {
	return m.server.URL
}

// ---------------------------------------------------------------------------
// 1. E2E: Real agent.Session + Mock Inference Engine Tool Execution Loop
// ---------------------------------------------------------------------------

func TestTUI_E2E_RealSession_ToolExecutionAndHistory(t *testing.T) {
	tempDir := t.TempDir()

	// Prepare mock responses: Turn 1 executes write_file, Turn 2 completes task
	srv := newMockInferenceServer(
		"I will create the greeting file.\n<action name=\"write_file\"><path>greeting.txt</path><content>Hello from lokol E2E!</content></action>",
		"File created successfully!\n<action name=\"task_finish\">All done</action>",
	)
	defer srv.Close()

	client := agent.NewClient(srv.URL())
	// Use REAL agent.Session!
	session := agent.NewSessionWithMode(client, tempDir, agent.ModeGeneral)

	hw := &probe.HardwareProfile{
		OS:        "linux",
		Arch:      "amd64",
		GPUName:   "NVIDIA GeForce RTX 4090",
		VRAMBytes: 24 * 1024 * 1024 * 1024,
	}

	// Start in safe mode (yoloMode = false)
	m := tui.NewWithSession(session, hw, false)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = newM.(tui.Model)

	// Step 1: User enters prompt
	m = m.WithInitialPrompt("Create greeting.txt")
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	if m.State() != tui.StateStreaming {
		t.Fatalf("expected StateStreaming after entering prompt, got: %v", m.State())
	}

	// Verify real session history contains the user message
	if len(session.History) < 2 {
		t.Fatalf("expected real session history to contain system and user prompt, got %d items", len(session.History))
	}
	if session.History[1].Content != "Create greeting.txt" {
		t.Errorf("expected user prompt in session history, got: %s", session.History[1].Content)
	}

	// Execute streaming turn on the real session (which hits our mock HTTP engine)
	tokenChan := make(chan string, 100)
	turn1Resp, err := session.StreamTurn(context.Background(), tokenChan)
	if err != nil {
		t.Fatalf("session.StreamTurn failed: %v", err)
	}

	// Deliver completed stream turn to TUI model
	newM, _ = m.Update(tui.StreamDoneMsg(turn1Resp))
	m = newM.(tui.Model)

	// In safe mode, an action proposal should transition to StateWaitingActionApproval
	if m.State() != tui.StateWaitingActionApproval {
		t.Fatalf("expected StateWaitingActionApproval, got: %v", m.State())
	}

	viewOutput := m.View()
	if !strings.Contains(viewOutput, "PROPOSED ACTION: write_file") {
		t.Errorf("expected view to render proposed action header, got: %s", viewOutput)
	}
	if !strings.Contains(viewOutput, "greeting.txt") {
		t.Errorf("expected view to show target file greeting.txt, got: %s", viewOutput)
	}

	// Step 2: User approves action by pressing 'y'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = newM.(tui.Model)

	if m.State() != tui.StateExecutingAction {
		t.Fatalf("expected StateExecutingAction after approval, got: %v", m.State())
	}

	// Execute action via REAL session.ExecuteAction
	act := m.PendingAction()
	if act == nil {
		t.Fatalf("expected pending action on model, got nil")
	}
	out, err := session.ExecuteAction(context.Background(), act)
	if err != nil {
		t.Fatalf("session.ExecuteAction failed: %v", err)
	}

	// Deliver action executed result to TUI
	newM, _ = m.Update(tui.ActionExecutedMsg(out))
	m = newM.(tui.Model)

	// Verify physical file was written to disk by real session tool execution!
	createdFile := filepath.Join(tempDir, "greeting.txt")
	content, err := os.ReadFile(createdFile)
	if err != nil {
		t.Fatalf("expected greeting.txt to be physically created by real session, got error: %v", err)
	}
	if string(content) != "Hello from lokol E2E!" {
		t.Errorf("unexpected file content: %s", string(content))
	}

	// Step 3: Turn 2 runs automatically after action execution to report result
	if m.State() != tui.StateStreaming {
		t.Fatalf("expected StateStreaming after action execution, got: %v", m.State())
	}

	tokenChan2 := make(chan string, 100)
	turn2Resp, err := session.StreamTurn(context.Background(), tokenChan2)
	if err != nil {
		t.Fatalf("turn 2 StreamTurn failed: %v", err)
	}
	newM, _ = m.Update(tui.StreamDoneMsg(turn2Resp))
	m = newM.(tui.Model)

	// Model should finish and return to StateIdle
	if m.State() != tui.StateIdle {
		t.Errorf("expected StateIdle after completion, got: %v", m.State())
	}

	// Check final viewport contains success confirmation
	viewportText := m.ViewportContent()
	if !strings.Contains(viewportText, "File created successfully!") && !strings.Contains(viewportText, "All done") {
		t.Errorf("expected completion message in viewport, got: %s", viewportText)
	}

	// Verify real session history now contains the action_result feedback
	hasActionResult := false
	for _, hist := range session.History {
		if hist.Role == "user" && strings.Contains(hist.Content, "<action_result") {
			hasActionResult = true
			break
		}
	}
	if !hasActionResult {
		t.Errorf("expected real session history to contain <action_result> message, history: %+v", session.History)
	}
}

// ---------------------------------------------------------------------------
// 2. E2E: Real agent.Session + YOLO Mode Autonomous Action Execution
// ---------------------------------------------------------------------------

func TestTUI_E2E_RealSession_YoloModeAutonomousExecution(t *testing.T) {
	tempDir := t.TempDir()

	srv := newMockInferenceServer(
		"Autonomous execution.\n<action name=\"write_file\"><path>auto.txt</path><content>auto-generated</content></action>",
		"Done.\n<action name=\"task_finish\">Finished</action>",
	)
	defer srv.Close()

	client := agent.NewClient(srv.URL())
	session := agent.NewSessionWithMode(client, tempDir, agent.ModeGeneral)

	// Start with yoloMode = true
	m := tui.NewWithSession(session, nil, true)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = newM.(tui.Model)

	// Send prompt
	m = m.WithInitialPrompt("Generate file autonomously")
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	// Stream Turn 1 using real session
	tokenChan := make(chan string, 100)
	turn1Resp, err := session.StreamTurn(context.Background(), tokenChan)
	if err != nil {
		t.Fatalf("session.StreamTurn failed: %v", err)
	}

	// Deliver to model
	newM, _ = m.Update(tui.StreamDoneMsg(turn1Resp))
	m = newM.(tui.Model)

	// In YOLO mode, model should bypass StateWaitingActionApproval and go directly to StateExecutingAction
	if m.State() != tui.StateExecutingAction {
		t.Fatalf("expected StateExecutingAction directly in YOLO mode, got: %v", m.State())
	}

	// Execute action via real session
	act := m.PendingAction()
	if act == nil {
		t.Fatalf("expected pending action in YOLO mode, got nil")
	}
	out, err := session.ExecuteAction(context.Background(), act)
	if err != nil {
		t.Fatalf("session.ExecuteAction failed: %v", err)
	}

	// Deliver result to model
	newM, _ = m.Update(tui.ActionExecutedMsg(out))
	m = newM.(tui.Model)

	// File must exist without any manual approval!
	autoFile := filepath.Join(tempDir, "auto.txt")
	if _, err := os.Stat(autoFile); err != nil {
		t.Fatalf("expected auto.txt to exist in YOLO mode, err: %v", err)
	}

	// Complete second turn
	tokenChan2 := make(chan string, 100)
	turn2Resp, err := session.StreamTurn(context.Background(), tokenChan2)
	if err != nil {
		t.Fatalf("turn 2 StreamTurn failed: %v", err)
	}
	newM, _ = m.Update(tui.StreamDoneMsg(turn2Resp))
	m = newM.(tui.Model)

	if m.State() != tui.StateIdle {
		t.Errorf("expected final StateIdle, got: %v", m.State())
	}
}

// ---------------------------------------------------------------------------
// 3. E2E: Real agent.Session + Slash Commands (/mode, /yolo, /clear)
// ---------------------------------------------------------------------------

func TestTUI_E2E_RealSession_SlashCommands(t *testing.T) {
	tempDir := t.TempDir()
	srv := newMockInferenceServer()
	defer srv.Close()

	client := agent.NewClient(srv.URL())
	session := agent.NewSessionWithMode(client, tempDir, agent.ModeGeneral)

	m := tui.NewWithSession(session, nil, false)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = newM.(tui.Model)

	// 1. Slash command: /mode coding
	// In the real session, this switches Mode and rebuilds the system prompt with the coding persona!
	m = m.WithInitialPrompt("/mode coding")
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	if session.GetMode() != agent.ModeCoding {
		t.Errorf("expected real session mode to be coding, got: %s", session.GetMode())
	}
	// Verify real session system prompt was rebuilt for coding
	if !strings.Contains(session.History[0].Content, "senior software engineer") &&
		!strings.Contains(session.History[0].Content, "coding") {
		t.Errorf("expected real session system prompt to be rebuilt for coding mode, got: %s", session.History[0].Content)
	}
	if !strings.Contains(m.View(), "[ CODE ]") {
		t.Errorf("expected TUI view to show [ CODE ] mode indicator, got: %s", m.View())
	}

	// 2. Slash command: /yolo toggle
	m = m.WithInitialPrompt("/yolo")
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	if !strings.Contains(m.ViewportContent(), "YOLO MODE ENGAGED") {
		t.Errorf("expected YOLO MODE ENGAGED in viewport, got: %s", m.ViewportContent())
	}
	if !strings.Contains(m.View(), "[YOLO]") {
		t.Errorf("expected active [YOLO] dash light in view, got: %s", m.View())
	}

	// Add messages to real session history (system prompt + mode switch event + 2 messages = 4)
	session.AppendUserMessage("History item 1")
	session.AppendAssistantMessage("History item 2")
	if len(session.History) != 4 {
		t.Fatalf("expected 4 history entries before clear, got: %d", len(session.History))
	}

	m = m.WithInitialPrompt("/clear")
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	// Real session history must be reset to single system prompt
	if len(session.History) != 1 {
		t.Errorf("expected real session history to be reset to 1 system prompt, got: %d", len(session.History))
	}
	if !strings.Contains(m.ViewportContent(), "Session Cleared") {
		t.Errorf("expected Session Cleared confirmation in viewport, got: %s", m.ViewportContent())
	}
}

// ---------------------------------------------------------------------------
// 4. E2E: Keystroke Injection & Interruption (Esc / Ctrl+C)
// ---------------------------------------------------------------------------

func TestTUI_E2E_KeystrokeInjectionAndInterruption(t *testing.T) {
	tempDir := t.TempDir()

	// Server that delays responding to simulate slow inference
	hungSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/slots") && strings.Contains(r.URL.RawQuery, "action=release") {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"ok"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/chat/completions") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Thinking...\"}}]}\n\n")
			if ok {
				flusher.Flush()
			}
			time.Sleep(2 * time.Second)
		}
	}))
	defer hungSrv.Close()

	client := agent.NewClient(hungSrv.URL)
	session := agent.NewSessionWithMode(client, tempDir, agent.ModeGeneral)

	m := tui.NewWithSession(session, nil, false)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = newM.(tui.Model)

	// Simulate typing "Hello world" character by character
	inputStr := "Hello world"
	for _, ch := range inputStr {
		newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = newM.(tui.Model)
	}

	// Press Enter to start turn
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(tui.Model)

	if m.State() != tui.StateStreaming {
		t.Fatalf("expected StateStreaming after enter, got: %v", m.State())
	}

	// Press Esc while streaming to abort
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(tui.Model)

	if m.State() != tui.StateIdle {
		t.Errorf("expected StateIdle after Esc abort, got: %v", m.State())
	}
	if !strings.Contains(m.ViewportContent(), "Stream aborted (Esc)") {
		t.Errorf("expected Stream aborted message in viewport, got: %s", m.ViewportContent())
	}
}

// ---------------------------------------------------------------------------
// 5. Layout Invariance across Terminal Geometries
// ---------------------------------------------------------------------------

func TestTUI_E2E_LayoutInvariance_AcrossTerminalDimensions(t *testing.T) {
	tempDir := t.TempDir()
	srv := newMockInferenceServer()
	defer srv.Close()

	client := agent.NewClient(srv.URL())
	session := agent.NewSessionWithMode(client, tempDir, agent.ModeGeneral)

	dimensions := []struct {
		name   string
		width  int
		height int
	}{
		{"Standard 80x24", 80, 24},
		{"Modern 120x40", 120, 40},
		{"Narrow 40x20", 40, 20},
		{"Wide 160x50", 160, 50},
		{"Tall 80x60", 80, 60},
	}

	hw := &probe.HardwareProfile{
		OS:        "linux",
		Arch:      "amd64",
		GPUName:   "NVIDIA RTX 3060",
		VRAMBytes: 12 * 1024 * 1024 * 1024,
	}

	for _, dim := range dimensions {
		t.Run(dim.name, func(t *testing.T) {
			m := tui.NewWithSession(session, hw, false)
			newM, _ := m.Update(tea.WindowSizeMsg{Width: dim.width, Height: dim.height})
			m = newM.(tui.Model)

			view := m.View()
			if view == "" {
				t.Fatalf("empty view rendered for dimension %s", dim.name)
			}

			// View should not crash or panic, and should render header
			if !strings.Contains(view, "lokol") {
				t.Errorf("expected view to contain lokol header for %s", dim.name)
			}

			// Ensure viewport content line length does not grossly breach boundaries
			lines := strings.Split(m.ViewportContent(), "\n")
			for i, line := range lines {
				// Allow small margin for ANSI escapes
				cleanLen := len([]rune(line))
				if dim.width >= 40 && cleanLen > dim.width+10 {
					t.Errorf("[%s] line %d exceeds terminal width: len %d vs width %d", dim.name, i, cleanLen, dim.width)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. Live Bubble Tea Event Loop Program Execution
// ---------------------------------------------------------------------------

func TestTUI_E2E_LiveBubbleTeaProgram_SendMessages(t *testing.T) {
	tempDir := t.TempDir()
	srv := newMockInferenceServer("Live program test response.\n<action name=\"task_finish\">Done</action>")
	defer srv.Close()

	client := agent.NewClient(srv.URL())
	session := agent.NewSessionWithMode(client, tempDir, agent.ModeGeneral)

	m := tui.NewWithSession(session, nil, false)

	in := bytes.NewBuffer(nil)
	out := bytes.NewBuffer(nil)

	p := tea.NewProgram(
		m,
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithoutRenderer(),
	)

	// Run program in background goroutine
	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	// Send window size
	p.Send(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Inject slash command via program Send
	p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	p.Send(tea.KeyMsg{Type: tea.KeyEnter})

	// Quit program cleanly
	time.Sleep(50 * time.Millisecond)
	p.Send(tea.QuitMsg{})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("tea.Program run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("tea.Program did not terminate cleanly within timeout")
	}
}
