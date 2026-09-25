// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

const ProtocolVersion = "2024-11-05"

// JSON-RPC 2.0 Error Codes
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
)

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ToolInputSchema describes JSON Schema for tool parameters.
type ToolInputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]PropertyDoc `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// PropertyDoc describes a single tool parameter.
type PropertyDoc struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// Tool represents an MCP tool definition and handler.
type Tool struct {
	Name        string                                                                 `json:"name"`
	Description string                                                                 `json:"description"`
	InputSchema ToolInputSchema                                                        `json:"inputSchema"`
	Handler     func(ctx context.Context, args map[string]any) (content string, isError bool, err error) `json:"-"`
}

// TextContent represents text content inside an MCP tool call result.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolCallResult represents the result payload of tools/call.
type ToolCallResult struct {
	Content []TextContent `json:"content"`
	IsError bool          `json:"isError"`
}

// Server is an MCP JSON-RPC stdio server.
type Server struct {
	Name    string
	Version string

	mu    sync.RWMutex
	tools map[string]Tool
}

// NewServer creates a new MCP server with the given name and version.
func NewServer(name, version string) *Server {
	return &Server{
		Name:    name,
		Version: version,
		tools:   make(map[string]Tool),
	}
}

// RegisterTool registers an MCP tool.
func (s *Server) RegisterTool(tool Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
}

// GetTools returns the list of registered tools.
func (s *Server) GetTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tools := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, t)
	}
	return tools
}

// HandleMessage parses and processes a single JSON-RPC message.
// If the message is a notification (no ID), returns nil response.
func (s *Server) HandleMessage(ctx context.Context, raw []byte) (*Response, error) {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error: &RPCError{
				Code:    ErrCodeParseError,
				Message: fmt.Sprintf("Parse error: %v", err),
			},
		}, nil
	}

	if req.JSONRPC != "2.0" {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    ErrCodeInvalidRequest,
				Message: "Invalid JSON-RPC version: must be '2.0'",
			},
		}, nil
	}

	// Notifications have no ID and expect no response
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		res := map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    s.Name,
				"version": s.Version,
			},
		}
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  res,
		}, nil

	case "notifications/initialized":
		return nil, nil

	case "ping":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{},
		}, nil

	case "tools/list":
		s.mu.RLock()
		toolList := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			toolList = append(toolList, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			})
		}
		s.mu.RUnlock()

		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": toolList,
			},
		}, nil

	case "tools/call":
		var callParams struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &callParams); err != nil {
			return &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &RPCError{
					Code:    ErrCodeInvalidParams,
					Message: fmt.Sprintf("Invalid params for tools/call: %v", err),
				},
			}, nil
		}

		s.mu.RLock()
		tool, exists := s.tools[callParams.Name]
		s.mu.RUnlock()

		if !exists {
			return &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &RPCError{
					Code:    ErrCodeMethodNotFound,
					Message: fmt.Sprintf("Tool not found: %s", callParams.Name),
				},
			}, nil
		}

		content, isError, err := tool.Handler(ctx, callParams.Arguments)
		if err != nil {
			return &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: ToolCallResult{
					Content: []TextContent{
						{Type: "text", Text: err.Error()},
					},
					IsError: true,
				},
			}, nil
		}

		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: ToolCallResult{
				Content: []TextContent{
					{Type: "text", Text: content},
				},
				IsError: isError,
			},
		}, nil

	default:
		if isNotification {
			return nil, nil
		}
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    ErrCodeMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}, nil
	}
}

// Serve reads JSON-RPC requests line-by-line from in, dispatches them, and writes responses to out.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	// Allow large messages up to 4MB
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		resp, err := s.HandleMessage(ctx, line)
		if err != nil {
			return err
		}
		if resp == nil {
			// Notification, no reply expected
			continue
		}

		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}

		if _, err := out.Write(append(data, '\n')); err != nil {
			return err
		}
	}

	return scanner.Err()
}
