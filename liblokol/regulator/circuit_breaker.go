// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TrajectoryEntry represents an action evaluated in the trajectory history.
type TrajectoryEntry struct {
	Signature string
	Action    ActionCandidate
	Timestamp time.Time
}

// LoopCircuitBreakerStage tracks action trajectories and halts repetitive execution loops (ADR 0025).
type LoopCircuitBreakerStage struct {
	mu             sync.Mutex
	history        []TrajectoryEntry
	windowSize     int
	repeatLimit    int
}

// NewLoopCircuitBreakerStage creates an initialized circuit breaker with standard thresholds.
func NewLoopCircuitBreakerStage() *LoopCircuitBreakerStage {
	return &LoopCircuitBreakerStage{
		windowSize:  30,
		repeatLimit: 4,
	}
}

func (s *LoopCircuitBreakerStage) Name() string {
	return "loop_circuit_breaker"
}

// Reset clears the trajectory history for a new session.
func (s *LoopCircuitBreakerStage) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
}

// ComputeSignature normalizes an action candidate into a canonical semantic signature.
func (s *LoopCircuitBreakerStage) ComputeSignature(action ActionCandidate) string {
	normName := strings.ToLower(strings.TrimSpace(action.Name))
	normCmd := strings.Join(strings.Fields(action.Command), " ")
	normPath := strings.ToLower(strings.TrimSpace(action.Path))

	raw := fmt.Sprintf("%s|%s|%s", normName, normPath, normCmd)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:8])
}

// Evaluate analyzes the candidate action against recent trajectory history to detect loops or oscillation.
func (s *LoopCircuitBreakerStage) Evaluate(ctx context.Context, action ActionCandidate, workDir string) PermissionResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	sig := s.ComputeSignature(action)

	// 1. Check for consecutive identical repetitions
	consecutive := 0
	for i := len(s.history) - 1; i >= 0; i-- {
		if s.history[i].Signature == sig {
			consecutive++
		} else {
			break
		}
	}

	// Turn 4+ (consecutive >= 3 prior + current candidate = 4th attempt): Trip breaker
	if consecutive >= s.repeatLimit-1 {
		return PermissionResult{
			Status:      StatusBlocked,
			Reason:      fmt.Sprintf("Circuit breaker tripped: action %s repeated %d times consecutively without progress", action.Name, consecutive+1),
			RiskLevel:   RiskLevelCritical,
			Target:      action.Name,
			Stage:       s.Name(),
			Remediation: fmt.Sprintf("Autonomous loop halted to prevent resource exhaustion. You have executed this EXACT action %d times. Shift strategy entirely, inspect alternate files, or verify with run_test.", consecutive+1),
		}
	}

	// Turn 3 (consecutive == 2): High-risk strategy shift directive
	if consecutive == 2 {
		s.record(sig, action)
		return PermissionResult{
			Status:      StatusWarning,
			Reason:      fmt.Sprintf("Loop warning: action %s attempted 3 times consecutively", action.Name),
			RiskLevel:   RiskLevelHigh,
			Target:      action.Name,
			Stage:       s.Name(),
			Remediation: fmt.Sprintf("You have executed this exact action 3 times consecutively. DO NOT repeat it again. Change tools or modify arguments."),
		}
	}

	// Turn 2 (consecutive == 1): Medium guidance injection
	if consecutive == 1 {
		s.record(sig, action)
		return PermissionResult{
			Status:      StatusWarning,
			Reason:      fmt.Sprintf("Loop warning: action %s repeated consecutively", action.Name),
			RiskLevel:   RiskLevelMedium,
			Target:      action.Name,
			Stage:       s.Name(),
			Remediation: "Consider adjusting parameters or verifying output before re-executing identical commands.",
		}
	}

	// 2. Check for ping-pong state oscillation (A -> B -> A -> B -> A)
	if len(s.history) >= 4 {
		h := s.history
		n := len(h)
		if sig == h[n-2].Signature && h[n-1].Signature == h[n-3].Signature && sig != h[n-1].Signature {
			s.record(sig, action)
			return PermissionResult{
				Status:      StatusWarning,
				Reason:      "State oscillation detected: alternating cyclically between identical actions",
				RiskLevel:   RiskLevelHigh,
				Target:      action.Name,
				Stage:       s.Name(),
				Remediation: "Break out of cyclic alternation. Execute a code modification or test verification tool instead of repeatedly inspecting alternating files.",
			}
		}
	}

	s.record(sig, action)
	return PermissionResult{
		Status:    StatusAllowed,
		RiskLevel: RiskLevelNone,
		Stage:     s.Name(),
	}
}

func (s *LoopCircuitBreakerStage) record(sig string, action ActionCandidate) {
	s.history = append(s.history, TrajectoryEntry{
		Signature: sig,
		Action:    action,
		Timestamp: time.Now(),
	})
	if len(s.history) > s.windowSize {
		s.history = s.history[len(s.history)-s.windowSize:]
	}
}
