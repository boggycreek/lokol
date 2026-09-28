// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestLokolMCP_Run_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol-mcp", "--version"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for --version, got %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "lokol-mcp") {
		t.Errorf("expected version output to contain 'lokol-mcp', got: %s", out)
	}
}

func TestLokolMCP_Run_Help(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{"lokol-mcp", flag}, nil, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0 for %s, got %d", flag, code)
		}
		errStr := stderr.String()
		if !strings.Contains(errStr, "version") {
			t.Errorf("expected usage output to mention 'version', got: %s", errStr)
		}
	}
}

func TestLokolMCP_Run_UnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol-mcp", "-unknown-flag-xyz"}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code for unknown flag, got %d", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "flag provided but not defined") {
		t.Errorf("expected flag error on stderr, got: %s", errStr)
	}
}

func TestLokolMCP_Run_InProcessJSONRPC(t *testing.T) {
	stdinInput := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n") + "\n"

	stdin := strings.NewReader(stdinInput)
	var stdout, stderr bytes.Buffer

	code := run([]string{"lokol-mcp"}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for in-process JSON-RPC, got %d, stderr: %s", code, stderr.String())
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, `"protocolVersion":"2024-11-05"`) {
		t.Errorf("expected initialize response, got: %s", outStr)
	}
	if !strings.Contains(outStr, `"read_outline"`) || !strings.Contains(outStr, `"write_file"`) {
		t.Errorf("expected tools list in output, got: %s", outStr)
	}
}
