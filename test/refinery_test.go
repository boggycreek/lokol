// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lokol_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boggycreek/lokol/pkg/tools/refinery"
)

func TestReadWindow(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "sample.txt")
	lines := []string{
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
	}
	if err := os.WriteFile(f, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	out, err := refinery.ReadWindow(f, 2, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "line 2") || !strings.Contains(out, "line 4") {
		t.Errorf("expected lines 2-4, got: %s", out)
	}
	if strings.Contains(out, "line 1") || strings.Contains(out, "line 5") {
		t.Errorf("did not expect line 1 or line 5, got: %s", out)
	}
}

func TestReadOutline(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "service.go")
	content := `package auth

type User struct {
	ID string
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Login(user, pass string) (string, error) {
	return "token", nil
}
`
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	outline, err := refinery.ReadOutline(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(outline, "type User struct") {
		t.Errorf("expected User struct in outline, got: %s", outline)
	}
	if !strings.Contains(outline, "func NewService()") {
		t.Errorf("expected NewService func in outline, got: %s", outline)
	}
	if !strings.Contains(outline, "func (s *Service) Login") {
		t.Errorf("expected Login method in outline, got: %s", outline)
	}
	// Verify implementation bodies are omitted
	if strings.Contains(outline, "return \"token\"") {
		t.Errorf("outline should omit function bodies, but found return token")
	}
}

func TestRunTestVerifier(t *testing.T) {
	ctx := context.Background()

	// 1. Success case
	res, err := refinery.RunTestVerifier(ctx, "echo 'PASS: TestFoo'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Errorf("expected test to pass")
	}
	if !strings.Contains(res.Summary, "✓") {
		t.Errorf("expected checkmark summary, got: %s", res.Summary)
	}

	// 2. Failure case with Go test style output (exit code 1)
	failCmd := `bash -c 'printf -- "--- FAIL: TestAuth (0.01s)\n    auth_test.go:42: expected 200, got 401\nFAIL\n" >&2; exit 1'`
	failRes, _ := refinery.RunTestVerifier(ctx, failCmd)
	if failRes.Passed {
		t.Errorf("expected test to fail")
	}
	if !strings.Contains(failRes.ErrorOutput, "auth_test.go:42") {
		t.Errorf("expected extracted assertion, got: %s", failRes.ErrorOutput)
	}
}

func TestGetEnvironment(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Basic environment detection in empty temp dir
	env, err := refinery.GetEnvironment(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error getting environment: %v", err)
	}

	if env.WorkingDirectory != tmpDir {
		t.Errorf("expected WorkingDirectory %q, got %q", tmpDir, env.WorkingDirectory)
	}
	if env.OS == "" || env.Arch == "" {
		t.Errorf("expected non-empty OS/Arch, got %s/%s", env.OS, env.Arch)
	}
	if env.Shell == "" {
		t.Errorf("expected non-empty Shell")
	}
	if env.Git.IsRepo {
		t.Errorf("expected Git.IsRepo to be false in temp dir, got true")
	}

	// 2. Format JSON validation
	jsonStr, err := env.FormatJSON()
	if err != nil {
		t.Fatalf("FormatJSON failed: %v", err)
	}
	if !strings.Contains(jsonStr, "\"working_directory\"") || !strings.Contains(jsonStr, "\"os\"") {
		t.Errorf("expected JSON to contain working_directory and os keys, got:\n%s", jsonStr)
	}

	// 3. Test git detection
	gitCmd := exec.Command("git", "init", tmpDir)
	if err := gitCmd.Run(); err == nil {
		envGit, err := refinery.GetEnvironment(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error in git dir: %v", err)
		}
		if !envGit.Git.IsRepo {
			t.Errorf("expected Git.IsRepo to be true after git init")
		}
	}
}

func TestFindFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file hierarchy
	// tmpDir/
	//   main.go
	//   calc.go
	//   calc_test.go
	//   README.md
	//   pkg/
	//     helper.go
	//     helper_test.go
	//   .gitignore
	//   ignored_file.log
	//   ignored_dir/
	//     secret.txt
	//   .git/
	//     config
	//   node_modules/
	//     package.json
	files := map[string]string{
		"main.go":                  "package main",
		"calc.go":                  "package main",
		"calc_test.go":             "package main",
		"README.md":                "# Test Repo",
		"pkg/helper.go":            "package pkg",
		"pkg/helper_test.go":       "package pkg",
		"ignored_file.log":         "log data",
		"ignored_dir/secret.txt":   "secret",
		".git/config":              "[core]",
		"node_modules/pkg.json":    "{}",
		".gitignore":               "*.log\nignored_dir/\n",
	}

	for relPath, content := range files {
		fullPath := filepath.Join(tmpDir, relPath)
		_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", relPath, err)
		}
	}

	t.Run("Glob matching *.go", func(t *testing.T) {
		out, err := refinery.FindFiles("*.go", tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "main.go") || !strings.Contains(out, "calc.go") || !strings.Contains(out, "pkg/helper.go") {
			t.Errorf("expected all Go files in output, got:\n%s", out)
		}
		if strings.Contains(out, "README.md") {
			t.Errorf("README.md should not match *.go: %s", out)
		}
		if !strings.Contains(out, "(5 files)") {
			t.Errorf("expected count (5 files), got: %s", out)
		}
	})

	t.Run("Substring matching without wildcards", func(t *testing.T) {
		out, err := refinery.FindFiles("helper", tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "pkg/helper.go") || !strings.Contains(out, "pkg/helper_test.go") {
			t.Errorf("expected helper files, got:\n%s", out)
		}
		if strings.Contains(out, "main.go") {
			t.Errorf("main.go should not match 'helper': %s", out)
		}
	})

	t.Run("Excludes default noisy directories (.git, node_modules)", func(t *testing.T) {
		out, err := refinery.FindFiles("*", tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, ".git/") || strings.Contains(out, "node_modules") {
			t.Errorf("expected .git/ and node_modules to be excluded, got:\n%s", out)
		}
	})

	t.Run("Respects .gitignore rules", func(t *testing.T) {
		out, err := refinery.FindFiles("*", tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "ignored_file.log") {
			t.Errorf("expected *.log to be ignored by .gitignore, got:\n%s", out)
		}
		if strings.Contains(out, "ignored_dir") || strings.Contains(out, "secret.txt") {
			t.Errorf("expected ignored_dir/ to be ignored by .gitignore, got:\n%s", out)
		}
	})

	t.Run("Truncation and match bounding", func(t *testing.T) {
		out, err := refinery.FindFiles("*.go", tmpDir, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		// 3 file lines + empty line + 1 summary line = 5 lines
		if !strings.Contains(out, "[Showing 3 of 5 matches. Refine pattern to narrow search.]") {
			t.Errorf("expected truncation note, got:\n%s", out)
		}
		matchedFiles := 0
		for _, l := range lines {
			if strings.HasSuffix(l, ".go") {
				matchedFiles++
			}
		}
		if matchedFiles != 3 {
			t.Errorf("expected exactly 3 returned files, got %d", matchedFiles)
		}
	})

	t.Run("Zero matches", func(t *testing.T) {
		out, err := refinery.FindFiles("*.rs", tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "No files matching") {
			t.Errorf("expected 'No files matching', got: %s", out)
		}
	})
}

func TestParseFindFilesPayload(t *testing.T) {
	// 1. Structured XML tags
	xml := "<pattern>*.go</pattern>\n<path>src</path>\n<max_results>25</max_results>"
	in, err := refinery.ParseFindFilesPayload(xml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Pattern != "*.go" || in.Path != "src" || in.MaxResults != 25 {
		t.Errorf("unexpected parsed input: %+v", in)
	}

	// 2. Plain text fallback
	plain := "pkg/*.go"
	inPlain, err := refinery.ParseFindFilesPayload(plain)
	if err != nil {
		t.Fatalf("unexpected error on plain: %v", err)
	}
	if inPlain.Pattern != "pkg/*.go" || inPlain.MaxResults != 50 {
		t.Errorf("unexpected parsed plain input: %+v", inPlain)
	}
}

func TestSearchCode(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup directory structure
	// tmpDir/
	//   main.go
	//   calc.go
	//   sub/
	//     helper.go
	//   .gitignore
	//   ignored.go
	//   node_modules/
	//     vendor.go

	mainContent := `package main

import "fmt"

func main() {
	fmt.Println("Starting application...")
	Calculate(10, 20)
}
`
	calcContent := `package main

func Calculate(a, b int) int {
	// Simple addition
	return a + b
}

func calculateDouble(x int) int {
	return x * 2
}
`
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	helperContent := `package sub

func HelperFunc() string {
	return "help"
}
`
	nodeDir := filepath.Join(tmpDir, "node_modules")
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		t.Fatal(err)
	}
	vendorContent := `// Calculate inside node_modules`

	gitIgnoreContent := "ignored.go\n*.log\n"

	ignoredContent := `func CalculateInIgnored() {}`

	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "calc.go"), []byte(calcContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "helper.go"), []byte(helperContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitIgnoreContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "ignored.go"), []byte(ignoredContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeDir, "vendor.go"), []byte(vendorContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("Literal case-insensitive search", func(t *testing.T) {
		out, err := refinery.SearchCode(tmpDir, refinery.SearchCodeInput{
			Pattern: "calculate",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "main.go:7: Calculate(10, 20)") {
			t.Errorf("expected match in main.go, got:\n%s", out)
		}
		if !strings.Contains(out, "calc.go:3: func Calculate(a, b int) int {") {
			t.Errorf("expected match in calc.go, got:\n%s", out)
		}
		if !strings.Contains(out, "calc.go:8: func calculateDouble(x int) int {") {
			t.Errorf("expected match in calc.go line 8, got:\n%s", out)
		}
	})

	t.Run("Regex search", func(t *testing.T) {
		out, err := refinery.SearchCode(tmpDir, refinery.SearchCodeInput{
			Pattern: `func [A-Z][a-zA-Z0-9]+\(`,
			IsRegex: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "calc.go:3: func Calculate(a, b int) int {") {
			t.Errorf("expected Calculate match, got:\n%s", out)
		}
		if !strings.Contains(out, "sub/helper.go:3: func HelperFunc() string {") {
			t.Errorf("expected HelperFunc match, got:\n%s", out)
		}
		if strings.Contains(out, "calculateDouble") {
			t.Errorf("did not expect lowercase calculateDouble to match regex, got:\n%s", out)
		}
	})

	t.Run("Path scoped search", func(t *testing.T) {
		out, err := refinery.SearchCode(tmpDir, refinery.SearchCodeInput{
			Pattern: "func",
			Path:    "sub",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "sub/helper.go") {
			t.Errorf("expected helper.go match, got:\n%s", out)
		}
		if strings.Contains(out, "calc.go") || strings.Contains(out, "main.go") {
			t.Errorf("expected only sub directory files, got:\n%s", out)
		}
	})

	t.Run("Excludes default noisy dirs and gitignore", func(t *testing.T) {
		out, err := refinery.SearchCode(tmpDir, refinery.SearchCodeInput{
			Pattern: "Calculate",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "node_modules") {
			t.Errorf("expected node_modules to be excluded, got:\n%s", out)
		}
		if strings.Contains(out, "ignored.go") {
			t.Errorf("expected ignored.go to be excluded by .gitignore, got:\n%s", out)
		}
	})

	t.Run("Bounding max results", func(t *testing.T) {
		out, err := refinery.SearchCode(tmpDir, refinery.SearchCodeInput{
			Pattern:    "func",
			MaxResults: 2,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		// 2 match lines + 1 truncation note line = 3 lines
		if len(lines) != 3 {
			t.Errorf("expected 3 lines (2 matches + note), got %d:\n%s", len(lines), out)
		}
		if !strings.Contains(out, "[... truncated at 2 matches") {
			t.Errorf("expected truncation note, got:\n%s", out)
		}
	})

	t.Run("Zero matches", func(t *testing.T) {
		out, err := refinery.SearchCode(tmpDir, refinery.SearchCodeInput{
			Pattern: "nonexistent_symbol_xyz",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "No matches found for \"nonexistent_symbol_xyz\"") {
			t.Errorf("expected 'No matches found', got: %s", out)
		}
	})
}

func TestParseSearchCodePayload(t *testing.T) {
	// 1. Structured XML tags
	xml := "<pattern>func Execute</pattern>\n<path>pkg/agent</path>\n<regex>true</regex>\n<max_results>15</max_results>"
	in, err := refinery.ParseSearchCodePayload(xml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Pattern != "func Execute" || in.Path != "pkg/agent" || !in.IsRegex || in.MaxResults != 15 {
		t.Errorf("unexpected parsed input: %+v", in)
	}

	// 2. Plain text fallback
	plain := "myFunction"
	inPlain, err := refinery.ParseSearchCodePayload(plain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inPlain.Pattern != "myFunction" || inPlain.IsRegex || inPlain.MaxResults != 30 {
		t.Errorf("unexpected parsed plain input: %+v", inPlain)
	}
}

func TestGitDiffSummary(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// 1. Error on non-git directory
	_, err := refinery.GitDiffSummary(ctx, tmpDir, refinery.GitDiffSummaryInput{})
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("expected not a git repository error, got: %v", err)
	}

	// 2. Init git repo
	cmds := [][]string{
		{"git", "init", tmpDir},
		{"git", "-C", tmpDir, "config", "user.email", "tester@example.com"},
		{"git", "-C", tmpDir, "config", "user.name", "Tester"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed setup command %v: %v\nOutput: %s", c, err, string(out))
		}
	}

	// 3. Initial commit
	initialFile := filepath.Join(tmpDir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("line 1\nline 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", tmpDir, "add", "initial.txt").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit").Run()

	// 4. Test clean working state
	cleanOut, err := refinery.GitDiffSummary(ctx, tmpDir, refinery.GitDiffSummaryInput{})
	if err != nil {
		t.Fatalf("unexpected error on clean repo: %v", err)
	}
	if !strings.Contains(cleanOut, "Working directory clean") {
		t.Errorf("expected clean working directory, got:\n%s", cleanOut)
	}

	// 5. Create staged, unstaged, and untracked changes
	stagedFile := filepath.Join(tmpDir, "staged.txt")
	if err := os.WriteFile(stagedFile, []byte("staged content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", tmpDir, "add", "staged.txt").Run()

	// Modify initial.txt unstaged
	if err := os.WriteFile(initialFile, []byte("line 1\nline 2 modified\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Untracked file
	untrackedFile := filepath.Join(tmpDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 6. Test GitDiffSummary with changes
	diffOut, err := refinery.GitDiffSummary(ctx, tmpDir, refinery.GitDiffSummaryInput{})
	if err != nil {
		t.Fatalf("unexpected error on dirty repo: %v", err)
	}

	if !strings.Contains(diffOut, "Staged changes") || !strings.Contains(diffOut, "staged.txt") {
		t.Errorf("expected staged changes containing staged.txt, got:\n%s", diffOut)
	}
	if !strings.Contains(diffOut, "Unstaged changes") || !strings.Contains(diffOut, "initial.txt") {
		t.Errorf("expected unstaged changes containing initial.txt, got:\n%s", diffOut)
	}
	if !strings.Contains(diffOut, "Untracked files") || !strings.Contains(diffOut, "untracked.txt") {
		t.Errorf("expected untracked files containing untracked.txt, got:\n%s", diffOut)
	}
	if !strings.Contains(diffOut, "Diffstat:") {
		t.Errorf("expected Diffstat section, got:\n%s", diffOut)
	}
	if !strings.Contains(diffOut, "Unified Diff (bounded):") {
		t.Errorf("expected Unified Diff section, got:\n%s", diffOut)
	}
}

func TestParseGitDiffSummaryPayload(t *testing.T) {
	// 1. Structured XML tags
	xml := "<path>pkg/agent</path>\n<staged>true</staged>\n<max_lines>50</max_lines>"
	in, err := refinery.ParseGitDiffSummaryPayload(xml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Path != "pkg/agent" || !in.Staged || in.MaxLines != 50 {
		t.Errorf("unexpected parsed input: %+v", in)
	}

	// 2. Plain text fallback
	plain := "pkg"
	inPlain, err := refinery.ParseGitDiffSummaryPayload(plain)
	if err != nil {
		t.Fatalf("unexpected error on plain: %v", err)
	}
	if inPlain.Path != "pkg" || inPlain.Staged || inPlain.MaxLines != 100 {
		t.Errorf("unexpected parsed plain input: %+v", inPlain)
	}
}



