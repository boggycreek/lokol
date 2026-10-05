// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// Client represents a connected IPC client communicating with the lokol-daemon.
type Client struct {
	conn      net.Conn
	reader    *bufio.Reader
	writeMu   sync.Mutex
	reqID     uint64
	pendingMu sync.Mutex
	pending   map[string]chan Response

	eventMu   sync.RWMutex
	onEvent   func(Notification)
	closeOnce sync.Once
	closed    chan struct{}
}

// Dial connects to a lokol-daemon over a UNIX domain socket or TCP address.
func Dial(network, address string) (*Client, error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}

	c := &Client{
		conn:    conn,
		reader:  bufio.NewReader(conn),
		pending: make(map[string]chan Response),
		closed:  make(chan struct{}),
	}

	go c.readLoop()
	return c, nil
}

// SetEventHandler registers a callback for server notifications/events.
func (c *Client) SetEventHandler(handler func(Notification)) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.onEvent = handler
}

func (c *Client) readLoop() {
	defer c.Close()

	for {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			return
		}

		if len(line) == 0 || (len(line) == 1 && line[0] == '\n') {
			continue
		}

		// Try unmarshaling as Response (has ID)
		var resp Response
		if jsonErr := json.Unmarshal(line, &resp); jsonErr == nil && len(resp.ID) > 0 {
			idStr := string(resp.ID)
			c.pendingMu.Lock()
			ch, ok := c.pending[idStr]
			if ok {
				delete(c.pending, idStr)
			}
			c.pendingMu.Unlock()

			if ok {
				ch <- resp
				continue
			}
		}

		// Try unmarshaling as Notification (event)
		var notif Notification
		if jsonErr := json.Unmarshal(line, &notif); jsonErr == nil && notif.Method != "" {
			c.eventMu.RLock()
			handler := c.onEvent
			c.eventMu.RUnlock()
			if handler != nil {
				handler(notif)
			}
		}
	}
}

// Call sends a JSON-RPC request and awaits the corresponding response.
func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	idNum := atomic.AddUint64(&c.reqID, 1)
	idRaw, _ := json.Marshal(idNum)

	var paramsRaw json.RawMessage
	if params != nil {
		pBytes, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params error: %w", err)
		}
		paramsRaw = pBytes
	}

	req := Request{
		JSONRPC: "2.0",
		ID:      idRaw,
		Method:  method,
		Params:  paramsRaw,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}

	respChan := make(chan Response, 1)
	idStr := string(idRaw)

	c.pendingMu.Lock()
	c.pending[idStr] = respChan
	c.pendingMu.Unlock()

	c.writeMu.Lock()
	_, err = c.conn.Write(append(reqBytes, '\n'))
	c.writeMu.Unlock()

	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, idStr)
		c.pendingMu.Unlock()
		return fmt.Errorf("write request error: %w", err)
	}

	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, idStr)
		c.pendingMu.Unlock()
		return ctx.Err()
	case <-c.closed:
		return fmt.Errorf("client connection closed")
	case resp := <-respChan:
		if resp.Error != nil {
			return fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		if result != nil && resp.Result != nil {
			resBytes, err := json.Marshal(resp.Result)
			if err != nil {
				return err
			}
			return json.Unmarshal(resBytes, result)
		}
		return nil
	}
}

// Ping sends a daemon.ping health check.
func (c *Client) Ping(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.Call(ctx, MethodDaemonPing, nil, &result)
	return result, err
}

// CreateSession initializes a new session on the daemon.
func (c *Client) CreateSession(ctx context.Context, params SessionCreateParams) (map[string]any, error) {
	var result map[string]any
	err := c.Call(ctx, MethodSessionCreate, params, &result)
	return result, err
}

// Prompt sends a prompt to the daemon session.
func (c *Client) Prompt(ctx context.Context, params SessionPromptParams) (*EventTurnFinishedPayload, error) {
	var result EventTurnFinishedPayload
	err := c.Call(ctx, MethodSessionPrompt, params, &result)
	return &result, err
}

// ApproveAction sends approval for a pending action.
func (c *Client) ApproveAction(ctx context.Context, sessionID, actionID string) error {
	params := ActionDecisionParams{
		SessionID: sessionID,
		ActionID:  actionID,
	}
	return c.Call(ctx, MethodSessionApproveAction, params, nil)
}

// RejectAction sends rejection for a pending action.
func (c *Client) RejectAction(ctx context.Context, sessionID, actionID, reason string) error {
	params := ActionDecisionParams{
		SessionID: sessionID,
		ActionID:  actionID,
		Reason:    reason,
	}
	return c.Call(ctx, MethodSessionRejectAction, params, nil)
}

// Abort cancels the active turn on the session.
func (c *Client) Abort(ctx context.Context, sessionID string) error {
	return c.Call(ctx, MethodSessionAbort, SessionAbortParams{SessionID: sessionID}, nil)
}

// Reset clears session history back to initial state.
func (c *Client) Reset(ctx context.Context, sessionID string) error {
	return c.Call(ctx, MethodSessionReset, SessionResetParams{SessionID: sessionID}, nil)
}

// GetStatus queries current session and slot metrics.
func (c *Client) GetStatus(ctx context.Context, sessionID string) (map[string]any, error) {
	var result map[string]any
	err := c.Call(ctx, MethodSessionGetStatus, SessionStatusParams{SessionID: sessionID}, &result)
	return result, err
}

// Close closes the client socket connection.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		err = c.conn.Close()
	})
	return err
}
