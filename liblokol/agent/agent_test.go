// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
)

func TestParseAction(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantAction  bool
		wantName    string
		wantCommand string
		wantThought string
	}{
		{
			name:        "valid bash action",
			input:       "I will check the git status.\n<action name=\"exec_bash\">\ngit status\n</action>",
			wantAction:  true,
			wantName:    "exec_bash",
			wantCommand: "git status",
			wantThought: "I will check the git status.",
		},
		{
			name:        "valid finish action",
			input:       "Done!\n<action name=\"task_finish\">\nRefactoring complete.\n</action>",
			wantAction:  true,
			wantName:    "task_finish",
			wantCommand: "Refactoring complete.",
			wantThought: "Done!",
		},
		{
			name:        "action with single quotes and spaces",
			input:       "Running command.\n<action name='exec_bash' >\nls -la\n</action>",
			wantAction:  true,
			wantName:    "exec_bash",
			wantCommand: "ls -la",
			wantThought: "Running command.",
		},
		{
			name:        "action wrapped in markdown xml codeblock",
			input:       "Here is the change:\n```xml\n<action name=\"write_file\">\n<path>foo.txt</path>\n<content>bar</content>\n</action>\n```",
			wantAction:  true,
			wantName:    "write_file",
			wantCommand: "<path>foo.txt</path>\n<content>bar</content>",
			wantThought: "Here is the change:",
		},
		{
			name:        "get_environment action",
			input:       "I will check the environment.\n<action name=\"get_environment\">\n</action>",
			wantAction:  true,
			wantName:    "get_environment",
			wantCommand: "",
			wantThought: "I will check the environment.",
		},
		{
			name:        "no action present",
			input:       "Here is an explanation of the problem.",
			wantAction:  false,
			wantName:    "",
			wantCommand: "",
			wantThought: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act := agent.ParseAction(tt.input)
			if tt.wantAction {
				if act == nil {
					t.Fatalf("expected action, got nil")
				}
				if act.Name != tt.wantName {
					t.Errorf("got name %q, want %q", act.Name, tt.wantName)
				}
				if act.Command != tt.wantCommand {
					t.Errorf("got command %q, want %q", act.Command, tt.wantCommand)
				}
				if act.CleanThought != tt.wantThought {
					t.Errorf("got thought %q, want %q", act.CleanThought, tt.wantThought)
				}
			} else {
				if act != nil {
					t.Fatalf("expected nil action, got %+v", act)
				}
			}
		})
	}
}

func TestExecuteReplaceFile(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	filePath := tmpDir + "/test.txt"
	initialContent := "Hello world\r\nFoo bar baz\r\nEnding line"
	if err := os.WriteFile(filePath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	// Test CRLF normalization and exact match
	payload := fmt.Sprintf("<path>%s</path>\n<target>Foo bar baz</target>\n<replacement>Quik replaced this</replacement>", filePath)
	out, err := agent.ExecuteReplaceFile(context.Background(), payload, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Successfully replaced") {
		t.Errorf("unexpected output: %s", out)
	}

	updated, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	expected := "Hello world\nQuik replaced this\nEnding line"
	if string(updated) != expected {
		t.Errorf("got %q, want %q", string(updated), expected)
	}

	// Test whitespace-trimmed fallback matching
	targetWithPadding := "\n\nQuik replaced this\n\n"
	payloadFallback := fmt.Sprintf("<path>%s</path>\n<target>%s</target>\n<replacement>Fallback matched</replacement>", filePath, targetWithPadding)
	out2, err := agent.ExecuteReplaceFile(context.Background(), payloadFallback, tmpDir)
	if err != nil {
		t.Fatalf("fallback replacement failed: %v", err)
	}
	if !strings.Contains(out2, "Successfully replaced") {
		t.Errorf("unexpected output: %s", out2)
	}
}

func TestExecuteWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/nested/subdir/hello.go"
	payload := fmt.Sprintf("<path>%s</path>\n<content>package main\n\nfunc main() {}\n</content>", filePath)

	out, err := agent.ExecuteWriteFile(context.Background(), payload, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Successfully wrote") {
		t.Errorf("unexpected output: %s", out)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}
	if string(data) != "package main\n\nfunc main() {}" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestGetSlotStatus(t *testing.T) {
	client := agent.NewClient("http://127.0.0.1:8080")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	status, err := client.GetSlotStatus(ctx)
	if err != nil {
		t.Logf("llama-server might not be reachable from test: %v", err)
		return
	}

	if status.NCtx <= 0 {
		t.Errorf("expected positive NCtx, got %d", status.NCtx)
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	t.Run("with working directory", func(t *testing.T) {
		prompt := agent.BuildSystemPrompt("/home/user/project")
		if !strings.Contains(prompt, "<cwd>/home/user/project</cwd>") {
			t.Errorf("expected prompt to contain <cwd>/home/user/project</cwd>, got:\n%s", prompt[:300])
		}
		if !strings.Contains(prompt, "<environment>") || !strings.Contains(prompt, "</environment>") {
			t.Error("expected prompt to contain <environment> tag")
		}
		if !strings.Contains(prompt, "You are lokol") {
			t.Error("expected prompt to contain identity header")
		}
	})

	t.Run("environment protocol and action_result documentation", func(t *testing.T) {
		prompt := agent.BuildSystemPrompt("/tmp")
		if !strings.Contains(prompt, "<action_result>") || !strings.Contains(prompt, "</action_result>") {
			t.Error("expected prompt to document reciprocal <action_result> tags")
		}
		if !strings.Contains(prompt, "Tool Execution Protocol:") {
			t.Error("expected prompt to document Tool Execution Protocol")
		}
	})

	t.Run("action formats and protocol included", func(t *testing.T) {
		prompt := agent.BuildSystemPrompt("/tmp")
		if !strings.Contains(prompt, "<action name=\"task_finish\">") {
			t.Error("expected prompt to document task_finish action format")
		}
		if !strings.Contains(prompt, "<action name=\"find_files\">") {
			t.Error("expected prompt to document find_files action format")
		}
		if !strings.Contains(prompt, "<action name=\"tool_help\">") {
			t.Error("expected prompt to document tool_help action format")
		}
		if !strings.Contains(prompt, "get_environment:") || !strings.Contains(prompt, "exec_bash:") {
			t.Error("expected prompt to list specialized tools get_environment and exec_bash")
		}
	})

	t.Run("custom HostEnvironment", func(t *testing.T) {
		env := agent.HostEnvironment{
			AgentName:    "lokol",
			OperatorName: "User",
			Cwd:          "/workspace/repo",
			OS:           "linux",
			Shell:        "/bin/bash",
		}
		prompt := agent.BuildSystemPromptWithEnv(env)
		expectedTag := "<environment>\n<agent_name>lokol</agent_name>\n<operator_name>User</operator_name>\n<cwd>/workspace/repo</cwd>\n<os>linux</os>\n<shell>/bin/bash</shell>\n</environment>"
		if !strings.Contains(prompt, expectedTag) {
			t.Errorf("expected prompt to contain formatted environment tag:\n%s\ngot:\n%s", expectedTag, prompt[:300])
		}
	})

	t.Run("path is cleaned", func(t *testing.T) {
		prompt := agent.BuildSystemPrompt("/home/user/./project/../project/")
		if !strings.Contains(prompt, "<cwd>/home/user/project</cwd>") {
			t.Errorf("expected cleaned path, got:\n%s", prompt[:300])
		}
	})
}

func TestExecuteGetEnvironment(t *testing.T) {
	tmpDir := t.TempDir()

	// Test ExecuteGetEnvironment with empty payload and workDir
	out, err := agent.ExecuteGetEnvironment(context.Background(), "", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error executing get_environment: %v", err)
	}

	if !strings.Contains(out, tmpDir) {
		t.Errorf("expected get_environment output to contain %q, got: %s", tmpDir, out)
	}
	if !strings.Contains(out, "\"working_directory\"") || !strings.Contains(out, "\"os\"") {
		t.Errorf("expected get_environment JSON keys, got: %s", out)
	}
}

func TestDispatchAction_DefenseInDepthBoundaryEnforcement(t *testing.T) {
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside_target.txt")
	ctx := context.Background()

	// 1. write_file to absolute path outside workDir
	actWriteAbs := &agent.Action{
		Name:    "write_file",
		Command: fmt.Sprintf("<path>%s</path><content>pwned</content>", outsideFile),
	}
	_, err := agent.DispatchAction(ctx, actWriteAbs, workDir)
	if err == nil {
		t.Fatalf("expected DispatchAction write_file outside workDir to fail, got nil")
	}
	if _, err := os.Stat(outsideFile); !os.IsNotExist(err) {
		t.Fatalf("file %s was written despite boundary violation", outsideFile)
	}

	// 2. write_file with directory traversal (../../)
	traversalTarget := filepath.Join(workDir, "..", "..", "traversal_pwn.txt")
	actWriteRel := &agent.Action{
		Name:    "write_file",
		Command: "<path>../../traversal_pwn.txt</path><content>pwned</content>",
	}
	_, err = agent.DispatchAction(ctx, actWriteRel, workDir)
	if err == nil {
		t.Fatalf("expected DispatchAction relative traversal to fail, got nil")
	}
	if _, err := os.Stat(traversalTarget); !os.IsNotExist(err) {
		t.Fatalf("file %s was written despite boundary violation", traversalTarget)
	}

	// 3. replace_file outside workDir
	targetForReplace := filepath.Join(outsideDir, "existing.txt")
	if err := os.WriteFile(targetForReplace, []byte("original text"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	actReplace := &agent.Action{
		Name:    "replace_file",
		Command: fmt.Sprintf("<path>%s</path><target>original</target><replacement>hacked</replacement>", targetForReplace),
	}
	_, err = agent.DispatchAction(ctx, actReplace, workDir)
	if err == nil {
		t.Fatalf("expected DispatchAction replace_file outside workDir to fail, got nil")
	}

	// 4. read_outline outside workDir
	actOutline := &agent.Action{
		Name:    "read_outline",
		Command: fmt.Sprintf("<path>%s</path>", targetForReplace),
	}
	_, err = agent.DispatchAction(ctx, actOutline, workDir)
	if err == nil {
		t.Fatalf("expected DispatchAction read_outline outside workDir to fail, got nil")
	}

	// 5. read_window outside workDir
	actWindow := &agent.Action{
		Name:    "read_window",
		Command: fmt.Sprintf("<path>%s</path><start_line>1</start_line><end_line>5</end_line>", targetForReplace),
	}
	_, err = agent.DispatchAction(ctx, actWindow, workDir)
	if err == nil {
		t.Fatalf("expected DispatchAction read_window outside workDir to fail, got nil")
	}

	// 6. Direct ExecuteWriteFile call without runner
	_, err = agent.ExecuteWriteFile(ctx, fmt.Sprintf("<path>%s</path><content>direct_pwn</content>", outsideFile), workDir)
	if err == nil {
		t.Fatalf("expected direct ExecuteWriteFile to fail boundary check, got nil")
	}

	// 7. Direct ExecuteReplaceFile call without runner
	_, err = agent.ExecuteReplaceFile(ctx, fmt.Sprintf("<path>%s</path><target>original</target><replacement>direct</replacement>", targetForReplace), workDir)
	if err == nil {
		t.Fatalf("expected direct ExecuteReplaceFile to fail boundary check, got nil")
	}
}

func TestTools_TOCTOUSymlinkMitigation(t *testing.T) {
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideSecret := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideSecret, []byte("super_secret_token"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Create a symlink inside workDir pointing to outsideSecret
	symlinkPath := filepath.Join(workDir, "sneaky_symlink.txt")
	if err := os.Symlink(outsideSecret, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	ctx := context.Background()

	// Attempt write_file through the symlink
	_, err := agent.ExecuteWriteFile(ctx, fmt.Sprintf("<path>%s</path><content>overwrite_secret</content>", symlinkPath), workDir)
	if err == nil {
		t.Fatalf("expected ExecuteWriteFile to fail when writing through symlink pointing outside workDir")
	}

	// Confirm outside secret was NOT overwritten
	content, err := os.ReadFile(outsideSecret)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(content) != "super_secret_token" {
		t.Fatalf("secret was overwritten despite symlink protection: %s", string(content))
	}
}

func TestSession_PersonaConfiguration(t *testing.T) {
	tmpDir := t.TempDir()
	client := agent.NewClient("http://127.0.0.1:8080")
	s := agent.NewSession(client, tmpDir)

	agentName, opName := s.GetPersona()
	if agentName == "" || opName == "" {
		t.Fatalf("expected non-empty default persona, got agent=%q op=%q", agentName, opName)
	}

	// Dynamic persona reconfiguration
	s.SetPersona("Aria", "Alice")
	newAgent, newOp := s.GetPersona()
	if newAgent != "Aria" || newOp != "Alice" {
		t.Errorf("expected Aria/Alice, got %s/%s", newAgent, newOp)
	}

	// Check system prompt was updated with new persona and tags
	if len(s.History) == 0 || s.History[0].Role != "system" {
		t.Fatalf("expected system message in history")
	}
	sysPrompt := s.History[0].Content
	if !strings.Contains(sysPrompt, "You are Aria") {
		t.Errorf("expected updated prompt with 'You are Aria', got:\n%s", sysPrompt[:200])
	}
	if !strings.Contains(sysPrompt, "<agent_name>Aria</agent_name>") || !strings.Contains(sysPrompt, "<operator_name>Alice</operator_name>") {
		t.Errorf("expected persona XML tags in environment block, got:\n%s", sysPrompt[:300])
	}
}


