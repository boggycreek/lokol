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

func TestLK_Run_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lk", "--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for --version, got %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "lk") {
		t.Errorf("expected stdout to contain 'lk', got: %s", out)
	}
}

func TestLK_Run_Help(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{"lk", flag}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0 for %s, got %d", flag, code)
		}
		out := stderr.String()
		if !strings.Contains(out, "Usage: lk") {
			t.Errorf("expected stderr to contain 'Usage: lk' for %s, got: %s", flag, out)
		}
	}
}

func TestLK_Run_InvalidMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lk", "-m", "nonexistent-mode"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 for invalid mode, got %d", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "Error:") {
		t.Errorf("expected error message on stderr, got: %s", errStr)
	}
}

func TestLK_Run_UnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lk", "-unknown-flag-xyz"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code for unknown flag, got %d", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "flag provided but not defined") {
		t.Errorf("expected flag error on stderr, got: %s", errStr)
	}
}
