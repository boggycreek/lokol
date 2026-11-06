// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ipc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/catalog"
	"github.com/boggycreek/lokol/liblokol/ipc"
)

// helper to start a mock llama-server streaming SSE chunks
func startMockLLMServer(t *testing.T, turnHandler func(turn int, req agent.StreamChatRequest) []string) (*httptest.Server, *int) {
	turnCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if r.URL.Path == "/slots" {
				_, _ = w.Write([]byte(`[{"id":0,"state":0,"n_ctx":4096,"n_past":100}]`))
			} else {
				_, _ = w.Write([]byte(`{"data":[{"id":"mock-model"}]}`))
			}
			return
		}

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

	mockLLM, _ := startMockLLMServer(t, nil)
	defer mockLLM.Close()

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

	// Call DecideAction directly on session state with invalid action ID
	state, _ := srv.ResolveSession("sess_test_1")
	if err := state.DecideAction("invalid_action", true, ""); err == nil {
		t.Errorf("expected error deciding action on invalid action ID")
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

	// Missing params
	err = client.Call(context.Background(), ipc.MethodSessionPrompt, nil, nil)
	if err == nil {
		t.Fatalf("expected error calling session.prompt without params")
	}
}

func TestIPC_DuplicateSessionID(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "dup.sock")

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

	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "dup_sess",
	})
	if err != nil {
		t.Fatalf("CreateSession 1 failed: %v", err)
	}

	// Duplicate session ID must be rejected
	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "dup_sess",
	})
	if err == nil {
		t.Fatalf("expected duplicate session creation to fail, got nil")
	}

	// Direct SessionState Close and Reset while turn is active
	sessState, _ := srv.ResolveSession("dup_sess")
	sessState.Close()
	sessState.Reset()
}

func TestIPC_SessionClose(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "close.sock")

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

	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "to_close",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Close session
	if err := client.CloseSession(context.Background(), "to_close"); err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	// Must no longer exist
	_, err = client.GetStatus(context.Background(), "to_close")
	if err == nil {
		t.Fatalf("expected session not found error after close")
	}
}

func TestIPC_ReadOnlyDoesNotCreateDefaultSession(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "readonly.sock")

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

	// Calling GetStatus with no active sessions must fail without auto-creating a default session
	_, err = client.GetStatus(context.Background(), "")
	if err == nil {
		t.Fatalf("expected GetStatus to fail when no session exists")
	}
}

func TestIPC_ClientDisconnectDuringApproval_Recovers(t *testing.T) {
	tmpDir := t.TempDir()
	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		return []string{
			"<action name=\"write_file\">\n",
			"<path>never_written.txt</path>\n",
			"<content>test</content>\n",
			"</action>",
		}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "disconnect.sock")
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

	client1, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial 1 failed: %v", err)
	}

	yolo := false
	_, err = client1.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_disconnect",
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	proposed := make(chan struct{}, 1)
	client1.SetEventHandler(func(notif ipc.Notification) {
		if notif.Method == ipc.EventActionProposed {
			select {
			case proposed <- struct{}{}:
			default:
			}
		}
	})

	go func() {
		_, _ = client1.Prompt(context.Background(), ipc.SessionPromptParams{
			SessionID: "sess_disconnect",
			Prompt:    "Write file",
		})
	}()

	// Wait for action proposed
	select {
	case <-proposed:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for action proposal")
	}

	// Abruptly close client 1
	_ = client1.Close()

	// Wait for turn context cancellation to propagate and release session lock
	var status map[string]any
	client2, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial 2 failed: %v", err)
	}
	defer client2.Close()

	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		status, err = client2.GetStatus(context.Background(), "sess_disconnect")
		if err == nil && status["is_busy"] == false {
			break
		}
	}

	if status["is_busy"] != false {
		t.Errorf("session remained busy after client disconnect: %v", status)
	}
}

func TestIPC_PartialPrefixFlush(t *testing.T) {
	tmpDir := t.TempDir()

	// Model emits a partial action tag prefix and finishes
	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		return []string{"Formula is x <", " 5"}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "prefix.sock")
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

	var tokens strings.Builder
	var mu sync.Mutex
	client.SetEventHandler(func(notif ipc.Notification) {
		if notif.Method == ipc.EventToken {
			var p ipc.EventTokenPayload
			bytes, _ := json.Marshal(notif.Params)
			_ = json.Unmarshal(bytes, &p)
			mu.Lock()
			tokens.WriteString(p.Token)
			mu.Unlock()
		}
	})

	_, err = client.Prompt(context.Background(), ipc.SessionPromptParams{
		Prompt: "What is the formula?",
	})
	if err != nil {
		t.Fatalf("Prompt failed: %v", err)
	}

	// Conversational feedback prompt path (catalog.IsConversationalFeedback returns true)
	_, err = client.Prompt(context.Background(), ipc.SessionPromptParams{
		Prompt: "thanks",
	})
	if err != nil {
		t.Fatalf("Prompt failed: %v", err)
	}

	mu.Lock()
	received := tokens.String()
	mu.Unlock()

	if !strings.Contains(received, "<") {
		t.Errorf("expected buffered token '<' to be flushed, got: %q", received)
	}
}

func TestIPC_OversizedRequestLine(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "oversized.sock")

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

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send 2.5MB line without newline
	oversized := make([]byte, 2500000)
	for i := range oversized {
		oversized[i] = 'A'
	}
	oversized = append(oversized, '\n')
	_, _ = conn.Write(oversized)

	// Server should respond with parse error and close
	reply := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := conn.Read(reply)
	if !strings.Contains(string(reply[:n]), "exceeded maximum limit") {
		t.Errorf("expected error about exceeding line limit, got: %s", string(reply[:n]))
	}
}

func TestIPC_ConcurrentControlUnderRaceDetector(t *testing.T) {
	tmpDir := t.TempDir()

	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		return []string{"Streaming chunk 1... ", "chunk 2... ", "<action name=\"task_finish\">done</action>"}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "race.sock")
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

	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_race",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = client.Prompt(context.Background(), ipc.SessionPromptParams{
			SessionID: "sess_race",
			Prompt:    "Start streaming",
		})
	}()

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.GetStatus(context.Background(), "sess_race")
		}()
	}

	// Exercise compaction during concurrent Prompt and GetStatus calls
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		if sess, resolveErr := srv.ResolveSession("sess_race"); resolveErr == nil && sess != nil {
			_ = sess.Session.Compact(context.Background(), nil)
		}
	}()

	wg.Wait()
}

func TestIPC_PanicPath_AssertErrorAndSingleTurnFinished(t *testing.T) {
	// Register a panic-inducing tool in catalog
	catalog.DefaultRegistry.Register(&catalog.SimpleTool{
		NameVal:    "simulate_panic",
		SummaryVal: "Simulate a panic for testing",
		ExecuteFn: func(ctx context.Context, cmd string, workDir ...string) (string, error) {
			panic("simulated panic in tool execution")
		},
	})

	tmpDir := t.TempDir()
	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		return []string{"<action name=\"simulate_panic\">crash</action>"}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "panic_test.sock")
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
	go func() { _ = srv.Serve(ctx) }()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	var turnFinishedCount int
	var mu sync.Mutex
	client.SetEventHandler(func(n ipc.Notification) {
		if n.Method == ipc.EventTurnFinished {
			mu.Lock()
			turnFinishedCount++
			mu.Unlock()
		}
	})

	yolo := true
	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_panic",
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	res, err := client.Prompt(context.Background(), ipc.SessionPromptParams{
		SessionID: "sess_panic",
		Prompt:    "trigger panic",
	})
	if err == nil {
		t.Fatalf("expected RPC error from panic, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result on panic error, got %+v", res)
	}
	if !strings.Contains(err.Error(), "internal engine panic") {
		t.Errorf("expected error message to mention internal engine panic, got: %v", err)
	}

	mu.Lock()
	count := turnFinishedCount
	mu.Unlock()
	if count != 1 {
		t.Errorf("expected exactly 1 turn_finished notification, got %d", count)
	}
}

func TestIPC_LoopDetectPath_SingleTurnFinished(t *testing.T) {
	tmpDir := t.TempDir()
	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		return []string{"<action name=\"tool_help\"><tool>all</tool></action>"}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "loop_test.sock")
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
	go func() { _ = srv.Serve(ctx) }()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	var turnFinishedCount int
	var mu sync.Mutex
	client.SetEventHandler(func(n ipc.Notification) {
		if n.Method == ipc.EventTurnFinished {
			mu.Lock()
			turnFinishedCount++
			mu.Unlock()
		}
	})

	yolo := true
	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_loop",
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = client.Prompt(context.Background(), ipc.SessionPromptParams{
		SessionID: "sess_loop",
		Prompt:    "trigger loop",
	})
	if err == nil {
		t.Fatalf("expected loop detection error, got nil")
	}
	if !strings.Contains(err.Error(), "loop detected") {
		t.Errorf("expected loop detected error message, got: %v", err)
	}

	mu.Lock()
	count := turnFinishedCount
	mu.Unlock()
	if count != 1 {
		t.Errorf("expected exactly 1 turn_finished notification, got %d", count)
	}
}

func TestIPC_ActionInProse_TokensStreamed(t *testing.T) {
	tmpDir := t.TempDir()
	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		return []string{
			"Here is an example: ",
			"<action in a sentence",
			" should still stream. ",
			"<action name=\"task_finish\">done</action>",
		}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "prose_test.sock")
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
	go func() { _ = srv.Serve(ctx) }()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	var streamedText strings.Builder
	var mu sync.Mutex
	client.SetEventHandler(func(n ipc.Notification) {
		if n.Method == ipc.EventToken {
			var p ipc.EventTokenPayload
			bytes, _ := json.Marshal(n.Params)
			_ = json.Unmarshal(bytes, &p)
			mu.Lock()
			streamedText.WriteString(p.Token)
			mu.Unlock()
		}
	})

	yolo := true
	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_prose",
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = client.Prompt(context.Background(), ipc.SessionPromptParams{
		SessionID: "sess_prose",
		Prompt:    "tell me about actions",
	})
	if err != nil {
		t.Fatalf("Prompt failed: %v", err)
	}

	mu.Lock()
	text := streamedText.String()
	mu.Unlock()

	if !strings.Contains(text, "<action in a sentence should still stream.") {
		t.Errorf("expected prose with '<action' to be streamed, got: %q", text)
	}
	if strings.Contains(text, "<action name=") {
		t.Errorf("expected action tag to be suppressed from token stream, got: %q", text)
	}
}

func TestIPC_SessionCloseAndReset_RequireExplicitSessionID(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "explicit_id.sock")
	srv := ipc.NewServer(ipc.ServerConfig{DefaultWorkDir: tmpDir})
	if err := srv.Listen("unix", sockPath); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// 1. session.close with empty session_id
	closeReq := `{"jsonrpc":"2.0","id":"1","method":"session.close","params":{"session_id":""}}` + "\n"
	_, _ = conn.Write([]byte(closeReq))

	buf := make([]byte, 1024)
	n, _ := conn.Read(buf)
	if !strings.Contains(string(buf[:n]), "session_id is required for session.close") {
		t.Errorf("expected error requiring session_id on close, got: %s", string(buf[:n]))
	}

	// 2. session.reset with empty session_id
	resetReq := `{"jsonrpc":"2.0","id":"2","method":"session.reset","params":{"session_id":""}}` + "\n"
	_, _ = conn.Write([]byte(resetReq))

	n, _ = conn.Read(buf)
	if !strings.Contains(string(buf[:n]), "session_id is required for session.reset") {
		t.Errorf("expected error requiring session_id on reset, got: %s", string(buf[:n]))
	}
}

func TestIPC_MutatingActionsGatedInInteractiveMode(t *testing.T) {
	tmpDir := t.TempDir()

	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		switch turn {
		case 1:
			return []string{"<action name=\"exec_bash\">echo safe</action>"}
		case 2:
			return []string{"<action name=\"tool_help\"><tool>all</tool></action>"}
		default:
			return []string{"<action name=\"task_finish\">done</action>"}
		}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "gate.sock")
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
	go func() { _ = srv.Serve(ctx) }()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	proposedChan := make(chan ipc.EventActionProposedPayload, 10)
	client.SetEventHandler(func(n ipc.Notification) {
		if n.Method == ipc.EventActionProposed {
			var p ipc.EventActionProposedPayload
			bytes, _ := json.Marshal(n.Params)
			_ = json.Unmarshal(bytes, &p)
			proposedChan <- p
		}
	})

	yolo := false
	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_gate",
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	promptDone := make(chan error, 1)
	go func() {
		_, err := client.Prompt(context.Background(), ipc.SessionPromptParams{
			SessionID: "sess_gate",
			Prompt:    "run actions",
		})
		promptDone <- err
	}()

	// Turn 1: exec_bash must require approval even though echo is risk None
	select {
	case act1 := <-proposedChan:
		if act1.Name != "exec_bash" {
			t.Errorf("expected act1 to be exec_bash, got %s", act1.Name)
		}
		if !act1.RequiresApproval {
			t.Errorf("expected exec_bash in non-YOLO mode to require approval")
		}
		_ = client.ApproveAction(context.Background(), "sess_gate", act1.ActionID)
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for act1")
	}

	// Turn 2: tool_help is read-only and should NOT require approval
	select {
	case act2 := <-proposedChan:
		if act2.Name != "tool_help" {
			t.Errorf("expected act2 to be tool_help, got %s", act2.Name)
		}
		if act2.RequiresApproval {
			t.Errorf("expected tool_help (read-only) NOT to require approval, but got RequiresApproval=true (risk=%s reason=%s)", act2.RiskLevel, act2.Reason)
		}
		if act2.RequiresApproval {
			_ = client.ApproveAction(context.Background(), "sess_gate", act2.ActionID)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for act2")
	}

	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("prompt failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for prompt completion")
	}
}

func TestIPC_NonReadingClient_WriteTimeoutOrClose(t *testing.T) {
	tmpDir := t.TempDir()
	// Mock server that generates continuous stream of tokens
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 500; i++ {
			chunk := agent.ChatChunkResponse{
				Choices: []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				}{
					{Delta: struct {
						Content string `json:"content"`
					}{Content: fmt.Sprintf("chunk-%d-%s ", i, strings.Repeat("X", 512))}},
				},
			}
			bytesChunk, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", bytesChunk)
			flusher.Flush()
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "slow_read.sock")
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
	go func() { _ = srv.Serve(ctx) }()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	// Send create and prompt
	req := `{"jsonrpc":"2.0","id":"1","method":"session.create","params":{"session_id":"sess_slow"}}` + "\n"
	_, _ = conn.Write([]byte(req))
	promptReq := `{"jsonrpc":"2.0","id":"2","method":"session.prompt","params":{"session_id":"sess_slow","prompt":"stream"}}` + "\n"
	_, _ = conn.Write([]byte(promptReq))

	// Close read side to simulate an abandoned reader / broken pipe
	if unixConn, ok := conn.(*net.UnixConn); ok {
		_ = unixConn.CloseRead()
	} else {
		_ = conn.Close()
	}

	// Wait briefly; server should detect write error and cleanly clean up connection
	time.Sleep(100 * time.Millisecond)

	// Verify server remains alive and accepts new connections
	c2, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("server died after slow/broken reader: %v", err)
	}
	defer c2.Close()
	pong, err := c2.Ping(context.Background())
	if err != nil || pong["status"] != "pong" {
		t.Fatalf("failed to ping server: %v", err)
	}
}

func TestIPC_RingBufferDirectUnit(t *testing.T) {
	rb := ipc.NewEventRingBuffer(3)
	if rb.Count() != 0 {
		t.Fatalf("expected initial count 0, got %d", rb.Count())
	}

	rb.Append(ipc.Notification{Method: "event.token", Seq: 1})
	rb.Append(ipc.Notification{Method: "event.token", Seq: 2})
	rb.Append(ipc.Notification{Method: "event.token", Seq: 3})
	if rb.Count() != 3 {
		t.Fatalf("expected count 3, got %d", rb.Count())
	}

	all := rb.All()
	if len(all) != 3 || all[0].Seq != 1 || all[2].Seq != 3 {
		t.Fatalf("unexpected all events: %+v", all)
	}

	// Overwrite oldest (Seq 1)
	rb.Append(ipc.Notification{Method: "event.token", Seq: 4})
	if rb.Count() != 3 {
		t.Fatalf("expected count 3, got %d", rb.Count())
	}

	all = rb.All()
	if len(all) != 3 || all[0].Seq != 2 || all[2].Seq != 4 {
		t.Fatalf("unexpected events after wrap: %+v", all)
	}

	since2 := rb.EventsSince(2)
	if len(since2) != 2 || since2[0].Seq != 3 || since2[1].Seq != 4 {
		t.Fatalf("expected events with Seq > 2 to be [3, 4], got: %+v", since2)
	}

	// Default fallback capacity
	defaultRb := ipc.NewEventRingBuffer(0)
	if defaultRb.Count() != 0 {
		t.Fatalf("expected count 0")
	}
}

func TestIPC_MonotonicSequenceContinuity(t *testing.T) {
	tmpDir := t.TempDir()
	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		return []string{
			"Here is token 1. ",
			"Here is token 2. ",
			"<action name=\"task_finish\">done</action>",
		}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "seq_continuity.sock")
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
	go func() { _ = srv.Serve(ctx) }()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	var eventsMu sync.Mutex
	var receivedSeqs []uint64

	client.SetEventHandler(func(n ipc.Notification) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		receivedSeqs = append(receivedSeqs, n.Seq)
	})

	yolo := true
	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_seq",
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = client.Prompt(context.Background(), ipc.SessionPromptParams{
		SessionID: "sess_seq",
		Prompt:    "emit tokens",
	})
	if err != nil {
		t.Fatalf("Prompt failed: %v", err)
	}

	eventsMu.Lock()
	seqs := append([]uint64(nil), receivedSeqs...)
	eventsMu.Unlock()

	if len(seqs) < 2 {
		t.Fatalf("expected at least 2 events emitted, got %d: %v", len(seqs), seqs)
	}

	for i := 0; i < len(seqs); i++ {
		if i == 0 {
			if seqs[i] != 1 {
				t.Errorf("expected first event to have seq 1, got %d", seqs[i])
			}
		} else {
			if seqs[i] != seqs[i-1]+1 {
				t.Errorf("expected monotonic sequence seq[%d]=%d to follow seq[%d]=%d", i, seqs[i], i-1, seqs[i-1])
			}
		}
	}
}

func TestIPC_ReconnectAndReplayCursor(t *testing.T) {
	tmpDir := t.TempDir()

	// Headless streaming prompt: client 1 subscribes, disconnects mid-stream, client 2 reconnects
	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		return []string{
			"Token 1 ",
			"Token 2 ",
			"Token 3 ",
			"<action name=\"task_finish\">done</action>",
		}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "reconnect.sock")
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
	go func() { _ = srv.Serve(ctx) }()

	// Client 1 connects and starts session
	client1, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial client1 failed: %v", err)
	}

	yolo := true
	_, err = client1.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_reconnect",
		WorkDir:   tmpDir,
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Prompt execution produces events into session ring buffer
	promptDone := make(chan error, 1)
	go func() {
		_, err := client1.Prompt(context.Background(), ipc.SessionPromptParams{
			SessionID: "sess_reconnect",
			Prompt:    "Generate tokens",
		})
		promptDone <- err
	}()

	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("Prompt failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for prompt to finish")
	}

	// Client 1 closes connection
	_ = client1.Close()

	// Client 2 connects to the headless daemon and requests replay of missed events
	client2, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial client2 failed: %v", err)
	}
	defer client2.Close()

	var c2Events []ipc.Notification
	var c2Mu sync.Mutex
	client2.SetEventHandler(func(n ipc.Notification) {
		c2Mu.Lock()
		c2Events = append(c2Events, n)
		c2Mu.Unlock()
	})

	// Replay from cursor seq = 2 (should replay events with seq > 2)
	subRes, err := client2.Subscribe(context.Background(), "sess_reconnect", 2)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	if subRes.SessionID != "sess_reconnect" {
		t.Errorf("expected session_id sess_reconnect, got %s", subRes.SessionID)
	}
	if subRes.Replayed < 1 {
		t.Errorf("expected at least 1 replayed event, got %d", subRes.Replayed)
	}

	time.Sleep(100 * time.Millisecond)

	c2Mu.Lock()
	events := append([]ipc.Notification(nil), c2Events...)
	c2Mu.Unlock()

	if len(events) != subRes.Replayed {
		t.Errorf("expected client2 to receive %d replayed events, got %d", subRes.Replayed, len(events))
	}
	for _, ev := range events {
		if ev.Seq <= 2 {
			t.Errorf("received event with seq %d <= since_seq 2", ev.Seq)
		}
	}
}

func TestIPC_OutOfOrderAndDuplicateDrop(t *testing.T) {
	// Since ipc.Dial takes network/address, let's write to a small listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	serverConnChan := make(chan net.Conn, 1)
	go func() {
		conn, _ := ln.Accept()
		serverConnChan <- conn
	}()

	client, err := ipc.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	serverConn := <-serverConnChan
	defer serverConn.Close()

	var receivedSeqs []uint64
	var mu sync.Mutex
	client.SetEventHandler(func(n ipc.Notification) {
		mu.Lock()
		receivedSeqs = append(receivedSeqs, n.Seq)
		mu.Unlock()
	})

	client.SetLastSeenSeq(10)

	// Send seq 5 (stale/out-of-order), seq 10 (duplicate), seq 11 (valid), seq 9 (stale), seq 12 (valid)
	_, _ = serverConn.Write([]byte(`{"jsonrpc":"2.0","method":"event.token","params":{"token":"5"},"seq":5}` + "\n"))
	_, _ = serverConn.Write([]byte(`{"jsonrpc":"2.0","method":"event.token","params":{"token":"10"},"seq":10}` + "\n"))
	_, _ = serverConn.Write([]byte(`{"jsonrpc":"2.0","method":"event.token","params":{"token":"11"},"seq":11}` + "\n"))
	_, _ = serverConn.Write([]byte(`{"jsonrpc":"2.0","method":"event.token","params":{"token":"9"},"seq":9}` + "\n"))
	_, _ = serverConn.Write([]byte(`{"jsonrpc":"2.0","method":"event.token","params":{"token":"12"},"seq":12}` + "\n"))

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	got := append([]uint64(nil), receivedSeqs...)
	mu.Unlock()

	if len(got) != 2 || got[0] != 11 || got[1] != 12 {
		t.Fatalf("expected only [11, 12] received after dropping <= 10 and out-of-order, got: %v", got)
	}
	if client.LastSeenSeq() != 12 {
		t.Errorf("expected LastSeenSeq 12, got %d", client.LastSeenSeq())
	}
}

func TestIPC_SubscribeInvalidParamsAndNonExistentSession(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "sub_err.sock")

	srv := ipc.NewServer(ipc.ServerConfig{DefaultWorkDir: tmpDir})
	if err := srv.Listen("unix", sockPath); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	// Non-existent session
	_, err = client.Subscribe(context.Background(), "no_such_session", 0)
	if err == nil {
		t.Fatalf("expected error subscribing to non-existent session, got nil")
	}

	// session.prompt with empty session_id and multiple sessions present (auto-resolve ambiguity)
	_, _ = client.CreateSession(context.Background(), ipc.SessionCreateParams{SessionID: "sess_a"})
	_, _ = client.CreateSession(context.Background(), ipc.SessionCreateParams{SessionID: "sess_b"})
	_, err = client.Prompt(context.Background(), ipc.SessionPromptParams{
		SessionID: "",
		Prompt:    "ambiguous",
	})
	if err == nil {
		t.Fatalf("expected error for prompt with ambiguous session")
	}

	// session.get_status with empty session_id and default fallback
	_, _ = client.CreateSession(context.Background(), ipc.SessionCreateParams{SessionID: "default"})
	status, err := client.GetStatus(context.Background(), "")
	if err != nil || status["session_id"] != "default" {
		t.Errorf("expected default session status, got %v (err: %v)", status, err)
	}

	// Invalid params (malformed json to server dispatch)
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer conn.Close()

	req := `{"jsonrpc":"2.0","id":"1","method":"session.subscribe","params":"bad_type"}` + "\n"
	_, _ = conn.Write([]byte(req))

	buf := make([]byte, 1024)
	n, _ := conn.Read(buf)
	if !strings.Contains(string(buf[:n]), "cannot unmarshal") && !strings.Contains(string(buf[:n]), "error") {
		t.Errorf("expected invalid params error, got: %s", string(buf[:n]))
	}
}

func TestIPC_CoverageEdges(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Session creation with Spec file (valid and invalid)
	specPath := filepath.Join(tmpDir, "test_spec.yaml")
	_ = os.WriteFile(specPath, []byte(`
version: "1.0"
title: "Test Spec"
verify_cmd: "echo pass"
constraints:
  target_files:
    allowed: ["safe.txt"]
    forbidden: ["forbidden.txt"]
`), 0644)

	yolo := true
	state, err := ipc.NewSessionState("spec_sess", nil, ipc.SessionCreateParams{
		WorkDir:  tmpDir,
		SpecFile: specPath,
		YOLO:     &yolo,
	})
	if err != nil {
		t.Fatalf("NewSessionState with spec failed: %v", err)
	}
	if state.SpecMachine == nil {
		t.Fatalf("expected SpecMachine to be initialized")
	}

	// Invalid spec file
	_, err = ipc.NewSessionState("bad_spec", nil, ipc.SessionCreateParams{
		WorkDir:  tmpDir,
		SpecFile: filepath.Join(tmpDir, "non_existent.yaml"),
	})
	if err == nil {
		t.Fatalf("expected error for non-existent spec file")
	}

	// 2. Default NewServer with default fields
	srvDef := ipc.NewServer(ipc.ServerConfig{})
	if srvDef.Addr() != nil {
		t.Errorf("expected nil Addr before Listen")
	}

	// 3. session.get_status and session.reset and session.close with invalid params
	sockPath := filepath.Join(tmpDir, "cov_edges.sock")
	srv := ipc.NewServer(ipc.ServerConfig{DefaultWorkDir: tmpDir})
	if err := srv.Listen("unix", sockPath); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Invalid params for session.create
	_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","id":"1","method":"session.create","params":"invalid"}` + "\n"))
	buf := make([]byte, 1024)
	n, _ := conn.Read(buf)
	if !strings.Contains(string(buf[:n]), "error") {
		t.Errorf("expected error for invalid create params")
	}

	// Invalid params for session.approve_action
	_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","id":"2","method":"session.approve_action","params":"invalid"}` + "\n"))
	n, _ = conn.Read(buf)
	if !strings.Contains(string(buf[:n]), "error") {
		t.Errorf("expected error for invalid approve params")
	}

	// Invalid params for session.reject_action
	_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","id":"3","method":"session.reject_action","params":"invalid"}` + "\n"))
	n, _ = conn.Read(buf)
	if !strings.Contains(string(buf[:n]), "error") {
		t.Errorf("expected error for invalid reject params")
	}

	// Invalid params for session.reset
	_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","id":"4","method":"session.reset","params":"invalid"}` + "\n"))
	n, _ = conn.Read(buf)
	if !strings.Contains(string(buf[:n]), "error") {
		t.Errorf("expected error for invalid reset params")
	}

	// Invalid params for session.close
	_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","id":"5","method":"session.close","params":"invalid"}` + "\n"))
	n, _ = conn.Read(buf)
	if !strings.Contains(string(buf[:n]), "error") {
		t.Errorf("expected error for invalid close params")
	}

	// 4. Client call error handling on bad json in response
	c, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer c.Close()

	// Unregistered session approve/reject
	err = c.ApproveAction(context.Background(), "no_sess", "act_1")
	if err == nil {
		t.Errorf("expected error for ApproveAction on missing session")
	}
	err = c.RejectAction(context.Background(), "no_sess", "act_1", "reason")
	if err == nil {
		t.Errorf("expected error for RejectAction on missing session")
	}

	// Call invalid json unmarshal into incompatible type (e.g. slice from map result)
	var badTarget []int
	err = c.Call(context.Background(), ipc.MethodDaemonPing, nil, &badTarget)
	if err == nil {
		t.Errorf("expected unmarshal error into slice from ping result map")
	}

	// Call with unmarshalable params
	unmarshalableParams := make(chan int)
	err = c.Call(context.Background(), ipc.MethodDaemonPing, unmarshalableParams, nil)
	if err == nil {
		t.Errorf("expected error for unmarshalable params")
	}

	// Call cancelled context
	canceledCtx, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	err = c.Call(canceledCtx, ipc.MethodDaemonPing, nil, nil)
	if err == nil {
		t.Errorf("expected error for cancelled context call")
	}

	// Call on closed client
	cClosed, err := ipc.Dial("unix", sockPath)
	if err == nil {
		_ = cClosed.Close()
		err = cClosed.Call(context.Background(), ipc.MethodDaemonPing, nil, nil)
		if err == nil {
			t.Errorf("expected error calling on closed client")
		}
	}
}

func TestIPC_SpecMachineVerificationFlows(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "gate_spec.yaml")
	_ = os.WriteFile(specPath, []byte(`---
title: "Spec Verification Gate"
preflight_cmd: "echo preflight"
verify_cmd: "echo passed"
target_files:
  - "allowed.txt"
---
`), 0644)

	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		switch turn {
		case 1:
			// Attempt to write forbidden file
			return []string{
				"<action name=\"write_file\">\n",
				"<path>forbidden.txt</path>\n",
				"<content>bad</content>\n",
				"</action>",
			}
		case 2:
			// Attempt forbidden bash command
			return []string{
				"<action name=\"exec_bash\">cat forbidden.txt</action>",
			}
		case 3:
			// Run verification test
			return []string{
				"<action name=\"run_test\">TestAll</action>",
			}
		case 4:
			// Run exec_bash matching verify_cmd
			return []string{
				"<action name=\"exec_bash\">echo passed</action>",
			}
		default:
			return []string{"<action name=\"task_finish\">done</action>"}
		}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "spec_flow.sock")
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
	go func() { _ = srv.Serve(ctx) }()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	yolo := true
	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_spec_gate",
		WorkDir:   tmpDir,
		SpecFile:  specPath,
		YOLO:      &yolo,
		MaxTurns:  10,
	})
	if err != nil {
		t.Fatalf("CreateSession with spec failed: %v", err)
	}

	res, err := client.Prompt(context.Background(), ipc.SessionPromptParams{
		SessionID: "sess_spec_gate",
		Prompt:    "execute with spec gates",
	})
	if err != nil {
		t.Fatalf("Prompt failed: %v", err)
	}
	if res.Status != "completed" {
		t.Errorf("expected completed turn, got %s", res.Status)
	}
}

func TestIPC_BusySessionRejection(t *testing.T) {
	tmpDir := t.TempDir()

	mockLLM, _ := startMockLLMServer(t, func(turn int, req agent.StreamChatRequest) []string {
		time.Sleep(100 * time.Millisecond)
		return []string{"<action name=\"task_finish\">done</action>"}
	})
	defer mockLLM.Close()

	sockPath := filepath.Join(tmpDir, "busy.sock")
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
	go func() { _ = srv.Serve(ctx) }()

	client1, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial client1 failed: %v", err)
	}
	defer client1.Close()

	client2, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial client2 failed: %v", err)
	}
	defer client2.Close()

	yolo := true
	_, err = client1.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_busy",
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	go func() {
		_, _ = client1.Prompt(context.Background(), ipc.SessionPromptParams{
			SessionID: "sess_busy",
			Prompt:    "slow prompt",
		})
	}()

	time.Sleep(20 * time.Millisecond)

	// Second client attempting concurrent prompt on same session must be rejected with error
	_, err = client2.Prompt(context.Background(), ipc.SessionPromptParams{
		SessionID: "sess_busy",
		Prompt:    "concurrent prompt",
	})
	if err == nil {
		t.Fatalf("expected error when prompting busy session, got nil")
	}
	if !strings.Contains(err.Error(), "executing a prompt") {
		t.Errorf("expected error about executing a prompt, got: %v", err)
	}
}

func TestIPC_PromptPreflightFailure(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "failing_preflight.yaml")
	_ = os.WriteFile(specPath, []byte(`---
title: "Failing Preflight"
target_files:
  - "/etc/passwd"
---
`), 0644)

	sockPath := filepath.Join(tmpDir, "preflight_fail.sock")
	srv := ipc.NewServer(ipc.ServerConfig{DefaultWorkDir: tmpDir})
	if err := srv.Listen("unix", sockPath); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()

	client, err := ipc.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	yolo := true
	_, err = client.CreateSession(context.Background(), ipc.SessionCreateParams{
		SessionID: "sess_preflight_fail",
		SpecFile:  specPath,
		YOLO:      &yolo,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = client.Prompt(context.Background(), ipc.SessionPromptParams{
		SessionID: "sess_preflight_fail",
		Prompt:    "trigger preflight failure",
	})
	if err == nil {
		t.Fatalf("expected error from failed spec preflight, got nil")
	}
	if !strings.Contains(err.Error(), "spec preflight check failed") {
		t.Errorf("expected preflight check failed error, got: %v", err)
	}
}
