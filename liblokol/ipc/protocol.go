// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ipc

import (
	"encoding/json"
)

// JSON-RPC 2.0 standard error codes
const (
	ErrCodeParseError      = -32700
	ErrCodeInvalidRequest  = -32600
	ErrCodeMethodNotFound  = -32601
	ErrCodeInvalidParams   = -32602
	ErrCodeInternal        = -32603
	ErrCodeSessionNotFound = -32001
	ErrCodeSessionBusy     = -32002
	ErrCodeActionNotFound  = -32003
)

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Notification represents a JSON-RPC 2.0 notification emitted by the daemon (events).
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// RPCError details a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Methods supported by the IPC daemon
const (
	MethodSessionCreate        = "session.create"
	MethodSessionPrompt        = "session.prompt"
	MethodSessionApproveAction = "session.approve_action"
	MethodSessionRejectAction  = "session.reject_action"
	MethodSessionAbort         = "session.abort"
	MethodSessionReset         = "session.reset"
	MethodSessionGetStatus     = "session.get_status"
	MethodDaemonPing           = "daemon.ping"
)

// Streaming events dispatched by the IPC daemon
const (
	EventToken          = "event.token"
	EventActionProposed = "event.action_proposed"
	EventActionResult   = "event.action_result"
	EventCompaction     = "event.compaction"
	EventTurnFinished   = "event.turn_finished"
	EventSessionStatus  = "event.status"
)

// SessionCreateParams defines parameters for session.create
type SessionCreateParams struct {
	SessionID    string `json:"session_id,omitempty"`
	WorkDir      string `json:"work_dir,omitempty"`
	Mode         string `json:"mode,omitempty"`
	AgentName    string `json:"agent_name,omitempty"`
	OperatorName string `json:"operator_name,omitempty"`
	YOLO         *bool  `json:"yolo,omitempty"`
	MaxTurns     int    `json:"max_turns,omitempty"`
	SpecFile     string `json:"spec_file,omitempty"`
}

// SessionPromptParams defines parameters for session.prompt
type SessionPromptParams struct {
	SessionID string `json:"session_id,omitempty"`
	Prompt    string `json:"prompt"`
}

// ActionDecisionParams defines parameters for session.approve_action or session.reject_action
type ActionDecisionParams struct {
	SessionID string `json:"session_id,omitempty"`
	ActionID  string `json:"action_id"`
	Reason    string `json:"reason,omitempty"`
}

// SessionAbortParams defines parameters for session.abort
type SessionAbortParams struct {
	SessionID string `json:"session_id,omitempty"`
}

// SessionResetParams defines parameters for session.reset
type SessionResetParams struct {
	SessionID string `json:"session_id,omitempty"`
}

// SessionStatusParams defines parameters for session.get_status
type SessionStatusParams struct {
	SessionID string `json:"session_id,omitempty"`
}

// EventTokenPayload defines the payload for event.token
type EventTokenPayload struct {
	SessionID string `json:"session_id"`
	Token     string `json:"token"`
}

// EventActionProposedPayload defines the payload for event.action_proposed
type EventActionProposedPayload struct {
	SessionID        string `json:"session_id"`
	ActionID         string `json:"action_id"`
	Name             string `json:"name"`
	Command          string `json:"command"`
	Target           string `json:"target"`
	RiskLevel        string `json:"risk_level"`
	RequiresApproval bool   `json:"requires_approval"`
	Reason           string `json:"reason,omitempty"`
	Remediation      string `json:"remediation,omitempty"`
}

// EventActionResultPayload defines the payload for event.action_result
type EventActionResultPayload struct {
	SessionID string `json:"session_id"`
	ActionID  string `json:"action_id"`
	Name      string `json:"name"`
	Output    string `json:"output"`
	Error     string `json:"error,omitempty"`
}

// EventCompactionPayload defines the payload for event.compaction
type EventCompactionPayload struct {
	SessionID      string `json:"session_id"`
	Type           string `json:"type"` // "micro" or "macro"
	ReclaimedChars int    `json:"reclaimed_chars,omitempty"`
	HistoryLen     int    `json:"history_len"`
}

// EventTurnFinishedPayload defines the payload for event.turn_finished
type EventTurnFinishedPayload struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"` // "completed", "aborted", "error"
	Reply     string `json:"reply,omitempty"`
	Turns     int    `json:"turns"`
	Error     string `json:"error,omitempty"`
}
