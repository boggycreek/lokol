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

func TestUserRejectionStage_RecordRejection(t *testing.T) {
	ctx := context.Background()

	t.Run("record with direct action.Path", func(t *testing.T) {
		stage := regulator.NewUserRejectionStage()
		candidate := regulator.ActionCandidate{
			Name: "write_file",
			Path: "  sub/../config.json  ",
		}

		stage.RecordRejection(candidate)

		// Target should be normalized
		normalizedCandidate := regulator.ActionCandidate{
			Name: "write_file",
			Path: "config.json",
		}
		if !stage.HasRejection(normalizedCandidate) {
			t.Errorf("expected stage to match rejection for normalized path")
		}

		res := stage.Evaluate(ctx, normalizedCandidate, ".")
		if res.Status != regulator.StatusBlocked {
			t.Errorf("expected candidate to be blocked, got %s", res.Status)
		}
		if res.Target != "config.json" {
			t.Errorf("expected target 'config.json', got %q", res.Target)
		}
	})

	t.Run("record with empty action.Path and command containing path XML tag", func(t *testing.T) {
		stage := regulator.NewUserRejectionStage()
		candidate := regulator.ActionCandidate{
			Name:    "replace_file",
			Path:    "",
			Command: "<path>src/index.ts</path><target>old</target><replacement>new</replacement>",
		}

		stage.RecordRejection(candidate)

		// Should match candidate with extracted path
		queryCandidate := regulator.ActionCandidate{
			Name: "read_window",
			Path: "src/index.ts",
		}
		if !stage.HasRejection(queryCandidate) {
			t.Errorf("expected stage to have rejection for target extracted from command XML")
		}

		res := stage.Evaluate(ctx, queryCandidate, ".")
		if res.Status != regulator.StatusBlocked {
			t.Errorf("expected queryCandidate to be blocked, got %s", res.Status)
		}
		if res.Target != "src/index.ts" {
			t.Errorf("expected target 'src/index.ts', got %q", res.Target)
		}
	})

	t.Run("record with empty action.Path and command without path tag", func(t *testing.T) {
		stage := regulator.NewUserRejectionStage()
		candidate := regulator.ActionCandidate{
			Name:    "exec_bash",
			Path:    "",
			Command: "rm -rf /tmp/scratch",
		}

		stage.RecordRejection(candidate)

		if !stage.HasRejection(candidate) {
			t.Errorf("expected stage to have rejection for command-only action")
		}

		// Different command should not match
		diffCandidate := regulator.ActionCandidate{
			Name:    "exec_bash",
			Path:    "",
			Command: "ls -la /tmp/scratch",
		}
		if stage.HasRejection(diffCandidate) {
			t.Errorf("expected different command to not be rejected")
		}

		// Exactly identical command should be blocked
		res := stage.Evaluate(ctx, candidate, ".")
		if res.Status != regulator.StatusBlocked {
			t.Errorf("expected exact command to be blocked, got %s", res.Status)
		}
		// Target defaults to action.Name when target is empty
		if res.Target != "exec_bash" {
			t.Errorf("expected display target to fallback to action name 'exec_bash', got %q", res.Target)
		}
	})

	t.Run("record with completely empty candidate", func(t *testing.T) {
		stage := regulator.NewUserRejectionStage()
		candidate := regulator.ActionCandidate{
			Name:    "custom_tool",
			Path:    "",
			Command: "",
		}

		stage.RecordRejection(candidate)
		if !stage.HasRejection(candidate) {
			t.Errorf("expected empty candidate to be recorded and match")
		}

		res := stage.Evaluate(ctx, candidate, ".")
		if res.Status != regulator.StatusBlocked {
			t.Errorf("expected empty candidate to be blocked, got %s", res.Status)
		}
	})

	t.Run("concurrency safety across multiple goroutines", func(t *testing.T) {
		stage := regulator.NewUserRejectionStage()
		const goroutines = 20
		const iterations = 50

		done := make(chan bool)
		for i := 0; i < goroutines; i++ {
			go func(id int) {
				for j := 0; j < iterations; j++ {
					act := regulator.ActionCandidate{
						Name:    "write_file",
						Path:    "file.go",
						Command: "<path>file.go</path>",
					}
					stage.RecordRejection(act)
					_ = stage.HasRejection(act)
					_ = stage.Evaluate(ctx, act, ".")
				}
				done <- true
			}(i)
		}

		for i := 0; i < goroutines; i++ {
			<-done
		}

		query := regulator.ActionCandidate{Name: "write_file", Path: "file.go"}
		if !stage.HasRejection(query) {
			t.Errorf("expected stage to have rejection after concurrent writes")
		}
	})
}
