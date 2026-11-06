// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/ipc"
)

func TestProfileCLI_Flags(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		data := map[string]any{
			"choices": []map[string]any{
				{
					"delta": map[string]string{
						"content": "<action name=\"task_finish\">\nAudited successfully\n</action>",
					},
				},
			},
		}
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", string(b))
		flusher.Flush()
	}))
	defer mockServer.Close()

	// Test --profile flag in exec
	var stdout, stderr bytes.Buffer
	code := run([]string{"lokol", "exec", "--engine=" + mockServer.URL, "--profile=security-audit", "Audit the code"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 with profile flag, got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Complete") {
		t.Errorf("expected Complete in stdout: %s", stdout.String())
	}

	// Test unknown profile error
	var errOut bytes.Buffer
	codeErr := run([]string{"lokol", "exec", "--profile=non-existent-profile", "hello"}, &stdout, &errOut)
	if codeErr != 1 {
		t.Errorf("expected exit code 1 for non-existent profile, got %d", codeErr)
	}
}

func TestProfileServe_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "profile_serve.sock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runServeWithProfile(sockPath, "", "http://127.0.0.1:8080", tmpDir, "coding", "security-audit", true, 5, ioDiscard{}, ioDiscard{})

	var client *ipc.Client
	var err error
	for i := 0; i < 20; i++ {
		time.Sleep(20 * time.Millisecond)
		client, err = ipc.Dial("unix", sockPath)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer client.Close()

	res, err := client.CreateSession(ctx, ipc.SessionCreateParams{
		WorkDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if res["session_id"] == nil || res["session_id"] == "" {
		t.Errorf("expected session_id in create response: %v", res)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
