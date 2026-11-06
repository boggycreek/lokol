// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// canonicalizePath normalizes a file path relative to the session working directory.
func (s *Session) canonicalizePath(p string) string {
	cleaned := filepath.Clean(strings.TrimSpace(p))
	if cleaned == "" || cleaned == "." {
		return ""
	}
	if filepath.IsAbs(cleaned) {
		workDir := s.WorkDir
		if workDir == "" {
			return cleaned
		}
		rel, err := filepath.Rel(workDir, cleaned)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Clean(rel)
		}
		return cleaned
	}
	return strings.TrimPrefix(cleaned, "./")
}

// RecordInspectedFile registers a file path that has been inspected (e.g. via read_window or read_outline).
func (s *Session) RecordInspectedFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordInspectedFileUnsafe(path)
}

func (s *Session) recordInspectedFileUnsafe(path string) {
	norm := s.canonicalizePath(path)
	if norm == "" {
		return
	}
	if s.inspectedFiles == nil {
		s.inspectedFiles = make(map[string]struct{})
	}
	s.inspectedFiles[norm] = struct{}{}
}

// RecordModifiedFile registers a file path that has been modified (e.g. via write_file or replace_file).
func (s *Session) RecordModifiedFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordModifiedFileUnsafe(path)
}

func (s *Session) recordModifiedFileUnsafe(path string) {
	norm := s.canonicalizePath(path)
	if norm == "" {
		return
	}
	if s.modifiedFiles == nil {
		s.modifiedFiles = make(map[string]struct{})
	}
	s.modifiedFiles[norm] = struct{}{}
}

// GetInspectedFiles returns a sorted slice of all unique file paths inspected in this session.
func (s *Session) GetInspectedFiles() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.inspectedFiles) == 0 {
		return nil
	}
	files := make([]string, 0, len(s.inspectedFiles))
	for f := range s.inspectedFiles {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// GetModifiedFiles returns a sorted slice of all unique file paths modified in this session.
func (s *Session) GetModifiedFiles() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.modifiedFiles) == 0 {
		return nil
	}
	files := make([]string, 0, len(s.modifiedFiles))
	for f := range s.modifiedFiles {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// FormatWorkspaceFrontierSummary formats cumulative inspected and modified files into
// a clean, structured context ledger summary. If no files have been accessed or modified,
// an empty string is returned.
func (s *Session) FormatWorkspaceFrontierSummary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.formatWorkspaceFrontierSummaryUnsafe()
}

func (s *Session) formatWorkspaceFrontierSummaryUnsafe() string {
	inspected := make([]string, 0, len(s.inspectedFiles))
	for f := range s.inspectedFiles {
		inspected = append(inspected, f)
	}
	sort.Strings(inspected)

	modified := make([]string, 0, len(s.modifiedFiles))
	for f := range s.modifiedFiles {
		modified = append(modified, f)
	}
	sort.Strings(modified)

	if len(inspected) == 0 && len(modified) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<workspace_frontier>\n")
	if len(inspected) > 0 {
		b.WriteString(fmt.Sprintf("  <inspected_files>%s</inspected_files>\n", strings.Join(inspected, ", ")))
	}
	if len(modified) > 0 {
		b.WriteString(fmt.Sprintf("  <modified_files>%s</modified_files>\n", strings.Join(modified, ", ")))
	}
	b.WriteString("</workspace_frontier>")
	return b.String()
}

// BuildCompactionLedger merges an optional spec-level ledger with the cumulative
// workspace frontier ledger into a comprehensive context ledger for macro-compaction.
func (s *Session) BuildCompactionLedger(specLedger string) string {
	frontier := s.FormatWorkspaceFrontierSummary()

	var parts []string
	if strings.TrimSpace(specLedger) != "" {
		parts = append(parts, fmt.Sprintf("SPECIFICATION & TEST ASSERTION LEDGER:\n%s", strings.TrimSpace(specLedger)))
	}
	if strings.TrimSpace(frontier) != "" {
		parts = append(parts, fmt.Sprintf("ACCESSED & MODIFIED FILES LEDGER:\n%s", strings.TrimSpace(frontier)))
	}

	if len(parts) == 0 {
		return "Intermediate turns compacted to preserve context budget."
	}

	parts = append(parts, "Intermediate turns compacted to preserve context budget.")
	return strings.Join(parts, "\n\n")
}
