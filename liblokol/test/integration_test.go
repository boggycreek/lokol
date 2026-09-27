// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

//go:build integration

package lokol_test

import (
	"context"
	"net/http"
	"os"
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
