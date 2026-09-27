// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main_test

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

	"github.com/boggycreek/lokol/liblokol/mcp"
)

func TestLokolMCP_SubprocessIntegration(t *testing.T) {
	// Build or locate lokol-mcp
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "lokol-mcp")

	cmdBuild := exec.Command("go", "build", "-o", binPath, ".")
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

	exec.Command("git", "init", tmpDir).Run()

	// Prepare stdin JSON-RPC requests
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_outline","arguments":{"path":%q}}}`, sampleFile),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_window","arguments":{"path":%q,"start_line":1,"end_line":6}}}`, sampleFile),
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"test_verifier","arguments":{"command":"echo 'PASS: tests ok'"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"run_test","arguments":{"command":"echo 'FAIL: test assertion'; exit 1"}}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_environment","arguments":{"path":%q}}}`, tmpDir),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"find_files","arguments":{"path":%q,"pattern":"*.go"}}}`, tmpDir),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"search_code","arguments":{"path":%q,"pattern":"NewUser"}}}`, tmpDir),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"git_diff_summary","arguments":{"path":%q}}}`, tmpDir),
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
	if len(lines) != 10 { // 11 requests minus 1 notification = 10 responses
		t.Fatalf("expected 10 response lines, got %d:\n%s", len(lines), stdout.String())
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
	for _, expected := range []string{"read_outline", "read_window", "test_verifier", "run_test", "get_environment", "find_files", "search_code", "git_diff_summary"} {
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

	// Verify get_environment result
	var envResp struct {
		Result mcp.ToolCallResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[6]), &envResp); err != nil {
		t.Fatalf("failed to parse get_environment resp: %v", err)
	}
	if envResp.Result.IsError || len(envResp.Result.Content) == 0 {
		t.Fatalf("expected successful get_environment result, got: %+v", envResp.Result)
	}
	envText := envResp.Result.Content[0].Text
	if !strings.Contains(envText, "working_directory") || !strings.Contains(envText, tmpDir) {
		t.Errorf("expected get_environment output to contain working_directory %q, got: %s", tmpDir, envText)
	}

	// Verify find_files result
	var findFilesResp struct {
		Result mcp.ToolCallResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[7]), &findFilesResp); err != nil {
		t.Fatalf("failed to parse find_files resp: %v", err)
	}
	if findFilesResp.Result.IsError || len(findFilesResp.Result.Content) == 0 {
		t.Fatalf("expected successful find_files result, got: %+v", findFilesResp.Result)
	}
	findFilesText := findFilesResp.Result.Content[0].Text
	if !strings.Contains(findFilesText, "sample.go") {
		t.Errorf("expected find_files output to contain 'sample.go', got: %s", findFilesText)
	}

	// Verify search_code result
	var searchCodeResp struct {
		Result mcp.ToolCallResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[8]), &searchCodeResp); err != nil {
		t.Fatalf("failed to parse search_code resp: %v", err)
	}
	if searchCodeResp.Result.IsError || len(searchCodeResp.Result.Content) == 0 {
		t.Fatalf("expected successful search_code result, got: %+v", searchCodeResp.Result)
	}
	searchCodeText := searchCodeResp.Result.Content[0].Text
	if !strings.Contains(searchCodeText, "sample.go") || !strings.Contains(searchCodeText, "NewUser") {
		t.Errorf("expected search_code output to contain sample.go and NewUser, got: %s", searchCodeText)
	}

	// Verify git_diff_summary result
	var diffResp struct {
		Result mcp.ToolCallResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[9]), &diffResp); err != nil {
		t.Fatalf("failed to parse git_diff_summary resp: %v", err)
	}
	if diffResp.Result.IsError || len(diffResp.Result.Content) == 0 {
		t.Fatalf("expected successful git_diff_summary result, got: %+v", diffResp.Result)
	}
	diffText := diffResp.Result.Content[0].Text
	if !strings.Contains(diffText, "Untracked files") || !strings.Contains(diffText, "sample.go") {
		t.Errorf("expected git_diff_summary output to contain sample.go, got: %s", diffText)
	}
}
