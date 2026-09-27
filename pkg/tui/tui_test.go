// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tui

import (
	"strings"
	"testing"

	"github.com/boggycreek/lokol/pkg/agent"
	"github.com/boggycreek/lokol/pkg/probe"
	tea "github.com/charmbracelet/bubbletea"
)

func TestInternalizedInferencesAndCleanFinalPresentation(t *testing.T) {
	client := agent.NewClient("http://127.0.0.1:8080")
	hw := &probe.HardwareProfile{OS: "linux", Arch: "amd64", CPUCores: 4}
	m := New(client, hw, true, t.TempDir()) // YOLO mode active

	// Simulate user submitting prompt
	m.state = stateStreaming
	m.stepCount = 0

	// Step 1: Model generates an intermediate action with clean thought
	actionTurn1 := "I will examine the directory to find where the bug is located.\n<action name=\"read_outline\">\n<path>.</path>\n</action>"
	newM, cmd := m.Update(streamDoneMsg(actionTurn1))
	m = newM.(Model)

	// In default (internalized) mode:
	// Intermediate thought and tool execution banners MUST NOT be in the chat viewport!
	content := m.ViewportContent()
	if strings.Contains(content, "I will examine the directory") {
		t.Fatalf("expected intermediate thought to be internalized, but found in viewport: %q", content)
	}
	if strings.Contains(content, "⚡ Reading Outline") {
		t.Fatalf("expected tool execution banner to be internalized in YOLO mode, but found: %q", content)
	}
	if m.stepCount != 1 {
		t.Fatalf("expected stepCount 1, got %d", m.stepCount)
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
	newM, _ = m.Update(actionExecutedMsg("main.go\ncalc.go"))
	m = newM.(Model)

	// Tool completion message must NOT pollute the chat viewport
	content = m.ViewportContent()
	if strings.Contains(content, "Executed successfully") {
		t.Fatalf("expected 'Executed successfully' to be internalized, but found in viewport: %q", content)
	}
	if m.state != stateStreaming {
		t.Fatalf("expected stateStreaming after action execution, got %v", m.state)
	}

	// Step 3: Model completes task with task_finish
	finishTurn := "All tests pass. The issue is resolved.\n<action name=\"task_finish\">\nCorrected off-by-one error in calc.go.\n</action>"
	newM, _ = m.Update(streamDoneMsg(finishTurn))
	m = newM.(Model)

	if m.state != stateIdle {
		t.Fatalf("expected stateIdle after task_finish, got %v", m.state)
	}
	if m.stepCount != 0 {
		t.Fatalf("expected stepCount reset to 0, got %d", m.stepCount)
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

func TestTUI_VerboseToggle(t *testing.T) {
	client := agent.NewClient("http://127.0.0.1:8080")
	m := New(client, nil, true, t.TempDir())

	if m.verbose {
		t.Fatal("expected verbose to be false by default")
	}

	// Toggle with Ctrl+V
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	m = newM.(Model)
	if !m.verbose {
		t.Fatal("expected verbose to be true after Ctrl+V")
	}
	if !strings.Contains(m.ViewportContent(), "VERBOSE ON") {
		t.Fatalf("expected verbose ON message in viewport, got: %q", m.ViewportContent())
	}

	// In verbose mode, intermediate actions ARE logged to the viewport
	m.state = stateStreaming
	actionTurn := "Investigating files.\n<action name=\"read_outline\">\n<path>.</path>\n</action>"
	newM, _ = m.Update(streamDoneMsg(actionTurn))
	m = newM.(Model)

	content := m.ViewportContent()
	if !strings.Contains(content, "Investigating files.") {
		t.Fatalf("expected intermediate thought in viewport when verbose=true, got: %q", content)
	}
	if !strings.Contains(content, "⚡ Reading Outline") {
		t.Fatalf("expected tool execution banner in viewport when verbose=true, got: %q", content)
	}
}
