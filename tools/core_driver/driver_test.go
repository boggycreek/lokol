// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
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

func TestLoadScenarios_ValidAndCommentLines(t *testing.T) {
	tempDir := t.TempDir()
	jsonlPath := filepath.Join(tempDir, "test_scenarios.jsonl")

	content := `# Comment line
{"id":"sc_1","name":"Scenario One","prompt":"Do foo","mode":"general","max_wall_clock_sec":10.0,"max_gpu_pegged_sec":5.0,"laya_instructions":"Check foo","laya_threshold":0.6,"forbidden_substrings":["bar"],"required_substrings":["foo"],"description":"First"}
{"id":"sc_2","name":"Scenario Two","prompt":"Do baz","mode":"coding","max_wall_clock_sec":15.0,"max_gpu_pegged_sec":6.0,"laya_instructions":"","laya_threshold":0.0,"forbidden_substrings":[],"required_substrings":["baz"],"description":"Second"}
`
	if err := os.WriteFile(jsonlPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test scenarios: %v", err)
	}

	scenarios, err := LoadScenarios(jsonlPath)
	if err != nil {
		t.Fatalf("unexpected error loading scenarios: %v", err)
	}

	if len(scenarios) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(scenarios))
	}
	if scenarios[0].ID != "sc_1" || scenarios[0].Mode != "general" {
		t.Errorf("scenario 0 mismatch: %+v", scenarios[0])
	}
	if scenarios[1].ID != "sc_2" || scenarios[1].Mode != "coding" {
		t.Errorf("scenario 1 mismatch: %+v", scenarios[1])
	}
}

func TestScenarioRunner_InProcessExecution_Passing(t *testing.T) {
	// Mock inference engine server responding with SSE tokens
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "flusher unsupported", 500)
				return
			}
			tokens := []string{"Hello", " world,", " this", " satisfies", " required", " conditions."}
			for _, tok := range tokens {
				chunk := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", tok)
				w.Write([]byte(chunk))
				flusher.Flush()
			}
			w.Write([]byte("data: [DONE]\n\n"))
			flusher.Flush()
		case "/slots":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":0,"n_ctx":2048,"n_prompt_tokens":100,"is_processing":false,"next_token":[{"n_decoded":15}]}]`))
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"mock-llama-3"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := agent.NewClient(server.URL)
	runner := NewScenarioRunner(client, t.TempDir(), t.TempDir(), false, false)

	sc := Scenario{
		ID:                  "sc_mock_pass",
		Name:                "Mock Passing Scenario",
		Prompt:              "Greet me",
		Mode:                "general",
		MaxWallClockSec:     5.0,
		RequiredSubstrings:  []string{"satisfies"},
		ForbiddenSubstrings: []string{"forbidden_error"},
	}

	res := runner.RunScenario(context.Background(), sc)
	if !res.Passed {
		t.Fatalf("expected scenario to pass, failed with: %v", res.FailureReasons)
	}
	if !strings.Contains(res.AssistantText, "Hello world, this satisfies required conditions.") {
		t.Errorf("unexpected assistant text: %q", res.AssistantText)
	}
	if res.Duration <= 0 {
		t.Errorf("expected positive duration, got %v", res.Duration)
	}
}

func TestScenarioRunner_InProcessExecution_FailsForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Encountered fatal forbidden_error in response.\"}}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	client := agent.NewClient(server.URL)
	runner := NewScenarioRunner(client, t.TempDir(), t.TempDir(), false, false)

	sc := Scenario{
		ID:                  "sc_mock_forbidden",
		Name:                "Mock Forbidden Scenario",
		Prompt:              "Do something bad",
		Mode:                "general",
		MaxWallClockSec:     5.0,
		ForbiddenSubstrings: []string{"forbidden_error"},
	}

	res := runner.RunScenario(context.Background(), sc)
	if res.Passed {
		t.Fatal("expected scenario to fail due to forbidden substring, but passed")
	}
	foundReason := false
	for _, r := range res.FailureReasons {
		if strings.Contains(r, "forbidden_error") {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Errorf("expected forbidden_error in failure reasons: %v", res.FailureReasons)
	}
}

func TestScenarioRunner_InProcessExecution_FailsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slots" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
			return
		}
		// Hang until context cancelled
		select {
		case <-r.Context().Done():
		case <-time.After(1 * time.Second):
		}
	}))
	defer server.Close()

	client := agent.NewClient(server.URL)
	runner := NewScenarioRunner(client, t.TempDir(), t.TempDir(), false, false)

	sc := Scenario{
		ID:              "sc_mock_timeout",
		Name:            "Mock Timeout Scenario",
		Prompt:          "Take forever",
		Mode:            "general",
		MaxWallClockSec: 0.1, // 100ms timeout
	}

	res := runner.RunScenario(context.Background(), sc)
	if res.Passed {
		t.Fatal("expected scenario to fail due to timeout, but passed")
	}
	if len(res.FailureReasons) == 0 {
		t.Errorf("expected failure reasons on timeout")
	}
}
