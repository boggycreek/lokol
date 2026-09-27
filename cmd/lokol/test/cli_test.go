// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestLokolCLI_Version(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lokol version failed: %v, output: %s", err, out)
	}
	output := string(out)
	if !strings.Contains(output, "lokol") {
		t.Errorf("expected version output to contain 'lokol', got: %s", output)
	}
}

func TestLokolCLI_Usage(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go")
	out, _ := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected usage output, got: %s", output)
	}
	if !strings.Contains(output, "lk") {
		t.Errorf("expected usage to mention 'lk', got: %s", output)
	}
	if !strings.Contains(output, "lokol exec") {
		t.Errorf("expected usage to mention 'lokol exec', got: %s", output)
	}
}

func TestLokolCLI_UnknownCommand(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "unknowncmd")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for unknown subcommand, got nil (output: %s)", out)
	}
	output := string(out)
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected usage output on unknown subcommand, got: %s", output)
	}
}

func TestLokolCLI_ProbeSimulation(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "probe", "--simulate-vram-gib=8.0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		t.Fatalf("lokol probe failed: %v, stderr: %s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "lokol System Hardware Capability Probe") {
		t.Errorf("expected probe banner, got: %s", output)
	}
	if !strings.Contains(output, "Target Tier") {
		t.Errorf("expected Target Tier in probe output, got: %s", output)
	}
}
