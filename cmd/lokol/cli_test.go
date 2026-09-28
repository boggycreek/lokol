// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var lokolBin string

// TestMain compiles the lokol binary once into a temporary directory for high-speed blackbox execution.
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "lokol-bin-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp test dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, "lokol")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build lokol binary: %v\nOutput: %s\n", err, out)
		os.Exit(1)
	}
	lokolBin = binPath

	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// 1. Version Subcommand & Flag
// ---------------------------------------------------------------------------

func TestLokolCLI_Version_Subcommand(t *testing.T) {
	cmd := exec.Command(lokolBin, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lokol version failed: %v, output: %s", err, out)
	}
	output := string(out)
	if !strings.Contains(output, "lokol") {
		t.Errorf("expected version output to contain 'lokol', got: %s", output)
	}
	if !strings.Contains(output, "commit:") {
		t.Errorf("expected version output to contain 'commit:', got: %s", output)
	}
}

func TestLokolCLI_Version_TopLevelFlag(t *testing.T) {
	cmd := exec.Command(lokolBin, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lokol --version failed: %v, output: %s", err, out)
	}
	output := string(out)
	if !strings.Contains(output, "lokol") {
		t.Errorf("expected version output to contain 'lokol', got: %s", output)
	}
}

// ---------------------------------------------------------------------------
// 2. Usage & Unknown Command Validation
// ---------------------------------------------------------------------------

func TestLokolCLI_Usage_NoArgs(t *testing.T) {
	cmd := exec.Command(lokolBin)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit code when run without args, got nil")
	}
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
	if !strings.Contains(output, "lokol probe") {
		t.Errorf("expected usage to mention 'lokol probe', got: %s", output)
	}
}

func TestLokolCLI_Usage_HelpFlag(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		cmd := exec.Command(lokolBin, flag)
		out, _ := cmd.CombinedOutput()
		output := string(out)
		if !strings.Contains(output, "Usage:") {
			t.Errorf("expected usage banner with %s, got: %s", flag, output)
		}
	}
}

func TestLokolCLI_UnknownSubcommand(t *testing.T) {
	cmd := exec.Command(lokolBin, "unknowncmd")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for unknown subcommand, got nil (output: %s)", out)
	}
	output := string(out)
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected usage output on unknown subcommand, got: %s", output)
	}
}

// ---------------------------------------------------------------------------
// 3. Hardware Probing Matrix (`lokol probe`)
// ---------------------------------------------------------------------------

func TestLokolCLI_ProbeSimulation(t *testing.T) {
	cmd := exec.Command(lokolBin, "probe", "--simulate-vram-gib=8.0")
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
	if !strings.Contains(output, "Simulated GPU (8.0 GiB VRAM)") {
		t.Errorf("expected simulated VRAM acknowledgement, got: %s", output)
	}
}

func TestLokolCLI_ProbeWithMode(t *testing.T) {
	testCases := []struct {
		mode         string
		expectedMode string
		expectedModel string
	}{
		{"coding", "coding", "Qwen 2.5 Coder"},
		{"moe", "moe", "MoE"},
		{"general", "general", "Llama 3.1 8B"},
	}

	for _, tc := range testCases {
		cmd := exec.Command(lokolBin, "probe", "--simulate-vram-gib=8.0", "-m", tc.mode)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			t.Fatalf("lokol probe mode %s failed: %v, stderr: %s", tc.mode, err, stderr.String())
		}
		output := stdout.String()
		if !strings.Contains(output, "Target Mode    : "+tc.expectedMode) {
			t.Errorf("expected Target Mode : %s, got: %s", tc.expectedMode, output)
		}
		if !strings.Contains(output, tc.expectedModel) {
			t.Errorf("expected model %s in %s recommendation, got: %s", tc.expectedModel, tc.mode, output)
		}
	}
}

func TestLokolCLI_ProbeTiers(t *testing.T) {
	tiers := []struct {
		vramGiB      string
		expectedTier string
	}{
		{"16.0", "Tier 1"},
		{"8.0", "Tier 2"},
		{"4.0", "Tier 3"},
	}

	for _, tt := range tiers {
		cmd := exec.Command(lokolBin, "probe", "--simulate-vram-gib="+tt.vramGiB)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			t.Fatalf("probe tier %s failed: %v", tt.expectedTier, err)
		}
		if !strings.Contains(stdout.String(), tt.expectedTier) {
			t.Errorf("expected %s for %s GiB, got: %s", tt.expectedTier, tt.vramGiB, stdout.String())
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Setup & Update Subcommands
// ---------------------------------------------------------------------------

func TestLokolCLI_SetupHelp(t *testing.T) {
	cmd := exec.Command(lokolBin, "setup", "-h")
	out, _ := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "download-model") {
		t.Errorf("expected setup help to describe --download-model, got: %s", output)
	}
	if !strings.Contains(output, "install-llama") {
		t.Errorf("expected setup help to describe --install-llama, got: %s", output)
	}
}

func TestLokolCLI_UpdateHelp(t *testing.T) {
	cmd := exec.Command(lokolBin, "update", "-h")
	out, _ := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "list") {
		t.Errorf("expected update help to describe --list, got: %s", output)
	}
	if !strings.Contains(output, "pre") {
		t.Errorf("expected update help to describe --pre, got: %s", output)
	}
}

// ---------------------------------------------------------------------------
// 5. Exec Parameter Validation
// ---------------------------------------------------------------------------

func TestLokolCLI_ExecMissingPrompt(t *testing.T) {
	cmd := exec.Command(lokolBin, "exec")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected error for exec without prompt, got nil")
	}
	if !strings.Contains(stderr.String(), "prompt required") {
		t.Errorf("expected 'prompt required' error message, got: %s", stderr.String())
	}
}

func TestLokolCLI_TopLevelPromptFlagMissing(t *testing.T) {
	cmd := exec.Command(lokolBin, "-p", "")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected error for empty -p flag, got nil")
	}
}

// ---------------------------------------------------------------------------
// 6. Blackbox End-to-End Execution with Mock Engine
// ---------------------------------------------------------------------------

func TestLokolCLI_Exec_MockExecution_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Emit response with task_finish
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello from local agent!\\n<action name=\\\"task_finish\\\">All tasks done.</action>\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer mockServer.Close()

	cmd := exec.Command(lokolBin, "exec", "--engine="+mockServer.URL, "Say hello")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("lokol exec failed: %v, stderr: %s", err, stderr.String())
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "All tasks done.") && !strings.Contains(outStr, "Complete") {
		t.Errorf("expected completion message on stdout, got: %s", outStr)
	}
}

func TestLokolCLI_TopLevel_PromptFlag_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Top-level prompt executed successfully.\\n<action name=\\\"task_finish\\\">Done</action>\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer mockServer.Close()

	cmd := exec.Command(lokolBin, "-p", "Test top level prompt", "--engine="+mockServer.URL)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("lokol -p failed: %v, stderr: %s", err, stderr.String())
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "Complete") {
		t.Errorf("expected completion output, got: %s", outStr)
	}
}

func TestLokolCLI_Exec_StdoutStderrStreamSeparation(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Action step followed by task finish
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"<action name=\\\"find_files\\\"><pattern>*.go</pattern></action>\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer mockServer.Close()

	cmd := exec.Command(lokolBin, "exec", "--engine="+mockServer.URL, "--max-turns=1", "Find files")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Expect exit after max turns reached
	_ = cmd.Run()

	errStr := stderr.String()
	// In non-verbose mode, intermediate tool action progress must route to stderr
	if !strings.Contains(errStr, "[Step 1] Executing find_files:") {
		t.Errorf("expected clean intermediate step notification on stderr, got: %s", errStr)
	}
}

func TestLokolCLI_Exec_VerboseFlag(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		// Send thought tokens in first chunk so runner streams them before action suppression
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Thinking about the problem...\\n\"}}]}\n\n")
		if ok {
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"<action name=\\\"task_finish\\\">Done</action>\"}}]}\n\n")
		if ok {
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer mockServer.Close()

	cmd := exec.Command(lokolBin, "exec", "-v", "--engine="+mockServer.URL, "Solve problem")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("lokol exec -v failed: %v", err)
	}

	outStr := stdout.String()
	// Verbose mode prints raw streamed tokens to stdout
	if !strings.Contains(outStr, "Thinking about the problem...") {
		t.Errorf("expected streamed thinking tokens on stdout in verbose mode, got: %s", outStr)
	}
}

func TestLokolCLI_Exec_SignalInterruption(t *testing.T) {
	// Mock server that hangs simulating a long inference stream
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if ok {
			flusher.Flush()
		}
		// Block to simulate waiting for model generation
		time.Sleep(5 * time.Second)
	}))
	defer mockServer.Close()

	cmd := exec.Command(lokolBin, "exec", "--engine="+mockServer.URL, "Hang request")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start command: %v", err)
	}

	// Give it 150ms to establish connection and start streaming
	time.Sleep(150 * time.Millisecond)

	// Send SIGINT
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("failed to send SIGINT: %v", err)
	}

	err := cmd.Wait()
	if err == nil {
		t.Fatalf("expected error on SIGINT, got nil")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}

	// Exit code 130 is standard bash SIGINT (128 + 2)
	if exitErr.ExitCode() != 130 {
		t.Errorf("expected exit code 130 on SIGINT, got %d", exitErr.ExitCode())
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "[Execution interrupted by signal. Slot released.]") {
		t.Errorf("expected slot release notification on stdout, got: %s", outStr)
	}
}
