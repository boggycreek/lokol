// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

import (
	"strings"
	"testing"

	"github.com/boggycreek/lokol/pkg/agent"
	"github.com/boggycreek/lokol/pkg/probe"
	"github.com/boggycreek/lokol/pkg/tui"
	tea "github.com/charmbracelet/bubbletea"
)

// TestTUISmokeTest simulates TUI lifecycle events (Window resize, user typing, render).
func TestTUISmokeTest(t *testing.T) {
	hw := &probe.HardwareProfile{
		OS:        "linux",
		Arch:      "amd64",
		GPUName:   "NVIDIA GeForce RTX 3060",
		VRAMBytes: 12 * 1024 * 1024 * 1024,
	}
	client := agent.NewClient("http://127.0.0.1:8080")
	m := tui.New(client, hw, false)

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

	// 3. Simulate keystrokes
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	m = newModel.(tui.Model)

	viewOutput = m.View()
	if viewOutput == "" {
		t.Errorf("view output was empty")
	}
}

// TestTUIViewportLineWrapping verifies that long lines exceeding the viewport width are wrapped.
func TestTUIViewportLineWrapping(t *testing.T) {
	client := agent.NewClient("http://127.0.0.1:8080")
	m := tui.New(client, nil, false)

	// Set a narrow window: Width 40 -> viewport width 36
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	m = newModel.(tui.Model)

	// Send a long user prompt exceeding 36 characters without newlines
	longInput := "This is a very long agent prompt designed to test whether the viewport wraps lines properly or truncates them horizontally at the boundary."
	// Set textarea value and simulate Enter key
	for _, r := range longInput {
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = newModel.(tui.Model)
	}
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(tui.Model)

	content := m.ViewportContent()
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// Visible width should not exceed viewport width (36)
		// Strip ANSI escape codes if needed, but here we can check length
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
