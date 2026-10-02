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

func TestWindowReadCache_DeduplicationAndInvalidation(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "sample.txt")
	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cache := agent.NewWindowReadCache()

	// 1. Initial check: not redundant
	redundant, _, err := cache.CheckRedundant("sample.txt", 1, 3, tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if redundant {
		t.Fatalf("expected not redundant before read")
	}

	// 2. Record read
	cache.RecordRead("sample.txt", 1, 5, tempDir)

	// 3. Exact range or sub-range should now be redundant
	redundant, notice, err := cache.CheckRedundant("sample.txt", 1, 3, tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !redundant {
		t.Fatalf("expected sub-range [1, 3] to be redundant when [1, 5] was read")
	}
	if !strings.Contains(notice, "already been read in this session") {
		t.Errorf("expected notice message, got: %s", notice)
	}

	// 4. Modifying file on disk updates mod time/size -> invalidates cache
	time.Sleep(10 * time.Millisecond) // ensure mtime advances
	if err := os.WriteFile(filePath, []byte(content+"line 6\n"), 0644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	redundant, _, err = cache.CheckRedundant("sample.txt", 1, 3, tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if redundant {
		t.Fatalf("expected modified file on disk to NOT be redundant")
	}

	// 5. Invalidate method explicitly clears entry
	cache.RecordRead("sample.txt", 1, 6, tempDir)
	cache.Invalidate("sample.txt", tempDir)
	redundant, _, _ = cache.CheckRedundant("sample.txt", 1, 6, tempDir)
	if redundant {
		t.Fatalf("expected Invalidate to clear cache")
	}
}

func TestSession_ExecuteAction_DeduplicatesWindowReads(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "code.go")
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	session := agent.NewSessionWithMode(nil, tempDir, agent.ModeCoding)
	ctx := context.Background()

	act := &agent.Action{
		Name:    "read_window",
		Command: fmt.Sprintf("<path>%s</path><start_line>1</start_line><end_line>5</end_line>", filePath),
	}

	// Turn 1: Initial read succeeds with actual lines
	out1, err := session.ExecuteAction(ctx, act)
	if err != nil {
		t.Fatalf("turn 1 read failed: %v", err)
	}
	if !strings.Contains(out1, "package main") {
		t.Fatalf("expected real content in turn 1, got: %s", out1)
	}

	// Turn 2: Exact same read on unchanged file is suppressed
	out2, err := session.ExecuteAction(ctx, act)
	if err != nil {
		t.Fatalf("turn 2 read failed: %v", err)
	}
	if !strings.Contains(out2, "Notice: File window") || !strings.Contains(out2, "Content was not re-sent to conserve context tokens") {
		t.Fatalf("expected deduplication notice in turn 2, got: %s", out2)
	}
	if strings.Contains(out2, "package main") {
		t.Fatalf("expected full content omitted to conserve tokens in turn 2")
	}

	// Turn 3: Edit the file -> cache invalidated -> subsequent read returns fresh content
	editAct := &agent.Action{
		Name:    "replace_file",
		Command: fmt.Sprintf("<path>%s</path><target>println(\"hello\")</target><replacement>println(\"world\")</replacement>", filePath),
	}
	_, err = session.ExecuteAction(ctx, editAct)
	if err != nil {
		t.Fatalf("replace_file failed: %v", err)
	}

	out3, err := session.ExecuteAction(ctx, act)
	if err != nil {
		t.Fatalf("turn 3 read failed: %v", err)
	}
	if !strings.Contains(out3, "println(\"world\")") {
		t.Fatalf("expected updated content after edit, got: %s", out3)
	}
}
