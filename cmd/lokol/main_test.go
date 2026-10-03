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

func TestLokol_Run_VersionSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol", "version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for 'version', got %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "lokol") {
		t.Errorf("expected version output to contain 'lokol', got: %s", out)
	}
	if !strings.Contains(out, "commit:") {
		t.Errorf("expected version output to contain 'commit:', got: %s", out)
	}
}

func TestLokol_Run_VersionTopLevelFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol", "--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for '--version', got %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "lokol") {
		t.Errorf("expected version output to contain 'lokol', got: %s", out)
	}
}

func TestLokol_Run_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 for no args, got %d", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "Usage:") {
		t.Errorf("expected usage output on stderr, got: %s", errStr)
	}
}

func TestLokol_Run_HelpFlag(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{"lokol", flag}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0 for %s, got %d", flag, code)
		}
		out := stdout.String()
		if !strings.Contains(out, "Usage:") {
			t.Errorf("expected usage output on stdout for %s, got: %s", flag, out)
		}
	}
}

func TestLokol_Run_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol", "nonexistent-cmd"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 for unknown subcommand, got %d", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "Usage:") {
		t.Errorf("expected usage on stderr for unknown subcommand, got: %s", errStr)
	}
}

func TestLokol_Run_ProbeSimulation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol", "probe", "--simulate-vram-gib=8.0", "-m", "coding"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for probe simulation, got %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "lokol System Hardware Capability Probe") {
		t.Errorf("expected probe banner, got: %s", out)
	}
	if !strings.Contains(out, "Target Mode    : coding") {
		t.Errorf("expected Target Mode : coding, got: %s", out)
	}
	if !strings.Contains(out, "Tier 2") {
		t.Errorf("expected Tier 2 for 8.0 GiB, got: %s", out)
	}
}

func TestLokol_Run_ExecMissingPrompt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol", "exec"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 for exec without prompt, got %d", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "prompt required for exec") {
		t.Errorf("expected 'prompt required for exec' in stderr, got: %s", errStr)
	}
}

func TestLokol_Run_ConfigCommands(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// 1. Show config (defaults)
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol", "config", "show"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for config show, got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "agent_name") || !strings.Contains(stdout.String(), "lokol") {
		t.Errorf("expected default agent_name in config show, got: %s", stdout.String())
	}

	// 2. Set agent name
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"lokol", "config", "set-name", "Aria"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for set-name, got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Aria") {
		t.Errorf("expected confirmation of Aria, got: %s", stdout.String())
	}

	// 3. Set operator name
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"lokol", "config", "set-operator", "Alice"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for set-operator, got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Alice") {
		t.Errorf("expected confirmation of Alice, got: %s", stdout.String())
	}

	// 4. Verify config show reflects new values
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"lokol", "config"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for config, got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Aria") || !strings.Contains(stdout.String(), "Alice") {
		t.Errorf("expected Aria and Alice in config, got: %s", stdout.String())
	}
}

func TestLokol_Run_ExecSpec_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol", "exec", "--spec", "nonexistent-spec-file.md"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 for missing spec file, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Error loading spec") {
		t.Errorf("expected 'Error loading spec' in stderr, got: %s", stderr.String())
	}
}

