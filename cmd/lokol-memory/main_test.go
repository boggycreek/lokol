// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestLokolMemory_Run_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol-memory", "--version"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for --version, got %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "lokol-memory") {
		t.Errorf("expected version output to contain 'lokol-memory', got: %s", out)
	}
}

func TestLokolMemory_Run_Help(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{"lokol-memory", flag}, nil, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0 for %s, got %d", flag, code)
		}
		errStr := stderr.String()
		if !strings.Contains(errStr, "memory-root") {
			t.Errorf("expected usage output to mention 'memory-root', got: %s", errStr)
		}
	}
}

func TestLokolMemory_Run_UnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol-memory", "-unknown-flag-xyz"}, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code for unknown flag, got %d", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "flag provided but not defined") {
		t.Errorf("expected flag error on stderr, got: %s", errStr)
	}
}

func TestLokolMemory_Run_InProcessJSONRPC(t *testing.T) {
	tmpDir := t.TempDir()

	stdinInput := strings.Join([]string{
		// 1. Initialize
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		// 2. Tools list
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		// 3. Store memory
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"store_memory","arguments":{"category":"semantic","domain":"auth","title":"OAuth Bearer Flow","abstract":"Validates token signatures via local key cache.","summary":"Requests pass BearerAuth middleware.\nActionable Rules: 1. Check Bearer prefix 2. Verify sig.","details":"Technical background and sample curl commands","extra_data":{"jwks_ttl":300}}}}`,
		// 4. List memories (Tier 1)
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_memories","arguments":{"category":"semantic"}}}`,
		// 5. Recall memory (Tier 2)
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"recall_memory","arguments":{"query":"OAuth Bearer"}}}`,
		// 6. Summarize episodic
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"summarize_episodic","arguments":{"period":"day"}}}`,
	}, "\n") + "\n"

	stdin := strings.NewReader(stdinInput)
	var stdout, stderr bytes.Buffer

	code := run([]string{"lokol-memory", "--memory-root=" + tmpDir}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for in-process JSON-RPC, got %d, stderr: %s", code, stderr.String())
	}

	outStr := stdout.String()

	// Verify initialize response
	if !strings.Contains(outStr, `"protocolVersion":"2024-11-05"`) {
		t.Errorf("expected initialize response, got: %s", outStr)
	}

	// Verify tools/list contains all 5 cognitive tools
	expectedTools := []string{"store_memory", "recall_memory", "list_memories", "get_memory", "summarize_episodic"}
	for _, tool := range expectedTools {
		if !strings.Contains(outStr, fmt.Sprintf(`"name":"%s"`, tool)) {
			t.Errorf("expected tool %s in tools/list, got: %s", tool, outStr)
		}
	}

	// Verify store_memory response
	if !strings.Contains(outStr, "Memory stored successfully") {
		t.Errorf("expected store_memory success, got: %s", outStr)
	}

	// Verify list_memories Tier 1 signpost output
	if !strings.Contains(outStr, "OAuth Bearer Flow") {
		t.Errorf("expected list_memories to include OAuth Bearer Flow, got: %s", outStr)
	}

	// Verify recall_memory Tier 2 output
	if !strings.Contains(outStr, "=== Memory: OAuth Bearer Flow") || !strings.Contains(outStr, "BearerAuth middleware") {
		t.Errorf("expected recall_memory Tier 2 digest, got: %s", outStr)
	}

	// Verify summarize_episodic response
	if !strings.Contains(outStr, "Episodic Summary") {
		t.Errorf("expected episodic summary response, got: %s", outStr)
	}
}
