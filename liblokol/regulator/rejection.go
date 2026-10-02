// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RejectionEntry records an action candidate rejected by the operator.
type RejectionEntry struct {
	Action    ActionCandidate
	Target    string
	Timestamp time.Time
}

// UserRejectionStage persists operator rejection signals across turns and halts retrying them (lokol-kih.2).
type UserRejectionStage struct {
	mu         sync.RWMutex
	rejections map[string]RejectionEntry
}

// NewUserRejectionStage creates an initialized user rejection stage.
func NewUserRejectionStage() *UserRejectionStage {
	return &UserRejectionStage{
		rejections: make(map[string]RejectionEntry),
	}
}

func (s *UserRejectionStage) Name() string {
	return "user_rejection"
}

// RecordRejection adds an action candidate to the persistent rejection registry.
func (s *UserRejectionStage) RecordRejection(action ActionCandidate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	target := normalizeTargetPath(action.Path)
	if target == "" && action.Command != "" {
		target = normalizeTargetPath(ExtractTagContent(action.Command, "path"))
	}

	key := rejectionKey(action.Name, target, action.Command)
	s.rejections[key] = RejectionEntry{
		Action:    action,
		Target:    target,
		Timestamp: time.Now(),
	}
}

// ClearRejections removes all recorded rejections.
func (s *UserRejectionStage) ClearRejections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejections = make(map[string]RejectionEntry)
}

// ReconcilePrompt unblocks previously rejected targets if the operator explicitly mentions them in a new prompt.
func (s *UserRejectionStage) ReconcilePrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lowerPrompt := strings.ToLower(prompt)
	for key, entry := range s.rejections {
		if entry.Target != "" {
			base := strings.ToLower(filepath.Base(entry.Target))
			full := strings.ToLower(entry.Target)
			if strings.Contains(lowerPrompt, base) || strings.Contains(lowerPrompt, full) {
				delete(s.rejections, key)
				continue
			}
		}
		if entry.Action.Name == "exec_bash" && entry.Action.Command != "" {
			firstWord := strings.Fields(strings.ToLower(entry.Action.Command))
			if len(firstWord) > 0 && strings.Contains(lowerPrompt, firstWord[0]) {
				delete(s.rejections, key)
			}
		}
	}
}

// HasRejection reports whether an action candidate matches an active rejection.
func (s *UserRejectionStage) HasRejection(action ActionCandidate) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	target := normalizeTargetPath(action.Path)
	if target == "" && action.Command != "" {
		target = normalizeTargetPath(ExtractTagContent(action.Command, "path"))
	}

	for _, entry := range s.rejections {
		if matchesRejection(action, target, entry) {
			return true
		}
	}
	return false
}

// Evaluate intercepts candidate actions that match a prior user rejection.
func (s *UserRejectionStage) Evaluate(ctx context.Context, action ActionCandidate, workDir string) PermissionResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	target := normalizeTargetPath(action.Path)
	if target == "" && action.Command != "" {
		target = normalizeTargetPath(ExtractTagContent(action.Command, "path"))
	}

	for _, entry := range s.rejections {
		if matchesRejection(action, target, entry) {
			displayTarget := target
			if displayTarget == "" {
				displayTarget = action.Name
			}
			return PermissionResult{
				Status:      StatusBlocked,
				Reason:      fmt.Sprintf("Action %s on %q was explicitly rejected by the operator in a prior turn", action.Name, displayTarget),
				RiskLevel:   RiskLevelHigh,
				Target:      displayTarget,
				Stage:       s.Name(),
				Remediation: fmt.Sprintf("The operator previously rejected %s on %q. Shift strategy to an alternate file, or ask the operator for clarification if this file is truly needed.", action.Name, displayTarget),
			}
		}
	}

	return PermissionResult{
		Status:    StatusAllowed,
		RiskLevel: RiskLevelNone,
		Stage:     s.Name(),
	}
}

func matchesRejection(action ActionCandidate, target string, entry RejectionEntry) bool {
	// 1. If target file paths match, disallow access to that rejected path
	if target != "" && entry.Target != "" {
		if target == entry.Target || filepath.Base(target) == filepath.Base(entry.Target) {
			return true
		}
	}

	// 2. If identical command and tool name
	if action.Name == entry.Action.Name && strings.TrimSpace(action.Command) == strings.TrimSpace(entry.Action.Command) {
		return true
	}

	return false
}

func normalizeTargetPath(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}

func rejectionKey(name, target, cmd string) string {
	return fmt.Sprintf("%s|%s|%s", strings.ToLower(name), strings.ToLower(target), strings.TrimSpace(cmd))
}
