package quik_test

import (
	"strings"
	"testing"

	"github.com/boggycreek/quik/pkg/agent"
	"github.com/boggycreek/quik/pkg/probe"
	"github.com/boggycreek/quik/pkg/tui"
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
	if !strings.Contains(viewOutput, "quik") {
		t.Errorf("expected view to contain 'quik', got %s", viewOutput)
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
