// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package spec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpec_Parse_FrontMatter(t *testing.T) {
	content := `---
title: "Fix Buffer Overflow in Parser"
description: "Ensure action buffer does not overflow on large payloads"
target_files:
  - "lib/parser.go"
  - "lib/parser_test.go"
preflight: "go version"
verify: "go test -v ./lib/..."
max_turns: 12
acceptance_criteria:
  - Bounded ring buffer implemented
  - Tests passing
---

# Details
Please inspect lib/parser.go and resolve the issue.
`

	s, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if s.Title != "Fix Buffer Overflow in Parser" {
		t.Errorf("expected Title %q, got %q", "Fix Buffer Overflow in Parser", s.Title)
	}
	if s.Description != "Ensure action buffer does not overflow on large payloads" {
		t.Errorf("expected Description %q, got %q", "Ensure action buffer does not overflow on large payloads", s.Description)
	}
	if len(s.TargetFiles) != 2 || s.TargetFiles[0] != "lib/parser.go" || s.TargetFiles[1] != "lib/parser_test.go" {
		t.Errorf("unexpected target files: %v", s.TargetFiles)
	}
	if s.PreflightCmd != "go version" {
		t.Errorf("expected preflight %q, got %q", "go version", s.PreflightCmd)
	}
	if s.VerifyCmd != "go test -v ./lib/..." {
		t.Errorf("expected verify %q, got %q", "go test -v ./lib/...", s.VerifyCmd)
	}
	if s.MaxTurns != 12 {
		t.Errorf("expected max_turns 12, got %d", s.MaxTurns)
	}
	if len(s.AcceptanceCriteria) != 2 {
		t.Errorf("expected 2 acceptance criteria, got %v", s.AcceptanceCriteria)
	}

	prompt := s.Prompt()
	if !strings.Contains(prompt, "[SPECIFICATION: Fix Buffer Overflow in Parser]") {
		t.Errorf("prompt missing title header: %s", prompt)
	}
	if !strings.Contains(prompt, "Constrained Target Files") {
		t.Errorf("prompt missing constrained target files section: %s", prompt)
	}
}

func TestSpec_Parse_MarkdownSections(t *testing.T) {
	content := `# Optimize Memory Cache

## Description
Reduce cache eviction overhead under load.

## Target Files
- pkg/cache/lru.go
- pkg/cache/*_test.go

## Preflight Check
git status

## Verification Command
go test -v ./pkg/cache/...

## Acceptance Criteria
- [ ] LRU eviction is O(1)
- [x] Zero memory leaks
`

	s, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if s.Title != "Optimize Memory Cache" {
		t.Errorf("expected title %q, got %q", "Optimize Memory Cache", s.Title)
	}
	if !strings.Contains(s.Description, "Reduce cache eviction overhead") {
		t.Errorf("expected description to contain text, got %q", s.Description)
	}
	if len(s.TargetFiles) != 2 {
		t.Errorf("expected 2 target files, got %v", s.TargetFiles)
	}
	if s.PreflightCmd != "git status" {
		t.Errorf("expected preflight 'git status', got %q", s.PreflightCmd)
	}
	if s.VerifyCmd != "go test -v ./pkg/cache/..." {
		t.Errorf("expected verify 'go test -v ./pkg/cache/...', got %q", s.VerifyCmd)
	}
	if len(s.AcceptanceCriteria) != 2 {
		t.Errorf("expected 2 criteria, got %v", s.AcceptanceCriteria)
	}
}

func TestSpec_Load_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	specFile := filepath.Join(tmpDir, "SPEC.md")

	content := `---
title: "Temp Spec"
verify: "echo ok"
---
`
	if err := os.WriteFile(specFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	s, err := Load(specFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if s.Title != "Temp Spec" || s.VerifyCmd != "echo ok" {
		t.Errorf("unexpected loaded spec: %+v", s)
	}
}

func TestStateMachine_TargetConstraints(t *testing.T) {
	spec := &Spec{
		Title: "Test Constraints",
		TargetFiles: []string{
			"lib/foo.go",
			"lib/bar/*.go",
		},
	}

	sm := NewStateMachine(spec, "/workspace/project")

	// Allowed paths
	allowedPaths := []string{
		"lib/foo.go",
		"/workspace/project/lib/foo.go",
		"./lib/foo.go",
		"lib/bar/baz.go",
		"/workspace/project/lib/bar/qux.go",
	}

	for _, p := range allowedPaths {
		if err := sm.CheckTargetConstraint(p); err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", p, err)
		}
	}

	// Blocked paths
	blockedPaths := []string{
		"lib/secret.go",
		"/etc/passwd",
		"main.go",
		"lib/other/file.go",
	}

	for _, p := range blockedPaths {
		if err := sm.CheckTargetConstraint(p); err == nil {
			t.Errorf("expected %q to be blocked, but was allowed", p)
		} else if !strings.Contains(err.Error(), "target constraint violation") {
			t.Errorf("expected target constraint violation error, got: %v", err)
		}
	}
}

func TestStateMachine_Preflight_SuccessAndFailure(t *testing.T) {
	ctx := context.Background()

	// 1. Preflight Success
	specPass := &Spec{
		Title:        "Passing Preflight",
		PreflightCmd: "check-env",
	}
	smPass := NewStateMachine(specPass, t.TempDir())
	smPass.Executor = func(ctx context.Context, cmd, workDir string) (string, int, error) {
		return "env ok", 0, nil
	}

	if err := smPass.RunPreflight(ctx); err != nil {
		t.Fatalf("expected preflight to pass, got: %v", err)
	}
	if smPass.State != StateExecuting {
		t.Errorf("expected StateExecuting, got %s", smPass.State)
	}
	if smPass.PreflightResult == nil || !smPass.PreflightResult.Passed {
		t.Errorf("expected passed preflight result: %+v", smPass.PreflightResult)
	}

	// 2. Preflight Failure
	specFail := &Spec{
		Title:        "Failing Preflight",
		PreflightCmd: "broken-check",
	}
	smFail := NewStateMachine(specFail, t.TempDir())
	smFail.Executor = func(ctx context.Context, cmd, workDir string) (string, int, error) {
		return "missing dependency", 1, errors.New("exit 1")
	}

	if err := smFail.RunPreflight(ctx); err == nil {
		t.Fatalf("expected preflight failure, got nil")
	}
	if smFail.State != StateFailed {
		t.Errorf("expected StateFailed, got %s", smFail.State)
	}
	if smFail.PreflightResult == nil || smFail.PreflightResult.Passed {
		t.Errorf("expected failed preflight result: %+v", smFail.PreflightResult)
	}
}

func TestStateMachine_Preflight_BoundaryEscape(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	specEscape := &Spec{
		Title:       "Escape Spec",
		TargetFiles: []string{"../../etc/passwd"},
	}
	smEscape := NewStateMachine(specEscape, workDir)

	if err := smEscape.RunPreflight(ctx); err == nil {
		t.Fatalf("expected preflight boundary escape failure, got nil")
	} else if !strings.Contains(err.Error(), "escapes workspace boundary") {
		t.Errorf("unexpected error message: %v", err)
	}
	if smEscape.State != StateFailed {
		t.Errorf("expected StateFailed, got %s", smEscape.State)
	}
}

func TestStateMachine_VerificationGate_LockAndRelease(t *testing.T) {
	ctx := context.Background()
	spec := &Spec{
		Title:     "Feature Gate",
		VerifyCmd: "go test ./...",
	}

	sm := NewStateMachine(spec, t.TempDir())

	// Step 1: Tests fail (exit code 1)
	sm.Executor = func(ctx context.Context, cmd, workDir string) (string, int, error) {
		return "FAIL: TestFoo", 1, errors.New("exit status 1")
	}

	res, _ := sm.Verify(ctx)
	if res.Passed {
		t.Errorf("expected verification to fail")
	}
	if sm.State != StateExecuting {
		t.Errorf("expected state to remain StateExecuting to allow self-correction, got %s", sm.State)
	}

	ledgerFail := sm.ToCompactionLedger()
	if !strings.Contains(ledgerFail, "passed=\"false\"") || !strings.Contains(ledgerFail, "exit_code=\"1\"") {
		t.Errorf("unexpected compaction ledger on fail: %s", ledgerFail)
	}

	// Step 2: Agent fixes bug, tests now pass (exit code 0)
	sm.Executor = func(ctx context.Context, cmd, workDir string) (string, int, error) {
		return "PASS: TestFoo\nok", 0, nil
	}

	resPass, err := sm.Verify(ctx)
	if err != nil {
		t.Fatalf("unexpected error on passing verify: %v", err)
	}
	if !resPass.Passed {
		t.Errorf("expected verification to pass")
	}
	if sm.State != StateCompleted {
		t.Errorf("expected state to transition to StateCompleted, got %s", sm.State)
	}

	ledgerPass := sm.ToCompactionLedger()
	if !strings.Contains(ledgerPass, "passed=\"true\"") || !strings.Contains(ledgerPass, "exit_code=\"0\"") {
		t.Errorf("unexpected compaction ledger on pass: %s", ledgerPass)
	}
}
