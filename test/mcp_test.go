// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boggycreek/lokol/pkg/mcp"
	"github.com/boggycreek/lokol/pkg/tools/refinery"
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

func TestLokolMCP_SubprocessIntegration(t *testing.T) {
	// Build or locate lokol-mcp
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lokol-mcp")

	cmdBuild := exec.Command("go", "build", "-o", binPath, "github.com/boggycreek/lokol/cmd/lokol-mcp")
	if out, err := cmdBuild.CombinedOutput(); err != nil {
		t.Fatalf("failed to build lokol-mcp binary: %v\nOutput: %s", err, string(out))
	}

	// Create a test file for read_outline and read_window
	sampleFile := filepath.Join(tmpDir, "sample.go")
	sampleContent := `package sample

type User struct {
	ID   int
	Name string
}

func NewUser(id int, name string) *User {
	return &User{ID: id, Name: name}
}
`
	if err := os.WriteFile(sampleFile, []byte(sampleContent), 0644); err != nil {
		t.Fatalf("failed to write sample file: %v", err)
	}

	// Prepare stdin JSON-RPC requests
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_outline","arguments":{"path":%q}}}`, sampleFile),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_window","arguments":{"path":%q,"start_line":1,"end_line":6}}}`, sampleFile),
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"test_verifier","arguments":{"command":"echo 'PASS: tests ok'"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"run_test","arguments":{"command":"echo 'FAIL: test assertion'; exit 1"}}}`,
	}

	inputData := strings.Join(requests, "\n") + "\n"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Stdin = strings.NewReader(inputData)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("lokol-mcp subprocess failed: %v\nStderr: %s", err, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 6 { // 7 requests minus 1 notification = 6 responses
		t.Fatalf("expected 6 response lines, got %d:\n%s", len(lines), stdout.String())
	}

	// Verify initialize response
	var initResp struct {
		ID     int `json:"id"`
		Result struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("failed to parse init resp: %v", err)
	}
	if initResp.Result.ServerInfo.Name != "lokol-mcp" {
		t.Errorf("got server name %q, want 'lokol-mcp'", initResp.Result.ServerInfo.Name)
	}

	// Verify tools/list contains read_outline, read_window, test_verifier, run_test
	var listResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &listResp); err != nil {
		t.Fatalf("failed to parse list resp: %v", err)
	}
	toolNames := make(map[string]bool)
	for _, t := range listResp.Result.Tools {
		toolNames[t.Name] = true
	}
	for _, expected := range []string{"read_outline", "read_window", "test_verifier", "run_test"} {
		if !toolNames[expected] {
			t.Errorf("expected tool %q in tools/list, got: %+v", expected, toolNames)
		}
	}

	// Verify read_outline result
	var outlineResp struct {
		Result mcp.ToolCallResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &outlineResp); err != nil {
		t.Fatalf("failed to parse outline resp: %v", err)
	}
	if outlineResp.Result.IsError || len(outlineResp.Result.Content) == 0 {
		t.Fatalf("expected successful outline, got: %+v", outlineResp.Result)
	}
	outlineText := outlineResp.Result.Content[0].Text
	if !strings.Contains(outlineText, "type User struct") || !strings.Contains(outlineText, "func NewUser") {
		t.Errorf("outline does not contain expected symbols: %s", outlineText)
	}

	// Verify read_window result
	var windowResp struct {
		Result mcp.ToolCallResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[3]), &windowResp); err != nil {
		t.Fatalf("failed to parse window resp: %v", err)
	}
	if windowResp.Result.IsError || len(windowResp.Result.Content) == 0 {
		t.Fatalf("expected successful window, got: %+v", windowResp.Result)
	}
	windowText := windowResp.Result.Content[0].Text
	if !strings.Contains(windowText, "Lines 1-6") || !strings.Contains(windowText, "package sample") {
		t.Errorf("window does not contain expected lines: %s", windowText)
	}

	// Verify test_verifier success result
	var testPassResp struct {
		Result mcp.ToolCallResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[4]), &testPassResp); err != nil {
		t.Fatalf("failed to parse test_verifier resp: %v", err)
	}
	passText := testPassResp.Result.Content[0].Text
	if !strings.Contains(passText, "PASS") {
		t.Errorf("expected pass text to contain PASS, got: %s", passText)
	}

	// Verify run_test failure result (contains FAIL error output)
	var testFailResp struct {
		Result mcp.ToolCallResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[5]), &testFailResp); err != nil {
		t.Fatalf("failed to parse run_test failure resp: %v", err)
	}
	failText := testFailResp.Result.Content[0].Text
	if !strings.Contains(failText, "FAIL") {
		t.Errorf("expected fail text to contain FAIL, got: %s", failText)
	}
}

// Suppress unused refinery import if needed
var _ = refinery.ReadOutline
