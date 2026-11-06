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

	// Test standard Load and Save using XDG_CONFIG_HOME
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	if err := config.Save(initial); err != nil {
		t.Fatalf("failed to save default config: %v", err)
	}
	stdLoaded, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load default config: %v", err)
	}
	if stdLoaded.GetAgentName() != "Aria" {
		t.Errorf("expected Aria, got %s", stdLoaded.GetAgentName())
	}

	// Test nil receiver or empty fields on methods
	var nilCfg *config.Config
	if nilCfg.GetAgentName() != config.DefaultAgentName {
		t.Errorf("expected default agent name on nil config")
	}
	if nilCfg.GetOperatorName() != config.DefaultOperatorName {
		t.Errorf("expected default operator name on nil config")
	}

	// Test SaveTo nil error
	if err := config.SaveTo(cfgPath, nil); err == nil {
		t.Errorf("expected error saving nil config")
	}

	// Test DataDir & MemoryDir
	t.Setenv("XDG_DATA_HOME", tmpDir)
	if config.DataDir() != filepath.Join(tmpDir, "lokol") {
		t.Errorf("unexpected DataDir: %s", config.DataDir())
	}
	if config.MemoryDir() != filepath.Join(tmpDir, "lokol", "memory") {
		t.Errorf("unexpected MemoryDir: %s", config.MemoryDir())
	}

	// Test DataDir fallback when XDG_DATA_HOME is empty
	t.Setenv("XDG_DATA_HOME", "")
	if config.DataDir() == "" {
		t.Errorf("expected non-empty DataDir fallback")
	}

	// Test invalid JSON on LoadFrom
	corruptPath := filepath.Join(tmpDir, "corrupt.json")
	_ = os.WriteFile(corruptPath, []byte("invalid json"), 0644)
	if _, err := config.LoadFrom(corruptPath); err == nil {
		t.Errorf("expected error loading corrupt config")
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

	// When XDG_CONFIG_HOME is empty, ConfigDir uses user home dir
	t.Setenv("XDG_CONFIG_HOME", "")
	if config.ConfigDir() == "" {
		t.Errorf("expected fallback ConfigDir when XDG_CONFIG_HOME empty")
	}
}
