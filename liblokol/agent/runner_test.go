// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
)

// TestRunner_LoopCircuitBreaker verifies that the runner halts runaway loops
// when the agent executes the exact same failed action repeatedly.
func TestRunner_LoopCircuitBreaker(t *testing.T) {
	tmpDir := t.TempDir()

	turnsExecuted := 0
	var receivedIntervention bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.StreamChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Check if runner injected loop intervention into the user message
		for _, msg := range req.Messages {
			if strings.Contains(msg.Content, "SYSTEM INTERVENTION: Loop detected") {
				receivedIntervention = true
			}
		}

		turnsExecuted++
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Model stubbornly repeats the exact same non-existent file replacement
		tokens := []string{
			"I will edit the file.\n",
			"<action name=\"replace_file\">\n",
			"<path>missing.txt</path>\n<target>foo</target>\n<replacement>bar</replacement>\n",
			"</action>",
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
	defer server.Close()

	client := agent.NewClient(server.URL)
	runner := &agent.Runner{
		Client:   client,
		MaxTurns: 10,
		YOLO:     true,
		WorkDir:  tmpDir,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := runner.Run(ctx, "Fix the missing file")

	t.Run("TripBreakerWithError", func(t *testing.T) {
		if err == nil {
			t.Fatal("expected runner to trip circuit breaker on loop, got nil error")
		}
		if !strings.Contains(err.Error(), "loop detected") {
			t.Fatalf("expected loop detected error, got: %v", err)
		}
	})

	t.Run("InjectInterventionNudge", func(t *testing.T) {
		if !receivedIntervention {
			t.Errorf("runner did not inject loop intervention into conversation turns")
		}
	})

	t.Run("PreventTurnExhaustion", func(t *testing.T) {
		if turnsExecuted >= 10 {
			t.Fatalf("expected breaker to trip before max turns (10), but executed %d turns", turnsExecuted)
		}
	})
}

// TestCoreAgent_Session_StreamingTurnAndActionExecution verifies that agent.Session
// coordinates multi-turn tool loops, executing host actions and ingesting reciprocal action_results
// independently of any presentation layer.
func TestCoreAgent_Session_StreamingTurnAndActionExecution(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "core_test.txt")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.StreamChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		hasActionResult := false
		for _, msg := range req.Messages {
			if msg.Role == "user" && strings.Contains(msg.Content, "<action_result>") {
				hasActionResult = true
				break
			}
		}

		var tokens []string
		if !hasActionResult {
			// Turn 1: Propose writing a file
			tokens = []string{
				"I will create the requested file.\n",
				"<action name=\"write_file\">\n",
				fmt.Sprintf("<path>%s</path>\n", targetFile),
				"<content>hello from core session\n</content>\n",
				"</action>",
			}
		} else {
			// Turn 2: Received action_result, finish task
			tokens = []string{
				"File verified.\n",
				"<action name=\"task_finish\">\n",
				"core_test.txt created and verified\n",
				"</action>",
			}
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
	defer server.Close()

	client := agent.NewClient(server.URL)
	session := agent.NewSession(client, tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Initial user request
	session.AppendUserMessage("Create core_test.txt")

	// 2. Turn 1 Stream
	tokenChan := make(chan string, 100)
	turn1Resp, err := session.StreamTurn(ctx, tokenChan)
	if err != nil {
		t.Fatalf("turn 1 stream failed: %v", err)
	}
	session.AppendAssistantMessage(turn1Resp)

	// 3. Parse action and execute via session
	act := agent.ParseAction(turn1Resp)
	if act == nil || act.Name != "write_file" {
		t.Fatalf("expected write_file action, got: %+v", act)
	}

	execOut, execErr := session.ExecuteAction(ctx, act)
	if execErr != nil {
		t.Fatalf("session.ExecuteAction failed: %v", execErr)
	}
	session.AppendActionResult(execOut, execErr)

	// Verify file was written to disk
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "hello from core session" {
		t.Errorf("got %q, want %q", string(data), "hello from core session")
	}

	// 4. Turn 2 Stream (ingesting reciprocal action_result)
	tokenChan2 := make(chan string, 100)
	turn2Resp, err := session.StreamTurn(ctx, tokenChan2)
	if err != nil {
		t.Fatalf("turn 2 stream failed: %v", err)
	}
	session.AppendAssistantMessage(turn2Resp)

	finishAct := agent.ParseAction(turn2Resp)
	if finishAct == nil || finishAct.Name != "task_finish" {
		t.Fatalf("expected task_finish in turn 2, got: %+v", finishAct)
	}
	if !strings.Contains(finishAct.Command, "core_test.txt created and verified") {
		t.Errorf("unexpected finish summary: %s", finishAct.Command)
	}

	// 5. Verify conversation history integrity
	if len(session.History) != 5 {
		t.Fatalf("expected 5 messages in session history (system, user, assistant, user result, assistant finish), got %d", len(session.History))
	}
	if session.History[3].Role != "user" || !strings.Contains(session.History[3].Content, "<action_result>") {
		t.Errorf("expected <action_result> in message 3, got: %+v", session.History[3])
	}
}

// TestSimulatedRunner_StreamingMultiTurnLoop verifies end-to-end multi-turn tool loops
// driven by agent.Runner wrapping the core session.
func TestSimulatedRunner_StreamingMultiTurnLoop(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "hello.txt")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.StreamChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		hasActionResult := false
		for _, msg := range req.Messages {
			if msg.Role == "user" && strings.Contains(msg.Content, "<action_result>") {
				hasActionResult = true
				break
			}
		}

		var tokens []string
		if !hasActionResult {
			tokens = []string{
				"I will create the requested file.\n",
				"<action name=\"write_file\">\n",
				fmt.Sprintf("<path>%s</path>\n", targetFile),
				"<content>hello world from lokol\n</content>\n",
				"</action>",
			}
		} else {
			tokens = []string{
				"File has been created and verified.\n",
				"<action name=\"task_finish\">\n",
				"hello.txt created successfully\n",
				"</action>",
			}
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
	defer server.Close()

	client := agent.NewClient(server.URL)

	var recordedEvents []string
	runner := &agent.Runner{
		Client:   client,
		MaxTurns: 5,
		YOLO:     true,
		WorkDir:  tmpDir,
		OnOutput: func(role, content string) {
			recordedEvents = append(recordedEvents, role+":"+content)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	summary, err := runner.Run(ctx, "Create hello.txt")
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	if !strings.Contains(summary, "hello.txt created successfully") {
		t.Errorf("unexpected summary: %q", summary)
	}

	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "hello world from lokol" {
		t.Errorf("got file content %q, want %q", string(data), "hello world from lokol")
	}

	hasWriteFile := false
	hasFinish := false
	for _, ev := range recordedEvents {
		if strings.HasPrefix(ev, "write_file:") {
			hasWriteFile = true
		}
		if strings.HasPrefix(ev, "finish:") {
			hasFinish = true
		}
	}
	if !hasWriteFile {
		t.Error("expected write_file event in recorded output")
	}
	if !hasFinish {
		t.Error("expected finish event in recorded output")
	}
}

// TestSimulatedRunner_BashExecutionFeedbackLoop tests multi-turn bash execution and result ingestion.
func TestSimulatedRunner_BashExecutionFeedbackLoop(t *testing.T) {
	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.StreamChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		hasActionResult := false
		for _, msg := range req.Messages {
			if strings.Contains(msg.Content, "echo-feedback-test") {
				hasActionResult = true
				break
			}
		}

		var tokens []string
		if !hasActionResult {
			tokens = []string{
				"I will execute bash echo.\n",
				"<action name=\"exec_bash\">\n",
				"echo echo-feedback-test\n",
				"</action>",
			}
		} else {
			tokens = []string{
				"Echo verified.\n",
				"<action name=\"task_finish\">\n",
				"echo verified successfully\n",
				"</action>",
			}
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
	defer server.Close()

	client := agent.NewClient(server.URL)
	runner := &agent.Runner{
		Client:   client,
		MaxTurns: 5,
		YOLO:     true,
		WorkDir:  tmpDir,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	summary, err := runner.Run(ctx, "Test bash echo")
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	if !strings.Contains(summary, "echo verified successfully") {
		t.Errorf("unexpected summary: %q", summary)
	}
}
