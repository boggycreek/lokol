// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Spec represents a structured execution specification for lokol (ADR-aligned).
type Spec struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	TargetFiles        []string `json:"target_files"`
	PreflightCmd       string   `json:"preflight_cmd,omitempty"`
	VerifyCmd          string   `json:"verify_cmd,omitempty"`
	MaxTurns           int      `json:"max_turns,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	RawContent         string   `json:"raw_content,omitempty"`
	Path               string   `json:"path,omitempty"`
}

// Load reads and parses a SPEC.md file from disk.
func Load(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file %s: %w", path, err)
	}
	spec, err := Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse spec file %s: %w", path, err)
	}
	spec.Path = path
	return spec, nil
}

// Parse extracts a Spec from markdown content with optional YAML-style front matter
// or markdown section headers.
func Parse(content string) (*Spec, error) {
	spec := &Spec{
		RawContent:  content,
		TargetFiles: make([]string, 0),
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return spec, nil
	}

	bodyStartIdx := 0

	// Check for front matter delimited by ---
	if strings.TrimSpace(lines[0]) == "---" {
		fmEndIdx := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				fmEndIdx = i
				break
			}
		}

		if fmEndIdx > 0 {
			parseFrontMatter(lines[1:fmEndIdx], spec)
			bodyStartIdx = fmEndIdx + 1
		}
	}

	// Parse markdown body for sections or fallback fields
	parseMarkdownBody(lines[bodyStartIdx:], spec)

	if spec.Title == "" {
		spec.Title = "Autonomous Task Specification"
	}

	return spec, nil
}

// parseFrontMatter parses simple YAML key-value lines without external dependencies.
func parseFrontMatter(lines []string, spec *Spec) {
	inTargetFiles := false
	inCriteria := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(trimmed[2:])
			item = strings.Trim(item, `"'`)
			if inTargetFiles {
				spec.TargetFiles = append(spec.TargetFiles, item)
			} else if inCriteria {
				spec.AcceptanceCriteria = append(spec.AcceptanceCriteria, item)
			}
			continue
		}

		// Any new top-level key ends array parsing
		inTargetFiles = false
		inCriteria = false

		colonIdx := strings.Index(trimmed, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(trimmed[:colonIdx]))
		val := strings.TrimSpace(trimmed[colonIdx+1:])
		val = strings.Trim(val, `"'`)

		switch key {
		case "title", "name":
			if spec.Title == "" {
				spec.Title = val
			}
		case "description", "desc", "objective":
			if spec.Description == "" {
				spec.Description = val
			}
		case "preflight", "preflight_cmd", "preflight_check":
			spec.PreflightCmd = val
		case "verify", "verify_cmd", "verify_command", "test_cmd":
			spec.VerifyCmd = val
		case "max_turns", "turns":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				spec.MaxTurns = n
			}
		case "target_files", "targets":
			if val != "" {
				// inline comma-separated or bracketed list: [foo.go, bar.go]
				val = strings.Trim(val, "[]")
				parts := strings.Split(val, ",")
				for _, p := range parts {
					clean := strings.TrimSpace(strings.Trim(p, `"'`))
					if clean != "" {
						spec.TargetFiles = append(spec.TargetFiles, clean)
					}
				}
			} else {
				inTargetFiles = true
			}
		case "acceptance_criteria", "criteria":
			if val != "" {
				spec.AcceptanceCriteria = append(spec.AcceptanceCriteria, val)
			} else {
				inCriteria = true
			}
		}
	}
}

// parseMarkdownBody extracts title, description, targets, verification, and criteria from markdown.
func parseMarkdownBody(lines []string, spec *Spec) {
	currentSection := ""
	var descBuilder strings.Builder

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "# ") {
			if spec.Title == "" || spec.Title == "Autonomous Task Specification" {
				spec.Title = strings.TrimSpace(line[2:])
			}
			currentSection = "header"
			continue
		}

		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			heading := strings.ToLower(strings.TrimSpace(strings.TrimLeft(line, "#")))
			switch {
			case strings.Contains(heading, "target"):
				currentSection = "targets"
			case strings.Contains(heading, "preflight"):
				currentSection = "preflight"
			case strings.Contains(heading, "verify") || strings.Contains(heading, "verification") || strings.Contains(heading, "test"):
				currentSection = "verification"
			case strings.Contains(heading, "criteria") || strings.Contains(heading, "acceptance") || strings.Contains(heading, "requirement"):
				currentSection = "criteria"
			case strings.Contains(heading, "desc") || strings.Contains(heading, "objective") || strings.Contains(heading, "overview"):
				currentSection = "description"
			default:
				currentSection = ""
			}
			continue
		}

		switch currentSection {
		case "targets":
			if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
				file := strings.TrimSpace(line[2:])
				file = strings.Trim(file, "`\"'")
				if file != "" {
					spec.TargetFiles = append(spec.TargetFiles, file)
				}
			}
		case "preflight":
			if spec.PreflightCmd == "" && !strings.HasPrefix(line, "```") {
				cmd := strings.Trim(line, "`")
				spec.PreflightCmd = cmd
			}
		case "verification":
			if spec.VerifyCmd == "" && !strings.HasPrefix(line, "```") {
				cmd := strings.Trim(line, "`")
				spec.VerifyCmd = cmd
			}
		case "criteria":
			if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
				item := strings.TrimSpace(line[2:])
				item = strings.TrimPrefix(item, "[ ] ")
				item = strings.TrimPrefix(item, "[x] ")
				spec.AcceptanceCriteria = append(spec.AcceptanceCriteria, item)
			}
		case "description":
			descBuilder.WriteString(rawLine + "\n")
		default:
			if spec.Description == "" && !strings.HasPrefix(line, "```") && !strings.HasPrefix(line, "#") {
				descBuilder.WriteString(rawLine + "\n")
			}
		}
	}

	if spec.Description == "" && descBuilder.Len() > 0 {
		spec.Description = strings.TrimSpace(descBuilder.String())
	}

	// Clean and deduplicate target files
	cleanedTargets := make([]string, 0, len(spec.TargetFiles))
	seen := make(map[string]bool)
	for _, t := range spec.TargetFiles {
		clean := filepath.Clean(t)
		if !seen[clean] && clean != "." && clean != "" {
			seen[clean] = true
			cleanedTargets = append(cleanedTargets, clean)
		}
	}
	spec.TargetFiles = cleanedTargets
}

// Prompt formats a prompt suitable for consumption by an autonomous agent loop.
func (s *Spec) Prompt() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[SPECIFICATION: %s]\n\n", s.Title))

	if s.Description != "" {
		b.WriteString(fmt.Sprintf("Objective:\n%s\n\n", s.Description))
	}

	if len(s.TargetFiles) > 0 {
		b.WriteString("Constrained Target Files (modifications strictly limited to these):\n")
		for _, f := range s.TargetFiles {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
		b.WriteString("\n")
	}

	if s.VerifyCmd != "" {
		b.WriteString(fmt.Sprintf("Automated Verification Gate:\n`%s`\n(Note: task_finish is locked until this command exits with code 0)\n\n", s.VerifyCmd))
	}

	if len(s.AcceptanceCriteria) > 0 {
		b.WriteString("Acceptance Criteria:\n")
		for _, c := range s.AcceptanceCriteria {
			b.WriteString(fmt.Sprintf("- [ ] %s\n", c))
		}
		b.WriteString("\n")
	}

	b.WriteString("Instructions:\n")
	b.WriteString("1. Inspect the codebase using read_outline or read_window.\n")
	if len(s.TargetFiles) > 0 {
		b.WriteString("2. Apply edits ONLY to the specified target files using replace_file or write_file.\n")
	} else {
		b.WriteString("2. Apply edits using replace_file or write_file.\n")
	}
	if s.VerifyCmd != "" {
		b.WriteString(fmt.Sprintf("3. Verify your implementation using exec_bash with: %s\n", s.VerifyCmd))
		b.WriteString("4. Only call task_finish once all tests and verifications exit cleanly with code 0.\n")
	} else {
		b.WriteString("3. Call task_finish when your implementation is complete and verified.\n")
	}

	return strings.TrimSpace(b.String())
}
