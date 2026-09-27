// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// LayaEvaluationResult holds the output of the Laya non-autoregressive decision model.
type LayaEvaluationResult struct {
	Passed       bool    `json:"passed"`
	Probability  float64 `json:"probability"`
	QualityLevel float64 `json:"quality_level"`
	Threshold    float64 `json:"threshold"`
}


// checkLayaAvailable verifies uv and tools/laya/judge.py are present and ready.
func checkLayaAvailable(repoRoot string) bool {
	judgeScript := filepath.Join(repoRoot, "tools", "laya", "judge.py")
	if _, err := os.Stat(judgeScript); err != nil {
		return false
	}
	cmd := exec.Command("uv", "run", "--python", ".venv", "tools/laya/judge.py", "--health")
	cmd.Dir = repoRoot
	return cmd.Run() == nil
}

// runLayaJudge invokes the local Laya judge model via uv to evaluate agent output.
func runLayaJudge(t *testing.T, repoRoot, state, instructions string, threshold float64) (*LayaEvaluationResult, error) {
	judgeScript := filepath.Join(repoRoot, "tools", "laya", "judge.py")
	payload := map[string]interface{}{
		"state":        state,
		"instructions": instructions,
		"threshold":    threshold,
	}
	inputBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal laya payload: %w", err)
	}

	cmd := exec.Command("uv", "run", "--python", ".venv", judgeScript, "--json")
	cmd.Dir = repoRoot
	cmd.Stdin = bytes.NewReader(inputBytes)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("laya judge returned no stdout (err=%v, stderr=%s)", runErr, stderr.String())
	}

	var res LayaEvaluationResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("unmarshal laya output: %w (stdout: %s, stderr: %s)", err, stdout.String(), stderr.String())
	}

	return &res, nil
}
