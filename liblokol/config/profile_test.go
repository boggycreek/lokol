// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/boggycreek/lokol/liblokol/config"
)

func TestProfile_Defaults(t *testing.T) {
	def := config.DefaultCodingProfile()
	if def.Name != "coding" {
		t.Fatalf("expected profile name 'coding', got %s", def.Name)
	}
	if def.Mode != "coding" {
		t.Fatalf("expected profile mode 'coding', got %s", def.Mode)
	}
	if len(def.Tools) != 0 {
		t.Fatalf("expected nil or empty tools for default coding profile, got %v", def.Tools)
	}
}

func TestProfile_BuiltinProfiles(t *testing.T) {
	builtins := config.BuiltinProfiles()
	expected := []string{"coding", "security-audit", "refactor", "doc-gen"}
	for _, name := range expected {
		p, ok := builtins[name]
		if !ok || p == nil {
			t.Fatalf("expected builtin profile %q to exist", name)
		}
		if p.Name != name {
			t.Fatalf("expected profile name %q, got %q", name, p.Name)
		}
	}

	sec := builtins["security-audit"]
	if !sec.Regulator.ReadOnly {
		t.Errorf("expected security-audit to be read_only")
	}
	if sec.Regulator.MaxRiskLevel != "low" {
		t.Errorf("expected max risk level low, got %s", sec.Regulator.MaxRiskLevel)
	}
}

func TestProfile_ParseJSON(t *testing.T) {
	jsonPayload := `{
		"name": "custom-audit",
		"description": "Custom audit profile",
		"mode": "general",
		"tools": ["find_files", "read_window"],
		"system_prompt_overlay": "Audit rigorously.",
		"regulator": {
			"read_only": true,
			"blocked_actions": ["write_file", "exec_bash"],
			"disallowed_commands": ["rm", "dd"],
			"disallowed_paths": [".env"],
			"max_risk_level": "medium"
		}
	}`

	p, err := config.ParseProfile([]byte(jsonPayload))
	if err != nil {
		t.Fatalf("failed to parse JSON profile: %v", err)
	}

	if p.Name != "custom-audit" {
		t.Errorf("expected name custom-audit, got %s", p.Name)
	}
	if !p.Regulator.ReadOnly {
		t.Errorf("expected read_only to be true")
	}
	if len(p.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(p.Tools))
	}
	if len(p.Regulator.BlockedActions) != 2 {
		t.Errorf("expected 2 blocked actions, got %d", len(p.Regulator.BlockedActions))
	}
	if p.Regulator.MaxRiskLevel != "medium" {
		t.Errorf("expected max risk level medium, got %s", p.Regulator.MaxRiskLevel)
	}
}

func TestProfile_ParseYAML(t *testing.T) {
	yamlPayload := `
name: yaml-audit
description: YAML audit profile
mode: coding
tools:
  - find_files
  - read_window
system_prompt_overlay: |
  Line 1 of overlay.
	Line 2 of overlay with tab.

  Line 4 after empty line.
regulator:
  read_only: true
  blocked_actions:
    - write_file
    - exec_bash
  disallowed_commands:
    - rm
  disallowed_paths:
    - "*.key"
  max_risk_level: low
`
	p, err := config.ParseProfile([]byte(yamlPayload))
	if err != nil {
		t.Fatalf("failed to parse YAML profile: %v", err)
	}

	if p.Name != "yaml-audit" {
		t.Errorf("expected name yaml-audit, got %s", p.Name)
	}
	if !p.Regulator.ReadOnly {
		t.Errorf("expected read_only to be true")
	}
	if len(p.Tools) != 2 || p.Tools[0] != "find_files" {
		t.Errorf("unexpected tools: %v", p.Tools)
	}
	if len(p.Regulator.BlockedActions) != 2 {
		t.Errorf("unexpected blocked actions: %v", p.Regulator.BlockedActions)
	}
	if len(p.Regulator.DisallowedCommands) != 1 || p.Regulator.DisallowedCommands[0] != "rm" {
		t.Errorf("unexpected disallowed commands: %v", p.Regulator.DisallowedCommands)
	}
	if len(p.Regulator.DisallowedPaths) != 1 || p.Regulator.DisallowedPaths[0] != "*.key" {
		t.Errorf("unexpected disallowed paths: %v", p.Regulator.DisallowedPaths)
	}
	if p.Regulator.MaxRiskLevel != "low" {
		t.Errorf("expected low max risk level, got %s", p.Regulator.MaxRiskLevel)
	}
}

func TestProfile_ParseYAML_InlineLists(t *testing.T) {
	yamlPayload := `
name: inline-prof
tools: [find_files, read_window, read_outline]
regulator:
  blocked_actions: [write_file, exec_bash]
  disallowed_commands: [curl, wget]
  disallowed_paths: [.git, .env]
`
	p, err := config.ParseProfile([]byte(yamlPayload))
	if err != nil {
		t.Fatalf("failed to parse YAML with inline lists: %v", err)
	}
	if len(p.Tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(p.Tools))
	}
	if len(p.Regulator.BlockedActions) != 2 {
		t.Errorf("expected 2 blocked actions, got %d", len(p.Regulator.BlockedActions))
	}
	if len(p.Regulator.DisallowedCommands) != 2 {
		t.Errorf("expected 2 disallowed commands, got %d", len(p.Regulator.DisallowedCommands))
	}
	if len(p.Regulator.DisallowedPaths) != 2 {
		t.Errorf("expected 2 disallowed paths, got %d", len(p.Regulator.DisallowedPaths))
	}
}

func TestProfile_ParseErrors(t *testing.T) {
	if _, err := config.ParseProfile([]byte("")); err == nil {
		t.Errorf("expected error for empty profile")
	}
	if _, err := config.ParseProfile([]byte("description: missing name")); err == nil {
		t.Errorf("expected error for profile missing name")
	}
}

func TestProfile_Merge(t *testing.T) {
	base := &config.Profile{
		Name:                "base",
		Description:         "base desc",
		Mode:                "coding",
		Tools:               []string{"find_files", "read_window"},
		SystemPromptOverlay: "Base instructions.",
		Extends:             "parent",
		Regulator: config.RegulatorRules{
			BlockedActions:     []string{"write_file"},
			DisallowedCommands: []string{"rm"},
			MaxRiskLevel:       "medium",
		},
	}

	overlay := &config.Profile{
		Name:                "override",
		Description:         "override desc",
		Mode:                "general",
		Tools:               []string{"read_outline"},
		SystemPromptOverlay: "Additional instructions.",
		Extends:             "new-parent",
		Regulator: config.RegulatorRules{
			ReadOnly:           true,
			BlockedActions:     []string{"exec_bash"},
			DisallowedCommands: []string{"dd"},
			DisallowedPaths:    []string{".env"},
			MaxRiskLevel:       "low",
		},
	}

	merged := base.Merge(overlay)
	if merged.Name != "override" {
		t.Errorf("expected merged name 'override', got %s", merged.Name)
	}
	if merged.Mode != "general" {
		t.Errorf("expected merged mode 'general', got %s", merged.Mode)
	}
	if len(merged.Tools) != 1 || merged.Tools[0] != "read_outline" {
		t.Errorf("expected overlay tools, got %v", merged.Tools)
	}
	if merged.SystemPromptOverlay != "Base instructions.\n\nAdditional instructions." {
		t.Errorf("unexpected merged overlay: %q", merged.SystemPromptOverlay)
	}
	if merged.Extends != "new-parent" {
		t.Errorf("expected new-parent, got %s", merged.Extends)
	}
	if !merged.Regulator.ReadOnly {
		t.Errorf("expected read_only true")
	}
	if len(merged.Regulator.BlockedActions) != 2 {
		t.Errorf("expected 2 merged blocked actions, got %v", merged.Regulator.BlockedActions)
	}
	if len(merged.Regulator.DisallowedCommands) != 2 {
		t.Errorf("expected 2 merged disallowed commands, got %v", merged.Regulator.DisallowedCommands)
	}
	if len(merged.Regulator.DisallowedPaths) != 1 {
		t.Errorf("expected 1 disallowed path, got %v", merged.Regulator.DisallowedPaths)
	}
	if merged.Regulator.MaxRiskLevel != "low" {
		t.Errorf("expected max risk level 'low', got %s", merged.Regulator.MaxRiskLevel)
	}

	// Nil safety
	var nilP *config.Profile
	m1 := nilP.Merge(overlay)
	if m1.Name != "override" {
		t.Errorf("expected override name on nil base merge")
	}
	m2 := base.Merge(nil)
	if m2.Name != "base" {
		t.Errorf("expected base name on nil overlay merge")
	}
	m3 := nilP.Merge(nil)
	if m3.Name != "coding" {
		t.Errorf("expected default coding profile on dual nil merge")
	}
}

func TestProfile_DiscoveryAndLoading(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")
	xdgDir := filepath.Join(tmpDir, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	// 1. Fallback to empty/default -> standard coding profile
	def, err := config.LoadProfile("", workspaceDir)
	if err != nil {
		t.Fatalf("expected no error loading default profile, got: %v", err)
	}
	if def.Name != "coding" {
		t.Errorf("expected default name coding, got %s", def.Name)
	}

	// 2. Builtin fallback
	sec, err := config.LoadProfile("security-audit", workspaceDir)
	if err != nil {
		t.Fatalf("expected no error loading builtin security-audit, got: %v", err)
	}
	if sec.Name != "security-audit" || !sec.Regulator.ReadOnly {
		t.Errorf("unexpected security-audit profile: %+v", sec)
	}

	// 3. User XDG profile directory loading
	userProfileDir := filepath.Join(xdgDir, "lokol", "profiles")
	if err := os.MkdirAll(userProfileDir, 0755); err != nil {
		t.Fatalf("failed to create user profile dir: %v", err)
	}
	userJSON := `{"name": "user-preset", "mode": "moe"}`
	if err := os.WriteFile(filepath.Join(userProfileDir, "user-preset.json"), []byte(userJSON), 0644); err != nil {
		t.Fatalf("failed to write user profile: %v", err)
	}

	userProf, err := config.LoadProfile("user-preset", workspaceDir)
	if err != nil {
		t.Fatalf("expected user profile to load, got error: %v", err)
	}
	if userProf.Mode != "moe" {
		t.Errorf("expected mode moe, got %s", userProf.Mode)
	}

	// 4. Workspace profile takes precedence over user config
	wsProfileDir := filepath.Join(workspaceDir, ".lokol", "profiles")
	if err := os.MkdirAll(wsProfileDir, 0755); err != nil {
		t.Fatalf("failed to create ws profile dir: %v", err)
	}
	wsJSON := `{"name": "user-preset", "mode": "coding"}`
	if err := os.WriteFile(filepath.Join(wsProfileDir, "user-preset.json"), []byte(wsJSON), 0644); err != nil {
		t.Fatalf("failed to write ws profile: %v", err)
	}

	wsProf, err := config.LoadProfile("user-preset", workspaceDir)
	if err != nil {
		t.Fatalf("expected ws profile to load, got error: %v", err)
	}
	if wsProf.Mode != "coding" {
		t.Errorf("expected workspace profile precedence (coding), got %s", wsProf.Mode)
	}

	// 5. Direct path loading
	directPath := filepath.Join(tmpDir, "direct.json")
	if err := os.WriteFile(directPath, []byte(`{"name": "direct", "mode": "general"}`), 0644); err != nil {
		t.Fatalf("failed to write direct profile: %v", err)
	}
	directProf, err := config.LoadProfile(directPath, workspaceDir)
	if err != nil {
		t.Fatalf("failed to load direct path profile: %v", err)
	}
	if directProf.Name != "direct" {
		t.Errorf("expected direct profile name, got %s", directProf.Name)
	}

	// 6. Profile inheritance (extends)
	childJSON := `{"name": "child-audit", "extends": "security-audit", "description": "Customized audit"}`
	childPath := filepath.Join(wsProfileDir, "child-audit.json")
	if err := os.WriteFile(childPath, []byte(childJSON), 0644); err != nil {
		t.Fatalf("failed to write child profile: %v", err)
	}
	childProf, err := config.LoadProfile("child-audit", workspaceDir)
	if err != nil {
		t.Fatalf("failed to load extending profile: %v", err)
	}
	if childProf.Description != "Customized audit" {
		t.Errorf("expected customized description, got %s", childProf.Description)
	}
	if !childProf.Regulator.ReadOnly {
		t.Errorf("expected child profile to inherit read_only: true from security-audit")
	}

	// Test failing extends
	brokenExtendsJSON := `{"name": "broken-child", "extends": "non-existent-parent"}`
	brokenPath := filepath.Join(wsProfileDir, "broken-child.json")
	_ = os.WriteFile(brokenPath, []byte(brokenExtendsJSON), 0644)
	if _, err := config.LoadProfile("broken-child", workspaceDir); err == nil {
		t.Errorf("expected error resolving non-existent extended profile")
	}

	// 7. Non-existent profile
	if _, err := config.LoadProfile("does-not-exist", workspaceDir); err == nil {
		t.Errorf("expected error loading non-existent profile")
	}
}

func TestSampleProfiles_ParseCleanly(t *testing.T) {
	samples := []string{
		filepath.Join("..", "..", ".lokol", "profiles", "security-audit.json"),
		filepath.Join("..", "..", ".lokol", "profiles", "refactor.json"),
		filepath.Join("..", "..", ".lokol", "profiles", "doc-gen.json"),
	}

	for _, sample := range samples {
		data, err := os.ReadFile(sample)
		if err != nil {
			t.Fatalf("failed to read sample %s: %v", sample, err)
		}
		p, err := config.ParseProfile(data)
		if err != nil {
			t.Fatalf("failed to parse sample %s: %v", sample, err)
		}
		if p.Name == "" {
			t.Errorf("sample %s has empty name", sample)
		}
	}
}
