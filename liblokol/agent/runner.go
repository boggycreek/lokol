// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/boggycreek/lokol/liblokol/regulator"
)

// Runner manages an autonomous execution loop (e.g. for batch execution or self-improvement).
type Runner struct {
	Client          *Client
	MaxTurns        int
	YOLO            bool
	WorkDir         string // Current working directory for host environment and prompt context
	Mode            Mode   // Operational mode (defaults to ModeCoding for autonomous execution)
	CodebaseContext string // Optional pre-loaded codebase context
	Session         *Session
	Regulator       *regulator.Regulator // Persistent regulator instance across turns (lokol-gml.9)
	OnOutput        func(role, content string)
}

// Run executes an autonomous loop on a given user prompt.
func (r *Runner) Run(ctx context.Context, initialPrompt string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	workDir := r.WorkDir
	if workDir == "" {
		workDir = "."
	}

	if r.Regulator == nil {
		r.Regulator = regulator.New(workDir)
	}

	session := r.Session
	if session == nil {
		mode := r.Mode
		if mode == "" {
			mode = ModeCoding
		}
		session = NewSessionWithMode(r.Client, workDir, mode, r.CodebaseContext)
	}
	session.AppendUserMessage(initialPrompt)

	if r.MaxTurns <= 0 {
		r.MaxTurns = 20
	}

	consecutiveNoAction := 0
	lastSig := ""
	repeatCount := 0
	editFailures := make(map[string]int)

	for turn := 0; turn < r.MaxTurns; turn++ {
		tokenChan := make(chan string, 100)
		var assistantReply strings.Builder

		errChan := make(chan error, 1)
		go func() {
			_, err := session.StreamTurn(ctx, tokenChan)
			close(tokenChan)
			errChan <- err
		}()

		var pendingTokens strings.Builder
		suppressTokens := false

		for token := range tokenChan {
			assistantReply.WriteString(token)
			if suppressTokens {
				continue
			}

			pendingTokens.WriteString(token)
			str := pendingTokens.String()

			if strings.Contains(str, "<action") {
				suppressTokens = true
				continue
			}

			// If str ends with a prefix of "<action", wait for next tokens before printing
			if strings.HasSuffix(str, "<") ||
				strings.HasSuffix(str, "<a") ||
				strings.HasSuffix(str, "<ac") ||
				strings.HasSuffix(str, "<act") ||
				strings.HasSuffix(str, "<acti") ||
				strings.HasSuffix(str, "<actio") {
				continue
			}

			if r.OnOutput != nil {
				r.OnOutput("token", str)
			}
			pendingTokens.Reset()
		}

		if err := <-errChan; err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", fmt.Errorf("error during turn %d: %w", turn+1, err)
		}

		replyText := assistantReply.String()
		session.AppendAssistantMessage(replyText)

		act := ParseAction(replyText)
		if act == nil {
			consecutiveNoAction++
			if consecutiveNoAction >= 2 || turn == r.MaxTurns-1 {
				// No action proposed twice in a row; conversational turn done
				return replyText, nil
			}
			// Nudge agent to execute an action in autonomous mode
			session.AppendUserMessage("Please execute your next action using an <action name=\"...\"> tag, or call <action name=\"task_finish\"> if your task is complete.")
			continue
		}
		consecutiveNoAction = 0

		if act.Name == "task_finish" {
			if r.OnOutput != nil {
				r.OnOutput("finish", act.Command)
			}
			return act.Command, nil
		}

		currentSig := act.Name + ":" + strings.TrimSpace(act.Command)
		if currentSig == lastSig {
			repeatCount++
		} else {
			lastSig = currentSig
			repeatCount = 1
		}

		if repeatCount >= 4 {
			return "", fmt.Errorf("agent aborted: loop detected (action %s repeated %d times consecutively without progress)", act.Name, repeatCount)
		}

		targetSummary := act.TargetSummary()
		if r.OnOutput != nil {
			r.OnOutput(act.Name, targetSummary)
		}

		// Enforce regulator permissions using persistent regulator instance (lokol-gml.9)
		actionPath := targetSummary
		if act.Name == "find_files" || act.Name == "search_code" {
			actionPath = regulator.ExtractTagContent(act.Command, "path")
		}

		perm := r.Regulator.CheckPermission(ctx, regulator.ActionCandidate{
			Name:    act.Name,
			Command: act.Command,
			Path:    actionPath,
		})

		// Security: In autonomous mode, block both StatusBlocked and RiskLevelHigh warnings (lokol-gml.1)
		if perm.Status == regulator.StatusBlocked || (r.YOLO && perm.RiskLevel == regulator.RiskLevelHigh) {
			if r.OnOutput != nil {
				r.OnOutput("regulator", fmt.Sprintf("[PERMISSION DENIED] %s", perm.Reason))
			}
			msg := fmt.Sprintf("[PERMISSION DENIED]: %s", perm.Reason)
			if perm.Remediation != "" {
				msg += fmt.Sprintf("\n[REMEDIATION]: %s", perm.Remediation)
			}
			toolResult := fmt.Sprintf("<action_result>\n%s\n</action_result>", msg)
			session.AppendUserMessage(toolResult)
			continue
		}

		out, err := session.ExecuteAction(ctx, act)

		// Format reciprocal action_result with loop intervention heuristics
		var toolResult string
		if err != nil {
			switch act.Name {
			case "exec_bash":
				toolResult = fmt.Sprintf("<action_result>\n[Exit error: %v]\n%s\n</action_result>", err, out)
				if repeatCount >= 2 {
					toolResult += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have executed this EXACT command %d times consecutively and it failed. DO NOT repeat the same command. Change your strategy or inspect the environment.]", repeatCount)
				}
			case "replace_file":
				path := targetSummary
				editFailures[path]++
				toolResult = fmt.Sprintf("<action_result>\n[Edit error: %v]\n</action_result>", err)
				if repeatCount >= 2 {
					toolResult += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have attempted this EXACT edit %d times consecutively and it failed. The target text was not found in %s. DO NOT repeat this action. Use <action name=\"read_window\"> to inspect lines in %s, or use <action name=\"write_file\"> to rewrite %s completely with the updated content.]", repeatCount, path, path, path)
				} else if editFailures[path] >= 2 {
					toolResult += fmt.Sprintf("\n\n[SYSTEM HINT: Exact block replacement in %s has failed %d times. Consider using <action name=\"write_file\"> to rewrite %s with the full updated content.]", path, editFailures[path], path)
				}
			case "read_window":
				toolResult = fmt.Sprintf("<action_result>\n[Read error: %v]\n</action_result>", err)
				if repeatCount >= 2 {
					toolResult += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have attempted this exact read %d times consecutively and it failed: %v. DO NOT repeat this action. If you are trying to inspect a directory, use <action name=\"find_files\"><pattern>*</pattern></action> instead.]", repeatCount, err)
				}
			case "read_outline":
				toolResult = fmt.Sprintf("<action_result>\n[Outline error: %v]\n</action_result>", err)
				if repeatCount >= 2 {
					toolResult += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have attempted this exact outline %d times consecutively and it failed: %v. DO NOT repeat this action. If you are trying to inspect a directory, use <action name=\"find_files\"><pattern>*</pattern></action> instead.]", repeatCount, err)
				}
			case "run_test":
				toolResult = fmt.Sprintf("<action_result>\n[Test execution error: %v]\n</action_result>", err)
			case "get_environment":
				toolResult = fmt.Sprintf("<action_result>\n[Environment error: %v]\n</action_result>", err)
			default:
				toolResult = fmt.Sprintf("<action_result>\n[Error: %v]\n%s\n</action_result>", err, out)
				if repeatCount >= 2 {
					toolResult += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have attempted this EXACT action %d times consecutively and it failed. DO NOT repeat this action. Change your strategy or call <action name=\"task_finish\">.]", repeatCount)
				}
			}
		} else {
			if act.Name == "replace_file" || act.Name == "write_file" {
				editFailures[targetSummary] = 0
			}
			toolResult = fmt.Sprintf("<action_result>\n%s\n</action_result>", out)
		}

		if repeatCount >= 2 && err == nil {
			if act.Name == "read_window" {
				toolResult += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have read this exact line window %d times consecutively. You now have the contents. Take action to edit the file (<action name=\"replace_file\"> or <action name=\"write_file\">) or proceed with your next step.]", repeatCount)
			}
		} else if repeatCount >= 2 && err != nil && act.Name == "run_test" {
			toolResult += fmt.Sprintf("\n\n[SYSTEM INTERVENTION: Loop detected. You have run the exact same test command %d times consecutively and tests are failing. Inspect or edit the source code with <action name=\"replace_file\"> or <action name=\"write_file\"> before re-running tests.]", repeatCount)
		}

		if r.OnOutput != nil {
			if err != nil {
				r.OnOutput("error", fmt.Sprintf("Error: %v\n%s", err, out))
			} else {
				r.OnOutput("result", out)
			}
		}

		session.History = append(session.History, Message{Role: "user", Content: toolResult})
	}

	return "", fmt.Errorf("exceeded max turns (%d) without completing task", r.MaxTurns)
}
