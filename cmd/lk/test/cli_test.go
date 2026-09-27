// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestLKCLI_Version(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lk --version failed: %v, output: %s", err, out)
	}
	output := string(out)
	if !strings.Contains(output, "lk") {
		t.Errorf("expected version output to contain 'lk', got: %s", output)
	}
}

func TestLKCLI_Usage(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "--help")
	out, _ := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected usage output, got: %s", output)
	}
	if !strings.Contains(output, "lk [options] [prompt]") {
		t.Errorf("expected usage synopsis, got: %s", output)
	}
	if !strings.Contains(output, "-engine") {
		t.Errorf("expected -engine in options, got: %s", output)
	}
}
