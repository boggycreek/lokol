// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boggycreek/lokol/pkg/agent"
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
	initialContent := "Hello world\nFoo bar baz\nEnding line"
	if err := os.WriteFile(filePath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	payload := fmt.Sprintf("<path>%s</path>\n<target>Foo bar baz</target>\n<replacement>Quik replaced this</replacement>", filePath)
	out, err := agent.ExecuteReplaceFile(context.Background(), payload)
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
}

func TestExecuteWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/nested/subdir/hello.go"
	payload := fmt.Sprintf("<path>%s</path>\n<content>package main\n\nfunc main() {}\n</content>", filePath)

	out, err := agent.ExecuteWriteFile(context.Background(), payload)
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

