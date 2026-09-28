// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package guardrail

import (
	"context"
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

// PermissionResult details the outcome of regulator checks.
type PermissionResult struct {
	Status      Status
	Reason      string
	RiskLevel   RiskLevel
	OutOfBounds bool
	Target      string
	Stage       string
	Remediation string
}

// Guardrail coordinates Tier 1 pure Go containment and Tier 2 semantic validation.
// It wraps the functional Pipeline defined in ADR 0023 for backward-compatible action gating.
type Guardrail struct {
	WorkDir           string
	SemanticEvaluator SemanticEvaluator
	pipeline          *Pipeline
}

// New creates an initialized Guardrail rooted at the specified workspace directory.
func New(workDir string) *Guardrail {
	return &Guardrail{
		WorkDir:  workDir,
		pipeline: DefaultPipeline(workDir),
	}
}

// NewWithEvaluator creates an initialized Guardrail with a semantic evaluator stage.
func NewWithEvaluator(workDir string, evaluator SemanticEvaluator) *Guardrail {
	return &Guardrail{
		WorkDir:           workDir,
		SemanticEvaluator: evaluator,
		pipeline:          DefaultPipeline(workDir, evaluator),
	}
}

// Pipeline returns the underlying functional regulator pipeline.
func (g *Guardrail) Pipeline() *Pipeline {
	if g.pipeline == nil {
		g.pipeline = DefaultPipeline(g.WorkDir, g.SemanticEvaluator)
	}
	return g.pipeline
}

// CheckPermission validates an action through the functional regulator pipeline.
func (g *Guardrail) CheckPermission(ctx context.Context, action ActionCandidate) PermissionResult {
	if g.pipeline == nil || (g.SemanticEvaluator != nil && len(g.pipeline.Stages()) == 3) {
		g.pipeline = DefaultPipeline(g.WorkDir, g.SemanticEvaluator)
	}
	return g.pipeline.Regulate(ctx, action)
}
