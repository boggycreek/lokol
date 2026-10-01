// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/boggycreek/lokol/liblokol/regulator"
)

type mockSlotProvider struct {
	metrics *regulator.SlotMetrics
	err     error
	calls   int
}

func (m *mockSlotProvider) GetSlotMetrics(ctx context.Context) (*regulator.SlotMetrics, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.metrics, nil
}

// ---------------------------------------------------------------------------
// 1. Tiered Operational Bands (ADR 0026)
// ---------------------------------------------------------------------------

func TestSlotGovernor_NominalBand(t *testing.T) {
	provider := &mockSlotProvider{
		metrics: &regulator.SlotMetrics{
			NCtx:          8192,
			NPromptTokens: 4096, // 50.0% utilization (< 70% warning threshold)
		},
	}

	stage := regulator.NewInferenceSlotGovernorStage(provider)
	ctx := context.Background()
	action := regulator.ActionCandidate{Name: "read_window", Path: "main.go"}

	res := stage.Evaluate(ctx, action, ".")
	if res.Status != regulator.StatusAllowed {
		t.Errorf("expected StatusAllowed for 50%% utilization, got: %s", res.Status)
	}
	if res.RiskLevel != regulator.RiskLevelNone {
		t.Errorf("expected RiskLevelNone, got: %s", res.RiskLevel)
	}
	if !strings.Contains(res.Reason, "nominal") {
		t.Errorf("expected nominal reason, got: %s", res.Reason)
	}
}

func TestSlotGovernor_WarningBand(t *testing.T) {
	provider := &mockSlotProvider{
		metrics: &regulator.SlotMetrics{
			NCtx:          10000,
			NPromptTokens: 7500, // 75.0% utilization (>= 70% and < 85%)
		},
	}

	stage := regulator.NewInferenceSlotGovernorStage(provider)
	ctx := context.Background()
	action := regulator.ActionCandidate{Name: "exec_bash", Command: "find ."}

	res := stage.Evaluate(ctx, action, ".")
	if res.Status != regulator.StatusWarning {
		t.Errorf("expected StatusWarning for 75%% utilization, got: %s", res.Status)
	}
	if res.RiskLevel != regulator.RiskLevelMedium {
		t.Errorf("expected RiskLevelMedium, got: %s", res.RiskLevel)
	}
	if !strings.Contains(res.Reason, "exceeds warning threshold") {
		t.Errorf("expected warning reason, got: %s", res.Reason)
	}
	if res.Remediation == "" || !strings.Contains(res.Remediation, "compact") {
		t.Errorf("expected actionable remediation guiding compaction, got: %q", res.Remediation)
	}
}

func TestSlotGovernor_CompactionBand(t *testing.T) {
	provider := &mockSlotProvider{
		metrics: &regulator.SlotMetrics{
			NCtx:          10000,
			NPromptTokens: 8800, // 88.0% utilization (>= 85% compaction threshold)
		},
	}

	stage := regulator.NewInferenceSlotGovernorStage(provider)
	ctx := context.Background()
	action := regulator.ActionCandidate{Name: "write_file", Path: "out.go"}

	res := stage.Evaluate(ctx, action, ".")
	if res.Status != regulator.StatusBlocked {
		t.Errorf("expected StatusBlocked for 88%% utilization, got: %s", res.Status)
	}
	if res.RiskLevel != regulator.RiskLevelHigh {
		t.Errorf("expected RiskLevelHigh, got: %s", res.RiskLevel)
	}
	if !strings.Contains(res.Reason, "critical") || !strings.Contains(res.Reason, "exceeds compaction threshold") {
		t.Errorf("expected critical compaction reason, got: %s", res.Reason)
	}
	if res.Remediation == "" || !strings.Contains(res.Remediation, "halt action sequence") {
		t.Errorf("expected compaction remediation directive, got: %q", res.Remediation)
	}
}

// ---------------------------------------------------------------------------
// 2. Custom Threshold Configuration
// ---------------------------------------------------------------------------

func TestSlotGovernor_CustomThresholds(t *testing.T) {
	provider := &mockSlotProvider{
		metrics: &regulator.SlotMetrics{
			NCtx:          1000,
			NPromptTokens: 600, // 60.0%
		},
	}

	// Custom: 50% warning, 75% compaction
	stage := regulator.NewSlotGovernorStage(
		provider,
		regulator.WithWarningThreshold(50.0),
		regulator.WithCompactionThreshold(75.0),
	)

	if stage.WarningThreshold() != 50.0 {
		t.Errorf("expected WarningThreshold 50.0, got: %f", stage.WarningThreshold())
	}
	if stage.CompactionThreshold() != 75.0 {
		t.Errorf("expected CompactionThreshold 75.0, got: %f", stage.CompactionThreshold())
	}

	ctx := context.Background()
	action := regulator.ActionCandidate{Name: "read_outline", Path: "main.go"}

	// 60% with 50% warning threshold -> StatusWarning
	res := stage.Evaluate(ctx, action, ".")
	if res.Status != regulator.StatusWarning {
		t.Errorf("expected StatusWarning, got: %s", res.Status)
	}

	// Increase to 80% (>= 75% compaction threshold) -> StatusBlocked
	provider.metrics.NPromptTokens = 800
	res2 := stage.Evaluate(ctx, action, ".")
	if res2.Status != regulator.StatusBlocked {
		t.Errorf("expected StatusBlocked, got: %s", res2.Status)
	}
}

// ---------------------------------------------------------------------------
// 3. Fallbacks, Errors & Boundary Conditions
// ---------------------------------------------------------------------------

func TestSlotGovernor_NilProvider(t *testing.T) {
	stage := regulator.NewInferenceSlotGovernorStage(nil)
	res := stage.Evaluate(context.Background(), regulator.ActionCandidate{Name: "find_files"}, ".")
	if res.Status != regulator.StatusAllowed {
		t.Errorf("expected StatusAllowed for nil provider, got: %s", res.Status)
	}
}

func TestSlotGovernor_ProviderErrorFailsSafely(t *testing.T) {
	provider := &mockSlotProvider{
		err: errors.New("connection refused to inference daemon"),
	}

	stage := regulator.NewInferenceSlotGovernorStage(provider)
	res := stage.Evaluate(context.Background(), regulator.ActionCandidate{Name: "find_files"}, ".")
	if res.Status != regulator.StatusWarning {
		t.Errorf("expected StatusWarning on provider error, got: %s", res.Status)
	}
	if res.RiskLevel != regulator.RiskLevelLow {
		t.Errorf("expected RiskLevelLow, got: %s", res.RiskLevel)
	}
	if !strings.Contains(res.Reason, "query failed") {
		t.Errorf("expected query failed reason, got: %s", res.Reason)
	}
}

func TestSlotGovernor_ZeroAndNegativeContext(t *testing.T) {
	provider := &mockSlotProvider{
		metrics: &regulator.SlotMetrics{
			NCtx:          0,
			NPromptTokens: 100,
		},
	}

	stage := regulator.NewInferenceSlotGovernorStage(provider)
	res := stage.Evaluate(context.Background(), regulator.ActionCandidate{Name: "read_window"}, ".")
	if res.Status != regulator.StatusAllowed {
		t.Errorf("expected StatusAllowed for NCtx=0, got: %s", res.Status)
	}
}

// ---------------------------------------------------------------------------
// 4. Pipeline Integration & Short Circuiting (ADR 0023 + ADR 0026)
// ---------------------------------------------------------------------------

func TestSlotGovernor_PipelineShortCircuitBeforeSemantic(t *testing.T) {
	provider := &mockSlotProvider{
		metrics: &regulator.SlotMetrics{
			NCtx:          1000,
			NPromptTokens: 900, // 90% (>= 85% compaction threshold)
		},
	}

	semEvaluator := &mockSemanticEvaluator{}

	pipeline := regulator.DefaultPipelineWithSlot(".", semEvaluator, provider)
	ctx := context.Background()
	action := regulator.ActionCandidate{Name: "write_file", Path: "pkg/gen.go", Command: "data"}

	res := pipeline.Regulate(ctx, action)
	if res.Status != regulator.StatusBlocked {
		t.Errorf("expected pipeline to block on critical slot pressure, got: %s", res.Status)
	}
	if res.Stage != "slot_governor" {
		t.Errorf("expected stage to be slot_governor, got: %s", res.Stage)
	}
	if semEvaluator.called {
		t.Errorf("expected expensive semantic stage to be skipped due to slot governor short-circuit")
	}
}

func TestSlotGovernor_FunctionAdapter(t *testing.T) {
	called := false
	adapter := regulator.SlotStatusFunc(func(ctx context.Context) (*regulator.SlotMetrics, error) {
		called = true
		return &regulator.SlotMetrics{NCtx: 2000, NPromptTokens: 100}, nil
	})

	stage := regulator.NewInferenceSlotGovernorStage(adapter)
	res := stage.Evaluate(context.Background(), regulator.ActionCandidate{Name: "run_test"}, ".")
	if !called {
		t.Errorf("expected SlotStatusFunc adapter to be called")
	}
	if res.Status != regulator.StatusAllowed {
		t.Errorf("expected StatusAllowed, got: %s", res.Status)
	}
}
