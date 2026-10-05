// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/version"
)

// ServerConfig configures an IPC daemon server.
type ServerConfig struct {
	EngineURL       string
	DefaultWorkDir  string
	DefaultYOLO     bool
	DefaultMode     string
	DefaultMaxTurns int
}

// Server coordinates the headless IPC daemon service over UNIX domain sockets or TCP.
type Server struct {
	cfg       ServerConfig
	listener  net.Listener
	network   string
	address   string
	startTime time.Time

	mu             sync.RWMutex
	sessions       map[string]*SessionState
	activeConns    map[net.Conn]struct{}
	sessionCounter uint64
	closeOnce      sync.Once

	client *agent.Client
}

// NewServer creates a new initialized IPC daemon server.
func NewServer(cfg ServerConfig) *Server {
	if cfg.EngineURL == "" {
		cfg.EngineURL = "http://127.0.0.1:8080"
	}
	if cfg.DefaultWorkDir == "" {
		cfg.DefaultWorkDir = "."
	}
	if cfg.DefaultMaxTurns <= 0 {
		cfg.DefaultMaxTurns = 20
	}

	return &Server{
		cfg:         cfg,
		startTime:   time.Now(),
		sessions:    make(map[string]*SessionState),
		activeConns: make(map[net.Conn]struct{}),
		client:      agent.NewClient(cfg.EngineURL),
	}
}

// Listen starts listening on the specified network ("unix" or "tcp") and address.
func (s *Server) Listen(network, address string) error {
	s.network = network
	s.address = address

	if network == "unix" {
		// Clean up existing stale socket file if present
		if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove existing socket file: %w", err)
		}
		if dir := filepath.Dir(address); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create socket directory: %w", err)
			}
		}
	}

	ln, err := net.Listen(network, address)
	if err != nil {
		return err
	}
	s.listener = ln
	return nil
}

// Addr returns the listener address.
func (s *Server) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Serve accepts client connections and handles JSON-RPC 2.0 communication.
func (s *Server) Serve(ctx context.Context) error {
	if s.listener == nil {
		return fmt.Errorf("server listener not initialized; call Listen first")
	}

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}

		s.mu.Lock()
		s.activeConns[conn] = struct{}{}
		s.mu.Unlock()

		go s.handleConn(ctx, conn)
	}
}

// Close gracefully closes the listener and active connections, cleaning up UNIX sockets.
func (s *Server) Close() error {
	var firstErr error
	s.closeOnce.Do(func() {
		if s.listener != nil {
			if err := s.listener.Close(); err != nil {
				firstErr = err
			}
		}

		s.mu.Lock()
		conns := make([]net.Conn, 0, len(s.activeConns))
		for conn := range s.activeConns {
			conns = append(conns, conn)
		}
		s.activeConns = make(map[net.Conn]struct{})
		s.mu.Unlock()

		for _, conn := range conns {
			_ = conn.Close()
		}

		if s.network == "unix" && s.address != "" {
			_ = os.Remove(s.address)
		}
	})

	return firstErr
}

func (s *Server) handleConn(serverCtx context.Context, conn net.Conn) {
	connCtx, cancelConn := context.WithCancel(serverCtx)
	defer func() {
		cancelConn()
		s.mu.Lock()
		delete(s.activeConns, conn)
		s.mu.Unlock()
		_ = conn.Close()
	}()

	var writeMu sync.Mutex
	writeMsg := func(v any) error {
		bytes, err := json.Marshal(v)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err = conn.Write(append(bytes, '\n'))
		if err != nil {
			_ = conn.Close()
		}
		return err
	}

	emitNotification := func(n Notification) {
		if err := writeMsg(n); err != nil {
			cancelConn()
		}
	}

	reader := bufio.NewReader(conn)
	const maxLineBytes = 2 * 1024 * 1024 // 2MB safety bound

	for {
		var line []byte
		var readErr error
		for {
			chunk, isPrefix, err := reader.ReadLine()
			if err != nil {
				readErr = err
				break
			}
			line = append(line, chunk...)
			if len(line) > maxLineBytes {
				readErr = fmt.Errorf("line exceeded maximum limit of %d bytes", maxLineBytes)
				break
			}
			if !isPrefix {
				break
			}
		}

		if readErr != nil {
			if len(line) > maxLineBytes {
				_ = writeMsg(Response{
					JSONRPC: "2.0",
					Error: &RPCError{
						Code:    ErrCodeParseError,
						Message: readErr.Error(),
					},
				})
			}
			return
		}

		if len(line) == 0 {
			continue
		}

		var req Request
		if parseErr := json.Unmarshal(line, &req); parseErr != nil {
			_ = writeMsg(Response{
				JSONRPC: "2.0",
				Error: &RPCError{
					Code:    ErrCodeParseError,
					Message: fmt.Sprintf("invalid JSON payload: %v", parseErr),
				},
			})
			continue
		}

		if req.JSONRPC != "2.0" || req.Method == "" {
			if len(req.ID) > 0 && string(req.ID) != "null" {
				_ = writeMsg(Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: &RPCError{
						Code:    ErrCodeInvalidRequest,
						Message: "missing or invalid jsonrpc version or method",
					},
				})
			}
			continue
		}

		go func(r Request) {
			resp := s.dispatch(connCtx, r, emitNotification)
			// JSON-RPC 2.0 notifications (requests with no ID or null ID) must not receive a response
			if len(r.ID) > 0 && string(r.ID) != "null" {
				if err := writeMsg(resp); err != nil {
					cancelConn()
				}
			}
		}(req)
	}
}

func (s *Server) dispatch(ctx context.Context, req Request, emit func(Notification)) Response {
	switch req.Method {
	case MethodDaemonPing:
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"status":    "pong",
				"version":   version.Version,
				"commit":    version.GitCommit,
				"uptime_s":  int(time.Since(s.startTime).Seconds()),
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
		}

	case MethodSessionCreate:
		var params SessionCreateParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()},
				}
			}
		}

		s.mu.Lock()
		sessID := params.SessionID
		if sessID != "" {
			if _, exists := s.sessions[sessID]; exists {
				s.mu.Unlock()
				return Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: &RPCError{
						Code:    ErrCodeInvalidParams,
						Message: fmt.Sprintf("session %q already exists", sessID),
					},
				}
			}
		} else {
			s.sessionCounter++
			sessID = fmt.Sprintf("sess_%d", s.sessionCounter)
		}

		if params.WorkDir == "" {
			params.WorkDir = s.cfg.DefaultWorkDir
		}
		if params.Mode == "" {
			params.Mode = s.cfg.DefaultMode
		}
		if params.YOLO == nil {
			yolo := s.cfg.DefaultYOLO
			params.YOLO = &yolo
		}
		if params.MaxTurns <= 0 {
			params.MaxTurns = s.cfg.DefaultMaxTurns
		}

		state, err := NewSessionState(sessID, s.client, params)
		if err != nil {
			s.mu.Unlock()
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInternal, Message: err.Error()},
			}
		}
		s.sessions[sessID] = state
		s.mu.Unlock()

		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"session_id": sessID,
				"status":     "ready",
				"work_dir":   state.WorkDir,
				"mode":       string(state.Mode),
				"yolo":       state.YOLO,
			},
		}

	case MethodSessionPrompt:
		if len(req.Params) == 0 {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInvalidParams, Message: "missing prompt params"},
			}
		}
		var params SessionPromptParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()},
			}
		}

		sess, err := s.resolveSession(params.SessionID, true)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeSessionNotFound, Message: err.Error()},
			}
		}

		res, promptErr := sess.Prompt(ctx, params.Prompt, emit)
		if promptErr != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInternal, Message: promptErr.Error()},
			}
		}

		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  res,
		}

	case MethodSessionApproveAction:
		if len(req.Params) == 0 {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInvalidParams, Message: "missing action approval params"},
			}
		}
		var params ActionDecisionParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()},
			}
		}

		sess, err := s.resolveSession(params.SessionID, false)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeSessionNotFound, Message: err.Error()},
			}
		}

		if err := sess.DecideAction(params.ActionID, true, params.Reason); err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeActionNotFound, Message: err.Error()},
			}
		}

		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"action_id": params.ActionID, "decision": "approved"},
		}

	case MethodSessionRejectAction:
		if len(req.Params) == 0 {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInvalidParams, Message: "missing action rejection params"},
			}
		}
		var params ActionDecisionParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()},
			}
		}

		sess, err := s.resolveSession(params.SessionID, false)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeSessionNotFound, Message: err.Error()},
			}
		}

		if err := sess.DecideAction(params.ActionID, false, params.Reason); err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeActionNotFound, Message: err.Error()},
			}
		}

		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"action_id": params.ActionID, "decision": "rejected"},
		}

	case MethodSessionAbort:
		var params SessionAbortParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}

		sess, err := s.resolveSession(params.SessionID, false)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeSessionNotFound, Message: err.Error()},
			}
		}

		if err := sess.Abort(ctx); err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInternal, Message: err.Error()},
			}
		}

		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"status": "aborted", "session_id": sess.ID},
		}

	case MethodSessionReset:
		var params SessionResetParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()},
				}
			}
		}
		if strings.TrimSpace(params.SessionID) == "" {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInvalidParams, Message: "session_id is required for session.reset"},
			}
		}

		sess, err := s.resolveSession(params.SessionID, false)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeSessionNotFound, Message: err.Error()},
			}
		}

		sess.Reset()
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"status": "reset", "session_id": sess.ID},
		}

	case MethodSessionClose:
		var params SessionCloseParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()},
				}
			}
		}
		if strings.TrimSpace(params.SessionID) == "" {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInvalidParams, Message: "session_id is required for session.close"},
			}
		}

		sess, err := s.resolveSession(params.SessionID, false)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeSessionNotFound, Message: err.Error()},
			}
		}

		s.mu.Lock()
		delete(s.sessions, sess.ID)
		s.mu.Unlock()

		sess.Close()
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"status": "closed", "session_id": sess.ID},
		}

	case MethodSessionGetStatus:
		var params SessionStatusParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}

		sess, err := s.resolveSession(params.SessionID, false)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeSessionNotFound, Message: err.Error()},
			}
		}

		status, err := sess.Status(ctx)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &RPCError{Code: ErrCodeInternal, Message: err.Error()},
			}
		}

		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  status,
		}

	default:
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    ErrCodeMethodNotFound,
				Message: fmt.Sprintf("method %q not recognized", req.Method),
			},
		}
	}
}

func (s *Server) resolveSession(id string, createIfDefault bool) (*SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id != "" {
		sess, ok := s.sessions[id]
		if !ok {
			return nil, fmt.Errorf("session %q not found", id)
		}
		return sess, nil
	}

	// Auto-resolve: if exactly one session exists, use it
	if len(s.sessions) == 1 {
		for _, sess := range s.sessions {
			return sess, nil
		}
	}

	// Default fallback: if a default session exists, use it
	if sess, ok := s.sessions["default"]; ok {
		return sess, nil
	}

	// Only create default session on-the-fly for prompt execution if requested
	if createIfDefault {
		def, err := NewSessionState("default", s.client, SessionCreateParams{
			WorkDir:  s.cfg.DefaultWorkDir,
			Mode:     s.cfg.DefaultMode,
			YOLO:     &s.cfg.DefaultYOLO,
			MaxTurns: s.cfg.DefaultMaxTurns,
		})
		if err != nil {
			return nil, err
		}
		s.sessions["default"] = def
		return def, nil
	}

	return nil, fmt.Errorf("no active session found")
}

// ResolveSession returns the active session state for the given session ID.
func (s *Server) ResolveSession(id string) (*SessionState, error) {
	return s.resolveSession(id, false)
}

