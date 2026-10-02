// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSession_LifecycleAndHistory(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient("http://127.0.0.1:8080")
	s := NewSession(client, tmpDir)

	if s.WorkDir != tmpDir {
		t.Fatalf("expected WorkDir %s, got %s", tmpDir, s.WorkDir)
	}
	if len(s.History) != 1 {
		t.Fatalf("expected initial history with 1 system prompt, got %d", len(s.History))
	}
	if s.History[0].Role != "system" {
		t.Errorf("expected role system, got %s", s.History[0].Role)
	}

	// Append user message
	s.AppendUserMessage("Please create a helper file.")
	if len(s.History) != 2 || s.History[1].Role != "user" || s.History[1].Content != "Please create a helper file." {
		t.Fatalf("user message append failed: %+v", s.History[1])
	}

	// Append assistant message
	s.AppendAssistantMessage("I will write the file.")
	if len(s.History) != 3 || s.History[2].Role != "assistant" {
		t.Fatalf("assistant message append failed: %+v", s.History[2])
	}

	// Append action result success
	s.AppendActionResult("Successfully wrote 12 bytes", nil)
	if len(s.History) != 4 || s.History[3].Role != "user" || !strings.Contains(s.History[3].Content, "Successfully wrote 12 bytes") {
		t.Fatalf("action result append failed: %+v", s.History[3])
	}

	// Append action result error
	s.AppendActionResult("file not found", fmt.Errorf("stat failed"))
	if len(s.History) != 5 || !strings.Contains(s.History[4].Content, "[Error: stat failed]") {
		t.Fatalf("error action result append failed: %+v", s.History[4])
	}

	// Reset
	s.Reset()
	if len(s.History) != 1 || s.History[0].Role != "system" {
		t.Fatalf("expected reset to restore 1 system prompt, got %d", len(s.History))
	}
}

func TestDispatchAction_Execution(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient("http://127.0.0.1:8080")
	s := NewSession(client, tmpDir)
	ctx := context.Background()

	// 1. write_file via session.ExecuteAction
	testFile := filepath.Join(tmpDir, "hello.txt")
	writeAct := &Action{
		Name:    "write_file",
		Command: fmt.Sprintf("<path>%s</path>\n<content>Hello World\nLine 2</content>", testFile),
	}
	out, err := s.ExecuteAction(ctx, writeAct)
	if err != nil {
		t.Fatalf("write_file failed: %v", err)
	}
	if !strings.Contains(out, "Successfully wrote") {
		t.Errorf("unexpected write_file output: %s", out)
	}

	// 2. read_window via DispatchAction
	readAct := &Action{
		Name:    "read_window",
		Command: fmt.Sprintf("<path>%s</path>\n<start>1</start>\n<end>2</end>", testFile),
	}
	out, err = DispatchAction(ctx, readAct, tmpDir)
	if err != nil {
		t.Fatalf("read_window failed: %v", err)
	}
	if !strings.Contains(out, "Hello World") {
		t.Errorf("unexpected read_window output: %s", out)
	}

	// 3. replace_file via DispatchAction
	replaceAct := &Action{
		Name:    "replace_file",
		Command: fmt.Sprintf("<path>%s</path>\n<target>Hello World</target>\n<replacement>Hello Lokol</replacement>", testFile),
	}
	out, err = DispatchAction(ctx, replaceAct, tmpDir)
	if err != nil {
		t.Fatalf("replace_file failed: %v", err)
	}
	data, _ := os.ReadFile(testFile)
	if !strings.Contains(string(data), "Hello Lokol") {
		t.Fatalf("file content not replaced: %s", string(data))
	}

	// 4. find_files via DispatchAction
	findAct := &Action{
		Name:    "find_files",
		Command: "<pattern>*.txt</pattern>",
	}
	out, err = DispatchAction(ctx, findAct, tmpDir)
	if err != nil {
		t.Fatalf("find_files failed: %v", err)
	}
	if !strings.Contains(out, "hello.txt") {
		t.Errorf("expected hello.txt in find_files output, got: %s", out)
	}

	// 5. search_code via DispatchAction
	searchAct := &Action{
		Name:    "search_code",
		Command: "<pattern>Lokol</pattern>",
	}
	out, err = DispatchAction(ctx, searchAct, tmpDir)
	if err != nil {
		t.Fatalf("search_code failed: %v", err)
	}
	if !strings.Contains(out, "hello.txt") || !strings.Contains(out, "Hello Lokol") {
		t.Errorf("expected hello.txt with Hello Lokol in search_code output, got: %s", out)
	}

	// 6. git_diff_summary via DispatchAction
	exec.Command("git", "init", tmpDir).Run()
	diffAct := &Action{
		Name:    "git_diff_summary",
		Command: "",
	}
	out, err = DispatchAction(ctx, diffAct, tmpDir)
	if err != nil {
		t.Fatalf("git_diff_summary failed: %v", err)
	}
	if !strings.Contains(out, "hello.txt") {
		t.Errorf("expected hello.txt in git_diff_summary output, got: %s", out)
	}

	// 7. task_finish via DispatchAction
	finishAct := &Action{
		Name:    "task_finish",
		Command: "Work complete.",
	}
	out, err = DispatchAction(ctx, finishAct, tmpDir)
	if err != nil {
		t.Fatalf("task_finish failed: %v", err)
	}
	if out != "Work complete." {
		t.Errorf("expected 'Work complete.', got: %s", out)
	}

	// 6. Unknown action
	unknownAct := &Action{
		Name:    "invalid_tool",
		Command: "do something",
	}
	_, err = DispatchAction(ctx, unknownAct, tmpDir)
	if err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("expected unknown action error, got: %v", err)
	}

	// 7. Nil action
	_, err = DispatchAction(ctx, nil, tmpDir)
	if err == nil || !strings.Contains(err.Error(), "action is nil") {
		t.Fatalf("expected action is nil error, got: %v", err)
	}
}

func TestAction_PresentationHelpers(t *testing.T) {
	tests := []struct {
		act          *Action
		wantTarget   string
		wantVerbose  string
	}{
		{
			act: &Action{
				Name:    "exec_bash",
				Command: "git status",
			},
			wantTarget:  "git status",
			wantVerbose: "⚡ Executing: git status",
		},
		{
			act: &Action{
				Name:    "replace_file",
				Command: "<path>pkg/foo.go</path>\n<target>a</target>\n<replacement>b</replacement>",
			},
			wantTarget:  "pkg/foo.go",
			wantVerbose: "⚡ Editing: pkg/foo.go",
		},
		{
			act: &Action{
				Name:    "write_file",
				Command: "<path>src/bar.go</path>\n<content>package src</content>",
			},
			wantTarget:  "src/bar.go",
			wantVerbose: "⚡ Writing: src/bar.go",
		},
		{
			act: &Action{
				Name:    "read_outline",
				Command: "<path>main.go</path>",
			},
			wantTarget:  "main.go",
			wantVerbose: "⚡ Reading Outline: main.go",
		},
		{
			act: &Action{
				Name:    "read_window",
				Command: "<path>main.go</path>\n<start>1</start>\n<end>10</end>",
			},
			wantTarget:  "main.go",
			wantVerbose: "⚡ Reading Window: main.go",
		},
		{
			act: &Action{
				Name:    "run_test",
				Command: "go test ./pkg/...",
			},
			wantTarget:  "go test ./pkg/...",
			wantVerbose: "⚡ Verifying Tests: go test ./pkg/...",
		},
		{
			act: &Action{
				Name:    "get_environment",
				Command: "",
			},
			wantTarget:  "",
			wantVerbose: "⚡ Inspecting Environment",
		},
		{
			act: &Action{
				Name:    "find_files",
				Command: "<pattern>*.go</pattern>",
			},
			wantTarget:  "*.go",
			wantVerbose: "⚡ Finding Files: *.go",
		},
		{
			act: &Action{
				Name:    "search_code",
				Command: "<pattern>NewUser</pattern>",
			},
			wantTarget:  "NewUser",
			wantVerbose: "⚡ Searching Code: NewUser",
		},
		{
			act: &Action{
				Name:    "git_diff_summary",
				Command: "",
			},
			wantTarget:  "working state",
			wantVerbose: "⚡ Diff Summary: working state",
		},
		{
			act: &Action{
				Name:    "task_finish",
				Command: "Finished successfully.",
			},
			wantTarget:  "Finished successfully.",
			wantVerbose: "⚡ Finishing Task: Finished successfully.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.act.Name, func(t *testing.T) {
			if got := tt.act.TargetSummary(); got != tt.wantTarget {
				t.Errorf("TargetSummary() = %q, want %q", got, tt.wantTarget)
			}
			if got := tt.act.VerboseDescription(); got != tt.wantVerbose {
				t.Errorf("VerboseDescription() = %q, want %q", got, tt.wantVerbose)
			}
		})
	}
}

func TestSession_ModeSwitch_ContextRefresh(t *testing.T) {
	s := NewSession(nil, t.TempDir())
	if s.GetMode() != ModeGeneral {
		t.Fatalf("expected initial ModeGeneral, got %s", s.GetMode())
	}

	// Switch to coding mode
	s.SetMode(ModeCoding)
	if s.GetMode() != ModeCoding {
		t.Fatalf("expected ModeCoding, got %s", s.GetMode())
	}

	// History should now contain the mode switch event (lokol-kih.9)
	lastMsg := s.History[len(s.History)-1]
	if lastMsg.Role != "user" || !strings.Contains(lastMsg.Content, "Mode Switched: Active persona is now coding") {
		t.Fatalf("expected mode switch event in session history, got: %+v", lastMsg)
	}
	if !strings.Contains(lastMsg.Content, "Re-evaluate ongoing tasks") {
		t.Errorf("expected re-evaluation directive in mode switch event, got: %s", lastMsg.Content)
	}
}

func TestSession_ContextMetaQuery_Introspection(t *testing.T) {
	s := NewSession(nil, t.TempDir())

	// Meta query about context window
	s.AppendUserMessage("Can you show me your context?")
	lastMsg := s.History[len(s.History)-1]
	if !strings.Contains(lastMsg.Content, "Context Introspection:") {
		t.Fatalf("expected Context Introspection guidance in message, got: %s", lastMsg.Content)
	}
	if !strings.Contains(lastMsg.Content, "DO NOT search the filesystem") {
		t.Errorf("expected instruction not to search filesystem in message, got: %s", lastMsg.Content)
	}
	if !strings.Contains(lastMsg.Content, "general") {
		t.Errorf("expected mode in introspection guidance, got: %s", lastMsg.Content)
	}
}

func TestSession_PrimaryToolGrounding_GetEnvironment(t *testing.T) {
	s := NewSession(nil, t.TempDir())
	ctx := context.Background()

	act := &Action{
		Name:    "get_environment",
		Command: "",
	}

	out, err := s.ExecuteAction(ctx, act)
	if err != nil {
		t.Fatalf("ExecuteAction failed for get_environment: %v", err)
	}

	if !strings.Contains(out, "Observation: The execution environment details requested by the operator are provided above in full") {
		t.Errorf("expected primary tool grounding observation in output, got: %s", out)
	}
	if !strings.Contains(out, "Do NOT execute tangential file reads") {
		t.Errorf("expected anti-tangential read directive, got: %s", out)
	}
}

