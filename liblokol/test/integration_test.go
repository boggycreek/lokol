// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

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

// checkLiveEngine checks if llama-server or a compatible engine is reachable.
func checkLiveEngine(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// TestLocalEngine_Integration runs against the local inference engine if available,
// testing core agent Session grounding and multi-turn reciprocal tool handling.
func TestLocalEngine_Integration(t *testing.T) {
	engineURL := os.Getenv("LOKOL_TEST_ENGINE_URL")
	if engineURL == "" {
		engineURL = "http://127.0.0.1:8080"
	}

	if !checkLiveEngine(engineURL) {
		t.Skipf("local inference engine not reachable at %s; skipping live integration tests", engineURL)
	}

	client := agent.NewClient(engineURL)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tmpDir := t.TempDir()

	t.Run("Grounding - Current working directory query via Session", func(t *testing.T) {
		session := agent.NewSession(client, tmpDir)
		session.AppendUserMessage("What is your current working directory?")

		tokenChan := make(chan string, 50)
		done := make(chan string, 1)
		errChan := make(chan error, 1)

		go func() {
			var sb strings.Builder
			for tok := range tokenChan {
				sb.WriteString(tok)
			}
			done <- sb.String()
		}()

		go func() {
			resp, err := session.StreamTurn(ctx, tokenChan)
			close(tokenChan)
			if err != nil {
				errChan <- err
				return
			}
			session.AppendAssistantMessage(resp)
			errChan <- nil
		}()

		if err := <-errChan; err != nil {
			t.Fatalf("session.StreamTurn failed: %v", err)
		}
		resp := <-done

		// Must not produce evasive chatbot personas
		lowerResp := strings.ToLower(resp)
		evasions := []string{
			"do not have a",
			"don't have a",
			"as an ai",
			"physical working directory",
			"cannot access",
		}
		for _, evasion := range evasions {
			if strings.Contains(lowerResp, evasion) {
				t.Errorf("model produced evasive chatbot response containing %q: %s", evasion, resp)
			}
		}

		// Must produce an active action or ground directly to the current working directory
		act := agent.ParseAction(resp)
		if act == nil {
			if !strings.Contains(resp, tmpDir) {
				t.Fatalf("expected active action invocation or cwd grounding to %s. Response: %s", tmpDir, resp)
			}
		} else if act.Name != "exec_bash" && act.Name != "task_finish" && act.Name != "get_environment" {
			t.Errorf("expected exec_bash, task_finish, or get_environment, got %q", act.Name)
		}
	})

	t.Run("Feedback Loop - Session action execution and reciprocal ingestion", func(t *testing.T) {
		session := agent.NewSession(client, tmpDir)
		session.AppendUserMessage("List files in this repo")
		session.AppendAssistantMessage("I will list the files in the directory.\n<action name=\"exec_bash\">\nls -la\n</action>")

		mockFilesOutput := "total 8\n-rw-r--r-- 1 lokol lokol 120 Jan 1 00:00 main.go\n-rw-r--r-- 1 lokol lokol 300 Jan 1 00:00 go.mod\n"
		session.AppendActionResult(mockFilesOutput, nil)

		tokenChan := make(chan string, 50)
		done := make(chan string, 1)
		errChan := make(chan error, 1)

		go func() {
			var sb strings.Builder
			for tok := range tokenChan {
				sb.WriteString(tok)
			}
			done <- sb.String()
		}()

		go func() {
			resp, err := session.StreamTurn(ctx, tokenChan)
			close(tokenChan)
			if err != nil {
				errChan <- err
				return
			}
			session.AppendAssistantMessage(resp)
			errChan <- nil
		}()

		if err := <-errChan; err != nil {
			t.Fatalf("session.StreamTurn failed: %v", err)
		}
		resp := <-done

		// Verify model ingests the output and responds with next action or task_finish
		act := agent.ParseAction(resp)
		if act == nil && !strings.Contains(resp, "main.go") && !strings.Contains(resp, "go.mod") {
			t.Errorf("model did not ingest action_result or produce follow-up: %s", resp)
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
