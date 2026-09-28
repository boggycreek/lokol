// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var lkBin string

func TestMain(m *testing.M) {
	// Compile the lk CLI binary once for blackbox tests
	tempDir, err := os.MkdirTemp("", "lk-test-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir for lk test binary: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)

	binPath := filepath.Join(tempDir, "lk")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build lk test binary: %v, output: %s\n", err, out)
		os.Exit(1)
	}

	lkBin = binPath
	os.Exit(m.Run())
}

func TestLKCLI_Version(t *testing.T) {
	cmd := exec.Command(lkBin, "--version")
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
	cmd := exec.Command(lkBin, "--help")
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
	if !strings.Contains(output, "-yolo") {
		t.Errorf("expected -yolo in options, got: %s", output)
	}
	if !strings.Contains(output, "-mode") {
		t.Errorf("expected -mode in options, got: %s", output)
	}
}

func TestLKCLI_InvalidMode(t *testing.T) {
	cmd := exec.Command(lkBin, "-m", "invalid_mode_name")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected error when passing invalid mode, got nil")
	}
	errOutput := stderr.String()
	if !strings.Contains(errOutput, "invalid mode") {
		t.Errorf("expected 'invalid mode' in stderr, got: %s", errOutput)
	}
}
