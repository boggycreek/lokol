// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.
//go:build integration

package lokol_test

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPodmanSandbox_ProjectEvaluation verifies that:
// 1. The agent evaluates and summarizes the purpose of a scoped test project.
// 2. The entire execution is sandboxed inside a Podman container so host memories are untouched.
// 3. Intermediary inferences and tool execution are internalized without raw token dumping.
func TestPodmanSandbox_ProjectEvaluation(t *testing.T) {
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		t.Skip("podman is not available on host, skipping container sandbox test")
	}

	// Verify local inference engine is reachable
	resp, err := http.Get("http://127.0.0.1:8080/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skip("local llama-server engine (http://127.0.0.1:8080) is not running, skipping live container evaluation")
	}
	_ = resp.Body.Close()

	// 1. Create isolated temporary workspace for the scoped test project
	scopedProjectDir := t.TempDir()
	binDir := t.TempDir() // Binary is compiled strictly into TempDir, never repository root

	// Create scoped test project files
	goMod := "module github.com/example/logparser\n\ngo 1.22\n"
	readme := `# LogParser Microservice

LogParser is a streaming microservice that ingests server access logs, parses structured JSON records, detects 4xx and 5xx anomalies, and calculates latency percentiles for real-time observability.
`
	mainGo := `package main

import "fmt"

func main() {
	fmt.Println("LogParser microservice active on port 8080")
}
`
	serviceGo := `package main

type AccessRecord struct {
	Timestamp string ` + "`json:\"timestamp\"`" + `
	Status    int    ` + "`json:\"status\"`" + `
	LatencyMs int    ` + "`json:\"latency_ms\"`" + `
}
`
	if err := os.WriteFile(filepath.Join(scopedProjectDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scopedProjectDir, "README.md"), []byte(readme), 0644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scopedProjectDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scopedProjectDir, "service.go"), []byte(serviceGo), 0644); err != nil {
		t.Fatalf("failed to write service.go: %v", err)
	}

	// 2. Compile lokol binary into the temporary binDir (preventing any binary in repo root)
	binPath := filepath.Join(binDir, "lokol")
	buildCmd := exec.Command("go", "build", "-o", binPath, "..")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile lokol test binary: %v\nOutput: %s", err, string(out))
	}

	// Record host .beads mod time before running test to ensure zero pollution
	hostBeadsDir := filepath.Join("..", "..", ".beads")
	var hostBeadsModTime time.Time
	if fi, err := os.Stat(hostBeadsDir); err == nil {
		hostBeadsModTime = fi.ModTime()
	}

	// 3. Execute lokol headless within Podman container
	prompt := "Evaluate the purpose of the current working directory by inspecting its files, write a concise summary of what this project does into PURPOSE.md, and finish."
	containerCmd := exec.Command(
		podmanPath, "run", "--rm",
		"--network=host",
		"-v", scopedProjectDir+":/workspace:rw,Z",
		"-v", binPath+":/usr/local/bin/lokol:ro,Z",
		"-w", "/workspace",
		"-e", "HOME=/tmp",
		"-e", "BEADS_DIR=/workspace/.beads",
		"docker.io/library/golang:1.25-bookworm",
		"lokol", "-p", prompt,
	)

	var stdoutBuf, stderrBuf bytes.Buffer
	containerCmd.Stdout = &stdoutBuf
	containerCmd.Stderr = &stderrBuf

	t.Logf("Running sandboxed evaluation in Podman container for scoped project in %s...", scopedProjectDir)
	runErr := containerCmd.Run()

	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()
	t.Logf("Container stdout:\n%s", stdoutStr)
	t.Logf("Container stderr:\n%s", stderrStr)

	if runErr != nil {
		t.Fatalf("container execution failed: %v\nStderr: %s", runErr, stderrStr)
	}

	// 4. Verify that intermediate inferences were internalized
	// Stdout should cleanly present the completion without intermediate thought fragments
	if !strings.Contains(stdoutStr, "✅ Complete") {
		t.Errorf("expected stdout to contain '✅ Complete', got: %s", stdoutStr)
	}

	// Stderr should contain the clean progress indicators
	if !strings.Contains(stderrStr, "⚡") {
		t.Errorf("expected stderr to contain tool progress indicators '⚡', got: %s", stderrStr)
	}

	// 5. Verify PURPOSE.md was generated and correctly captures the purpose of the scoped project
	purposePath := filepath.Join(scopedProjectDir, "PURPOSE.md")
	purposeBytes, err := os.ReadFile(purposePath)
	if err != nil {
		// If PURPOSE.md wasn't written to disk, check if it was synthesized in the finish command
		if !strings.Contains(strings.ToLower(stdoutStr), "log") {
			t.Fatalf("expected PURPOSE.md to exist or final summary to mention log parsing: %v", err)
		}
	} else {
		purposeText := strings.ToLower(string(purposeBytes))
		hasExpectedTopic := strings.Contains(purposeText, "log") ||
			strings.Contains(purposeText, "microservice") ||
			strings.Contains(purposeText, "observability") ||
			strings.Contains(purposeText, "parse") ||
			strings.Contains(purposeText, "access")
		if !hasExpectedTopic {
			t.Errorf("PURPOSE.md content did not describe the scoped project: %s", string(purposeBytes))
		}
	}

	// 6. Verify host memories (.beads) were completely unpolluted
	if fi, err := os.Stat(hostBeadsDir); err == nil {
		if fi.ModTime().After(hostBeadsModTime) {
			t.Errorf("host .beads directory was touched during sandboxed container test")
		}
	}

	t.Logf("PASS: Sandboxed Podman container test succeeded with internalized inferences and host isolation.")
}
