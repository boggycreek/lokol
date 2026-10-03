// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultAgentName is the default persona name for the agent.
	DefaultAgentName = "lokol"
	// DefaultOperatorName is the default name for the human operator.
	DefaultOperatorName = "User"
)

// Config represents persistent user preferences for lokol adhering to ADR-0006.
type Config struct {
	AgentName    string `json:"agent_name,omitempty"`
	OperatorName string `json:"operator_name,omitempty"`
}

// ConfigDir returns the directory where lokol configuration is stored
// conforming to ADR-0006 ($XDG_CONFIG_HOME/lokol or ~/.config/lokol).
func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "lokol")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "lokol")
	}
	return filepath.Join(home, ".config", "lokol")
}

// ConfigPath returns the path to the persistent config.json file.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

// Load loads the configuration from the standard config file location,
// returning defaults if the file does not exist.
func Load() (*Config, error) {
	return LoadFrom(ConfigPath())
}

// LoadFrom loads configuration from an explicit file path.
func LoadFrom(path string) (*Config, error) {
	cfg := &Config{
		AgentName:    DefaultAgentName,
		OperatorName: DefaultOperatorName,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config from %s: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config from %s: %w", path, err)
	}

	if strings.TrimSpace(cfg.AgentName) == "" {
		cfg.AgentName = DefaultAgentName
	}
	if strings.TrimSpace(cfg.OperatorName) == "" {
		cfg.OperatorName = DefaultOperatorName
	}

	return cfg, nil
}

// Save writes the configuration to the standard config file location.
func Save(cfg *Config) error {
	return SaveTo(ConfigPath(), cfg)
}

// SaveTo writes the configuration to an explicit file path.
func SaveTo(path string, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("cannot save nil config")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write config to %s: %w", path, err)
	}
	return nil
}

// GetAgentName returns the configured agent persona name, falling back to DefaultAgentName.
func (c *Config) GetAgentName() string {
	if c != nil && strings.TrimSpace(c.AgentName) != "" {
		return strings.TrimSpace(c.AgentName)
	}
	return DefaultAgentName
}

// GetOperatorName returns the configured operator name, falling back to DefaultOperatorName.
func (c *Config) GetOperatorName() string {
	if c != nil && strings.TrimSpace(c.OperatorName) != "" {
		return strings.TrimSpace(c.OperatorName)
	}
	return DefaultOperatorName
}

// DataDir returns the root directory for persistent lokol data ($XDG_DATA_HOME/lokol or ~/.local/share/lokol) per ADR-0006.
func DataDir() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "lokol")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".local", "share", "lokol")
	}
	return filepath.Join(home, ".local", "share", "lokol")
}

// MemoryDir returns the root directory for persistent partitioned memories per ADR-0010.
func MemoryDir() string {
	return filepath.Join(DataDir(), "memory")
}

