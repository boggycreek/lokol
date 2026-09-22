// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"strings"
)

// Runner manages an autonomous execution loop (e.g. for batch execution or self-improvement).
type Runner struct {
	Client   *Client
	MaxTurns int
	YOLO     bool
	WorkDir  string // Current working directory for system prompt context
	OnOutput func(role, content string)
}

// Run executes an autonomous loop on a given user prompt.
func (r *Runner) Run(ctx context.Context, initialPrompt string) (string, error) {
	history := []Message{
		{Role: "system", Content: BuildSystemPrompt(r.WorkDir)},
		{Role: "user", Content: initialPrompt},
	}

	if r.MaxTurns <= 0 {
		r.MaxTurns = 20
	}

	for turn := 0; turn < r.MaxTurns; turn++ {
		tokenChan := make(chan string, 100)
		var assistantReply strings.Builder

		errChan := make(chan error, 1)
		go func() {
			_, err := r.Client.StreamResponse(ctx, history, tokenChan)
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
			return "", fmt.Errorf("error during turn %d: %w", turn+1, err)
		}

		replyText := assistantReply.String()
		history = append(history, Message{Role: "assistant", Content: replyText})

		act := ParseAction(replyText)
		if act == nil {
			// No action proposed; conversational turn done
			return replyText, nil
		}

		if act.Name == "task_finish" {
			if r.OnOutput != nil {
				r.OnOutput("finish", act.Command)
			}
			return act.Command, nil
		}

		if act.Name == "exec_bash" {
			if r.OnOutput != nil {
				r.OnOutput("exec_bash", act.Command)
			}

			out, err := ExecuteBash(ctx, act.Command)
			toolResult := fmt.Sprintf("<action_result>\n%s\n</action_result>", out)
			if err != nil {
				toolResult = fmt.Sprintf("<action_result>\n[Exit error: %v]\n%s\n</action_result>", err, out)
			}

			if r.OnOutput != nil {
				if err != nil {
					r.OnOutput("error", fmt.Sprintf("Error: %v\n%s", err, out))
				} else {
					r.OnOutput("result", out)
				}
			}

			history = append(history, Message{Role: "user", Content: toolResult})
		} else if act.Name == "replace_file" {
			input, _ := ParseReplaceFileInput(act.Command)
			path := "file"
			if input != nil {
				path = input.Path
			}
			if r.OnOutput != nil {
				r.OnOutput("replace_file", path)
			}

			out, err := ExecuteReplaceFile(ctx, act.Command)
			toolResult := fmt.Sprintf("<action_result>\n%s\n</action_result>", out)
			if err != nil {
				toolResult = fmt.Sprintf("<action_result>\n[Edit error: %v]\n</action_result>", err)
			}

			if r.OnOutput != nil {
				if err != nil {
					r.OnOutput("error", fmt.Sprintf("Error: %v", err))
				} else {
					r.OnOutput("result", out)
				}
			}

			history = append(history, Message{Role: "user", Content: toolResult})
		} else if act.Name == "write_file" {
			input, _ := ParseWriteFileInput(act.Command)
			path := "file"
			if input != nil {
				path = input.Path
			}
			if r.OnOutput != nil {
				r.OnOutput("write_file", path)
			}

			out, err := ExecuteWriteFile(ctx, act.Command)
			toolResult := fmt.Sprintf("<action_result>\n%s\n</action_result>", out)
			if err != nil {
				toolResult = fmt.Sprintf("<action_result>\n[Write error: %v]\n</action_result>", err)
			}

			if r.OnOutput != nil {
				if err != nil {
					r.OnOutput("error", fmt.Sprintf("Error: %v", err))
				} else {
					r.OnOutput("result", out)
				}
			}

			history = append(history, Message{Role: "user", Content: toolResult})
		} else if act.Name == "read_outline" {
			if r.OnOutput != nil {
				r.OnOutput("read_outline", act.Command)
			}
			out, err := ExecuteReadOutline(ctx, act.Command)
			toolResult := fmt.Sprintf("<action_result>\n%s\n</action_result>", out)
			if err != nil {
				toolResult = fmt.Sprintf("<action_result>\n[Outline error: %v]\n</action_result>", err)
			}
			history = append(history, Message{Role: "user", Content: toolResult})
		} else if act.Name == "read_window" {
			if r.OnOutput != nil {
				r.OnOutput("read_window", act.Command)
			}
			out, err := ExecuteReadWindow(ctx, act.Command)
			toolResult := fmt.Sprintf("<action_result>\n%s\n</action_result>", out)
			if err != nil {
				toolResult = fmt.Sprintf("<action_result>\n[Read error: %v]\n</action_result>", err)
			}
			history = append(history, Message{Role: "user", Content: toolResult})
		} else if act.Name == "run_test" {
			if r.OnOutput != nil {
				r.OnOutput("run_test", act.Command)
			}
			out, err := ExecuteRunTest(ctx, act.Command)
			toolResult := fmt.Sprintf("<action_result>\n%s\n</action_result>", out)
			if err != nil {
				toolResult = fmt.Sprintf("<action_result>\n[Test execution error: %v]\n</action_result>", err)
			}
			history = append(history, Message{Role: "user", Content: toolResult})
		}
	}

	return "", fmt.Errorf("exceeded max turns (%d) without completing task", r.MaxTurns)
}

