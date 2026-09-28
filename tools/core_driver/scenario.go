// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
)

// Scenario represents a capability probing scenario loaded from JSONL.
type Scenario struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Prompt              string   `json:"prompt"`
	Mode                string   `json:"mode"`
	MaxWallClockSec     float64  `json:"max_wall_clock_sec"`
	MaxGPUPeggedSec     float64  `json:"max_gpu_pegged_sec"`
	LayaInstructions    string   `json:"laya_instructions"`
	LayaThreshold       float64  `json:"laya_threshold"`
	ForbiddenSubstrings []string `json:"forbidden_substrings"`
	RequiredSubstrings  []string `json:"required_substrings"`
	Description         string   `json:"description"`
}

// ScenarioResult captures the execution and evaluation metrics for a single scenario run.
type ScenarioResult struct {
	Scenario       Scenario
	Passed         bool
	Duration       time.Duration
	Turns          int
	ActionsCount   int
	TokensDecoded  int
	AssistantText  string
	LayaNoul       float64
	LayaQuality    string
	FailureReasons []string
	BeadID         string
}

// LoadScenarios parses scenarios from a JSONL file.
func LoadScenarios(path string) ([]Scenario, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open scenarios file: %w", err)
	}
	defer file.Close()

	var scenarios []Scenario
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var sc Scenario
		if err := json.Unmarshal([]byte(line), &sc); err != nil {
			return nil, fmt.Errorf("parse scenario at line %d: %w", lineNum, err)
		}
		scenarios = append(scenarios, sc)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan scenarios: %w", err)
	}
	return scenarios, nil
}

// ScenarioRunner executes scenarios directly against an agent.SessionCore.
type ScenarioRunner struct {
	Client     *agent.Client
	WorkDir    string
	RepoRoot   string
	FileBeads  bool
	Verbose    bool
	MaxTurns   int
}

// NewScenarioRunner creates a new in-process scenario runner.
func NewScenarioRunner(client *agent.Client, workDir, repoRoot string, fileBeads, verbose bool) *ScenarioRunner {
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	if repoRoot == "" {
		repoRoot = workDir
	}
	return &ScenarioRunner{
		Client:    client,
		WorkDir:   workDir,
		RepoRoot:  repoRoot,
		FileBeads: fileBeads,
		Verbose:   verbose,
		MaxTurns:  10,
	}
}

// RunScenario executes a single scenario in-process using agent.SessionCore.
func (r *ScenarioRunner) RunScenario(ctx context.Context, sc Scenario) *ScenarioResult {
	res := &ScenarioResult{
		Scenario: sc,
		Passed:   true,
	}

	mode, err := agent.ParseMode(sc.Mode)
	if err != nil {
		mode = agent.ModeGeneral
	}

	timeoutSec := sc.MaxWallClockSec
	if timeoutSec <= 0 {
		timeoutSec = 30.0
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec*float64(time.Second)))
	defer cancel()

	// Direct in-process session instantiation
	session := agent.NewSessionWithMode(r.Client, r.WorkDir, mode)
	session.AppendUserMessage(sc.Prompt)

	startTime := time.Now()
	var fullAssistantText strings.Builder
	var fullTranscript strings.Builder
	fullTranscript.WriteString("User: " + sc.Prompt + "\n")

	slotCtx, slotCancel := context.WithTimeout(runCtx, 2*time.Second)
	initialSlot, _ := session.GetSlotStatus(slotCtx)
	slotCancel()
	initialDecoded := 0
	if initialSlot != nil {
		initialDecoded = initialSlot.NDecoded
	}

	turn := 0
	for ; turn < r.MaxTurns; turn++ {
		select {
		case <-runCtx.Done():
			res.Passed = false
			res.FailureReasons = append(res.FailureReasons, fmt.Sprintf("Timeout exceeded (%.1fs)", timeoutSec))
			abortCtx, abortCancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = session.Abort(abortCtx)
			abortCancel()
			goto Evaluation
		default:
		}

		tokenChan := make(chan string, 128)
		errChan := make(chan error, 1)

		go func() {
			_, streamErr := session.StreamTurn(runCtx, tokenChan)
			close(tokenChan)
			errChan <- streamErr
		}()

		var turnReply strings.Builder
		for tok := range tokenChan {
			turnReply.WriteString(tok)
			if r.Verbose {
				fmt.Print(tok)
			}
		}
		if r.Verbose {
			fmt.Println()
		}

		streamErr := <-errChan
		if streamErr != nil {
			if runCtx.Err() != nil {
				res.Passed = false
				res.FailureReasons = append(res.FailureReasons, fmt.Sprintf("Turn timed out after %.1fs", timeoutSec))
				abortCtx, abortCancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = session.Abort(abortCtx)
				abortCancel()
				break
			}
			res.Passed = false
			res.FailureReasons = append(res.FailureReasons, fmt.Sprintf("Stream error: %v", streamErr))
			break
		}

		replyStr := turnReply.String()
		fullAssistantText.WriteString(replyStr)
		fullTranscript.WriteString("Assistant: " + replyStr + "\n")
		session.AppendAssistantMessage(replyStr)

		// Parse action
		act := agent.ParseAction(replyStr)

		// If no action was emitted, the agent concluded its response
		if act == nil {
			break
		}

		res.ActionsCount++
		if act.Name == "task_finish" || act.Name == "finish" {
			break
		}

		// Execute action in-process
		out, execErr := session.ExecuteAction(runCtx, act)
		if r.Verbose {
			fmt.Printf("⚡ [Action %s] out=%q err=%v\n", act.Name, out, execErr)
		}
		session.AppendActionResult(out, execErr)
		fullTranscript.WriteString(fmt.Sprintf("Tool %s: %s (err=%v)\n", act.Name, out, execErr))
	}

Evaluation:
	res.Duration = time.Since(startTime)
	res.Turns = turn + 1
	res.AssistantText = fullAssistantText.String()

	finalSlotCtx, finalSlotCancel := context.WithTimeout(context.Background(), 2*time.Second)
	finalSlot, _ := session.GetSlotStatus(finalSlotCtx)
	finalSlotCancel()
	if finalSlot != nil && finalSlot.NDecoded >= initialDecoded {
		res.TokensDecoded = finalSlot.NDecoded - initialDecoded
	}

	transcriptStr := fullTranscript.String()

	// 1. Check forbidden substrings
	for _, forbidden := range sc.ForbiddenSubstrings {
		if strings.Contains(transcriptStr, forbidden) {
			res.Passed = false
			res.FailureReasons = append(res.FailureReasons, fmt.Sprintf("Encountered forbidden substring: %q", forbidden))
		}
	}

	// 2. Check required substrings
	for _, required := range sc.RequiredSubstrings {
		if !strings.Contains(transcriptStr, required) {
			res.Passed = false
			res.FailureReasons = append(res.FailureReasons, fmt.Sprintf("Missing required substring: %q", required))
		}
	}

	// 3. Laya semantic evaluation
	if sc.LayaInstructions != "" {
		noul, quality, layaErr := r.evaluateLaya(transcriptStr, sc.LayaInstructions)
		res.LayaNoul = noul
		res.LayaQuality = quality
		if layaErr != nil && r.Verbose {
			fmt.Printf("[WARN] Laya evaluation error: %v\n", layaErr)
		}
		if sc.LayaThreshold > 0 && noul < sc.LayaThreshold {
			res.Passed = false
			res.FailureReasons = append(res.FailureReasons, fmt.Sprintf("Laya score %.2f fell below threshold %.2f (%s)", noul, sc.LayaThreshold, quality))
		}
	} else {
		res.LayaNoul = 1.0
		res.LayaQuality = "acceptable"
	}

	// 4. File bead if failed and requested
	if !res.Passed && r.FileBeads {
		beadID := r.fileBead(res)
		res.BeadID = beadID
	}

	return res
}

func (r *ScenarioRunner) evaluateLaya(transcript, instructions string) (float64, string, error) {
	judgeScript := filepath.Join(r.RepoRoot, "tools", "laya", "judge.py")
	if _, err := os.Stat(judgeScript); err != nil {
		return 1.0, "acceptable", nil
	}

	payload := map[string]interface{}{
		"state":        transcript,
		"instructions": instructions,
	}
	inputBytes, err := json.Marshal(payload)
	if err != nil {
		return 1.0, "acceptable", err
	}

	cmd := exec.Command("uv", "run", "--python", ".venv", "tools/laya/judge.py", "--json")
	cmd.Dir = r.RepoRoot
	cmd.Stdin = bytes.NewReader(inputBytes)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return 1.0, "acceptable", fmt.Errorf("judge run error: %w (%s)", err, stderr.String())
	}

	var data struct {
		Passed       bool    `json:"passed"`
		Probability  float64 `json:"probability"`
		QualityLevel float64 `json:"quality_level"`
		Quality      string  `json:"quality"`
		Threshold    float64 `json:"threshold"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		return 1.0, "acceptable", fmt.Errorf("unmarshal judge json: %w", err)
	}

	noul := data.Probability
	quality := data.Quality
	if quality == "" {
		if data.QualityLevel >= 3.5 {
			quality = "flawless"
		} else if data.QualityLevel >= 2.5 {
			quality = "good"
		} else if data.QualityLevel >= 1.5 {
			quality = "acceptable"
		} else if data.QualityLevel >= 0.5 {
			quality = "poor"
		} else {
			quality = "unacceptable"
		}
	}
	return noul, quality, nil
}

func (r *ScenarioRunner) fileBead(res *ScenarioResult) string {
	if _, err := exec.LookPath("bd"); err != nil {
		return ""
	}

	title := fmt.Sprintf("fix(core): capability probe failed on %s", res.Scenario.ID)
	var reasonsMd strings.Builder
	for _, reason := range res.FailureReasons {
		reasonsMd.WriteString(fmt.Sprintf("- %s\n", reason))
	}

	body := fmt.Sprintf("## Automated In-Process Core Driver Failure\n\n"+
		"- **Scenario**: `%s` (%s)\n"+
		"- **Mode**: `%s`\n"+
		"- **Prompt**: %q\n"+
		"- **Duration**: %.2fs\n"+
		"- **Turns**: %d\n"+
		"- **Actions**: %d\n"+
		"- **Tokens Decoded**: %d\n"+
		"- **Laya Score**: %.2f (%s)\n\n"+
		"### Failure Symptoms\n%s\n"+
		"### Assistant Output Excerpt\n```\n%s\n```\n",
		res.Scenario.ID, res.Scenario.Name,
		res.Scenario.Mode,
		res.Scenario.Prompt,
		res.Duration.Seconds(),
		res.Turns,
		res.ActionsCount,
		res.TokensDecoded,
		res.LayaNoul, res.LayaQuality,
		reasonsMd.String(),
		truncateString(res.AssistantText, 1500),
	)

	cmd := exec.Command("bd", "create", title,
		"-t", "bug",
		"-p", "P2",
		"-l", "agent-probe,core-defect,grounding",
		"-d", body,
	)
	cmd.Dir = r.RepoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if strings.Contains(l, "Created issue:") {
			parts := strings.Split(l, "Created issue:")
			if len(parts) > 1 {
				fields := strings.Fields(parts[1])
				if len(fields) > 0 {
					return fields[0]
				}
			}
		}
	}
	return ""
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n...[truncated]"
}
