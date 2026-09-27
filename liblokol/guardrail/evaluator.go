// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package guardrail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// SemanticEvaluator evaluates candidate actions using a non-autoregressive decision model.
type SemanticEvaluator interface {
	Evaluate(ctx context.Context, action ActionCandidate, workDir string) (PermissionResult, error)
}

// SubprocessLayaEvaluator executes Laya on CPU/AVX2 via a decoupled subprocess to avoid runtime coupling.
type SubprocessLayaEvaluator struct {
	RepoRoot   string
	PythonCmd  string
	Threshold  float64
}

// NewSubprocessLayaEvaluator initializes a Laya evaluator rooted at the specified repository path.
func NewSubprocessLayaEvaluator(repoRoot string) *SubprocessLayaEvaluator {
	return &SubprocessLayaEvaluator{
		RepoRoot:  repoRoot,
		Threshold: 0.5,
	}
}

// Evaluate submits the candidate action trajectory to Laya to evaluate its safety.
func (e *SubprocessLayaEvaluator) Evaluate(ctx context.Context, action ActionCandidate, workDir string) (PermissionResult, error) {
	judgeScript := filepath.Join(e.RepoRoot, "tools", "laya", "judge.py")

	// Construct state description of proposed action
	var stateDesc string
	switch action.Name {
	case "exec_bash":
		stateDesc = fmt.Sprintf("Working Directory: %s\nAction: Execute Bash Command\nCommand: %s", workDir, action.Command)
	default:
		stateDesc = fmt.Sprintf("Working Directory: %s\nAction: %s\nTarget: %s\nPayload: %s", workDir, action.Name, action.Path, action.Command)
	}

	instructions := "Is this candidate tool action safe, non-destructive, and strictly confined to the project workspace?"

	payload := map[string]interface{}{
		"state":        stateDesc,
		"instructions": instructions,
		"threshold":    e.Threshold,
	}

	inputBytes, err := json.Marshal(payload)
	if err != nil {
		return PermissionResult{Status: StatusAllowed}, fmt.Errorf("marshal laya payload: %w", err)
	}

	cmd := exec.CommandContext(ctx, "uv", "run", "--python", ".venv", judgeScript, "--json")
	cmd.Dir = e.RepoRoot
	cmd.Stdin = bytes.NewReader(inputBytes)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if stdout.Len() == 0 {
		return PermissionResult{Status: StatusAllowed}, fmt.Errorf("laya judge returned no output (err=%v, stderr=%s)", runErr, stderr.String())
	}

	var res struct {
		Passed       bool    `json:"passed"`
		Probability  float64 `json:"probability"`
		QualityLevel float64 `json:"quality_level"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return PermissionResult{Status: StatusAllowed}, fmt.Errorf("parse laya output: %w", err)
	}

	if !res.Passed {
		return PermissionResult{
			Status:    StatusWarning,
			Reason:    fmt.Sprintf("Semantic guardrail flagged action (safety confidence: %.2f)", res.Probability),
			RiskLevel: RiskLevelHigh,
			Target:    strings.TrimSpace(action.Command),
		}, nil
	}

	return PermissionResult{Status: StatusAllowed}, nil
}
