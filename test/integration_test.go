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

	"github.com/boggycreek/lokol/pkg/agent"
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

// TestLocalEngine_Integration runs against the local inference engine if available.
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

	t.Run("Grounding - Current working directory query", func(t *testing.T) {
		history := []agent.Message{
			{Role: "system", Content: agent.BuildSystemPrompt(tmpDir)},
			{Role: "user", Content: "What is your current working directory?"},
		}

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
			_, err := client.StreamResponse(ctx, history, tokenChan)
			close(tokenChan)
			errChan <- err
		}()

		if err := <-errChan; err != nil {
			t.Fatalf("StreamResponse failed: %v", err)
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

		// Must produce an active action
		act := agent.ParseAction(resp)
		if act == nil {
			t.Fatalf("expected active action invocation, got nil action. Response: %s", resp)
		}
		if act.Name != "exec_bash" && act.Name != "task_finish" {
			t.Errorf("expected exec_bash or task_finish, got %q", act.Name)
		}
	})

	t.Run("Grounding - List files query", func(t *testing.T) {
		history := []agent.Message{
			{Role: "system", Content: agent.BuildSystemPrompt(tmpDir)},
			{Role: "user", Content: "List files in this repo"},
		}

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
			_, err := client.StreamResponse(ctx, history, tokenChan)
			close(tokenChan)
			errChan <- err
		}()

		if err := <-errChan; err != nil {
			t.Fatalf("StreamResponse failed: %v", err)
		}
		resp := <-done

		lowerResp := strings.ToLower(resp)
		if strings.Contains(lowerResp, "as an ai") || strings.Contains(lowerResp, "cannot access") {
			t.Errorf("model produced evasive response: %s", resp)
		}

		act := agent.ParseAction(resp)
		if act == nil {
			t.Fatalf("expected active action invocation for 'List files in this repo', got none: %s", resp)
		}
	})

	t.Run("Feedback Loop - Ingest action_result", func(t *testing.T) {
		mockFilesOutput := "total 8\n-rw-r--r-- 1 lokol lokol 120 Jan 1 00:00 main.go\n-rw-r--r-- 1 lokol lokol 300 Jan 1 00:00 go.mod\n"
		actionResultTag := fmt.Sprintf("<action_result>\n%s\n</action_result>", mockFilesOutput)

		history := []agent.Message{
			{Role: "system", Content: agent.BuildSystemPrompt(tmpDir)},
			{Role: "user", Content: "List files in this repo"},
			{Role: "assistant", Content: "I will list the files in the directory.\n<action name=\"exec_bash\">\nls -la\n</action>"},
			{Role: "user", Content: actionResultTag},
		}

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
			_, err := client.StreamResponse(ctx, history, tokenChan)
			close(tokenChan)
			errChan <- err
		}()

		if err := <-errChan; err != nil {
			t.Fatalf("StreamResponse failed: %v", err)
		}
		resp := <-done

		// Verify model ingests the output and responds with next action or task_finish
		act := agent.ParseAction(resp)
		if act == nil && !strings.Contains(resp, "main.go") && !strings.Contains(resp, "go.mod") {
			t.Errorf("model did not ingest action_result or produce follow-up: %s", resp)
		}
	})
}

// TestSimulatedRunner_StreamingMultiTurnLoop verifies end-to-end multi-turn tool loops
// with streaming SSE and reciprocal action_result handling without requiring an external engine.
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

		// Determine turn from history
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
				"<content>hello world from lokol\n</content>\n",
				"</action>",
			}
		} else {
			// Turn 2: Received <action_result>, finish the task
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
					{
						Delta: struct {
							Content string `json:"content"`
						}{Content: tok},
					},
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

	// Verify file was actually created by runner executing write_file
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}
	if string(data) != "hello world from lokol" {
		t.Errorf("got file content %q, want %q", string(data), "hello world from lokol")
	}

	// Verify write_file and finish events occurred
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
					{
						Delta: struct {
							Content string `json:"content"`
						}{Content: tok},
					},
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
