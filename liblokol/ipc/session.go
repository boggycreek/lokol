// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ipc

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/catalog"
	"github.com/boggycreek/lokol/liblokol/regulator"
	"github.com/boggycreek/lokol/liblokol/spec"
)

type actionDecision struct {
	Approved bool
	Reason   string
}

// SessionState encapsulates state and execution control for an active IPC session.
type SessionState struct {
	ID           string
	WorkDir      string
	Mode         agent.Mode
	AgentName    string
	OperatorName string
	YOLO         bool
	MaxTurns     int
	Session      *agent.Session
	Regulator    *regulator.Regulator
	SpecMachine  *spec.StateMachine

	mu         sync.Mutex
	isBusy     bool
	cancelTurn context.CancelFunc
	turnDone   chan struct{}

	actionCounter   uint64
	pendingActionID string
	decisionChan    chan actionDecision
}

// NewSessionState creates an initialized session state.
func NewSessionState(id string, client *agent.Client, params SessionCreateParams) (*SessionState, error) {
	workDir := params.WorkDir
	if workDir == "" {
		workDir = "."
	}

	mode := agent.Mode(params.Mode)
	if mode == "" {
		mode = agent.ModeCoding
	}

	sess := agent.NewSessionWithMode(client, workDir, mode)
	if params.AgentName != "" || params.OperatorName != "" {
		sess.SetPersona(params.AgentName, params.OperatorName)
	}

	reg := regulator.New(workDir)
	if client != nil {
		reg.SetSlotStatusProvider(client)
	}
	reg.SetCompactor(sess)

	var sm *spec.StateMachine
	if params.SpecFile != "" {
		loadedSpec, err := spec.Load(params.SpecFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load spec %q: %w", params.SpecFile, err)
		}
		sm = spec.NewStateMachine(loadedSpec, workDir)
	}

	yolo := false
	if params.YOLO != nil {
		yolo = *params.YOLO
	}

	maxTurns := params.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 20
	}

	return &SessionState{
		ID:           id,
		WorkDir:      workDir,
		Mode:         mode,
		AgentName:    params.AgentName,
		OperatorName: params.OperatorName,
		YOLO:         yolo,
		MaxTurns:     maxTurns,
		Session:      sess,
		Regulator:    reg,
		SpecMachine:  sm,
	}, nil
}

func hasActionTagPrefix(s string) bool {
	prefixes := []string{
		"<", "<a", "<ac", "<act", "<acti", "<actio", "<action",
		"<action ", "<action n", "<action na", "<action nam", "<action name",
		"<action name=", "<action name =",
	}
	for _, p := range prefixes {
		if strings.HasSuffix(s, p) {
			return true
		}
	}
	return false
}

// Prompt initiates turn execution on the session, streaming events through the broadcaster.
func (s *SessionState) Prompt(ctx context.Context, prompt string, emit func(Notification)) (res *EventTurnFinishedPayload, err error) {
	s.mu.Lock()
	if s.isBusy {
		s.mu.Unlock()
		return nil, fmt.Errorf("session is currently executing a prompt")
	}
	s.isBusy = true
	turnCtx, cancel := context.WithCancel(ctx)
	s.cancelTurn = cancel
	s.turnDone = make(chan struct{})
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isBusy = false
		s.cancelTurn = nil
		s.pendingActionID = ""
		s.decisionChan = nil
		if s.turnDone != nil {
			close(s.turnDone)
			s.turnDone = nil
		}
		s.mu.Unlock()
	}()

	var emittedFinished bool
	emitTurnFinished := func(payload *EventTurnFinishedPayload) {
		if !emittedFinished {
			emittedFinished = true
			emit(Notification{
				JSONRPC: "2.0",
				Method:  EventTurnFinished,
				Params:  *payload,
			})
		}
	}

	turnCount := 0

	// Complete process and memory fault isolation: catch engine panics
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 2048)
			n := runtime.Stack(buf, false)
			fmt.Fprintf(os.Stderr, "[PANIC RECOVERED] session %s: %v\n%s\n", s.ID, r, buf[:n])
			res = &EventTurnFinishedPayload{
				SessionID: s.ID,
				Status:    "error",
				Turns:     turnCount,
				Error:     fmt.Sprintf("internal engine panic: %v", r),
			}
			err = fmt.Errorf("internal engine panic: %v", r)
			emitTurnFinished(res)
		}
	}()

	// Execute preflight check if bounded spec machine is configured
	if s.SpecMachine != nil && s.SpecMachine.State == spec.StatePending {
		if err := s.SpecMachine.RunPreflight(turnCtx); err != nil {
			errPayload := &EventTurnFinishedPayload{
				SessionID: s.ID,
				Status:    "error",
				Error:     fmt.Sprintf("spec preflight check failed: %v", err),
			}
			emitTurnFinished(errPayload)
			return errPayload, fmt.Errorf("spec preflight check failed: %w", err)
		}
	}

	s.Session.AppendUserMessage(prompt)

	consecutiveNoAction := 0
	lastSig := ""
	repeatCount := 0
	isConversational := catalog.IsConversationalFeedback(prompt)

	for turn := 0; turn < s.MaxTurns; turn++ {
		turnCount++
		if turnCtx.Err() != nil {
			abortPayload := &EventTurnFinishedPayload{
				SessionID: s.ID,
				Status:    "aborted",
				Turns:     turnCount,
				Error:     turnCtx.Err().Error(),
			}
			emitTurnFinished(abortPayload)
			return abortPayload, nil
		}

		tokenChan := make(chan string, 100)
		var assistantReply strings.Builder
		errChan := make(chan error, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					errChan <- fmt.Errorf("panic in streaming goroutine: %v", r)
				}
			}()
			_, err := s.Session.StreamTurn(turnCtx, tokenChan)
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

			// Check for parseable action start tag: <action name=
			if idx := strings.Index(str, "<action name="); idx != -1 {
				if idx > 0 {
					emit(Notification{
						JSONRPC: "2.0",
						Method:  EventToken,
						Params: EventTokenPayload{
							SessionID: s.ID,
							Token:     str[:idx],
						},
					})
				}
				suppressTokens = true
				continue
			}
			if idx := strings.Index(str, "<action name ="); idx != -1 {
				if idx > 0 {
					emit(Notification{
						JSONRPC: "2.0",
						Method:  EventToken,
						Params: EventTokenPayload{
							SessionID: s.ID,
							Token:     str[:idx],
						},
					})
				}
				suppressTokens = true
				continue
			}

			if hasActionTagPrefix(str) {
				continue
			}

			emit(Notification{
				JSONRPC: "2.0",
				Method:  EventToken,
				Params: EventTokenPayload{
					SessionID: s.ID,
					Token:     str,
				},
			})
			pendingTokens.Reset()
		}

		// Flush any remaining buffered tokens that were held back as potential action tag prefixes
		if !suppressTokens && pendingTokens.Len() > 0 {
			emit(Notification{
				JSONRPC: "2.0",
				Method:  EventToken,
				Params: EventTokenPayload{
					SessionID: s.ID,
					Token:     pendingTokens.String(),
				},
			})
			pendingTokens.Reset()
		}

		if err := <-errChan; err != nil {
			if turnCtx.Err() != nil {
				res := &EventTurnFinishedPayload{
					SessionID: s.ID,
					Status:    "aborted",
					Turns:     turnCount,
					Error:     turnCtx.Err().Error(),
				}
				emitTurnFinished(res)
				return res, nil
			}
			turnErr := fmt.Errorf("turn %d execution error: %w", turn+1, err)
			errPayload := &EventTurnFinishedPayload{
				SessionID: s.ID,
				Status:    "error",
				Turns:     turnCount,
				Error:     turnErr.Error(),
			}
			emitTurnFinished(errPayload)
			return errPayload, turnErr
		}

		replyText := assistantReply.String()
		s.Session.AppendAssistantMessage(replyText)

		act := agent.ParseAction(replyText)
		if act == nil {
			if isConversational {
				res := &EventTurnFinishedPayload{
					SessionID: s.ID,
					Status:    "completed",
					Reply:     replyText,
					Turns:     turnCount,
				}
				emitTurnFinished(res)
				return res, nil
			}
			consecutiveNoAction++
			if consecutiveNoAction >= 2 || turn == s.MaxTurns-1 {
				res := &EventTurnFinishedPayload{
					SessionID: s.ID,
					Status:    "completed",
					Reply:     replyText,
					Turns:     turnCount,
				}
				emitTurnFinished(res)
				return res, nil
			}
			s.Session.AppendUserMessage("Please execute your next action using an <action name=\"...\"> tag, or call <action name=\"task_finish\"> if your task is complete.")
			continue
		}

		consecutiveNoAction = 0

		if act.Name == "task_finish" {
			if s.SpecMachine != nil {
				verRes, err := s.SpecMachine.Verify(turnCtx)
				if err != nil {
					errPayload := &EventTurnFinishedPayload{
						SessionID: s.ID,
						Status:    "error",
						Turns:     turnCount,
						Error:     fmt.Sprintf("spec verification error: %v", err),
					}
					emitTurnFinished(errPayload)
					return errPayload, fmt.Errorf("spec verification error: %w", err)
				}
				if !verRes.Passed {
					failMsg := fmt.Sprintf("<action_result>\n[VERIFICATION GATE FAILED]: Verification command '%s' failed with exit code %d:\n%s\n\nYou cannot complete the task until all verification checks pass with exit 0. Inspect the failure, edit the code, and re-run tests.\n</action_result>", verRes.Command, verRes.ExitCode, verRes.Output)
					s.Session.AppendUserMessage(failMsg)
					continue
				}
			}

			res := &EventTurnFinishedPayload{
				SessionID: s.ID,
				Status:    "completed",
				Reply:     act.Command,
				Turns:     turnCount,
			}
			emitTurnFinished(res)
			return res, nil
		}

		currentSig := act.Name + ":" + strings.TrimSpace(act.Command)
		if currentSig == lastSig {
			repeatCount++
		} else {
			lastSig = currentSig
			repeatCount = 1
		}

		if repeatCount >= 4 {
			loopErr := fmt.Errorf("loop detected: action %s repeated %d times without progress", act.Name, repeatCount)
			errPayload := &EventTurnFinishedPayload{
				SessionID: s.ID,
				Status:    "error",
				Turns:     turnCount,
				Error:     loopErr.Error(),
			}
			emitTurnFinished(errPayload)
			return errPayload, loopErr
		}

		targetSummary := act.TargetSummary()

		// Enforce spec target file constraints
		if s.SpecMachine != nil {
			if act.Name == "replace_file" || act.Name == "write_file" {
				if err := s.SpecMachine.CheckTargetConstraint(targetSummary); err != nil {
					msg := fmt.Sprintf("<action_result>\n[PERMISSION DENIED: TARGET CONSTRAINT VIOLATION]: %v\n</action_result>", err)
					s.Session.AppendUserMessage(msg)
					continue
				}
			} else if act.Name == "exec_bash" {
				if err := s.SpecMachine.CheckBashTargetConstraint(act.Command); err != nil {
					msg := fmt.Sprintf("<action_result>\n[PERMISSION DENIED: TARGET CONSTRAINT VIOLATION]: %v\n</action_result>", err)
					s.Session.AppendUserMessage(msg)
					continue
				}
			}
		}

		actionPath := targetSummary
		if act.Name == "find_files" || act.Name == "search_code" {
			actionPath = regulator.ExtractTagContent(act.Command, "path")
		}

		perm := s.Regulator.CheckPermission(turnCtx, regulator.ActionCandidate{
			Name:    act.Name,
			Command: act.Command,
			Path:    actionPath,
		})

		actionID := fmt.Sprintf("act_%d", atomic.AddUint64(&s.actionCounter, 1))

		// In interactive mode (!s.YOLO), all mutating actions (exec_bash, write_file, replace_file)
		// and any action flagged with elevated risk or StatusWarning require operator approval.
		isMutating := act.Name == "exec_bash" || act.Name == "write_file" || act.Name == "replace_file"
		isElevatedRisk := perm.RiskLevel == regulator.RiskLevelMedium || perm.RiskLevel == regulator.RiskLevelHigh || perm.RiskLevel == regulator.RiskLevelCritical
		requiresApproval := !s.YOLO && (isMutating || isElevatedRisk || perm.Status == regulator.StatusWarning)
		var decisionChan chan actionDecision
		if requiresApproval {
			decisionChan = make(chan actionDecision, 1)
			s.mu.Lock()
			s.pendingActionID = actionID
			s.decisionChan = decisionChan
			s.mu.Unlock()
		}

		emit(Notification{
			JSONRPC: "2.0",
			Method:  EventActionProposed,
			Params: EventActionProposedPayload{
				SessionID:        s.ID,
				ActionID:         actionID,
				Name:             act.Name,
				Command:          act.Command,
				Target:           targetSummary,
				RiskLevel:        string(perm.RiskLevel),
				RequiresApproval: requiresApproval,
				Reason:           perm.Reason,
				Remediation:      perm.Remediation,
			},
		})

		// Block high-risk actions or regulator blocks in autonomous mode
		if perm.Status == regulator.StatusBlocked || (s.YOLO && perm.RiskLevel == regulator.RiskLevelHigh) {
			if decisionChan != nil {
				s.mu.Lock()
				s.pendingActionID = ""
				s.decisionChan = nil
				s.mu.Unlock()
			}
			msg := fmt.Sprintf("[PERMISSION DENIED]: %s", perm.Reason)
			if perm.Remediation != "" {
				msg += fmt.Sprintf("\n[REMEDIATION]: %s", perm.Remediation)
			}
			toolResult := fmt.Sprintf("<action_result>\n%s\n</action_result>", msg)
			s.Session.AppendUserMessage(toolResult)
			continue
		}

		// Wait for client approval if required
		if requiresApproval {

			select {
			case decision := <-decisionChan:
				if !decision.Approved {
					rejectReason := decision.Reason
					if rejectReason == "" {
						rejectReason = "Operator rejected proposed action."
					}
					s.Regulator.RecordRejection(regulator.ActionCandidate{
						Name:    act.Name,
						Command: act.Command,
						Path:    actionPath,
					})
					s.Session.AppendUserMessage(fmt.Sprintf("<action_result>\n[ACTION REJECTED BY OPERATOR]: %s\n</action_result>", rejectReason))
					continue
				}
			case <-turnCtx.Done():
				res := &EventTurnFinishedPayload{
					SessionID: s.ID,
					Status:    "aborted",
					Turns:     turnCount,
					Error:     turnCtx.Err().Error(),
				}
				emitTurnFinished(res)
				return res, nil
			}
		}

		out, err := s.Session.ExecuteAction(turnCtx, act)

		if s.SpecMachine != nil {
			if act.Name == "run_test" {
				passed := strings.HasPrefix(out, "✓ ")
				exitCode := 0
				if !passed {
					exitCode = 1
				}
				s.SpecMachine.LatestVerification = &spec.VerificationResult{
					Timestamp: time.Now(),
					Command:   "run_test " + act.Command,
					ExitCode:  exitCode,
					Output:    out,
					Passed:    passed,
				}
			} else if act.Name == "exec_bash" && s.SpecMachine.Spec != nil && s.SpecMachine.Spec.VerifyCmd != "" && strings.TrimSpace(act.Command) == strings.TrimSpace(s.SpecMachine.Spec.VerifyCmd) {
				verRes, _ := s.SpecMachine.Verify(turnCtx)
				if verRes != nil {
					s.SpecMachine.LatestVerification = verRes
				}
			}
		}

		var errStr string
		if err != nil {
			errStr = err.Error()
		}

		emit(Notification{
			JSONRPC: "2.0",
			Method:  EventActionResult,
			Params: EventActionResultPayload{
				SessionID: s.ID,
				ActionID:  actionID,
				Name:      act.Name,
				Output:    out,
				Error:     errStr,
			},
		})

		var toolResult string
		if err != nil {
			toolResult = fmt.Sprintf("<action_result>\n[Error: %v]\n%s\n</action_result>", err, out)
		} else {
			toolResult = fmt.Sprintf("<action_result>\n%s\n</action_result>", out)
		}
		s.Session.AppendUserMessage(toolResult)
	}

	res = &EventTurnFinishedPayload{
		SessionID: s.ID,
		Status:    "completed",
		Turns:     turnCount,
		Error:     fmt.Sprintf("reached max turns (%d)", s.MaxTurns),
	}
	emitTurnFinished(res)
	return res, nil
}

// DecideAction resolves a pending action approval gate.
func (s *SessionState) DecideAction(actionID string, approved bool, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pendingActionID == "" || s.pendingActionID != actionID || s.decisionChan == nil {
		return fmt.Errorf("no pending action with ID %s", actionID)
	}

	s.decisionChan <- actionDecision{Approved: approved, Reason: reason}
	s.pendingActionID = ""
	s.decisionChan = nil
	return nil
}

// Abort cancels any active turn and clears KV cache slots on the inference engine.
func (s *SessionState) Abort(ctx context.Context) error {
	s.mu.Lock()
	if s.cancelTurn != nil {
		s.cancelTurn()
	}
	s.mu.Unlock()

	return s.Session.Abort(ctx)
}

// Reset clears conversational history back to initial system prompt invariants, waiting for active turns to finish.
func (s *SessionState) Reset() {
	s.mu.Lock()
	if s.cancelTurn != nil {
		s.cancelTurn()
	}
	done := s.turnDone
	s.mu.Unlock()

	if done != nil {
		<-done
	}

	s.Session.Reset()
}

// Close gracefully cancels active turns and frees session resources.
func (s *SessionState) Close() {
	s.mu.Lock()
	if s.cancelTurn != nil {
		s.cancelTurn()
	}
	done := s.turnDone
	s.mu.Unlock()

	if done != nil {
		<-done
	}
}

// Status returns a snapshot of session metrics and configuration.
func (s *SessionState) Status(ctx context.Context) (map[string]any, error) {
	s.mu.Lock()
	isBusy := s.isBusy
	s.mu.Unlock()

	historyLen := s.Session.HistoryLen()
	agentName, operatorName := s.Session.GetPersona()

	metrics, err := s.Session.GetSlotMetrics(ctx)
	var slotData any
	if err == nil && metrics != nil {
		slotData = metrics
	}

	return map[string]any{
		"session_id":    s.ID,
		"work_dir":      s.WorkDir,
		"mode":          string(s.Mode),
		"agent_name":    agentName,
		"operator_name": operatorName,
		"yolo":          s.YOLO,
		"is_busy":       isBusy,
		"history_len":   historyLen,
		"slot_metrics":  slotData,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}, nil
}
