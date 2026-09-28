// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator_test

import (
	"context"
	"strings"
	"testing"

	"github.com/boggycreek/lokol/liblokol/regulator"
)

type mockSemanticEvaluator struct {
	evaluateFunc func(ctx context.Context, action regulator.ActionCandidate, workDir string) (regulator.PermissionResult, error)
	called       bool
}

func (m *mockSemanticEvaluator) Evaluate(ctx context.Context, action regulator.ActionCandidate, workDir string) (regulator.PermissionResult, error) {
	m.called = true
	if m.evaluateFunc != nil {
		return m.evaluateFunc(ctx, action, workDir)
	}
	return regulator.PermissionResult{Status: regulator.StatusAllowed}, nil
}

// ---------------------------------------------------------------------------
// 1. Cost-Ordered Short Circuiting & Execution Order
// ---------------------------------------------------------------------------

func TestPipeline_CostOrderedShortCircuit_StructuralStage(t *testing.T) {
	workDir := t.TempDir()

	boundaryExecuted := false
	shellExecuted := false
	mockSem := &mockSemanticEvaluator{}

	p := regulator.NewPipeline(
		workDir,
		regulator.NewStructuralStage(),
		regulator.NewNamedStage("mock_boundary", func(ctx context.Context, action regulator.ActionCandidate, workDir string) regulator.PermissionResult {
			boundaryExecuted = true
			return regulator.PermissionResult{Status: regulator.StatusAllowed}
		}),
		regulator.NewNamedStage("mock_shell", func(ctx context.Context, action regulator.ActionCandidate, workDir string) regulator.PermissionResult {
			shellExecuted = true
			return regulator.PermissionResult{Status: regulator.StatusAllowed}
		}),
		regulator.NewSemanticStage(mockSem),
	)

	// Action with null byte fails at Stage 1 (structural)
	action := regulator.ActionCandidate{
		Name:    "write_file",
		Path:    "pkg/\x00malicious.go",
		Command: "<content>test</content>",
	}

	res := p.Regulate(context.Background(), action)

	if res.Status != regulator.StatusBlocked {
		t.Fatalf("expected StatusBlocked from structural stage, got: %s", res.Status)
	}
	if res.Stage != "structural" {
		t.Errorf("expected Stage 'structural', got: %s", res.Stage)
	}
	if !strings.Contains(res.Reason, "null byte detected") {
		t.Errorf("expected null byte reason, got: %s", res.Reason)
	}
	if res.Remediation == "" {
		t.Errorf("expected non-empty remediation guidance")
	}

	// Downstream stages must NOT have executed
	if boundaryExecuted {
		t.Errorf("boundary stage executed despite structural failure")
	}
	if shellExecuted {
		t.Errorf("shell stage executed despite structural failure")
	}
	if mockSem.called {
		t.Errorf("semantic stage executed despite structural failure")
	}
}

func TestPipeline_CostOrderedShortCircuit_BoundaryStage(t *testing.T) {
	workDir := t.TempDir()

	shellExecuted := false
	mockSem := &mockSemanticEvaluator{}

	p := regulator.NewPipeline(
		workDir,
		regulator.NewStructuralStage(),
		regulator.NewBoundaryStage(),
		regulator.NewNamedStage("mock_shell", func(ctx context.Context, action regulator.ActionCandidate, workDir string) regulator.PermissionResult {
			shellExecuted = true
			return regulator.PermissionResult{Status: regulator.StatusAllowed}
		}),
		regulator.NewSemanticStage(mockSem),
	)

	// Action escaping boundary fails at Stage 2 (boundary)
	action := regulator.ActionCandidate{
		Name:    "write_file",
		Path:    "../../etc/passwd",
		Command: "<path>../../etc/passwd</path><content>evil</content>",
	}

	res := p.Regulate(context.Background(), action)

	if res.Status != regulator.StatusBlocked {
		t.Fatalf("expected StatusBlocked from boundary stage, got: %s", res.Status)
	}
	if res.Stage != "boundary" {
		t.Errorf("expected Stage 'boundary', got: %s", res.Stage)
	}
	if !res.OutOfBounds {
		t.Errorf("expected OutOfBounds to be true")
	}
	if !strings.Contains(res.Remediation, "escapes authorized workspace") {
		t.Errorf("expected boundary remediation guidance, got: %s", res.Remediation)
	}

	// Downstream stages must NOT have executed
	if shellExecuted {
		t.Errorf("shell stage executed despite boundary violation")
	}
	if mockSem.called {
		t.Errorf("semantic stage executed despite boundary violation")
	}
}

func TestPipeline_CostOrderedShortCircuit_StaticShellStage(t *testing.T) {
	workDir := t.TempDir()
	mockSem := &mockSemanticEvaluator{}

	p := regulator.DefaultPipeline(workDir, mockSem)

	// Critical shell command (rm -rf /) fails at Stage 3 (static_shell)
	action := regulator.ActionCandidate{
		Name:    "exec_bash",
		Command: "rm -rf /",
	}

	res := p.Regulate(context.Background(), action)

	if res.Status != regulator.StatusBlocked {
		t.Fatalf("expected StatusBlocked for critical shell command, got: %s", res.Status)
	}
	if res.Stage != "static_shell" {
		t.Errorf("expected Stage 'static_shell', got: %s", res.Stage)
	}
	if res.RiskLevel != regulator.RiskLevelCritical {
		t.Errorf("expected RiskLevelCritical, got: %s", res.RiskLevel)
	}
	if !strings.Contains(res.Remediation, "critical destructive signature") {
		t.Errorf("expected shell remediation guidance, got: %s", res.Remediation)
	}

	// Expensive semantic model must NOT have been called
	if mockSem.called {
		t.Errorf("expensive semantic evaluator called despite static shell critical rejection")
	}
}

// ---------------------------------------------------------------------------
// 2. Warning Aggregation & Precedence
// ---------------------------------------------------------------------------

func TestPipeline_WarningPropagation(t *testing.T) {
	workDir := t.TempDir()

	p := regulator.DefaultPipeline(workDir)

	// High risk command (sudo) produces StatusWarning
	action := regulator.ActionCandidate{
		Name:    "exec_bash",
		Command: "sudo apt-get update",
	}

	res := p.Regulate(context.Background(), action)

	if res.Status != regulator.StatusWarning {
		t.Fatalf("expected StatusWarning for destructive git reset, got: %s", res.Status)
	}
	if res.Stage != "static_shell" {
		t.Errorf("expected Stage 'static_shell', got: %s", res.Stage)
	}
	if res.RiskLevel != regulator.RiskLevelHigh {
		t.Errorf("expected RiskLevelHigh, got: %s", res.RiskLevel)
	}
}

func TestPipeline_BlockPreemptsPriorWarning(t *testing.T) {
	workDir := t.TempDir()

	// Pipeline where Stage 1 warns, but Stage 2 blocks
	p := regulator.NewPipeline(
		workDir,
		regulator.NewNamedStage("stage_warn", func(ctx context.Context, action regulator.ActionCandidate, workDir string) regulator.PermissionResult {
			return regulator.PermissionResult{
				Status:    regulator.StatusWarning,
				Reason:    "Early warning",
				RiskLevel: regulator.RiskLevelMedium,
			}
		}),
		regulator.NewNamedStage("stage_block", func(ctx context.Context, action regulator.ActionCandidate, workDir string) regulator.PermissionResult {
			return regulator.PermissionResult{
				Status:      regulator.StatusBlocked,
				Reason:      "Hard block",
				RiskLevel:   regulator.RiskLevelHigh,
				Remediation: "Do not execute",
			}
		}),
	)

	res := p.Regulate(context.Background(), regulator.ActionCandidate{Name: "test"})

	if res.Status != regulator.StatusBlocked {
		t.Fatalf("expected StatusBlocked to take precedence over warning, got: %s", res.Status)
	}
	if res.Stage != "stage_block" {
		t.Errorf("expected Stage 'stage_block', got: %s", res.Stage)
	}
}

// ---------------------------------------------------------------------------
// 3. Stage 4 Semantic Classification Evaluation
// ---------------------------------------------------------------------------

func TestPipeline_SemanticStage_EvaluatesWhenDeterministicPass(t *testing.T) {
	workDir := t.TempDir()

	mockSem := &mockSemanticEvaluator{
		evaluateFunc: func(ctx context.Context, action regulator.ActionCandidate, workDir string) (regulator.PermissionResult, error) {
			return regulator.PermissionResult{
				Status:    regulator.StatusWarning,
				Reason:    "Semantic model flagged potential side-effect",
				RiskLevel: regulator.RiskLevelHigh,
				Target:    action.Command,
			}, nil
		},
	}

	p := regulator.DefaultPipeline(workDir, mockSem)

	// Safe deterministic command that reaches semantic model
	action := regulator.ActionCandidate{
		Name:    "exec_bash",
		Command: "cat sensitive_config.yml",
	}

	res := p.Regulate(context.Background(), action)

	if !mockSem.called {
		t.Fatalf("expected semantic evaluator to be called")
	}
	if res.Status != regulator.StatusWarning {
		t.Errorf("expected StatusWarning from semantic stage, got: %s", res.Status)
	}
	if res.Stage != "semantic" {
		t.Errorf("expected Stage 'semantic', got: %s", res.Stage)
	}
	if !strings.Contains(res.Remediation, "semantic safety model") {
		t.Errorf("expected semantic remediation guidance, got: %s", res.Remediation)
	}
}

// ---------------------------------------------------------------------------
// 4. Custom Functional Stage Extensibility
// ---------------------------------------------------------------------------

func TestPipeline_CustomStageExtensibility(t *testing.T) {
	workDir := t.TempDir()

	// Add a custom organizational policy stage (e.g. forbid editing .github/workflows)
	customStage := regulator.NewNamedStage("ci_policy", func(ctx context.Context, action regulator.ActionCandidate, workDir string) regulator.PermissionResult {
		if strings.Contains(action.Path, ".github/workflows") {
			return regulator.PermissionResult{
				Status:      regulator.StatusBlocked,
				Reason:      "Modification of CI workflow files is forbidden by policy",
				RiskLevel:   regulator.RiskLevelHigh,
				Target:      action.Path,
				Remediation: "Workflow updates require manual administrator pull requests.",
			}
		}
		return regulator.PermissionResult{Status: regulator.StatusAllowed}
	})

	p := regulator.DefaultPipeline(workDir).AddStage(customStage)

	action := regulator.ActionCandidate{
		Name:    "write_file",
		Path:    ".github/workflows/ci.yml",
		Command: "<content>malicious</content>",
	}

	res := p.Regulate(context.Background(), action)

	if res.Status != regulator.StatusBlocked {
		t.Fatalf("expected StatusBlocked by custom CI policy stage, got: %s", res.Status)
	}
	if res.Stage != "ci_policy" {
		t.Errorf("expected Stage 'ci_policy', got: %s", res.Stage)
	}
	if !strings.Contains(res.Reason, "forbidden by policy") {
		t.Errorf("expected policy reason, got: %s", res.Reason)
	}
}
