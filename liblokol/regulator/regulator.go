// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator

import (
	"context"
)

// ActionCandidate represents the candidate action evaluated by the regulator.
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

// Regulator coordinates Tier 1 pure Go containment and Tier 2 semantic validation.
// It orchestrates the functional Pipeline defined in ADR 0023, ADR 0025, and ADR 0026.
type Regulator struct {
	WorkDir            string
	SemanticEvaluator  SemanticEvaluator
	SlotStatusProvider SlotStatusProvider
	pipeline           *Pipeline
}

// New creates an initialized Regulator rooted at the specified workspace directory.
func New(workDir string) *Regulator {
	return &Regulator{
		WorkDir:  workDir,
		pipeline: DefaultPipeline(workDir),
	}
}

// NewWithEvaluator creates an initialized Regulator with a semantic evaluator stage.
func NewWithEvaluator(workDir string, evaluator SemanticEvaluator) *Regulator {
	return &Regulator{
		WorkDir:           workDir,
		SemanticEvaluator: evaluator,
		pipeline:          DefaultPipeline(workDir, evaluator),
	}
}

// NewWithSlotProvider creates an initialized Regulator with both a semantic evaluator and slot status provider.
func NewWithSlotProvider(workDir string, evaluator SemanticEvaluator, slotProvider SlotStatusProvider) *Regulator {
	return &Regulator{
		WorkDir:            workDir,
		SemanticEvaluator:  evaluator,
		SlotStatusProvider: slotProvider,
		pipeline:           DefaultPipelineWithSlot(workDir, evaluator, slotProvider),
	}
}

// Pipeline returns the underlying functional regulator pipeline.
func (r *Regulator) Pipeline() *Pipeline {
	if r.pipeline == nil {
		if r.SlotStatusProvider != nil {
			r.pipeline = DefaultPipelineWithSlot(r.WorkDir, r.SemanticEvaluator, r.SlotStatusProvider)
		} else {
			r.pipeline = DefaultPipeline(r.WorkDir, r.SemanticEvaluator)
		}
	}
	return r.pipeline
}

// SetPipeline sets a custom pipeline on the regulator.
func (r *Regulator) SetPipeline(p *Pipeline) {
	r.pipeline = p
}

// SetSemanticEvaluator updates the semantic evaluator and updates the pipeline (lokol-gml.6).
func (r *Regulator) SetSemanticEvaluator(evaluator SemanticEvaluator) {
	r.SemanticEvaluator = evaluator
	if r.SlotStatusProvider != nil {
		r.pipeline = DefaultPipelineWithSlot(r.WorkDir, evaluator, r.SlotStatusProvider)
	} else {
		r.pipeline = DefaultPipeline(r.WorkDir, evaluator)
	}
}

// SetSlotStatusProvider updates the slot status provider and updates the pipeline (ADR 0026).
func (r *Regulator) SetSlotStatusProvider(provider SlotStatusProvider) {
	r.SlotStatusProvider = provider
	if provider != nil {
		r.pipeline = DefaultPipelineWithSlot(r.WorkDir, r.SemanticEvaluator, provider)
	} else {
		r.pipeline = DefaultPipeline(r.WorkDir, r.SemanticEvaluator)
	}
}

// CheckPermission validates an action through the functional regulator pipeline.
func (r *Regulator) CheckPermission(ctx context.Context, action ActionCandidate) PermissionResult {
	return r.Pipeline().Regulate(ctx, action)
}

// Regulate executes candidate action validation through the functional pipeline.
func (r *Regulator) Regulate(ctx context.Context, action ActionCandidate) PermissionResult {
	return r.CheckPermission(ctx, action)
}
