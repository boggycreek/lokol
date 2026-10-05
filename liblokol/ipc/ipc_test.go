// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ipc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/ipc"
)

// helper to start a mock llama-server streaming SSE chunks
func startMockLLMServer(t *testing.T, turnHandler func(turn int, req agent.StreamChatRequest) []string) (*httptest.Server, *int) {
	turnCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.StreamChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		mu.Lock()
		turnCount++
		currentTurn := turnCount
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		var tokens []string
		if turnHandler != nil {
			tokens = turnHandler(currentTurn, req)
		} else {
			tokens = []string{"Hello world! Task complete. <action name=\"task_finish\">done</action>"}
		}

		for _, tok := range tokens {
			chunk := agent.ChatChunkResponse{
				Choices: []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				}{
					{Delta: struct {
						Content string `json:"content"`
					}{Content: tok}},
				},
			}
			bytesChunk, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", bytesChunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))

	return server, &turnCount
}

func TestIPC_PingAndUDSLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "lokol-test.sock")

	srv := ipc.NewServer(ipc.ServerConfig{
		DefaultWorkDir: tmpDir,
	})

	if err := srv.Listen("unix", sockPath); err != nil {
		t.Fatalf("Listen unix failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Serve(ctx)
	}()

	// Connect client
	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial unix failed: %v", err)
	}
	defer client.Close()

	pingRes, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if pingRes["status"] != "pong" {
		t.Errorf("expected status=pong, got %v", pingRes["status"])
	}

	// Close server and verify socket removal
	if err := srv.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify socket file was cleaned up
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Errorf("expected socket file to be deleted on server close, but it still exists")
	}
}

func TestIPC_TCPListener(t *testing.T) {
	srv := ipc.NewServer(ipc.ServerConfig{})
	if err := srv.Listen("tcp", "127.0.0.1:0"); err != nil {
		t.Fatalf("Listen tcp failed: %v", err)
	}
	defer srv.Close()

	addr := srv.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Serve(ctx)
	}()

	client, err := ipc.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial tcp failed: %v", err)
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

func TestIPC_SessionLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "session.sock")

	srv := ipc.NewServer(ipc.ServerConfig{
		DefaultWorkDir: tmpDir,
	})
	if err := srv.Listen("unix", sockPath); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Serve(ctx)
	}()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	yolo := false
	sessRes, err := client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID:    "sess_test_1",
		WorkDir:      tmpDir,
		Mode:         "coding",
		AgentName:    "Loky",
		OperatorName: "Tester",
		YOLO:         &yolo,
		MaxTurns:     5,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sessRes["session_id"] != "sess_test_1" {
		t.Errorf("expected sess_test_1, got %v", sessRes["session_id"])
	}

	// Status query
	statusRes, err := client.GetStatus(context.Background(), "sess_test_1")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if statusRes["session_id"] != "sess_test_1" || statusRes["agent_name"] != "Loky" {
		t.Errorf("unexpected status: %v", statusRes)
	}

	// Reset
	if err := client.Reset(context.Background(), "sess_test_1"); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Abort
	if err := client.Abort(context.Background(), "sess_test_1"); err != nil {
		t.Fatalf("Abort failed: %v", err)
	}
}

func TestIPC_InteractiveActionApprovalFlow(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "output.txt")

	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		if turn == 1 {
			return []string{
				"I will write to a file.\n",
				"<action name=\"write_file\">\n",
				"<path>", testFile, "</path>\n",
				"<content>Hello from IPC!</content>\n",
				"</action>",
			}
		}
		return []string{"Done! <action name=\"task_finish\">success</action>"}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "interactive.sock")
	srv := ipc.NewServer(ipc.ServerConfig{
		EngineURL:      mockLLM.URL,
		DefaultWorkDir: tmpDir,
	})
	if err := srv.Listen("unix", sockPath); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Serve(ctx)
	}()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	yolo := false
	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_interactive",
		WorkDir:   tmpDir,
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	var eventsMu sync.Mutex
	var receivedTokens []string
	var actionResults []ipc.EventActionResultPayload
	proposedChan := make(chan ipc.EventActionProposedPayload, 1)

	client.SetEventHandler(func(notif ipc.Notification) {
		eventsMu.Lock()
		defer eventsMu.Unlock()

		switch notif.Method {
		case ipc.EventToken:
			var p ipc.EventTokenPayload
			bytes, _ := json.Marshal(notif.Params)
			_ = json.Unmarshal(bytes, &p)
			receivedTokens = append(receivedTokens, p.Token)
		case ipc.EventActionProposed:
			var p ipc.EventActionProposedPayload
			bytes, _ := json.Marshal(notif.Params)
			_ = json.Unmarshal(bytes, &p)
			select {
			case proposedChan <- p:
			default:
			}
		case ipc.EventActionResult:
			var p ipc.EventActionResultPayload
			bytes, _ := json.Marshal(notif.Params)
			_ = json.Unmarshal(bytes, &p)
			actionResults = append(actionResults, p)
		}
	})

	// Run prompt in goroutine since it will block waiting for action approval
	promptDone := make(chan error, 1)
	go func() {
		_, err := client.Prompt(context.Background(), ipc.SessionPromptParams{
			SessionID: "sess_interactive",
			Prompt:    "Create the output file",
		})
		promptDone <- err
	}()

	// Wait for action proposed notification
	select {
	case action := <-proposedChan:
		if !action.RequiresApproval {
			t.Errorf("expected RequiresApproval=true for interactive mode action")
		}
		// Approve the action
		err := client.ApproveAction(context.Background(), "sess_interactive", action.ActionID)
		if err != nil {
			t.Fatalf("ApproveAction failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for event.action_proposed")
	}

	// Await prompt completion
	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("Prompt returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for Prompt completion")
	}

	// Verify file was written
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}
	if string(content) != "Hello from IPC!" {
		t.Errorf("unexpected content: %s", string(content))
	}
}

func TestIPC_InteractiveActionRejectionFlow(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "forbidden.txt")

	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		if turn == 1 {
			return []string{
				"<action name=\"write_file\">\n",
				"<path>", testFile, "</path>\n",
				"<content>Forbidden</content>\n",
				"</action>",
			}
		}
		// On turn 2, check if rejection was recorded and conclude
		return []string{"Understood. <action name=\"task_finish\">aborted</action>"}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "reject.sock")
	srv := ipc.NewServer(ipc.ServerConfig{
		EngineURL:      mockLLM.URL,
		DefaultWorkDir: tmpDir,
	})
	if err := srv.Listen("unix", sockPath); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Serve(ctx)
	}()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	yolo := false
	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_reject",
		WorkDir:   tmpDir,
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	proposedChan := make(chan ipc.EventActionProposedPayload, 1)
	client.SetEventHandler(func(notif ipc.Notification) {
		if notif.Method == ipc.EventActionProposed {
			var p ipc.EventActionProposedPayload
			bytes, _ := json.Marshal(notif.Params)
			_ = json.Unmarshal(bytes, &p)
			select {
			case proposedChan <- p:
			default:
			}
		}
	})

	promptDone := make(chan error, 1)
	go func() {
		_, err := client.Prompt(context.Background(), ipc.SessionPromptParams{
			SessionID: "sess_reject",
			Prompt:    "Write forbidden file",
		})
		promptDone <- err
	}()

	select {
	case action := <-proposedChan:
		err := client.RejectAction(context.Background(), "sess_reject", action.ActionID, "Not allowed by policy")
		if err != nil {
			t.Fatalf("RejectAction failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for event.action_proposed")
	}

	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("Prompt returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for Prompt completion")
	}

	// Verify file was NOT created
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Errorf("file should not have been created after rejection")
	}
}

func TestIPC_FaultIsolation_PanicRecovery(t *testing.T) {
	tmpDir := t.TempDir()

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal llama-server crash", http.StatusInternalServerError)
	}))
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "fault.sock")
	srv := ipc.NewServer(ipc.ServerConfig{
		EngineURL:      mockLLM.URL,
		DefaultWorkDir: tmpDir,
	})
	if err := srv.Listen("unix", sockPath); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Serve(ctx)
	}()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	_, err = client.Prompt(context.Background(), ipc.SessionPromptParams{
		Prompt: "Hello, this will fail gracefully",
	})
	// Prompt should return an RPC error, but NOT crash the server
	if err == nil {
		t.Fatalf("expected error from bad json server, got nil")
	}

	// Daemon must remain alive and responsive!
	pingRes, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("daemon died after turn error: %v", err)
	}
	if pingRes["status"] != "pong" {
		t.Errorf("expected pong after recovery, got %v", pingRes["status"])
	}
}

func TestIPC_ProtocolErrors(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "proto.sock")

	srv := ipc.NewServer(ipc.ServerConfig{DefaultWorkDir: tmpDir})
	if err := srv.Listen("unix", sockPath); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Serve(ctx)
	}()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	// Non-existent method
	err = client.Call(context.Background(), "unknown.method", nil, nil)
	if err == nil {
		t.Fatalf("expected error calling unknown.method")
	}

	// Non-existent session
	err = client.Abort(context.Background(), "non_existent_sess_123")
	if err == nil {
		t.Fatalf("expected error for non-existent session")
	}
}
