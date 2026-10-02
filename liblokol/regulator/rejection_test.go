// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator_test

import (
	"context"
	"testing"

	"github.com/boggycreek/lokol/liblokol/regulator"
)

func TestUserRejectionStage_BlockRejectedActions(t *testing.T) {
	stage := regulator.NewUserRejectionStage()
	ctx := context.Background()

	// 1. Initial state: action is allowed
	candidate := regulator.ActionCandidate{
		Name:    "read_window",
		Path:    "README.md",
		Command: "<path>README.md</path><start_line>101</start_line><end_line>200</end_line>",
	}

	res := stage.Evaluate(ctx, candidate, ".")
	if res.Status != regulator.StatusAllowed {
		t.Fatalf("expected allowed before rejection, got %s", res.Status)
	}

	// 2. Record operator rejection
	stage.RecordRejection(candidate)
	if !stage.HasRejection(candidate) {
		t.Fatalf("expected stage to have rejection for candidate")
	}

	// 3. Candidate should now be blocked
	res = stage.Evaluate(ctx, candidate, ".")
	if res.Status != regulator.StatusBlocked {
		t.Fatalf("expected blocked after rejection, got %s (reason: %s)", res.Status, res.Reason)
	}
	if res.RiskLevel != regulator.RiskLevelHigh {
		t.Errorf("expected high risk, got %s", res.RiskLevel)
	}
	if res.Remediation == "" {
		t.Errorf("expected remediation guidance in result")
	}

	// 4. Same target path with different command range should also be blocked
	altCandidate := regulator.ActionCandidate{
		Name:    "read_window",
		Path:    "README.md",
		Command: "<path>README.md</path><start_line>1</start_line><end_line>100</end_line>",
	}
	res = stage.Evaluate(ctx, altCandidate, ".")
	if res.Status != regulator.StatusBlocked {
		t.Fatalf("expected blocked for same file target, got %s", res.Status)
	}

	// 5. Different file should still be allowed
	otherCandidate := regulator.ActionCandidate{
		Name:    "read_window",
		Path:    "go.mod",
		Command: "<path>go.mod</path><start_line>1</start_line><end_line>20</end_line>",
	}
	res = stage.Evaluate(ctx, otherCandidate, ".")
	if res.Status != regulator.StatusAllowed {
		t.Fatalf("expected other file to be allowed, got %s", res.Status)
	}

	// 6. Explicit re-request via ReconcilePrompt unblocks the file
	stage.ReconcilePrompt("Please go ahead and inspect README.md")
	if stage.HasRejection(candidate) {
		t.Fatalf("expected rejection to be cleared after user explicitly requested README.md")
	}

	res = stage.Evaluate(ctx, candidate, ".")
	if res.Status != regulator.StatusAllowed {
		t.Fatalf("expected allowed after reconciliation, got %s", res.Status)
	}
}

func TestRegulator_PipelineRejectionIntegration(t *testing.T) {
	r := regulator.New(".")
	ctx := context.Background()

	act := regulator.ActionCandidate{
		Name:    "exec_bash",
		Command: "rm -rf /tmp/testfile",
	}

	// Initially allowed through pipeline (assuming not in critical blocklist)
	res := r.CheckPermission(ctx, act)
	if res.Status == regulator.StatusBlocked {
		t.Fatalf("expected not blocked initially, got %s", res.Reason)
	}

	// Record rejection
	r.RecordRejection(act)

	// Now check permission through full pipeline
	res = r.CheckPermission(ctx, act)
	if res.Status != regulator.StatusBlocked {
		t.Fatalf("expected pipeline to block rejected action, got %s", res.Status)
	}

	// Clear rejections
	r.ClearRejections()
	res = r.CheckPermission(ctx, act)
	if res.Status == regulator.StatusBlocked {
		t.Fatalf("expected unblocked after ClearRejections, got %s", res.Status)
	}
}
