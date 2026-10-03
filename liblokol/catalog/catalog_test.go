// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package catalog

import (
	"context"
	"fmt"
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

func TestIntentRouter_ConversationalFeedbackAndSummaryQueries(t *testing.T) {
	feedbackCases := []struct {
		prompt   string
		expected bool
	}{
		{"Nice. Glad your regulator flagged my last prompt", true},
		{"Nice job!", true},
		{"Thanks!", true},
		{"Thank you so much", true},
		{"Understood.", true},
		{"Got it, sounds good.", true},
		{"Hello there", true},
		{"Good morning", true},
		// Actionable prompts must NOT be classified as pure conversational feedback
		{"Can you fix the bug?", false},
		{"Please find all files", false},
		{"Run go test", false},
		{"What does the project do?", false},
		{"Explain this function", false},
	}

	for _, tc := range feedbackCases {
		res := IsConversationalFeedback(tc.prompt)
		if res != tc.expected {
			t.Errorf("IsConversationalFeedback(%q) = %v, want %v", tc.prompt, res, tc.expected)
		}
	}

	// Test RouteIntent suppresses specialized tool injection on conversational feedback
	matched := DefaultRouter.RouteIntent(context.Background(), "Nice. Glad your regulator flagged my last prompt")
	if len(matched) != 0 {
		t.Errorf("expected 0 tools for conversational feedback, got %d", len(matched))
	}

	summaryCases := []struct {
		prompt   string
		expected bool
	}{
		{"What can you tell me about the current project?", true},
		{"Can you summarize that the project does?", true},
		{"What does the project do?", true},
		{"Summarize the current project", true},
		{"Tell me about this codebase", true},
		{"Explain this repository", true},
		// Non-summary queries
		{"Find all files matching *.go", false},
		{"Run tests in ./liblokol", false},
		{"Nice work!", false},
	}

	for _, tc := range summaryCases {
		res := IsProjectSummaryQuery(tc.prompt)
		if res != tc.expected {
			t.Errorf("IsProjectSummaryQuery(%q) = %v, want %v", tc.prompt, res, tc.expected)
		}
	}

	metaCases := []struct {
		prompt   string
		expected bool
	}{
		{"Can you show me your context?", true},
		{"Show me your context", true},
		{"What is in your context?", true},
		{"What is your system prompt?", true},
		{"What are your instructions?", true},
		{"What mode are you in?", true},
		{"Show your mode", true},
		{"What is your persona?", true},
		// Non-meta queries
		{"Read README.md", false},
		{"What does the project do?", false},
		{"Find all files", false},
	}

	for _, tc := range metaCases {
		res := IsContextMetaQuery(tc.prompt)
		if res != tc.expected {
			t.Errorf("IsContextMetaQuery(%q) = %v, want %v", tc.prompt, res, tc.expected)
		}
	}

	// Meta queries must not inject specialized tools in RouteIntent
	metaMatched := DefaultRouter.RouteIntent(context.Background(), "Can you show me your context?")
	if len(metaMatched) != 0 {
		t.Errorf("expected 0 tools for meta query, got %d", len(metaMatched))
	}
}

func TestExtractTagContent(t *testing.T) {
	// Standard tag
	xml := "<target>hello world</target>"
	if got := ExtractTagContent(xml, "target"); got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}

	// Tag prefix collision (<target_dir> before <target>)
	payload := "<target_dir>/some/dir</target_dir>\n<target>exact_target</target>"
	if got := ExtractTagContent(payload, "target"); got != "exact_target" {
		t.Errorf("expected 'exact_target', got %q", got)
	}

	// Colliding tag only, target missing
	payloadMissing := "<target_dir>/some/dir</target_dir>"
	if got := ExtractTagContent(payloadMissing, "target"); got != "" {
		t.Errorf("expected empty string when tag missing, got %q", got)
	}

	// Tag with attributes
	payloadAttr := `<target lang="en" priority="1">attr_target</target>`
	if got := ExtractTagContent(payloadAttr, "target"); got != "attr_target" {
		t.Errorf("expected 'attr_target', got %q", got)
	}
}

func TestBuiltinReplaceFile_CRLFAndMixed(t *testing.T) {
	tmpDir := t.TempDir()
	tool, ok := DefaultRegistry.Get("replace_file")
	if !ok {
		t.Fatalf("replace_file tool not found")
	}

	// 1. Uniform CRLF preservation
	crlfPath := filepath.Join(tmpDir, "crlf.txt")
	_ = os.WriteFile(crlfPath, []byte("line1\r\nline2\r\nline3\r\n"), 0644)
	payloadCRLF := "<path>" + crlfPath + "</path>\n<target>line2</target>\n<replacement>new_line2</replacement>"
	if _, err := tool.Execute(context.Background(), payloadCRLF, tmpDir); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	dataCRLF, _ := os.ReadFile(crlfPath)
	expectedCRLF := "line1\r\nnew_line2\r\nline3\r\n"
	if string(dataCRLF) != expectedCRLF {
		t.Errorf("got %q, want %q", string(dataCRLF), expectedCRLF)
	}

	// 2. Mixed line endings: bare LFs must not be converted to CRLF
	mixedPath := filepath.Join(tmpDir, "mixed.txt")
	_ = os.WriteFile(mixedPath, []byte("crlf_line\r\nlf_line1\nlf_line2\n"), 0644)
	payloadMixed := "<path>" + mixedPath + "</path>\n<target>crlf_line</target>\n<replacement>updated_crlf</replacement>"
	if _, err := tool.Execute(context.Background(), payloadMixed, tmpDir); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	dataMixed, _ := os.ReadFile(mixedPath)
	expectedMixed := "updated_crlf\r\nlf_line1\nlf_line2\n"
	if string(dataMixed) != expectedMixed {
		t.Errorf("got %q, want %q", string(dataMixed), expectedMixed)
	}
}

func TestTruncateOutputAtLine(t *testing.T) {
	// 1. Output within budget
	short := "line 1\nline 2\nline 3\n"
	if res := TruncateOutputAtLine(short, 100); res != short {
		t.Errorf("expected short output unchanged, got: %q", res)
	}

	// 2. Output exceeding budget with newlines
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %02d: detailed log content", i)
	}
	longOutput := strings.Join(lines, "\n") + "\n"
	budget := 100
	truncated := TruncateOutputAtLine(longOutput, budget)

	if len(truncated) == 0 {
		t.Fatalf("expected non-empty truncated output")
	}
	if !strings.Contains(truncated, "[truncated") || !strings.Contains(truncated, "more lines]") {
		t.Errorf("expected truncation marker, got: %s", truncated)
	}

	// Must end on a clean line boundary before marker
	parts := strings.Split(truncated, "\n[truncated")
	body := parts[0]
	lastLine := body[strings.LastIndex(body, "\n")+1:]
	if !strings.HasPrefix(lastLine, "line ") {
		t.Errorf("expected clean line cut, got: %q", lastLine)
	}
}

func TestExecBash_LineTruncationAndStderr(t *testing.T) {
	ctx := context.Background()
	tool, ok := DefaultRegistry.Get("exec_bash")
	if !ok {
		t.Fatal("exec_bash not found in DefaultRegistry")
	}

	// 1. Test line-based truncation on large stdout
	largeCmd := "seq 1 2000"
	out, err := tool.Execute(ctx, largeCmd)
	if err != nil {
		t.Fatalf("unexpected error executing bash: %v", err)
	}
	if !strings.Contains(out, "[truncated") || !strings.Contains(out, "more lines]") {
		t.Errorf("expected line truncation marker in large output, got: %s", out)
	}

	// Verify no split number mid-line before truncation marker
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "[truncated") {
			break
		}
		// Each line before truncation should be a valid number
		if strings.TrimSpace(l) != "" {
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(l), "%d", &n); err != nil {
				t.Errorf("expected integer line, got broken line: %q", l)
			}
		}
	}

	// 2. Test stderr preservation when stdout exceeds budget
	largeWithStderrCmd := "seq 1 2000\necho \"STDERR_DIAGNOSTIC_MARKER\" >&2"
	outWithStderr, err := tool.Execute(ctx, largeWithStderrCmd)
	if err != nil {
		t.Fatalf("unexpected error executing bash with stderr: %v", err)
	}
	if !strings.Contains(outWithStderr, "STDERR_DIAGNOSTIC_MARKER") {
		t.Errorf("expected stderr diagnostic marker preserved when stdout is large, got: %s", outWithStderr)
	}
}
