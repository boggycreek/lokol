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

func TestConfig_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "does-not-exist.json")

	cfg, err := config.LoadFrom(nonExistentPath)
	if err != nil {
		t.Fatalf("expected no error loading non-existent config, got: %v", err)
	}

	if cfg.GetAgentName() != config.DefaultAgentName {
		t.Errorf("expected default agent name %q, got %q", config.DefaultAgentName, cfg.GetAgentName())
	}
	if cfg.GetOperatorName() != config.DefaultOperatorName {
		t.Errorf("expected default operator name %q, got %q", config.DefaultOperatorName, cfg.GetOperatorName())
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "sub", "config.json")

	initial := &config.Config{
		AgentName:    "Aria",
		OperatorName: "Alice",
	}

	if err := config.SaveTo(cfgPath, initial); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loaded.GetAgentName() != "Aria" {
		t.Errorf("expected agent name 'Aria', got %q", loaded.GetAgentName())
	}
	if loaded.GetOperatorName() != "Alice" {
		t.Errorf("expected operator name 'Alice', got %q", loaded.GetOperatorName())
	}
}

func TestConfig_EmptyFieldsFallBackToDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "empty.json")

	if err := os.WriteFile(cfgPath, []byte(`{"agent_name": "", "operator_name": "  "}`), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.GetAgentName() != config.DefaultAgentName {
		t.Errorf("expected fallback to default agent name, got %q", loaded.GetAgentName())
	}
	if loaded.GetOperatorName() != config.DefaultOperatorName {
		t.Errorf("expected fallback to default operator name, got %q", loaded.GetOperatorName())
	}
}

func TestConfig_XDGEnvironmentOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	expectedDir := filepath.Join(tmpDir, "lokol")
	if config.ConfigDir() != expectedDir {
		t.Errorf("expected ConfigDir %q, got %q", expectedDir, config.ConfigDir())
	}

	expectedPath := filepath.Join(expectedDir, "config.json")
	if config.ConfigPath() != expectedPath {
		t.Errorf("expected ConfigPath %q, got %q", expectedPath, config.ConfigPath())
	}
}
