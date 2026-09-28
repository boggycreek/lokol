// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/refinery"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		input    string
		expected agent.Mode
		hasErr   bool
	}{
		{"general", agent.ModeGeneral, false},
		{"coding", agent.ModeCoding, false},
		{"moe", agent.ModeMoE, false},
		{"GENERAL", agent.ModeGeneral, false},
		{"Coding", agent.ModeCoding, false},
		{"chat", agent.ModeGeneral, false},
		{"code", agent.ModeCoding, false},
		{"expert", agent.ModeMoE, false},
		{"", agent.ModeGeneral, false},
		{"unknown", agent.Mode(""), true},
	}

	for _, tt := range tests {
		m, err := agent.ParseMode(tt.input)
		if tt.hasErr && err == nil {
			t.Errorf("ParseMode(%q) expected error, got nil", tt.input)
		}
		if !tt.hasErr && err != nil {
			t.Errorf("ParseMode(%q) unexpected error: %v", tt.input, err)
		}
		if m != tt.expected {
			t.Errorf("ParseMode(%q) = %v, expected %v", tt.input, m, tt.expected)
		}
	}
}

func TestBuildSystemPromptForMode_General(t *testing.T) {
	dir := t.TempDir()
	// Create an AGENTS.md in tempdir to test that general mode ignores it
	_ = os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Strict Coding Conventions\nMust run bd prime"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("Local notes"), 0644)

	env := agent.DetectHostEnvironment(dir)
	prompt := agent.BuildSystemPromptForMode(agent.ModeGeneral, env, "")

	// Must contain local environment
	if !strings.Contains(prompt, "<environment>") {
		t.Errorf("expected general prompt to contain <environment>")
	}
	if !strings.Contains(prompt, dir) {
		t.Errorf("expected general prompt to contain cwd %s", dir)
	}
	if !strings.Contains(prompt, "notes.txt") {
		t.Errorf("expected general prompt to list workspace files")
	}

	// Must NOT contain AGENTS.md codebase context
	if strings.Contains(prompt, "Strict Coding Conventions") {
		t.Errorf("general prompt should NOT ingest AGENTS.md")
	}

	// Must contain artifact creation & manipulation tools
	if !strings.Contains(prompt, "write_file") {
		t.Errorf("expected general prompt to include write_file tool")
	}
	if !strings.Contains(prompt, "replace_file") {
		t.Errorf("expected general prompt to include replace_file tool")
	}
	if !strings.Contains(prompt, "read_window") {
		t.Errorf("expected general prompt to include read_window tool")
	}

	// Should NOT contain software engineering gate tools
	if strings.Contains(prompt, "run_test") {
		t.Errorf("general prompt should NOT include run_test tool")
	}
	if strings.Contains(prompt, "read_outline") {
		t.Errorf("general prompt should NOT include read_outline tool")
	}
}

func TestBuildSystemPromptForMode_Coding(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Engineering Rules\nAlways verify with tests"), 0644)

	env := agent.DetectHostEnvironment(dir)
	codebaseCtx := refinery.LoadCodebaseContext(dir)
	prompt := agent.BuildSystemPromptForMode(agent.ModeCoding, env, codebaseCtx)

	// Must contain local environment
	if !strings.Contains(prompt, "<environment>") {
		t.Errorf("expected coding prompt to contain <environment>")
	}

	// Must contain codebase context with AGENTS.md
	if !strings.Contains(prompt, "<codebase_context>") {
		t.Errorf("expected coding prompt to contain <codebase_context>")
	}
	if !strings.Contains(prompt, "Engineering Rules") {
		t.Errorf("expected coding prompt to ingest AGENTS.md")
	}

	// Must contain full coding tools
	if !strings.Contains(prompt, "run_test") {
		t.Errorf("expected coding prompt to include run_test tool")
	}
	if !strings.Contains(prompt, "read_outline") {
		t.Errorf("expected coding prompt to include read_outline tool")
	}
	if !strings.Contains(prompt, "search_code") {
		t.Errorf("expected coding prompt to include search_code tool")
	}
	if !strings.Contains(prompt, "git_diff_summary") {
		t.Errorf("expected coding prompt to include git_diff_summary tool")
	}
}

func TestBuildSystemPromptForMode_MoE(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Engineering Rules"), 0644)

	env := agent.DetectHostEnvironment(dir)
	prompt := agent.BuildSystemPromptForMode(agent.ModeMoE, env, "")

	// Must contain local environment
	if !strings.Contains(prompt, "<environment>") {
		t.Errorf("expected moe prompt to contain <environment>")
	}

	// Must NOT contain AGENTS.md codebase context
	if strings.Contains(prompt, "<codebase_context>") {
		t.Errorf("moe prompt should NOT ingest <codebase_context>")
	}

	// Must contain MoE persona instructions
	if !strings.Contains(prompt, "Mixture-of-Experts") {
		t.Errorf("expected moe prompt to contain Mixture-of-Experts instructions")
	}

	// Must contain file/artifact tools
	if !strings.Contains(prompt, "write_file") {
		t.Errorf("expected moe prompt to include write_file tool")
	}
}

func TestSession_ModeSwitching(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Ingested Rules"), 0644)

	client := agent.NewClient("http://127.0.0.1:8080")
	sess := agent.NewSessionWithMode(client, dir, agent.ModeGeneral)

	if sess.GetMode() != agent.ModeGeneral {
		t.Fatalf("expected initial mode to be general, got %v", sess.GetMode())
	}

	// Switch to Coding
	sess.SetMode(agent.ModeCoding)
	if sess.GetMode() != agent.ModeCoding {
		t.Fatalf("expected mode to be coding, got %v", sess.GetMode())
	}

	// Switch to MoE
	sess.SetMode(agent.ModeMoE)
	if sess.GetMode() != agent.ModeMoE {
		t.Fatalf("expected mode to be moe, got %v", sess.GetMode())
	}
}
