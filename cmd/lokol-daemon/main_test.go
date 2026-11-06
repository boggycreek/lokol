// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/ipc"
)

func TestDaemon_Run_UnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"lokol-daemon", "-unknown-flag-xyz"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 for unknown flag, got %d", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "flag provided but not defined") {
		t.Errorf("expected flag error in stderr, got: %s", errStr)
	}
}

func TestDaemon_Run_Help(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := run(context.Background(), []string{"lokol-daemon", flag}, &stdout, &stderr)
		if code != 1 { // flag.ContinueOnError returns ErrHelp which causes Parse to return error
			t.Fatalf("expected exit code 1 for %s, got %d", flag, code)
		}
		errStr := stderr.String()
		if !strings.Contains(errStr, "-socket") || !strings.Contains(errStr, "-listen") {
			t.Errorf("expected usage info in stderr for %s, got: %s", flag, errStr)
		}
	}
}

func TestDaemon_Run_TCPListenAndInteractiveMode(t *testing.T) {
	// Find a free TCP port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // Release so daemon can bind

	tmpDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)

	go func() {
		code := run(ctx, []string{
			"lokol-daemon",
			"--listen", addr,
			"--work-dir", tmpDir,
			"--engine", "http://127.0.0.1:9999",
			"--mode", "general",
			"--max-turns", "10",
			// Note: yolo is false by default, testing interactive mode banner
		}, &stdout, &stderr)
		done <- code
	}()

	var client *ipc.Client
	for i := 0; i < 30; i++ {
		time.Sleep(50 * time.Millisecond)
		client, err = ipc.Dial("tcp", addr)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("failed to connect to daemon on %s: %v\nstderr: %s", addr, err, stderr.String())
	}

	pingRes, err := client.Ping(context.Background())
	if err != nil {
		client.Close()
		t.Fatalf("Ping failed: %v", err)
	}
	if pingRes["status"] != "pong" {
		t.Errorf("expected pong, got %v", pingRes["status"])
	}

	// Verify session creation picks up default configuration
	sessRes, err := client.CreateSession(context.Background(), ipc.SessionCreateParams{})
	if err != nil {
		client.Close()
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sessRes["work_dir"] != tmpDir {
		t.Errorf("expected work_dir %s, got %v", tmpDir, sessRes["work_dir"])
	}
	if sessRes["mode"] != "general" {
		t.Errorf("expected mode general, got %v", sessRes["mode"])
	}
	if sessRes["yolo"] != false {
		t.Errorf("expected yolo false, got %v", sessRes["yolo"])
	}
	client.Close()

	// Trigger graceful daemon shutdown via context cancellation
	cancel()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("expected daemon exit code 0, got %d, stderr: %s", code, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for daemon to shut down gracefully")
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "🛡️ Running in interactive mode (approval gates enabled)") {
		t.Errorf("expected interactive mode banner in stdout, got: %s", outStr)
	}
	if !strings.Contains(outStr, "lokol-daemon stopped gracefully.") {
		t.Errorf("expected graceful stop message in stdout, got: %s", outStr)
	}
}

func TestDaemon_Run_ListenFailure(t *testing.T) {
	// Occupy a port to force a listen failure
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind port: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"lokol-daemon",
		"--listen", addr,
	}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("expected exit code 1 for listen failure, got %d", code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "Error listening on") {
		t.Errorf("expected 'Error listening on' in stderr, got: %s", errStr)
	}
}

func TestDaemon_Run_DefaultSocketFallback(t *testing.T) {
	// Isolate TMPDIR so default socket fallback uses a safe temporary directory
	isolatedTemp := t.TempDir()
	t.Setenv("TMPDIR", isolatedTemp)

	defaultSock := filepath.Join(os.TempDir(), "lokol.sock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)

	go func() {
		code := run(ctx, []string{"lokol-daemon"}, &stdout, &stderr)
		done <- code
	}()

	var client *ipc.Client
	var err error
	for i := 0; i < 30; i++ {
		time.Sleep(50 * time.Millisecond)
		client, err = ipc.Dial("unix", defaultSock)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("failed to connect to default socket %s: %v\nstderr: %s", defaultSock, err, stderr.String())
	}
	client.Close()

	// Trigger graceful shutdown via context cancellation
	cancel()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d, stderr: %s", code, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for daemon to shut down")
	}

	if _, err := os.Stat(defaultSock); !os.IsNotExist(err) {
		t.Errorf("expected default socket to be removed after close, but still exists")
	}
}
