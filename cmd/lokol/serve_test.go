// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/ipc"
)

func TestLokolCLI_Serve_StartAndConnect(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "serve-test.sock")

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)

	go func() {
		code := run([]string{
			"lokol",
			"serve",
			"--ipc", sockPath,
			"--work-dir", tmpDir,
			"--yolo",
		}, &stdout, &stderr)
		done <- code
	}()

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
		t.Fatalf("failed to dial serve socket %s: %v\nstderr: %s", sockPath, err, stderr.String())
	}
	defer client.Close()

	pingRes, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if pingRes["status"] != "pong" {
		t.Errorf("expected status=pong, got %v", pingRes["status"])
	}
}
