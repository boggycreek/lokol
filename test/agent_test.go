package quik_test

import (
	"testing"

	"github.com/boggycreek/quik/pkg/agent"
)

func TestParseAction(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantAction  bool
		wantName    string
		wantCommand string
	}{
		{
			name:        "valid bash action",
			input:       "I will check the git status.\n<action name=\"exec_bash\">\ngit status\n</action>",
			wantAction:  true,
			wantName:    "exec_bash",
			wantCommand: "git status",
		},
		{
			name:        "valid finish action",
			input:       "Done!\n<action name=\"task_finish\">\nRefactoring complete.\n</action>",
			wantAction:  true,
			wantName:    "task_finish",
			wantCommand: "Refactoring complete.",
		},
		{
			name:        "no action present",
			input:       "Here is an explanation of the problem.",
			wantAction:  false,
			wantName:    "",
			wantCommand: "",
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
			} else {
				if act != nil {
					t.Fatalf("expected nil action, got %+v", act)
				}
			}
		})
	}
}
