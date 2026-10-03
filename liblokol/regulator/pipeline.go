// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Stage represents an individual inspection gate in the regulator pipeline.
// Stages must be stateless, pure, and ordered by computational cost.
type Stage interface {
	Name() string
	Evaluate(ctx context.Context, action ActionCandidate, workDir string) PermissionResult
}

// StageFunc is an adapter allowing ordinary functions to act as regulator Stages.
type StageFunc func(ctx context.Context, action ActionCandidate, workDir string) PermissionResult

func (f StageFunc) Name() string {
	return "custom"
}

func (f StageFunc) Evaluate(ctx context.Context, action ActionCandidate, workDir string) PermissionResult {
	return f(ctx, action, workDir)
}

// NamedStage wraps a StageFunc with a designated stage name.
type NamedStage struct {
	name string
	fn   func(ctx context.Context, action ActionCandidate, workDir string) PermissionResult
}

func NewNamedStage(name string, fn func(ctx context.Context, action ActionCandidate, workDir string) PermissionResult) *NamedStage {
	return &NamedStage{name: name, fn: fn}
}

func (s *NamedStage) Name() string {
	return s.name
}

func (s *NamedStage) Evaluate(ctx context.Context, action ActionCandidate, workDir string) PermissionResult {
	return s.fn(ctx, action, workDir)
}

// Pipeline orchestrates an ordered sequence of regulator stages.
// Execution short-circuits on StatusBlocked or RiskLevelHigh decisions, preventing
// unnecessary evaluation of downstream, computationally expensive stages (lokol-gml.7, lokol-gml.11).
type Pipeline struct {
	mu      sync.RWMutex
	workDir string
	stages  []Stage
}

// NewPipeline creates an initialized Pipeline rooted at workDir with the given stages.
func NewPipeline(workDir string, stages ...Stage) *Pipeline {
	if workDir == "" {
		workDir = "."
	}
	return &Pipeline{
		workDir: workDir,
		stages:  stages,
	}
}

// AddStage appends a new inspection stage to the end of the pipeline with concurrency safety.
func (p *Pipeline) AddStage(stage Stage) *Pipeline {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stages = append(p.stages, stage)
	return p
}

// Stages returns a snapshot copy of the registered inspection stages.
func (p *Pipeline) Stages() []Stage {
	p.mu.RLock()
	defer p.mu.RUnlock()
	stagesCopy := make([]Stage, len(p.stages))
	copy(stagesCopy, p.stages)
	return stagesCopy
}

// WorkDir returns the root workspace directory enforced by this pipeline.
func (p *Pipeline) WorkDir() string {
	return p.workDir
}

// Regulate executes candidate action validation through the ordered pipeline stages.
// It short-circuits immediately on StatusBlocked or RiskLevelHigh warnings (lokol-gml.7).
func (p *Pipeline) Regulate(ctx context.Context, action ActionCandidate) PermissionResult {
	workDir := p.WorkDir()

	p.mu.RLock()
	stages := make([]Stage, len(p.stages))
	copy(stages, p.stages)
	p.mu.RUnlock()

	var maxWarning *PermissionResult

	for _, stage := range stages {
		res := stage.Evaluate(ctx, action, workDir)
		res.Stage = stage.Name()

		// Immediate short circuit on blocked or high-risk hazard (lokol-gml.7)
		if res.Status == StatusBlocked || (res.Status == StatusWarning && res.RiskLevel == RiskLevelHigh) {
			return res
		}

		if res.Status == StatusWarning {
			if maxWarning == nil || warningRank(res.RiskLevel) > warningRank(maxWarning.RiskLevel) {
				resCopy := res
				maxWarning = &resCopy
			}
		}
	}

	if maxWarning != nil {
		return *maxWarning
	}

	return PermissionResult{
		Status:    StatusAllowed,
		Reason:    "Action verified within authorized workspace boundaries and safety policies",
		RiskLevel: RiskLevelNone,
		Stage:     "pipeline",
	}
}

func warningRank(level RiskLevel) int {
	switch level {
	case RiskLevelCritical:
		return 4
	case RiskLevelHigh:
		return 3
	case RiskLevelMedium:
		return 2
	case RiskLevelLow:
		return 1
	default:
		return 0
	}
}

// CheckPermission provides backward-compatible delegation to Regulate.
func (p *Pipeline) CheckPermission(ctx context.Context, action ActionCandidate) PermissionResult {
	return p.Regulate(ctx, action)
}

// ---------------------------------------------------------------------------
// Standard Cost-Ordered Regulator Stages (ADR 0023)
// ---------------------------------------------------------------------------

// NewStructuralStage creates Stage 1: Syntactic & Structural Validation (<1µs).
// Enforces schema syntax, rejects null bytes, and verifies parameter existence.
func NewStructuralStage() Stage {
	return NewNamedStage("structural", func(ctx context.Context, action ActionCandidate, workDir string) PermissionResult {
		// Null byte detection
		if strings.Contains(action.Path, "\x00") || strings.Contains(action.Command, "\x00") {
			return PermissionResult{
				Status:      StatusBlocked,
				Reason:      "Malformed payload: null byte detected in action parameters",
				RiskLevel:   RiskLevelHigh,
				Target:      action.Name,
				Remediation: "Ensure tool arguments are valid UTF-8 strings without null bytes.",
			}
		}

		// Parameter presence validation for path-requiring builtins
		switch action.Name {
		case "write_file", "replace_file", "read_window", "read_outline":
			path := ResolveActionPath(action)
			if path == "" {
				return PermissionResult{
					Status:      StatusBlocked,
					Reason:      fmt.Sprintf("Action %s missing required target file path", action.Name),
					RiskLevel:   RiskLevelMedium,
					Target:      action.Name,
					Remediation: fmt.Sprintf("Provide a valid <path> parameter within workspace for %s.", action.Name),
				}
			}
		case "find_files":
			// If neither pattern nor path specified, default is allowed (matches all)
		}

		return PermissionResult{Status: StatusAllowed, RiskLevel: RiskLevelNone}
	})
}

// NewBoundaryStage creates Stage 2: Filesystem Containment & Boundary Enforcement (~5-20µs).
// Resolves symlinks, detects parent traversals (..), and enforces workDir confinement.
func NewBoundaryStage() Stage {
	return NewNamedStage("boundary", func(ctx context.Context, action ActionCandidate, workDir string) PermissionResult {
		if err := ValidateFilesystemBounds(workDir, action.Name, action.Path, action.Command); err != nil {
			targetPath := ResolveActionPath(action)
			return PermissionResult{
				Status:      StatusBlocked,
				Reason:      fmt.Sprintf("Filesystem boundary violation: %v", err),
				RiskLevel:   RiskLevelHigh,
				OutOfBounds: true,
				Target:      targetPath,
				Remediation: fmt.Sprintf("Target path %q escapes authorized workspace %q. Confine all filesystem operations strictly within the project root.", targetPath, workDir),
			}
		}
		return PermissionResult{Status: StatusAllowed, RiskLevel: RiskLevelNone}
	})
}

// NewStaticShellStage creates Stage 3: Static Shell Command Analysis (~10-50µs).
// Inspects bash commands for dangerous patterns, fork bombs, disk formatting, and privilege escalation.
func NewStaticShellStage() Stage {
	return NewNamedStage("static_shell", func(ctx context.Context, action ActionCandidate, workDir string) PermissionResult {
		if action.Name != "exec_bash" {
			return PermissionResult{Status: StatusAllowed, RiskLevel: RiskLevelNone}
		}

		cmd := strings.TrimSpace(action.Command)
		risk := InspectShellRisk(workDir, cmd)

		if risk.Level == RiskLevelCritical {
			return PermissionResult{
				Status:      StatusBlocked,
				Reason:      fmt.Sprintf("Blocked dangerous shell operation: %s", risk.Reason),
				RiskLevel:   risk.Level,
				OutOfBounds: false,
				Target:      cmd,
				Remediation: fmt.Sprintf("The proposed command contains critical destructive signature (%s). Reformulate using scoped, non-destructive tools.", risk.Reason),
			}
		}

		if risk.Level == RiskLevelHigh || risk.Level == RiskLevelMedium {
			return PermissionResult{
				Status:      StatusWarning,
				Reason:      fmt.Sprintf("Security warning: %s", risk.Reason),
				RiskLevel:   risk.Level,
				OutOfBounds: false,
				Target:      cmd,
				Remediation: fmt.Sprintf("Command requires operator review: %s. Use caution before executing high-risk operations.", risk.Reason),
			}
		}

		return PermissionResult{Status: StatusAllowed, RiskLevel: RiskLevelNone}
	})
}

// NewSemanticStage creates Stage 4: Non-Autoregressive Semantic Decision Classification (~30-50ms).
// Evaluates ambiguous or context-dependent actions using Laya or decision models.
func NewSemanticStage(evaluator SemanticEvaluator) Stage {
	return NewNamedStage("semantic", func(ctx context.Context, action ActionCandidate, workDir string) PermissionResult {
		if evaluator == nil {
			return PermissionResult{Status: StatusAllowed, RiskLevel: RiskLevelNone}
		}

		semResult, err := evaluator.Evaluate(ctx, action, workDir)
		if err != nil {
			return PermissionResult{
				Status:      StatusWarning,
				Reason:      fmt.Sprintf("Semantic regulator evaluator unavailable: %v", err),
				RiskLevel:   RiskLevelMedium,
				Target:      action.Path,
				Remediation: "Semantic judge was unreachable; review the action manually before approval.",
			}
		}

		if semResult.Status != StatusAllowed {
			if semResult.Remediation == "" {
				semResult.Remediation = "Action flagged by semantic safety model. Confirm that the operation is non-destructive and strictly necessary."
			}
			return semResult
		}

		return PermissionResult{Status: StatusAllowed, RiskLevel: RiskLevelNone}
	})
}

// DefaultPipeline constructs the standard cost-ordered regulator pipeline (ADR 0023, ADR 0025).
// Always wires pure Go Stage 4 semantic evaluation in production (lokol-gml.2).
func DefaultPipeline(workDir string, evaluator ...SemanticEvaluator) *Pipeline {
	return DefaultPipelineWithSlot(workDir, firstEvaluator(evaluator...), nil)
}

// DefaultPipelineWithSlot constructs the standard cost-ordered regulator pipeline including an
// inference slot governor stage (ADR 0023, ADR 0025, ADR 0026).
func DefaultPipelineWithSlot(workDir string, evaluator SemanticEvaluator, slotProvider SlotStatusProvider) *Pipeline {
	if evaluator == nil {
		evaluator = NewHeuristicRiskScorer()
	}

	stages := []Stage{
		NewStructuralStage(),
		NewUserRejectionStage(),
		NewLoopCircuitBreakerStage(),
		NewBoundaryStage(),
		NewStaticShellStage(),
	}

	if slotProvider != nil {
		stages = append(stages, NewInferenceSlotGovernorStage(slotProvider))
	}

	stages = append(stages, NewSemanticStage(evaluator))

	return NewPipeline(workDir, stages...)
}

func firstEvaluator(evaluator ...SemanticEvaluator) SemanticEvaluator {
	if len(evaluator) > 0 && evaluator[0] != nil {
		return evaluator[0]
	}
	return nil
}

