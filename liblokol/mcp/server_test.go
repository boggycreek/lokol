// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/mcp"
)

func TestMCPServer_ProtocolBasics(t *testing.T) {
	server := mcp.NewServer("test-mcp", "v1.0.0")

	server.RegisterTool(mcp.Tool{
		Name:        "echo_tool",
		Description: "Echoes input text",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"text": {Type: "string", Description: "Text to echo"},
			},
			Required: []string{"text"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			txt, _ := args["text"].(string)
			if txt == "" {
				return "", true, fmt.Errorf("empty text")
			}
			return "echo: " + txt, false, nil
		},
	})

	ctx := context.Background()

	// 1. Test initialize
	initReq := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	resp, err := server.HandleMessage(ctx, initReq)
	if err != nil {
		t.Fatalf("unexpected error on initialize: %v", err)
	}
	if resp == nil || resp.Error != nil {
		t.Fatalf("expected valid response on initialize, got: %+v", resp)
	}
	initResult, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result on initialize, got %T", resp.Result)
	}
	if initResult["protocolVersion"] != mcp.ProtocolVersion {
		t.Errorf("got protocolVersion %v, want %v", initResult["protocolVersion"], mcp.ProtocolVersion)
	}

	// 2. Test notifications/initialized (expects nil response)
	notifReq := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	resp, err = server.HandleMessage(ctx, notifReq)
	if err != nil {
		t.Fatalf("unexpected error on notification: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for notification, got %+v", resp)
	}

	// 3. Test ping
	pingReq := []byte(`{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	resp, err = server.HandleMessage(ctx, pingReq)
	if err != nil {
		t.Fatalf("unexpected error on ping: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error in ping response: %+v", resp.Error)
	}

	// 4. Test tools/list
	listReq := []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
	resp, err = server.HandleMessage(ctx, listReq)
	if err != nil {
		t.Fatalf("unexpected error on tools/list: %v", err)
	}
	listResult, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result on tools/list, got %T", resp.Result)
	}
	tools, ok := listResult["tools"].([]map[string]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool in list, got %+v", listResult["tools"])
	}
	if tools[0]["name"] != "echo_tool" {
		t.Errorf("got tool name %q, want 'echo_tool'", tools[0]["name"])
	}

	// 5. Test tools/call (success)
	callReq := []byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"echo_tool","arguments":{"text":"hello"}}}`)
	resp, err = server.HandleMessage(ctx, callReq)
	if err != nil {
		t.Fatalf("unexpected error on tools/call: %v", err)
	}
	callResult, ok := resp.Result.(mcp.ToolCallResult)
	if !ok {
		t.Fatalf("expected ToolCallResult, got %T", resp.Result)
	}
	if callResult.IsError {
		t.Errorf("expected isError=false, got true")
	}
	if len(callResult.Content) != 1 || callResult.Content[0].Text != "echo: hello" {
		t.Errorf("unexpected content: %+v", callResult.Content)
	}

	// 6. Test tools/call (tool error)
	callErrReq := []byte(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"echo_tool","arguments":{"text":""}}}`)
	resp, err = server.HandleMessage(ctx, callErrReq)
	if err != nil {
		t.Fatalf("unexpected error on tools/call error test: %v", err)
	}
	callResult, ok = resp.Result.(mcp.ToolCallResult)
	if !ok || !callResult.IsError {
		t.Errorf("expected tool error result with IsError=true, got %+v", resp.Result)
	}

	// 7. Test unknown tool
	unknownReq := []byte(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"nonexistent","arguments":{}}}`)
	resp, err = server.HandleMessage(ctx, unknownReq)
	if err != nil {
		t.Fatalf("unexpected error on unknown tool: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != mcp.ErrCodeMethodNotFound {
		t.Errorf("expected MethodNotFound error, got %+v", resp.Error)
	}

	// 8. Test unknown method
	unknownMethodReq := []byte(`{"jsonrpc":"2.0","id":7,"method":"foo/bar"}`)
	resp, err = server.HandleMessage(ctx, unknownMethodReq)
	if err != nil {
		t.Fatalf("unexpected error on unknown method: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != mcp.ErrCodeMethodNotFound {
		t.Errorf("expected MethodNotFound error for unknown method, got %+v", resp.Error)
	}
}

func TestMCPServer_ServeStream(t *testing.T) {
	server := mcp.NewServer("test-stream", "v1.0.0")
	server.RegisterTool(mcp.Tool{
		Name: "ping_tool",
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			return "pong", false, nil
		},
	})

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ping_tool","arguments":{}}}`,
	}, "\n") + "\n"

	in := strings.NewReader(input)
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := server.Serve(ctx, in, &out); err != nil {
		t.Fatalf("server.Serve returned error: %v", err)
	}

	outputLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(outputLines) != 2 {
		t.Fatalf("expected 2 output lines (notification produces no output), got %d: %v", len(outputLines), outputLines)
	}

	var resp1, resp2 map[string]any
	if err := json.Unmarshal([]byte(outputLines[0]), &resp1); err != nil {
		t.Fatalf("failed to parse line 1: %v", err)
	}
	if resp1["id"].(float64) != 1 {
		t.Errorf("expected id=1, got %v", resp1["id"])
	}

	if err := json.Unmarshal([]byte(outputLines[1]), &resp2); err != nil {
		t.Fatalf("failed to parse line 2: %v", err)
	}
	if resp2["id"].(float64) != 2 {
		t.Errorf("expected id=2, got %v", resp2["id"])
	}
}
