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
	OnOutput func(role, content string)
}

// Run executes an autonomous loop on a given user prompt.
func (r *Runner) Run(ctx context.Context, initialPrompt string) (string, error) {
	history := []Message{
		{Role: "system", Content: SystemPrompt},
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
			reply, err := r.Client.StreamResponse(ctx, history, tokenChan)
			assistantReply.WriteString(reply)
			close(tokenChan)
			errChan <- err
		}()

		for token := range tokenChan {
			if r.OnOutput != nil {
				r.OnOutput("token", token)
			}
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
				r.OnOutput("result", out)
			}

			history = append(history, Message{Role: "user", Content: toolResult})
		}
	}

	return "", fmt.Errorf("exceeded max turns (%d) without completing task", r.MaxTurns)
}
