// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package spec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/boggycreek/lokol/liblokol/refinery"
	"github.com/boggycreek/lokol/liblokol/regulator"
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

// DefaultExecutor runs bash commands, capturing combined output with safe line and byte limits.
func DefaultExecutor(ctx context.Context, cmdStr string, workDir string) (string, int, error) {
	if risk := regulator.InspectShellRisk(workDir, cmdStr); risk.Level == regulator.RiskLevelCritical || risk.Level == regulator.RiskLevelHigh {
		return fmt.Sprintf("execution blocked by security regulator: %s (%s)", risk.Reason, risk.Level), -1, fmt.Errorf("command execution rejected: %s", risk.Reason)
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	if workDir != "" {
		cmd.Dir = workDir
	}

	buf := refinery.NewCappedBuffer(refinery.MaxTestOutputBytes)
	cmd.Stdout = buf
	cmd.Stderr = buf

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
		risk := regulator.InspectShellRisk(sm.WorkDir, sm.Spec.PreflightCmd)
		if risk.Level == regulator.RiskLevelCritical || risk.Level == regulator.RiskLevelHigh {
			sm.State = StateFailed
			res := &VerificationResult{
				Timestamp: time.Now(),
				Command:   sm.Spec.PreflightCmd,
				ExitCode:  -1,
				Output:    fmt.Sprintf("preflight check rejected by security regulator: %s (%s)", risk.Reason, risk.Level),
				Passed:    false,
			}
			sm.PreflightResult = res
			return fmt.Errorf("preflight check %q rejected by security regulator: %s (%s)", sm.Spec.PreflightCmd, risk.Reason, risk.Level)
		}

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
// Path matching is strictly canonicalized relative to the workspace; filepath.Base matching is disallowed.
func (sm *StateMachine) CheckTargetConstraint(targetPath string) error {
	if sm.Spec == nil || len(sm.Spec.TargetFiles) == 0 {
		return nil
	}

	cleanTarget := filepath.Clean(targetPath)
	var rel string
	var err error

	if filepath.IsAbs(cleanTarget) {
		rel, err = filepath.Rel(sm.WorkDir, cleanTarget)
	} else {
		abs := filepath.Join(sm.WorkDir, cleanTarget)
		rel, err = filepath.Rel(sm.WorkDir, abs)
	}

	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("target constraint violation: path %q escapes workspace boundary %s", targetPath, sm.WorkDir)
	}

	rel = filepath.Clean(rel)
	rel = strings.TrimPrefix(rel, "./")

	for _, allowed := range sm.Spec.TargetFiles {
		cleanAllowed := filepath.Clean(allowed)
		var allowedRel string
		if filepath.IsAbs(cleanAllowed) {
			allowedRel, err = filepath.Rel(sm.WorkDir, cleanAllowed)
		} else {
			absAllowed := filepath.Join(sm.WorkDir, cleanAllowed)
			allowedRel, err = filepath.Rel(sm.WorkDir, absAllowed)
		}
		if err != nil {
			continue
		}
		allowedRel = filepath.Clean(allowedRel)
		allowedRel = strings.TrimPrefix(allowedRel, "./")

		// Exact canonical relative match
		if rel == allowedRel {
			return nil
		}

		// Glob pattern match against canonical relative path
		if matched, _ := filepath.Match(allowedRel, rel); matched {
			return nil
		}
	}

	return fmt.Errorf("target constraint violation: modification of %q is not permitted by SPEC.md (allowed target files: %s)",
		targetPath, strings.Join(sm.Spec.TargetFiles, ", "))
}

var (
	bashRedirectRegex   = regexp.MustCompile(`(?:^|[^<>&0-9])(?:>>|>)\s*(?:"([^"]+)"|'([^']+)'|([^\s;&|<>]+))`)
	bashTeeRegex        = regexp.MustCompile(`\btee\s+(?:-[a-zA-Z]+\s+)*(?:"([^"]+)"|'([^']+)'|([^\s;&|<>-][^\s;&|<>]*))`)
	bashMutateRegex     = regexp.MustCompile(`\b(?:touch|rm|truncate)\s+(?:-[a-zA-Z0-9-]+\s+)*(?:"([^"]+)"|'([^']+)'|([^\s;&|<>-][^\s;&|<>]*))`)
	bashCpMvRegex       = regexp.MustCompile(`\b(?:cp|mv)\s+(?:-[a-zA-Z0-9-]+\s+)*.*?(?:"([^"]+)"|'([^']+)'|([^\s;&|<>-][^\s;&|<>]*))`)
	bashSedInplaceRegex = regexp.MustCompile(`\bsed\s+.*-i(?:\s*['"]?[a-zA-Z0-9._-]*['"]?)?\s+.*?(?:"([^"]+)"|'([^']+)'|([^\s;&|<>-][^\s;&|<>]*))`)
)

func extractRegexTarget(match []string) string {
	for i := 1; i < len(match); i++ {
		if match[i] != "" {
			return strings.TrimSpace(match[i])
		}
	}
	return ""
}

// CheckBashTargetConstraint scans a bash command for file modification vectors (redirections,
// writes, file removals, in-place edits) and verifies target paths against the spec's target constraints.
func (sm *StateMachine) CheckBashTargetConstraint(cmd string) error {
	if sm.Spec == nil || len(sm.Spec.TargetFiles) == 0 {
		return nil
	}

	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return nil
	}

	isSpecialTarget := func(target string) bool {
		switch target {
		case "/dev/null", "/dev/zero", "/dev/stdout", "/dev/stderr", "&1", "&2":
			return true
		default:
			return false
		}
	}

	// 1. Redirection operators (> and >>)
	for _, m := range bashRedirectRegex.FindAllStringSubmatch(trimmed, -1) {
		target := extractRegexTarget(m)
		if target == "" || isSpecialTarget(target) {
			continue
		}
		if err := sm.CheckTargetConstraint(target); err != nil {
			return fmt.Errorf("shell redirection violates target constraint: %w", err)
		}
	}

	// 2. tee writes
	for _, m := range bashTeeRegex.FindAllStringSubmatch(trimmed, -1) {
		target := extractRegexTarget(m)
		if target == "" || isSpecialTarget(target) {
			continue
		}
		if err := sm.CheckTargetConstraint(target); err != nil {
			return fmt.Errorf("shell tee operation violates target constraint: %w", err)
		}
	}

	// 3. Mutating file commands (touch, rm, truncate)
	for _, m := range bashMutateRegex.FindAllStringSubmatch(trimmed, -1) {
		target := extractRegexTarget(m)
		if target == "" || isSpecialTarget(target) {
			continue
		}
		if err := sm.CheckTargetConstraint(target); err != nil {
			return fmt.Errorf("shell file mutation (%s) violates target constraint: %w", target, err)
		}
	}

	// 4. cp / mv destination checks
	for _, m := range bashCpMvRegex.FindAllStringSubmatch(trimmed, -1) {
		target := extractRegexTarget(m)
		if target == "" || isSpecialTarget(target) {
			continue
		}
		if err := sm.CheckTargetConstraint(target); err != nil {
			return fmt.Errorf("shell copy/move target (%s) violates target constraint: %w", target, err)
		}
	}

	// 5. In-place stream modifications (sed -i)
	for _, m := range bashSedInplaceRegex.FindAllStringSubmatch(trimmed, -1) {
		target := extractRegexTarget(m)
		if target == "" || isSpecialTarget(target) {
			continue
		}
		if err := sm.CheckTargetConstraint(target); err != nil {
			return fmt.Errorf("shell in-place stream edit (%s) violates target constraint: %w", target, err)
		}
	}

	return nil
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

	risk := regulator.InspectShellRisk(sm.WorkDir, sm.Spec.VerifyCmd)
	if risk.Level == regulator.RiskLevelCritical || risk.Level == regulator.RiskLevelHigh {
		res := &VerificationResult{
			Timestamp: time.Now(),
			Command:   sm.Spec.VerifyCmd,
			ExitCode:  -1,
			Output:    fmt.Sprintf("verification check rejected by security regulator: %s (%s)", risk.Reason, risk.Level),
			Passed:    false,
		}
		sm.LatestVerification = res
		sm.State = StateExecuting
		return res, fmt.Errorf("verification check %q rejected by security regulator: %s (%s)", sm.Spec.VerifyCmd, risk.Reason, risk.Level)
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

// RecordVerification manually records a verification or test execution result (lokol-asw).
func (sm *StateMachine) RecordVerification(cmd string, code int, output string) {
	sm.LatestVerification = &VerificationResult{
		Timestamp: time.Now(),
		Command:   cmd,
		ExitCode:  code,
		Output:    output,
		Passed:    code == 0,
	}
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

	ver := sm.LatestVerification
	if ver == nil && sm.PreflightResult != nil {
		ver = sm.PreflightResult
	}

	if ver != nil {
		b.WriteString(fmt.Sprintf("  <latest_verification passed=\"%t\" exit_code=\"%d\">\n",
			ver.Passed, ver.ExitCode))
		b.WriteString(fmt.Sprintf("    <command>%s</command>\n", ver.Command))

		// Compact output to first 5 and last 10 lines of verification output
		lines := strings.Split(strings.TrimSpace(ver.Output), "\n")
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
