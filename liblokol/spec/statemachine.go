// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package spec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// State represents the lifecycle state of a spec-driven execution.
type State string

const (
	StatePending   State = "PENDING"
	StatePreflight State = "PREFLIGHT"
	StateExecuting State = "EXECUTING"
	StateVerifying State = "VERIFYING"
	StateCompleted State = "COMPLETED"
	StateFailed    State = "FAILED"
)

// VerificationResult holds the outcome of a preflight or verification command run.
type VerificationResult struct {
	Timestamp time.Time `json:"timestamp"`
	Command   string    `json:"command"`
	ExitCode  int       `json:"exit_code"`
	Output    string    `json:"output"`
	Passed    bool      `json:"passed"`
}

// CommandExecutor executes a shell command in a working directory.
type CommandExecutor func(ctx context.Context, cmd string, workDir string) (output string, exitCode int, err error)

// StateMachine coordinates bounded spec execution, target constraints, and verification gates.
type StateMachine struct {
	Spec               *Spec               `json:"spec"`
	WorkDir            string              `json:"work_dir"`
	State              State               `json:"state"`
	LatestVerification *VerificationResult `json:"latest_verification,omitempty"`
	PreflightResult    *VerificationResult `json:"preflight_result,omitempty"`
	Executor           CommandExecutor     `json:"-"`
}

// NewStateMachine creates a new bounded state machine for a parsed specification.
func NewStateMachine(s *Spec, workDir string) *StateMachine {
	if workDir == "" {
		workDir = "."
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err == nil {
		workDir = absWorkDir
	}

	return &StateMachine{
		Spec:     s,
		WorkDir:  workDir,
		State:    StatePending,
		Executor: DefaultExecutor,
	}
}

// DefaultExecutor runs bash commands, capturing combined output with safe line limits.
func DefaultExecutor(ctx context.Context, cmdStr string, workDir string) (string, int, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	if workDir != "" {
		cmd.Dir = workDir
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()

	// Truncate output to last 200 lines if overly verbose to prevent buffer saturation
	lines := strings.Split(output, "\n")
	if len(lines) > 200 {
		output = fmt.Sprintf("[Output truncated to last 200 lines (%d total)]\n%s",
			len(lines), strings.Join(lines[len(lines)-200:], "\n"))
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return output, exitErr.ExitCode(), nil
		}
		return output, -1, err
	}

	return output, 0, nil
}

// RunPreflight runs preflight checks and validates target file boundaries before execution begins.
func (sm *StateMachine) RunPreflight(ctx context.Context) error {
	if sm.Spec == nil {
		sm.State = StateExecuting
		return nil
	}

	sm.State = StatePreflight

	// Validate target file boundaries: none may escape the workspace root (ADR 0019)
	for _, target := range sm.Spec.TargetFiles {
		cleanTarget := filepath.Clean(target)
		if filepath.IsAbs(cleanTarget) {
			rel, err := filepath.Rel(sm.WorkDir, cleanTarget)
			if err != nil || strings.HasPrefix(rel, "..") {
				sm.State = StateFailed
				return fmt.Errorf("target file %q escapes workspace boundary %s", target, sm.WorkDir)
			}
		} else {
			abs := filepath.Join(sm.WorkDir, cleanTarget)
			rel, err := filepath.Rel(sm.WorkDir, abs)
			if err != nil || strings.HasPrefix(rel, "..") {
				sm.State = StateFailed
				return fmt.Errorf("target file %q escapes workspace boundary %s", target, sm.WorkDir)
			}
		}
	}

	// Run preflight command if configured
	if sm.Spec.PreflightCmd != "" {
		executor := sm.Executor
		if executor == nil {
			executor = DefaultExecutor
		}

		out, code, err := executor(ctx, sm.Spec.PreflightCmd, sm.WorkDir)
		res := &VerificationResult{
			Timestamp: time.Now(),
			Command:   sm.Spec.PreflightCmd,
			ExitCode:  code,
			Output:    out,
			Passed:    (code == 0 && err == nil),
		}
		sm.PreflightResult = res

		if !res.Passed {
			sm.State = StateFailed
			if err != nil {
				return fmt.Errorf("preflight check %q failed with error: %w (output: %s)", sm.Spec.PreflightCmd, err, out)
			}
			return fmt.Errorf("preflight check %q failed with exit code %d: %s", sm.Spec.PreflightCmd, code, out)
		}
	}

	sm.State = StateExecuting
	return nil
}

// CheckTargetConstraint verifies whether a given file path is allowed to be modified under this spec.
func (sm *StateMachine) CheckTargetConstraint(targetPath string) error {
	if sm.Spec == nil || len(sm.Spec.TargetFiles) == 0 {
		return nil
	}

	cleanTarget := filepath.Clean(targetPath)

	// If absolute, convert to relative to WorkDir
	if filepath.IsAbs(cleanTarget) {
		rel, err := filepath.Rel(sm.WorkDir, cleanTarget)
		if err == nil {
			cleanTarget = rel
		}
	}

	// Clean leading ./ if present
	cleanTarget = strings.TrimPrefix(cleanTarget, "./")

	for _, allowed := range sm.Spec.TargetFiles {
		cleanAllowed := filepath.Clean(allowed)
		if filepath.IsAbs(cleanAllowed) {
			rel, err := filepath.Rel(sm.WorkDir, cleanAllowed)
			if err == nil {
				cleanAllowed = rel
			}
		}
		cleanAllowed = strings.TrimPrefix(cleanAllowed, "./")

		// Exact match
		if cleanTarget == cleanAllowed {
			return nil
		}

		// Base name match
		if filepath.Base(cleanTarget) == cleanAllowed {
			return nil
		}

		// Glob pattern match
		if matched, _ := filepath.Match(cleanAllowed, cleanTarget); matched {
			return nil
		}
		if matched, _ := filepath.Match(cleanAllowed, filepath.Base(cleanTarget)); matched {
			return nil
		}
	}

	return fmt.Errorf("target constraint violation: modification of %q is not permitted by SPEC.md (allowed target files: %s)",
		targetPath, strings.Join(sm.Spec.TargetFiles, ", "))
}

// Verify runs the verification command, locking completion until exit code 0.
func (sm *StateMachine) Verify(ctx context.Context) (*VerificationResult, error) {
	if sm.Spec == nil || sm.Spec.VerifyCmd == "" {
		res := &VerificationResult{
			Timestamp: time.Now(),
			Command:   "(none)",
			ExitCode:  0,
			Output:    "No verification command configured in spec.",
			Passed:    true,
		}
		sm.LatestVerification = res
		sm.State = StateCompleted
		return res, nil
	}

	sm.State = StateVerifying

	executor := sm.Executor
	if executor == nil {
		executor = DefaultExecutor
	}

	out, code, err := executor(ctx, sm.Spec.VerifyCmd, sm.WorkDir)
	res := &VerificationResult{
		Timestamp: time.Now(),
		Command:   sm.Spec.VerifyCmd,
		ExitCode:  code,
		Output:    out,
		Passed:    (code == 0 && err == nil),
	}
	sm.LatestVerification = res

	if res.Passed {
		sm.State = StateCompleted
	} else {
		// Remain in or return to executing state to allow self-correction
		sm.State = StateExecuting
	}

	if err != nil && code == -1 {
		return res, err
	}

	return res, nil
}

// ToCompactionLedger produces a structured context ledger for downstream context compaction (lokol-asw).
func (sm *StateMachine) ToCompactionLedger() string {
	if sm.Spec == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("<spec_ledger>\n")
	b.WriteString(fmt.Sprintf("  <title>%s</title>\n", sm.Spec.Title))
	b.WriteString(fmt.Sprintf("  <state>%s</state>\n", sm.State))

	if len(sm.Spec.TargetFiles) > 0 {
		b.WriteString(fmt.Sprintf("  <target_files>%s</target_files>\n", strings.Join(sm.Spec.TargetFiles, ", ")))
	}

	if sm.LatestVerification != nil {
		b.WriteString(fmt.Sprintf("  <latest_verification passed=\"%t\" exit_code=\"%d\">\n",
			sm.LatestVerification.Passed, sm.LatestVerification.ExitCode))
		b.WriteString(fmt.Sprintf("    <command>%s</command>\n", sm.LatestVerification.Command))

		// Compact output to first 5 and last 10 lines of verification output
		lines := strings.Split(strings.TrimSpace(sm.LatestVerification.Output), "\n")
		var summaryLines []string
		if len(lines) <= 15 {
			summaryLines = lines
		} else {
			summaryLines = append(summaryLines, lines[:5]...)
			summaryLines = append(summaryLines, fmt.Sprintf("... [%d lines omitted] ...", len(lines)-15))
			summaryLines = append(summaryLines, lines[len(lines)-10:]...)
		}
		b.WriteString(fmt.Sprintf("    <summary>\n%s\n    </summary>\n", strings.Join(summaryLines, "\n")))
		b.WriteString("  </latest_verification>\n")
	}

	b.WriteString("</spec_ledger>")
	return b.String()
}

// MarshalJSON provides clean JSON serialization for persistence and inter-process inspection.
func (sm *StateMachine) MarshalJSON() ([]byte, error) {
	type Alias StateMachine
	return json.Marshal(&struct {
		*Alias
		CompactionLedger string `json:"compaction_ledger"`
	}{
		Alias:            (*Alias)(sm),
		CompactionLedger: sm.ToCompactionLedger(),
	})
}
