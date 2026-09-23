// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "install-llama.sh")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repository root containing install-llama.sh")
		}
		dir = parent
	}
}

func TestInstallLlamaScriptExecutable(t *testing.T) {
	root := findRepoRoot(t)
	scriptPath := filepath.Join(root, "install-llama.sh")

	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("install-llama.sh does not exist: %v", err)
	}

	if info.Mode()&0111 == 0 {
		t.Errorf("install-llama.sh is not executable: mode is %v", info.Mode())
	}
}

func TestInstallLlamaHelp(t *testing.T) {
	root := findRepoRoot(t)
	scriptPath := filepath.Join(root, "install-llama.sh")

	cmd := exec.Command(scriptPath, "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-llama.sh --help failed: %v, output: %s", err, string(out))
	}

	output := string(out)
	expectedPhrases := []string{
		"Usage: install-llama.sh",
		"--prebuilt",
		"--build-from-source",
		"--backend",
		"--bin-dir",
		"--install-dir",
		"--status",
		"--dry-run",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(output, phrase) {
			t.Errorf("expected --help output to contain %q, but was:\n%s", phrase, output)
		}
	}
}

func TestInstallLlamaDryRun(t *testing.T) {
	root := findRepoRoot(t)
	scriptPath := filepath.Join(root, "install-llama.sh")

	cmd := exec.Command(scriptPath, "--dry-run", "--backend", "cpu")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-llama.sh --dry-run failed: %v, output: %s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "[DRY-RUN]") {
		t.Errorf("expected [DRY-RUN] in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Backend  : cpu") && !strings.Contains(output, "Backend: cpu") {
		t.Errorf("expected cpu backend in output, got:\n%s", output)
	}
}

func TestInstallLlamaStatus(t *testing.T) {
	root := findRepoRoot(t)
	scriptPath := filepath.Join(root, "install-llama.sh")

	cmd := exec.Command(scriptPath, "--status")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-llama.sh --status failed: %v, output: %s", err, string(out))
	}

	output := string(out)
	expectedPhrases := []string{
		"llama.cpp & llama-server Diagnostic Status",
		"Engine Health Probe",
		"Hardware Acceleration Capabilities",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(output, phrase) {
			t.Errorf("expected --status output to contain %q, but was:\n%s", phrase, output)
		}
	}
}

func TestInstallLlamaInvalidBackend(t *testing.T) {
	root := findRepoRoot(t)
	scriptPath := filepath.Join(root, "install-llama.sh")

	cmd := exec.Command(scriptPath, "--backend", "nonexistent_backend")
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected install-llama.sh with invalid backend to fail, but succeeded")
	}

	stderrOut := errBuf.String()
	if !strings.Contains(stderrOut, "Unsupported backend") {
		t.Errorf("expected error about unsupported backend, got: %s", stderrOut)
	}
}

func TestInstallLlamaUnknownOption(t *testing.T) {
	root := findRepoRoot(t)
	scriptPath := filepath.Join(root, "install-llama.sh")

	cmd := exec.Command(scriptPath, "--unrecognized-flag-xyz")
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected unknown option to exit non-zero")
	}
}
