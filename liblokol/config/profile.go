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
	"strconv"
	"strings"
)

// RegulatorRules specifies safety and execution constraints enforced by the regulator for a profile.
type RegulatorRules struct {
	ReadOnly           bool     `json:"read_only,omitempty"`
	BlockedActions     []string `json:"blocked_actions,omitempty"`
	DisallowedCommands []string `json:"disallowed_commands,omitempty"`
	DisallowedPaths    []string `json:"disallowed_paths,omitempty"`
	MaxRiskLevel       string   `json:"max_risk_level,omitempty"`
}

// Profile represents a declarative agent workload preset.
type Profile struct {
	Name                string         `json:"name"`
	Description         string         `json:"description,omitempty"`
	Mode                string         `json:"mode,omitempty"`
	Tools               []string       `json:"tools,omitempty"`
	SystemPromptOverlay string         `json:"system_prompt_overlay,omitempty"`
	Regulator           RegulatorRules `json:"regulator,omitempty"`
	Extends             string         `json:"extends,omitempty"`
}

// DefaultCodingProfile returns the default built-in coding profile.
func DefaultCodingProfile() *Profile {
	return &Profile{
		Name:        "coding",
		Description: "Standard autonomous software engineering and coding profile",
		Mode:        "coding",
		Tools:       nil, // All tools enabled
	}
}

// BuiltinProfiles returns pre-defined workload profiles for common workflows.
func BuiltinProfiles() map[string]*Profile {
	return map[string]*Profile{
		"coding": DefaultCodingProfile(),
		"security-audit": {
			Name:        "security-audit",
			Description: "Read-only security assessment and static vulnerability audit",
			Mode:        "general",
			Tools: []string{
				"find_files",
				"read_window",
				"read_outline",
				"search_code",
				"git_diff_summary",
				"task_finish",
				"tool_help",
			},
			SystemPromptOverlay: "You are acting as a Security Auditor. Focus strictly on identifying vulnerabilities, unsafe input flows, secret exposure, and privilege escalation risks. Do not modify files.",
			Regulator: RegulatorRules{
				ReadOnly:       true,
				BlockedActions: []string{"write_file", "replace_file", "exec_bash"},
				MaxRiskLevel:   "low",
			},
		},
		"refactor": {
			Name:        "refactor",
			Description: "Targeted code refactoring with AST outline inspection and test verification",
			Mode:        "coding",
			Tools: []string{
				"find_files",
				"read_window",
				"read_outline",
				"search_code",
				"replace_file",
				"run_test",
				"git_diff_summary",
				"task_finish",
				"tool_help",
			},
			SystemPromptOverlay: "You are in Refactoring mode. Improve code structure, readability, and performance without altering observable behavior. Always run test verifications to confirm zero regression.",
			Regulator: RegulatorRules{
				BlockedActions: []string{"exec_bash"},
			},
		},
		"doc-gen": {
			Name:        "doc-gen",
			Description: "Documentation and architectural overview generation",
			Mode:        "general",
			Tools: []string{
				"find_files",
				"read_window",
				"read_outline",
				"search_code",
				"write_file",
				"replace_file",
				"task_finish",
				"tool_help",
			},
			SystemPromptOverlay: "You are in Documentation Generation mode. Generate comprehensive, precise, and well-structured markdown documentation based on actual source inspection.",
			Regulator: RegulatorRules{
				BlockedActions: []string{"exec_bash"},
				DisallowedPaths: []string{
					"*.go", "*.c", "*.cpp", "*.rs", "*.py", "*.js", "*.ts",
				},
			},
		},
	}
}

// Merge creates a new Profile by overlaying non-zero fields of overlay onto p.
func (p *Profile) Merge(overlay *Profile) *Profile {
	if p == nil && overlay == nil {
		return DefaultCodingProfile()
	}
	if p == nil {
		copyProf := *overlay
		return &copyProf
	}
	if overlay == nil {
		copyProf := *p
		return &copyProf
	}

	result := *p

	if overlay.Name != "" {
		result.Name = overlay.Name
	}
	if overlay.Description != "" {
		result.Description = overlay.Description
	}
	if overlay.Mode != "" {
		result.Mode = overlay.Mode
	}
	if len(overlay.Tools) > 0 {
		toolsCopy := make([]string, len(overlay.Tools))
		copy(toolsCopy, overlay.Tools)
		result.Tools = toolsCopy
	}
	if overlay.SystemPromptOverlay != "" {
		if result.SystemPromptOverlay != "" {
			result.SystemPromptOverlay = result.SystemPromptOverlay + "\n\n" + overlay.SystemPromptOverlay
		} else {
			result.SystemPromptOverlay = overlay.SystemPromptOverlay
		}
	}
	if overlay.Extends != "" {
		result.Extends = overlay.Extends
	}

	// Merge regulator rules
	result.Regulator.ReadOnly = p.Regulator.ReadOnly || overlay.Regulator.ReadOnly
	result.Regulator.BlockedActions = mergeUnique(p.Regulator.BlockedActions, overlay.Regulator.BlockedActions)
	result.Regulator.DisallowedCommands = mergeUnique(p.Regulator.DisallowedCommands, overlay.Regulator.DisallowedCommands)
	result.Regulator.DisallowedPaths = mergeUnique(p.Regulator.DisallowedPaths, overlay.Regulator.DisallowedPaths)

	if overlay.Regulator.MaxRiskLevel != "" {
		result.Regulator.MaxRiskLevel = overlay.Regulator.MaxRiskLevel
	}

	return &result
}

func mergeUnique(a, b []string) []string {
	seen := make(map[string]bool)
	var res []string
	for _, item := range a {
		item = strings.TrimSpace(item)
		if item != "" && !seen[item] {
			seen[item] = true
			res = append(res, item)
		}
	}
	for _, item := range b {
		item = strings.TrimSpace(item)
		if item != "" && !seen[item] {
			seen[item] = true
			res = append(res, item)
		}
	}
	return res
}

// ProfileDir returns the user-level profile directory ($XDG_CONFIG_HOME/lokol/profiles or ~/.config/lokol/profiles).
func ProfileDir() string {
	return filepath.Join(ConfigDir(), "profiles")
}

// ParseProfile parses a Profile from JSON or YAML data.
func ParseProfile(data []byte) (*Profile, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, fmt.Errorf("empty profile content")
	}

	// First attempt standard JSON parsing
	var prof Profile
	if err := json.Unmarshal(data, &prof); err == nil && prof.Name != "" {
		return &prof, nil
	}

	// Fallback to lightweight pure Go YAML parser
	parsed, err := parseProfileYAML(trimmed)
	if err != nil {
		return nil, fmt.Errorf("failed to parse profile data: %w", err)
	}
	return parsed, nil
}

// parseProfileYAML parses a simple YAML profile format without external dependencies.
func parseProfileYAML(content string) (*Profile, error) {
	lines := strings.Split(content, "\n")
	prof := &Profile{}

	var currentSection string
	var inOverlayBlock bool
	var overlayBuilder strings.Builder

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r\n")
		trimmed := strings.TrimSpace(line)

		if inOverlayBlock {
			// If indented line, append to block scalar
			if strings.HasPrefix(rawLine, "  ") || strings.HasPrefix(rawLine, "\t") || trimmed == "" {
				indent := "  "
				if strings.HasPrefix(rawLine, "  ") {
					overlayBuilder.WriteString(strings.TrimPrefix(rawLine, indent) + "\n")
				} else if strings.HasPrefix(rawLine, "\t") {
					overlayBuilder.WriteString(strings.TrimPrefix(rawLine, "\t") + "\n")
				} else {
					overlayBuilder.WriteString("\n")
				}
				continue
			}
			inOverlayBlock = false
			prof.SystemPromptOverlay = strings.TrimSpace(overlayBuilder.String())
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// List items
		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(trimmed[2:])
			item = strings.Trim(item, `"'`)
			switch currentSection {
			case "tools":
				prof.Tools = append(prof.Tools, item)
			case "regulator.blocked_actions":
				prof.Regulator.BlockedActions = append(prof.Regulator.BlockedActions, item)
			case "regulator.disallowed_commands":
				prof.Regulator.DisallowedCommands = append(prof.Regulator.DisallowedCommands, item)
			case "regulator.disallowed_paths":
				prof.Regulator.DisallowedPaths = append(prof.Regulator.DisallowedPaths, item)
			}
			continue
		}

		colonIdx := strings.Index(trimmed, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(trimmed[:colonIdx]))
		val := strings.TrimSpace(trimmed[colonIdx+1:])
		val = strings.Trim(val, `"'`)

		// Check for sub-section nesting under regulator
		if strings.HasPrefix(rawLine, "  ") || strings.HasPrefix(rawLine, "\t") {
			if strings.HasPrefix(currentSection, "regulator") {
				switch key {
				case "read_only":
					prof.Regulator.ReadOnly, _ = strconv.ParseBool(val)
					currentSection = "regulator"
				case "blocked_actions":
					currentSection = "regulator.blocked_actions"
					if val != "" {
						prof.Regulator.BlockedActions = parseInlineList(val)
					}
				case "disallowed_commands":
					currentSection = "regulator.disallowed_commands"
					if val != "" {
						prof.Regulator.DisallowedCommands = parseInlineList(val)
					}
				case "disallowed_paths":
					currentSection = "regulator.disallowed_paths"
					if val != "" {
						prof.Regulator.DisallowedPaths = parseInlineList(val)
					}
				case "max_risk_level":
					prof.Regulator.MaxRiskLevel = val
					currentSection = "regulator"
				}
				continue
			}
		}

		// Top-level keys
		switch key {
		case "name":
			prof.Name = val
			currentSection = ""
		case "description", "desc":
			prof.Description = val
			currentSection = ""
		case "mode":
			prof.Mode = val
			currentSection = ""
		case "extends":
			prof.Extends = val
			currentSection = ""
		case "tools":
			currentSection = "tools"
			if val != "" {
				prof.Tools = parseInlineList(val)
			}
		case "system_prompt_overlay", "overlay", "prompt_overlay":
			if val == "|" || val == ">" {
				inOverlayBlock = true
				overlayBuilder.Reset()
				currentSection = "overlay"
			} else {
				prof.SystemPromptOverlay = val
				currentSection = ""
			}
		case "regulator":
			currentSection = "regulator"
		default:
			currentSection = ""
		}
	}

	if inOverlayBlock {
		prof.SystemPromptOverlay = strings.TrimSpace(overlayBuilder.String())
	}

	if prof.Name == "" {
		return nil, fmt.Errorf("profile name is required")
	}

	return prof, nil
}

func parseInlineList(val string) []string {
	val = strings.Trim(val, "[]")
	parts := strings.Split(val, ",")
	var result []string
	for _, p := range parts {
		clean := strings.Trim(strings.TrimSpace(p), `"'`)
		if clean != "" {
			result = append(result, clean)
		}
	}
	return result
}

// LoadProfile loads a Profile by name, searching workspace, user config, and built-ins.
// If name is empty, it returns the standard DefaultCodingProfile.
func LoadProfile(name string, workDir ...string) (*Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "default" || name == "coding" {
		// Check if an explicit override exists in workspace or user config before falling back to builtin
		if p, err := findAndLoadProfileFile(name, workDir...); err == nil && p != nil {
			return resolveProfileInheritance(p, workDir...)
		}
		return DefaultCodingProfile(), nil
	}

	// 1. Direct file path check
	if strings.Contains(name, string(filepath.Separator)) || strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
		data, err := os.ReadFile(name)
		if err == nil {
			p, err := ParseProfile(data)
			if err != nil {
				return nil, fmt.Errorf("failed to parse profile file %s: %w", name, err)
			}
			return resolveProfileInheritance(p, workDir...)
		}
	}

	// 2. Discover in workspace and user config directories
	p, err := findAndLoadProfileFile(name, workDir...)
	if err == nil && p != nil {
		return resolveProfileInheritance(p, workDir...)
	}

	// 3. Fallback to built-in presets
	if builtin, ok := BuiltinProfiles()[name]; ok {
		return resolveProfileInheritance(builtin, workDir...)
	}

	return nil, fmt.Errorf("profile %q not found in workspace (.lokol/profiles/), user config (%s), or built-ins", name, ProfileDir())
}

func findAndLoadProfileFile(name string, workDir ...string) (*Profile, error) {
	wd := "."
	if len(workDir) > 0 && workDir[0] != "" {
		wd = workDir[0]
	}

	extensions := []string{".json", ".yaml", ".yml"}
	searchDirs := []string{
		filepath.Join(wd, ".lokol", "profiles"),
		ProfileDir(),
	}

	for _, dir := range searchDirs {
		for _, ext := range extensions {
			path := filepath.Join(dir, name+ext)
			data, err := os.ReadFile(path)
			if err == nil {
				p, err := ParseProfile(data)
				if err != nil {
					return nil, fmt.Errorf("failed to parse %s: %w", path, err)
				}
				return p, nil
			}
		}
	}

	return nil, os.ErrNotExist
}

func resolveProfileInheritance(p *Profile, workDir ...string) (*Profile, error) {
	if p == nil || p.Extends == "" {
		return p, nil
	}

	parent, err := LoadProfile(p.Extends, workDir...)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve extended profile %q: %w", p.Extends, err)
	}

	return parent.Merge(p), nil
}
