// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator

import (
	"context"
	"fmt"
)

// SlotMetrics represents real-time context token capacity and utilization metrics from the inference engine.
type SlotMetrics struct {
	NCtx          int  `json:"n_ctx"`
	NPromptTokens int  `json:"n_prompt_tokens"`
	IsProcessing  bool `json:"is_processing,omitempty"`
}

// SlotStatusProvider is an abstract interface for querying real-time inference slot metrics.
type SlotStatusProvider interface {
	GetSlotMetrics(ctx context.Context) (*SlotMetrics, error)
}

// SlotStatusFunc adapts a standalone function to the SlotStatusProvider interface.
type SlotStatusFunc func(ctx context.Context) (*SlotMetrics, error)

func (f SlotStatusFunc) GetSlotMetrics(ctx context.Context) (*SlotMetrics, error) {
	return f(ctx)
}

// Compactor is an abstract interface for triggering proactive context compaction
// when slot pressure reaches the Compaction Band (ADR 0026, ADR 0029, lokol-f78.5).
type Compactor interface {
	Compact(ctx context.Context, metrics *SlotMetrics) error
}

// CompactorFunc adapts a standalone function to the Compactor interface.
type CompactorFunc func(ctx context.Context, metrics *SlotMetrics) error

// Compact implements Compactor.
func (f CompactorFunc) Compact(ctx context.Context, metrics *SlotMetrics) error {
	return f(ctx, metrics)
}

// StubCompactor is a reference compactor implementation that records invocations and supports custom handlers (lokol-f78.5).
type StubCompactor struct {
	CompactedCount int
	OnCompact      func(ctx context.Context, metrics *SlotMetrics) error
}

// Compact implements Compactor.
func (s *StubCompactor) Compact(ctx context.Context, metrics *SlotMetrics) error {
	s.CompactedCount++
	if s.OnCompact != nil {
		return s.OnCompact(ctx, metrics)
	}
	return nil
}

// InferenceSlotGovernorStage regulates execution cadence and triggers proactive context compaction
// based on inference slot memory utilization and token window pressure (ADR 0026).
type InferenceSlotGovernorStage struct {
	provider            SlotStatusProvider
	warningThreshold    float64 // Default 70.0%
	compactionThreshold float64 // Default 85.0%
	compactor           Compactor
}

// SlotGovernorOption configures an InferenceSlotGovernorStage instance.
type SlotGovernorOption func(*InferenceSlotGovernorStage)

// WithWarningThreshold overrides the default 70% warning threshold.
func WithWarningThreshold(pct float64) SlotGovernorOption {
	return func(s *InferenceSlotGovernorStage) {
		if pct > 0 && pct <= 100 {
			s.warningThreshold = pct
		}
	}
}

// WithCompactionThreshold overrides the default 85% compaction threshold.
func WithCompactionThreshold(pct float64) SlotGovernorOption {
	return func(s *InferenceSlotGovernorStage) {
		if pct > 0 && pct <= 100 {
			s.compactionThreshold = pct
		}
	}
}

// WithCompactor registers a context compactor to be invoked when slot pressure reaches the Compaction Band.
func WithCompactor(c Compactor) SlotGovernorOption {
	return func(s *InferenceSlotGovernorStage) {
		s.compactor = c
	}
}

// NewInferenceSlotGovernorStage creates an initialized slot pressure governor stage per ADR 0026.
func NewInferenceSlotGovernorStage(provider SlotStatusProvider, opts ...SlotGovernorOption) *InferenceSlotGovernorStage {
	stage := &InferenceSlotGovernorStage{
		provider:            provider,
		warningThreshold:    70.0,
		compactionThreshold: 85.0,
	}
	for _, opt := range opts {
		opt(stage)
	}

	if stage.compactionThreshold <= stage.warningThreshold {
		stage.compactionThreshold = stage.warningThreshold + 10.0
		if stage.compactionThreshold > 100.0 {
			stage.compactionThreshold = 100.0
		}
	}

	return stage
}

// NewSlotGovernorStage is an alias for NewInferenceSlotGovernorStage.
func NewSlotGovernorStage(provider SlotStatusProvider, opts ...SlotGovernorOption) *InferenceSlotGovernorStage {
	return NewInferenceSlotGovernorStage(provider, opts...)
}

func (s *InferenceSlotGovernorStage) Name() string {
	return "slot_governor"
}

// WarningThreshold returns the configured warning threshold percentage.
func (s *InferenceSlotGovernorStage) WarningThreshold() float64 {
	return s.warningThreshold
}

// CompactionThreshold returns the configured compaction threshold percentage.
func (s *InferenceSlotGovernorStage) CompactionThreshold() float64 {
	return s.compactionThreshold
}

// Evaluate inspects the active inference slot metrics against tiered thresholds.
func (s *InferenceSlotGovernorStage) Evaluate(ctx context.Context, action ActionCandidate, workDir string) PermissionResult {
	if s.provider == nil {
		return PermissionResult{
			Status:    StatusAllowed,
			RiskLevel: RiskLevelNone,
			Stage:     s.Name(),
		}
	}

	metrics, err := s.provider.GetSlotMetrics(ctx)
	if err != nil {
		return PermissionResult{
			Status:      StatusWarning,
			Reason:      fmt.Sprintf("Inference slot metrics query failed: %v", err),
			RiskLevel:   RiskLevelLow,
			Target:      action.Name,
			Stage:       s.Name(),
			Remediation: "Engine slot endpoint did not respond. Check engine server health if context issues persist.",
		}
	}

	if metrics == nil || metrics.NCtx <= 0 {
		return PermissionResult{
			Status:      StatusWarning,
			Reason:      "Slot metrics unavailable (NCtx=0), proceeding with caution",
			RiskLevel:   RiskLevelLow,
			Target:      action.Name,
			Stage:       s.Name(),
			Remediation: "Engine slot reported zero or unavailable context window. Verify inference engine initialization if unexpected.",
		}
	}

	utilPct := (float64(metrics.NPromptTokens) / float64(metrics.NCtx)) * 100.0

	// 1. Compaction Band (>= compactionThreshold, default 85%)
	if utilPct >= s.compactionThreshold {
		if s.compactor != nil {
			if err := s.compactor.Compact(ctx, metrics); err == nil {
				return PermissionResult{
					Status:      StatusWarning,
					Reason:      fmt.Sprintf("Inference slot context pressure reached %.1f%% (%d/%d tokens); proactive context compaction triggered successfully", utilPct, metrics.NPromptTokens, metrics.NCtx),
					RiskLevel:   RiskLevelMedium,
					Target:      action.Name,
					Stage:       s.Name(),
					Remediation: "Context compaction succeeded. Older turns pruned or summarized; execution continuing with refreshed budget.",
				}
			}
		}

		return PermissionResult{
			Status:      StatusBlocked,
			Reason:      fmt.Sprintf("Inference slot context pressure critical: %.1f%% utilization (%d/%d tokens) exceeds compaction threshold (%.1f%%)", utilPct, metrics.NPromptTokens, metrics.NCtx, s.compactionThreshold),
			RiskLevel:   RiskLevelHigh,
			Target:      action.Name,
			Stage:       s.Name(),
			Remediation: fmt.Sprintf("Context budget exhausted (%.1f%% full). Proactively halt action sequence and trigger context compaction or conversation summarization before engine truncation.", utilPct),
		}
	}

	// 2. Warning Band (>= warningThreshold, default 70%)
	if utilPct >= s.warningThreshold {
		return PermissionResult{
			Status:      StatusWarning,
			Reason:      fmt.Sprintf("Inference slot context pressure high: %.1f%% utilization (%d/%d tokens) exceeds warning threshold (%.1f%%)", utilPct, metrics.NPromptTokens, metrics.NCtx, s.warningThreshold),
			RiskLevel:   RiskLevelMedium,
			Target:      action.Name,
			Stage:       s.Name(),
			Remediation: fmt.Sprintf("Context budget is approaching capacity (%.1f%% full). Restrict non-essential diagnostic tool emissions and prepare to compact context.", utilPct),
		}
	}

	// 3. Nominal Band (< warningThreshold)
	return PermissionResult{
		Status:    StatusAllowed,
		Reason:    fmt.Sprintf("Context utilization nominal: %.1f%% (%d/%d tokens)", utilPct, metrics.NPromptTokens, metrics.NCtx),
		RiskLevel: RiskLevelNone,
		Stage:     s.Name(),
	}
}
