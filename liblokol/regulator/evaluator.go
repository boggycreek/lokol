// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator

import (
	"context"
	"fmt"
	"strings"
)

// SemanticEvaluator evaluates candidate actions using a non-autoregressive decision model or native CPU evaluator.
type SemanticEvaluator interface {
	Evaluate(ctx context.Context, action ActionCandidate, workDir string) (PermissionResult, error)
}

// SemanticEvaluatorFunc adapts an ordinary function to the SemanticEvaluator interface.
type SemanticEvaluatorFunc func(ctx context.Context, action ActionCandidate, workDir string) (PermissionResult, error)

func (f SemanticEvaluatorFunc) Evaluate(ctx context.Context, action ActionCandidate, workDir string) (PermissionResult, error) {
	return f(ctx, action, workDir)
}

// NativeCPUEvaluator is a pure Go semantic decision evaluator executing on host CPU with zero Python dependencies (ADR-0020, ADR-0021).
type NativeCPUEvaluator struct {
	Threshold float64
}

// NewNativeCPUEvaluator creates a pure Go semantic decision evaluator for production runtime.
func NewNativeCPUEvaluator() *NativeCPUEvaluator {
	return &NativeCPUEvaluator{
		Threshold: 0.5,
	}
}

// Evaluate performs sub-millisecond semantic risk assessment on CPU.
func (e *NativeCPUEvaluator) Evaluate(ctx context.Context, action ActionCandidate, workDir string) (PermissionResult, error) {
	cmdLower := strings.ToLower(action.Command)
	pathLower := strings.ToLower(action.Path)

	// Anomaly / risk indicators for semantic scoring
	score := 1.0

	// 1. Covert credential access or evasion
	if strings.Contains(cmdLower, ".ssh") || strings.Contains(cmdLower, ".aws") || strings.Contains(cmdLower, "id_rsa") {
		score -= 0.6
	}
	if strings.Contains(pathLower, ".ssh") || strings.Contains(pathLower, ".aws") {
		score -= 0.6
	}

	// 2. Destructive command signatures
	if strings.Contains(cmdLower, "rm ") && (strings.Contains(cmdLower, "-r") || strings.Contains(cmdLower, "--recursive")) {
		if strings.Contains(cmdLower, " /") || strings.Contains(cmdLower, " ~") || strings.Contains(cmdLower, " $home") {
			score -= 0.8
		}
	}

	// 3. Process substitution and remote script execution
	if strings.Contains(cmdLower, "<(") || strings.Contains(cmdLower, ">(") {
		score -= 0.6
	}
	if (strings.Contains(cmdLower, "curl ") || strings.Contains(cmdLower, "wget ")) && strings.Contains(cmdLower, "|") {
		score -= 0.7
	}

	// 4. Overwrite of root or system configurations
	if strings.Contains(cmdLower, "/etc/") || strings.Contains(pathLower, "/etc/") {
		score -= 0.5
	}

	if score < e.Threshold {
		return PermissionResult{
			Status:      StatusWarning,
			Reason:      fmt.Sprintf("Semantic regulator flagged action (safety confidence: %.2f)", score),
			RiskLevel:   RiskLevelHigh,
			Target:      strings.TrimSpace(action.Command),
			Stage:       "semantic",
			Remediation: "Action flagged by semantic safety evaluator. Confirm that the operation is non-destructive and strictly necessary within the workspace.",
		}, nil
	}

	return PermissionResult{
		Status:    StatusAllowed,
		RiskLevel: RiskLevelNone,
		Stage:     "semantic",
	}, nil
}
