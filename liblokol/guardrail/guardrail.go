// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package guardrail

import (
	"context"
	"fmt"
	"strings"
)

// ActionCandidate represents the candidate action evaluated by the guardrail.
type ActionCandidate struct {
	Name    string
	Command string
	Path    string
}

// Status represents the permission decision for a candidate action.
type Status string

const (
	StatusAllowed Status = "allowed"
	StatusWarning Status = "warning"
	StatusBlocked Status = "blocked"
)

// PermissionResult details the outcome of guardrail checks.
type PermissionResult struct {
	Status      Status
	Reason      string
	RiskLevel   RiskLevel
	OutOfBounds bool
	Target      string
}

// Guardrail coordinates Tier 1 pure Go containment and Tier 2 semantic validation.
type Guardrail struct {
	WorkDir           string
	SemanticEvaluator SemanticEvaluator
}

// New creates an initialized Guardrail rooted at the specified workspace directory.
func New(workDir string) *Guardrail {
	return &Guardrail{
		WorkDir: workDir,
	}
}

// CheckPermission validates an action against filesystem boundary rules and static safety policies.
func (g *Guardrail) CheckPermission(ctx context.Context, action ActionCandidate) PermissionResult {
	workDir := g.WorkDir
	if workDir == "" {
		workDir = "."
	}

	// 1. Tier 1: Filesystem Boundary Enforcement
	if err := ValidateFilesystemBounds(workDir, action.Name, action.Path, action.Command); err != nil {
		targetPath := action.Path
		if targetPath == "" {
			targetPath = ExtractTagContent(action.Command, "path")
		}
		return PermissionResult{
			Status:      StatusBlocked,
			Reason:      fmt.Sprintf("Filesystem boundary violation: %v", err),
			RiskLevel:   RiskLevelHigh,
			OutOfBounds: true,
			Target:      targetPath,
		}
	}

	// 2. Tier 1: Deterministic Static Shell Risk Analysis
	if action.Name == "exec_bash" {
		cmd := strings.TrimSpace(action.Command)
		risk := InspectShellRisk(workDir, cmd)
		if risk.Level == RiskLevelCritical {
			return PermissionResult{
				Status:      StatusBlocked,
				Reason:      fmt.Sprintf("Blocked dangerous shell operation: %s", risk.Reason),
				RiskLevel:   risk.Level,
				OutOfBounds: false,
				Target:      cmd,
			}
		}
		if risk.Level == RiskLevelHigh || risk.Level == RiskLevelMedium {
			return PermissionResult{
				Status:      StatusWarning,
				Reason:      fmt.Sprintf("Security warning: %s", risk.Reason),
				RiskLevel:   risk.Level,
				OutOfBounds: false,
				Target:      cmd,
			}
		}
	}

	// 3. Tier 2: Non-Autoregressive Semantic Evaluation (if configured)
	if g.SemanticEvaluator != nil {
		semResult, err := g.SemanticEvaluator.Evaluate(ctx, action, workDir)
		if err != nil {
			return PermissionResult{
				Status:    StatusWarning,
				Reason:    fmt.Sprintf("Semantic guardrail evaluator unavailable: %v", err),
				RiskLevel: RiskLevelMedium,
				Target:    action.Path,
			}
		}
		if semResult.Status != StatusAllowed {
			return semResult
		}
	}

	return PermissionResult{
		Status:    StatusAllowed,
		Reason:    "Action verified within authorized workspace boundaries",
		RiskLevel: RiskLevelNone,
	}
}
