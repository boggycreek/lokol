// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/boggycreek/lokol/pkg/agent"
	"github.com/boggycreek/lokol/pkg/probe"
	"github.com/boggycreek/lokol/pkg/tools/refinery"
	"github.com/boggycreek/lokol/pkg/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPromptStructureHierarchy(t *testing.T) {
	// 1. Without codebase context: returns SystemPrompt unchanged
	emptySys := agent.BuildSystemPrompt("")
	if emptySys != agent.SystemPrompt {
		t.Errorf("expected SystemPrompt when codebase context is empty, got %q", emptySys)
	}

	// 2. With codebase context: Tier 1 invariant system prompt + Tier 2 codebase context
	codebaseCtx := "<project_guidelines source=\"AGENTS.md\">\nAlways run tests with -v\n</project_guidelines>"
	combinedSys := agent.BuildSystemPrompt(codebaseCtx)

	if !strings.HasPrefix(combinedSys, agent.SystemPrompt) {
		t.Errorf("expected combined system prompt to start with Invariant System Prompt")
	}
	if !strings.Contains(combinedSys, "<codebase_context>") || !strings.Contains(combinedSys, codebaseCtx) {
		t.Errorf("expected combined system prompt to contain <codebase_context> block with codebase context")
	}

	// 3. Test BuildInitialHistory produces [System, User]
	history := agent.BuildInitialHistory(codebaseCtx, "Fix bug in parser")
	if len(history) != 2 {
		t.Fatalf("expected 2 messages in initial history, got %d", len(history))
	}
	if history[0].Role != "system" || history[0].Content != combinedSys {
		t.Errorf("history[0] should be system message with combined prompt hierarchy")
	}
	if history[1].Role != "user" || history[1].Content != "Fix bug in parser" {
		t.Errorf("history[1] should be user message with initial prompt")
	}
}

func TestRefineryLoadCodebaseContext(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "refinery-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Case A: No guidelines file
	ctxNone := refinery.LoadCodebaseContext(tmpDir)
	if ctxNone != "" {
		t.Errorf("expected empty string when no guidelines file exists, got: %q", ctxNone)
	}

	// Case B: AGENTS.md exists
	agentsContent := "# Project Guidelines\n1. Use beads for tasks\n2. Run make test"
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte(agentsContent), 0644); err != nil {
		t.Fatalf("failed to write AGENTS.md: %v", err)
	}

	ctxAgents := refinery.LoadCodebaseContext(tmpDir)
	if !strings.Contains(ctxAgents, "<project_guidelines source=\"AGENTS.md\">") {
		t.Errorf("expected guidelines source=AGENTS.md, got: %s", ctxAgents)
	}
	if !strings.Contains(ctxAgents, "Use beads for tasks") {
		t.Errorf("expected guidelines content to be included, got: %s", ctxAgents)
	}
}

func TestCachePromptFlagInStreamRequest(t *testing.T) {
	var mu sync.Mutex
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil {
				mu.Lock()
				_ = json.Unmarshal(bodyBytes, &receivedBody)
				mu.Unlock()
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := agent.NewClient(server.URL)
	tokenChan := make(chan string, 10)
	history := []agent.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: "Hello"},
	}

	_, err := client.StreamResponse(context.Background(), history, tokenChan)
	if err != nil {
		t.Fatalf("StreamResponse failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	cachePromptVal, ok := receivedBody["cache_prompt"]
	if !ok {
		t.Fatalf("expected 'cache_prompt' in request body, but key was absent: %v", receivedBody)
	}
	if cachePromptBool, ok := cachePromptVal.(bool); !ok || !cachePromptBool {
		t.Errorf("expected 'cache_prompt' to be true, got: %v", cachePromptVal)
	}
}

func TestSlotAbortOnContextCancellation(t *testing.T) {
	var mu sync.Mutex
	abortCalled := false
	streamStarted := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if ok {
				_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"start\"}}]}\n\n")
				flusher.Flush()
			}
			close(streamStarted)

			// Hold connection open until client disconnects or cancels
			<-r.Context().Done()
			return
		}
		if strings.HasPrefix(r.URL.Path, "/slots") && r.Method == http.MethodPost {
			mu.Lock()
			abortCalled = true
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id_slot":0,"n_erased":100}`))
			return
		}
		if r.URL.Path == "/slots" && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":0,"n_ctx":4096,"n_prompt_tokens":100,"is_processing":true}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := agent.NewClient(server.URL)
	tokenChan := make(chan string, 10)
	history := []agent.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: "Generate lots of code"},
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-streamStarted
		// Simulate Ctrl+C / client context cancellation while streaming
		cancel()
	}()

	_, _ = client.StreamResponse(ctx, history, tokenChan)

	// Wait briefly for abort goroutine to send abort call
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	called := abortCalled
	mu.Unlock()

	if !called {
		t.Errorf("expected slot abort to be called when context was canceled, but was not")
	}
}

func TestAbortSlotAndAbortActiveSlots(t *testing.T) {
	erasedSlots := make(map[string]bool)
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/slots" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{"id":0,"n_ctx":4096,"n_prompt_tokens":100,"is_processing":true},
				{"id":1,"n_ctx":4096,"n_prompt_tokens":0,"is_processing":false}
			]`))
			return
		}
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/slots/") {
			action := r.URL.Query().Get("action")
			if action == "erase" {
				mu.Lock()
				erasedSlots[r.URL.Path] = true
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := agent.NewClient(server.URL)

	// Test AbortSlot directly
	err := client.AbortSlot(context.Background(), 0)
	if err != nil {
		t.Errorf("AbortSlot failed: %v", err)
	}

	// Test AbortActiveSlots
	err = client.AbortActiveSlots(context.Background())
	if err != nil {
		t.Errorf("AbortActiveSlots failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !erasedSlots["/slots/0"] {
		t.Errorf("expected /slots/0 to be erased, recorded: %v", erasedSlots)
	}
}

func TestTUIInterruptionAndSlotRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/slots") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":0,"n_ctx":4096,"n_prompt_tokens":0,"is_processing":false}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := agent.NewClient(server.URL)
	hw := &probe.HardwareProfile{OS: "linux", Arch: "amd64", CPUCores: 4}
	m := tui.New(client, hw, false)

	// Test Ctrl+C while idle triggers tea.Quit
	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_ = updatedModel
	if cmd == nil {
		t.Errorf("expected quit command on Ctrl+C when idle")
	}

	// Test Esc while idle does nothing (does not quit)
	_, escCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if escCmd != nil {
		t.Errorf("expected nil command on Esc when idle, got %v", escCmd)
	}
}
