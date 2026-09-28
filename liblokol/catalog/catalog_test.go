// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistry_DefaultTools(t *testing.T) {
	reg := DefaultRegistry

	expectedTools := []string{
		"find_files", "read_window", "replace_file", "task_finish", "tool_help",
		"exec_bash", "run_test", "search_code", "git_diff_summary", "read_outline",
		"write_file", "get_environment",
	}

	for _, name := range expectedTools {
		tool, exists := reg.Get(name)
		if !exists {
			t.Fatalf("expected tool %q to be registered in DefaultRegistry", name)
		}
		if tool.Name() != name {
			t.Errorf("tool.Name() = %q, want %q", tool.Name(), name)
		}
		if tool.Summary() == "" {
			t.Errorf("tool %q has empty summary", name)
		}
		if tool.SchemaXML() == "" {
			t.Errorf("tool %q has empty SchemaXML", name)
		}
	}
}

func TestRegistry_Partitioning(t *testing.T) {
	reg := DefaultRegistry

	foundational := reg.ListFoundational()
	if len(foundational) != 5 {
		t.Errorf("expected exactly 5 foundational tools, got %d", len(foundational))
	}

	specialized := reg.ListSpecialized()
	if len(specialized) != 7 {
		t.Errorf("expected exactly 7 specialized tools, got %d", len(specialized))
	}
}

func TestRegistry_Help(t *testing.T) {
	reg := DefaultRegistry

	// 1. Help for specific tool
	help, err := reg.Help("git_diff_summary")
	if err != nil {
		t.Fatalf("unexpected error getting help for git_diff_summary: %v", err)
	}
	if !strings.Contains(help, "<tool_documentation name=\"git_diff_summary\">") {
		t.Errorf("expected tool_documentation tag in help, got: %s", help)
	}
	if !strings.Contains(help, "<action name=\"git_diff_summary\">") {
		t.Errorf("expected action format in help, got: %s", help)
	}

	// 2. Directory listing when empty or 'all'
	dirHelp, err := reg.Help("all")
	if err != nil {
		t.Fatalf("unexpected error getting directory help: %v", err)
	}
	if !strings.Contains(dirHelp, "<tool_directory>") {
		t.Errorf("expected <tool_directory> tag, got: %s", dirHelp)
	}
	if !strings.Contains(dirHelp, "exec_bash") || !strings.Contains(dirHelp, "run_test") {
		t.Errorf("expected tool names in directory, got: %s", dirHelp)
	}

	// 3. Unknown tool error steering
	_, err = reg.Help("fake_tool_xyz")
	if err == nil {
		t.Fatalf("expected error for unknown tool, got nil")
	}
	if !strings.Contains(err.Error(), "Available specialized tools:") {
		t.Errorf("expected steering with valid tools in error, got: %v", err)
	}
}

func TestRegistry_FormatBasePrompt(t *testing.T) {
	reg := DefaultRegistry

	prompt := reg.FormatBasePrompt("general")
	if !strings.Contains(prompt, "Tool Execution Protocol:") {
		t.Errorf("expected protocol header in base prompt")
	}
	if !strings.Contains(prompt, "Foundational Actions:") {
		t.Errorf("expected foundational actions in base prompt")
	}
	if !strings.Contains(prompt, "Specialized Tools (inspect usage via <action name=\"tool_help\">") {
		t.Errorf("expected specialized tools index in base prompt")
	}

	// Verify size is lean (~250-400 words)
	words := len(strings.Fields(prompt))
	if words > 450 {
		t.Errorf("base prompt too long: %d words (expected lean prompt <= 450 words)", words)
	}
}

func TestIntentRouter_Routing(t *testing.T) {
	router := DefaultRouter
	ctx := context.Background()

	tests := []struct {
		prompt      string
		expectTools []string
	}{
		{
			prompt:      "List the Go packages in this repository.",
			expectTools: []string{"exec_bash"},
		},
		{
			prompt:      "Compare my working branch against main with git diff.",
			expectTools: []string{"git_diff_summary"},
		},
		{
			prompt:      "Run the unit tests to verify the change.",
			expectTools: []string{"run_test"},
		},
		{
			prompt:      "Search code for func NewSession in the repo.",
			expectTools: []string{"search_code"},
		},
		{
			prompt:      "Inspect what is in the current directory.",
			expectTools: []string{}, // Foundational find_files handles this; no specialized tool needed
		},
		{
			prompt:      "Check your environment MCP tools.",
			expectTools: []string{"get_environment"},
		},
	}

	for _, tc := range tests {
		matched := router.RouteIntent(ctx, tc.prompt)
		matchedNames := make([]string, 0, len(matched))
		for _, m := range matched {
			matchedNames = append(matchedNames, m.Name())
		}

		for _, expected := range tc.expectTools {
			found := false
			for _, m := range matchedNames {
				if m == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("prompt %q: expected tool %q in matched %v", tc.prompt, expected, matchedNames)
			}
		}

		if len(tc.expectTools) == 0 && len(matched) > 0 {
			t.Errorf("prompt %q: expected 0 specialized tools, got %v", tc.prompt, matchedNames)
		}
	}
}

func TestToolExecution_ReplaceFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.txt")
	if err := os.WriteFile(filePath, []byte("hello world\nline two\nline three\n"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := DefaultRegistry
	tool, ok := reg.Get("replace_file")
	if !ok {
		t.Fatal("replace_file tool not found in registry")
	}

	payload := `<path>sample.txt</path>
<target>
line two
</target>
<replacement>
line replaced
</replacement>`

	res, err := tool.Execute(context.Background(), payload, tmpDir)
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if !strings.Contains(res, "Successfully replaced") {
		t.Errorf("unexpected execute result: %s", res)
	}

	data, _ := os.ReadFile(filePath)
	if !strings.Contains(string(data), "line replaced") {
		t.Errorf("file content not replaced: %s", string(data))
	}
}
