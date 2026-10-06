// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/ipc"
)

func TestDaemonCLI_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol-daemon", "--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d. stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "lokol-daemon") {
		t.Errorf("expected version output, got: %s", stdout.String())
	}
}

func TestDaemonCLI_StartAndConnect(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "cli-test.sock")

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)

	// Run daemon in goroutine
	go func() {
		code := run([]string{
			"lokol-daemon",
			"--socket", sockPath,
			"--work-dir", tmpDir,
			"--yolo",
		}, &stdout, &stderr)
		done <- code
	}()

	// Wait briefly for socket to become available
	var client *ipc.Client
	var err error
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		client, err = ipc.Dial("unix", sockPath)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("failed to dial daemon socket %s: %v\nstderr: %s", sockPath, err, stderr.String())
	}
	defer client.Close()

	pingRes, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if pingRes["status"] != "pong" {
		t.Errorf("expected status=pong, got %v", pingRes["status"])
	}

	// Create and check session
	sessRes, err := client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "cli_sess",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sessRes["session_id"] != "cli_sess" {
		t.Errorf("expected session_id=cli_sess, got %v", sessRes["session_id"])
	}
}
