// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

//go:build integration

package lokol_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
)

// BenchmarkCase defines an evaluation test case with deterministic ground-truth verification.
type BenchmarkCase struct {
	Name     string
	Prompt   string
	MaxTurns int
	Setup    func(t *testing.T, dir string)
	Verify   func(t *testing.T, dir string) error
}

// TestEvalSuite_LiveEngine runs the graded benchmark suite against the live local LLM engine.
func TestEvalSuite_LiveEngine(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live evaluation benchmark in short mode")
	}

	if os.Getenv("LOKOL_LIVE_BENCHMARK") != "1" && os.Getenv("LOKOL_TEST_ENGINE_URL") == "" {
		t.Skip("skipping live engine evaluation suite; set LOKOL_LIVE_BENCHMARK=1 or LOKOL_TEST_ENGINE_URL to run live LLM benchmarks")
	}

	engineURL := os.Getenv("LOKOL_TEST_ENGINE_URL")
	if engineURL == "" {
		engineURL = "http://127.0.0.1:8080"
	}

	if !checkLiveEngine(engineURL) {
		t.Skipf("local inference engine not reachable at %s; skipping live eval benchmarks", engineURL)
	}

	client := agent.NewClient(engineURL)

	cases := []BenchmarkCase{
		{
			Name:     "Tier1_FileCreation_JSON",
			Prompt:   "Create a file named config.json containing {\"active\": true, \"version\": 1}, then finish.",
			MaxTurns: 5,
			Setup:    func(t *testing.T, dir string) {},
			Verify: func(t *testing.T, dir string) error {
				path := filepath.Join(dir, "config.json")
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("config.json was not created: %w", err)
				}
				var parsed map[string]interface{}
				if err := json.Unmarshal(data, &parsed); err != nil {
					return fmt.Errorf("config.json contains invalid JSON: %w (content: %q)", err, string(data))
				}
				if active, ok := parsed["active"].(bool); !ok || !active {
					return fmt.Errorf("expected active=true in config.json, got: %v", parsed["active"])
				}
				return nil
			},
		},
		{
			Name:     "Tier2_Directory_Inspection_Count",
			Prompt:   "Inspect the current directory, count how many .txt files exist, write ONLY the number into count.txt, and finish.",
			MaxTurns: 8,
			Setup: func(t *testing.T, dir string) {
				_ = os.WriteFile(filepath.Join(dir, "one.txt"), []byte("first"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "two.txt"), []byte("second"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "three.txt"), []byte("third"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Notes"), 0644)
			},
			Verify: func(t *testing.T, dir string) error {
				path := filepath.Join(dir, "count.txt")
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("count.txt was not created: %w", err)
				}
				content := strings.TrimSpace(string(data))
				if !strings.Contains(content, "3") {
					return fmt.Errorf("expected count '3' in count.txt, got %q", content)
				}
				return nil
			},
		},
		{
			Name:     "Tier3_BugFix_TestPass",
			Prompt:   "Run the tests with run_test, see what fails in calc.go, fix the bug, verify tests pass with run_test, and finish.",
			MaxTurns: 12,
			Setup: func(t *testing.T, dir string) {
				_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module calc\n\ngo 1.22\n"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package calc\n\nfunc Add(a, b int) int {\n\treturn a - b // bug\n}\n"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "calc_test.go"), []byte("package calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif got := Add(2, 3); got != 5 {\n\t\tt.Fatalf(\"Add(2, 3) = %d; want 5\", got)\n\t}\n}\n"), 0644)
			},
			Verify: func(t *testing.T, dir string) error {
				cmd := exec.Command("go", "test", "./...")
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("go test failed: %v\nOutput:\n%s", err, string(out))
				}
				return nil
			},
		},
		{
			Name:     "Tier4_BoundedWindow_Read",
			Prompt:   "Use read_window to inspect lines 35-50 of secrets.go, find the SecretToken value, write it into token.txt, and finish.",
			MaxTurns: 8,
			Setup: func(t *testing.T, dir string) {
				var sb strings.Builder
				sb.WriteString("package config\n\n")
				for i := 1; i <= 100; i++ {
					if i == 42 {
						sb.WriteString("const SecretToken = \"super-secret-xyz-789\"\n")
					} else {
						sb.WriteString(fmt.Sprintf("// line %d padding filler code\n", i))
					}
				}
				_ = os.WriteFile(filepath.Join(dir, "secrets.go"), []byte(sb.String()), 0644)
			},
			Verify: func(t *testing.T, dir string) error {
				data, err := os.ReadFile(filepath.Join(dir, "token.txt"))
				if err != nil {
					return fmt.Errorf("token.txt was not created: %w", err)
				}
				content := string(data)
				if !strings.Contains(content, "super-secret-xyz-789") {
					return fmt.Errorf("expected secret token in token.txt, got %q", content)
				}
				return nil
			},
		},
		{
			Name:     "Tier5_OutlineInspection",
			Prompt:   "Use read_outline on types.go to inspect the types, find the struct that manages buffers, write its exact type name to pool.txt, and finish.",
			MaxTurns: 8,
			Setup: func(t *testing.T, dir string) {
				typesContent := `package pool

// CacheManager handles general caching
type CacheManager struct {
	TTL int
}

// BufferPool manages reusable byte slices
type BufferPool struct {
	Capacity int
}

// QueueDispatcher distributes incoming jobs
type QueueDispatcher struct {
	Workers int
}
`
				_ = os.WriteFile(filepath.Join(dir, "types.go"), []byte(typesContent), 0644)
			},
			Verify: func(t *testing.T, dir string) error {
				data, err := os.ReadFile(filepath.Join(dir, "pool.txt"))
				if err != nil {
					return fmt.Errorf("pool.txt was not created: %w", err)
				}
				content := string(data)
				if !strings.Contains(content, "BufferPool") {
					return fmt.Errorf("expected 'BufferPool' in pool.txt, got %q", content)
				}
				return nil
			},
		},
		{
			Name:     "Tier6_GitAndBash",
			Prompt:   "Initialize a git repository using exec_bash, configure user.name 'TestBot' and user.email 'test@example.com', git add all files, commit with message 'init repo', and finish.",
			MaxTurns: 8,
			Setup: func(t *testing.T, dir string) {
				_ = os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0644)
			},
			Verify: func(t *testing.T, dir string) error {
				gitDir := filepath.Join(dir, ".git")
				if _, err := os.Stat(gitDir); err != nil {
					return fmt.Errorf(".git directory was not created: %w", err)
				}
				cmd := exec.Command("git", "log", "-1", "--pretty=%B")
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("git log failed: %v, output: %s", err, string(out))
				}
				if !strings.Contains(string(out), "init repo") {
					return fmt.Errorf("expected commit message 'init repo', got %q", string(out))
				}
				return nil
			},
		},
		{
			Name:     "Tier7_DirectorySummarization_Grounding",
			Prompt:   "Summarize the files in this directory and write a short overview into overview.md mentioning the files present, then finish.",
			MaxTurns: 8,
			Setup: func(t *testing.T, dir string) {
				_ = os.WriteFile(filepath.Join(dir, "server.go"), []byte("package main\n\nfunc RunServer() {}\n"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "client.go"), []byte("package main\n\nfunc RunClient() {}\n"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("port: 8080\n"), 0644)
			},
			Verify: func(t *testing.T, dir string) error {
				data, err := os.ReadFile(filepath.Join(dir, "overview.md"))
				if err != nil {
					return fmt.Errorf("overview.md was not created: %w", err)
				}
				content := string(data)
				if len(content) < 20 {
					return fmt.Errorf("overview.md is too short (%d bytes)", len(content))
				}
				// Verify it mentions at least one of the files from the directory
				hasFileMention := strings.Contains(content, "server.go") ||
					strings.Contains(content, "client.go") ||
					strings.Contains(content, "config.yaml")
				if !hasFileMention {
					return fmt.Errorf("overview.md did not mention any actual files present in directory: %q", content)
				}
				return nil
			},
		},
		{
			Name:     "Tier8_MultiTurn_FeatureAddition",
			Prompt:   "Inspect store.go using read_outline. Add a method Delete(key string) using replace_file that deletes key from s.data map. Then update store_test.go to test Delete, verify with run_test, and finish.",
			MaxTurns: 18,
			Setup: func(t *testing.T, dir string) {
				_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module store\n\ngo 1.22\n"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "store.go"), []byte("package store\n\ntype Store struct {\n\tdata map[string]string\n}\n\nfunc New() *Store {\n\treturn &Store{data: make(map[string]string)}\n}\n\nfunc (s *Store) Set(k, v string) {\n\ts.data[k] = v\n}\n\nfunc (s *Store) Get(k string) (string, bool) {\n\tv, ok := s.data[k]\n\treturn v, ok\n}\n"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "store_test.go"), []byte("package store\n\nimport \"testing\"\n\nfunc TestStore(t *testing.T) {\n\ts := New()\n\ts.Set(\"foo\", \"bar\")\n\tif v, ok := s.Get(\"foo\"); !ok || v != \"bar\" {\n\t\tt.Fatalf(\"want bar, got %s\", v)\n\t}\n}\n"), 0644)
			},
			Verify: func(t *testing.T, dir string) error {
				cmd := exec.Command("go", "test", "./...")
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("tests failed after feature addition: %v\nOutput: %s", err, string(out))
				}
				storeCode, err := os.ReadFile(filepath.Join(dir, "store.go"))
				if err != nil || !strings.Contains(string(storeCode), "Delete") {
					return fmt.Errorf("store.go does not contain Delete method")
				}
				testCode, err := os.ReadFile(filepath.Join(dir, "store_test.go"))
				if err != nil || !strings.Contains(string(testCode), "Delete") {
					return fmt.Errorf("store_test.go does not contain Delete test")
				}
				return nil
			},
		},
		{
			Name:     "Tier9_TargetedWindowEdit_LargeFile",
			Prompt:   "In server.go, find where DefaultPort is defined and change it from 8080 to 9090 using replace_file, then finish.",
			MaxTurns: 8,
			Setup: func(t *testing.T, dir string) {
				var sb strings.Builder
				sb.WriteString("package server\n\n")
				for i := 1; i <= 60; i++ {
					if i == 30 {
						sb.WriteString("const DefaultPort = 8080\n")
					} else {
						sb.WriteString(fmt.Sprintf("// padding line %d\n", i))
					}
				}
				_ = os.WriteFile(filepath.Join(dir, "server.go"), []byte(sb.String()), 0644)
			},
			Verify: func(t *testing.T, dir string) error {
				content, err := os.ReadFile(filepath.Join(dir, "server.go"))
				if err != nil {
					return err
				}
				s := string(content)
				if !strings.Contains(s, "9090") {
					return fmt.Errorf("server.go does not contain 9090")
				}
				if strings.Contains(s, "8080") {
					return fmt.Errorf("server.go still contains 8080")
				}
				return nil
			},
		},
		{
			Name:     "Tier10_ArchitectureExplanation_LayaScored",
			Prompt:   "Inspect user.go, auth.go, and db.go in this directory. Write an architectural overview into ARCHITECTURE.md using write_file explaining how AuthService uses DB to validate tokens, then finish.",
			MaxTurns: 12,
			Setup: func(t *testing.T, dir string) {
				_ = os.WriteFile(filepath.Join(dir, "user.go"), []byte("package auth\n\ntype User struct {\n\tID string\n\tEmail string\n}\n"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "db.go"), []byte("package auth\n\ntype DB struct{}\n\nfunc (d *DB) FindUserByToken(token string) (*User, error) {\n\treturn &User{ID: \"u-1\", Email: \"dev@example.com\"}, nil\n}\n"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "auth.go"), []byte("package auth\n\ntype AuthService struct {\n\tDatabase *DB\n}\n\nfunc (a *AuthService) ValidateToken(token string) (*User, error) {\n\treturn a.Database.FindUserByToken(token)\n}\n"), 0644)
			},
			Verify: func(t *testing.T, dir string) error {
				docBytes, err := os.ReadFile(filepath.Join(dir, "ARCHITECTURE.md"))
				if err != nil {
					return fmt.Errorf("ARCHITECTURE.md was not created: %w", err)
				}
				doc := string(docBytes)
				if len(doc) < 40 {
					return fmt.Errorf("ARCHITECTURE.md too short (%d bytes)", len(doc))
				}

				repoRoot := findRepoRoot(t)
				if checkLayaAvailable(repoRoot) {
					state := fmt.Sprintf("Codebase context:\nuser.go defines User struct with ID and Email.\ndb.go provides Database with FindUserByToken(token).\nauth.go provides AuthService with ValidateToken calling Database.FindUserByToken.\n\nAgent generated ARCHITECTURE.md:\n%s", doc)
					instructions := "Does ARCHITECTURE.md accurately explain how AuthService uses the database to validate tokens based on the code?"
					res, err := runLayaJudge(t, repoRoot, state, instructions, 0.5)
					if err != nil {
						t.Logf("Laya evaluation error: %v (falling back to lexical checks)", err)
					} else {
						t.Logf("Laya evaluation result: passed=%v prob=%.4f quality=%.2f", res.Passed, res.Probability, res.QualityLevel)
						if !res.Passed {
							return fmt.Errorf("Laya judge scored explanation below threshold (prob=%.4f < %.2f)", res.Probability, res.Threshold)
						}
					}
				} else {
					t.Logf("Laya not available, skipping decision model evaluation")
				}

				// Deterministic ground-truth keyword checks
				docLower := strings.ToLower(doc)
				if !strings.Contains(docLower, "auth") || !strings.Contains(docLower, "token") {
					return fmt.Errorf("ARCHITECTURE.md missing essential terms (auth/token)")
				}
				return nil
			},
		},
	}

	type EvalCaseReport struct {
		Name        string  `json:"name"`
		Passed      bool    `json:"passed"`
		DurationSec float64 `json:"duration_sec"`
		Error       string  `json:"error,omitempty"`
	}

	type EvalSuiteReport struct {
		Timestamp   string           `json:"timestamp"`
		Total       int              `json:"total"`
		Passed      int              `json:"passed"`
		Failed      int              `json:"failed"`
		DurationSec float64          `json:"duration_sec"`
		Cases       []EvalCaseReport `json:"cases"`
	}

	var reports []EvalCaseReport
	suiteStart := time.Now()

	for _, bc := range cases {
		t.Run(bc.Name, func(t *testing.T) {
			caseStart := time.Now()
			caseReport := EvalCaseReport{Name: bc.Name}

			dir := t.TempDir()
			bc.Setup(t, dir)

			runner := &agent.Runner{
				Client:   client,
				MaxTurns: bc.MaxTurns,
				YOLO:     true,
				WorkDir:  dir,
				OnOutput: func(role, content string) {
					t.Logf("[%s] %s", role, content)
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			t.Logf("Running eval case %q in %s...", bc.Name, dir)
			summary, err := runner.Run(ctx, bc.Prompt)
			if err != nil {
				caseReport.Passed = false
				caseReport.Error = fmt.Sprintf("runner failed: %v", err)
				caseReport.DurationSec = time.Since(caseStart).Seconds()
				reports = append(reports, caseReport)
				t.Fatalf("runner failed: %v (last summary: %s)", err, summary)
			}

			if err := bc.Verify(t, dir); err != nil {
				caseReport.Passed = false
				caseReport.Error = fmt.Sprintf("verify failed: %v", err)
				caseReport.DurationSec = time.Since(caseStart).Seconds()
				reports = append(reports, caseReport)
				t.Fatalf("verification failed for %q: %v", bc.Name, err)
			}

			caseReport.Passed = true
			caseReport.DurationSec = time.Since(caseStart).Seconds()
			reports = append(reports, caseReport)
			t.Logf("PASS: %q succeeded in %s (%.2fs)", bc.Name, dir, caseReport.DurationSec)
		})
	}

	// Persist benchmark run to subproject's gitignored data/ directory
	dataDir := filepath.Join("..", "data")
	_ = os.MkdirAll(dataDir, 0755)
	passedCount := 0
	for _, r := range reports {
		if r.Passed {
			passedCount++
		}
	}
	suiteReport := EvalSuiteReport{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Total:       len(reports),
		Passed:      passedCount,
		Failed:      len(reports) - passedCount,
		DurationSec: time.Since(suiteStart).Seconds(),
		Cases:       reports,
	}
	if data, err := json.MarshalIndent(suiteReport, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dataDir, "eval_results.json"), data, 0644)
		t.Logf("Wrote evaluation report to %s", filepath.Join(dataDir, "eval_results.json"))
	}
}
