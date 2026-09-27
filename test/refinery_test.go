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
