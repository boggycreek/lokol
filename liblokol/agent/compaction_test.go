// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"strings"
	"testing"
)

func TestClassifySlotPressure(t *testing.T) {
	tests := []struct {
		tokens   int
		nCtx     int
		expected PressureLevel
	}{
		{tokens: 1000, nCtx: 8192, expected: PressureNominal},   // ~12.2%
		{tokens: 4500, nCtx: 8192, expected: PressureNominal},   // ~54.9%
		{tokens: 5000, nCtx: 8192, expected: PressureWarning},   // ~61.0%
		{tokens: 6000, nCtx: 8192, expected: PressureWarning},   // ~73.2%
		{tokens: 6500, nCtx: 8192, expected: PressureCritical},  // ~79.3%
		{tokens: 8000, nCtx: 8192, expected: PressureCritical},  // ~97.6%
		{tokens: 0, nCtx: 0, expected: PressureNominal},         // edge case
	}

	for _, tc := range tests {
		got := ClassifySlotPressure(tc.tokens, tc.nCtx)
		if got != tc.expected {
			t.Errorf("ClassifySlotPressure(%d, %d) = %v; want %v", tc.tokens, tc.nCtx, got, tc.expected)
		}
	}
}

func TestEstimateTokenCount(t *testing.T) {
	if got := EstimateTokenCount(""); got != 0 {
		t.Fatalf("EstimateTokenCount(\"\") = %d; want 0", got)
	}
	if got := EstimateTokenCount("a"); got != 1 {
		t.Fatalf("EstimateTokenCount(\"a\") = %d; want 1", got)
	}
	text := strings.Repeat("abcd", 100) // 400 chars -> ~100 tokens
	if got := EstimateTokenCount(text); got != 100 {
		t.Fatalf("EstimateTokenCount(400 chars) = %d; want 100", got)
	}
}

func TestPruneStaleToolOutputs(t *testing.T) {
	largeWindowResult := "<action_result>\n=== File: /path/to/main.go (lines 1-100) ===\n" + strings.Repeat("fmt.Println(\"test line data\");\n", 50) + "</action_result>"
	recentWindowResult := "<action_result>\n=== File: /path/to/recent.go (lines 1-20) ===\n" + strings.Repeat("x := 10;\n", 20) + "</action_result>"

	history := []Message{
		{Role: "system", Content: "System Prompt Grounding"},
		{Role: "user", Content: "Please inspect main.go and add logging."},
		{Role: "assistant", Content: "Inspecting lines 1-100.\n<action name=\"read_window\"><path>main.go</path><start>1</start><end>100</end></action>"},
		{Role: "user", Content: largeWindowResult},
		{Role: "assistant", Content: "Replaced line.\n<action name=\"replace_file\"><path>main.go</path></action>"},
		{Role: "user", Content: "<action_result>\nSuccessfully replaced block\n</action_result>"},
		// Recent turn (preserveRecent = 1 should preserve this turn)
		{Role: "assistant", Content: "Inspecting recent.go.\n<action name=\"read_window\"><path>recent.go</path><start>1</start><end>20</end></action>"},
		{Role: "user", Content: recentWindowResult},
	}

	pruned, reclaimed := PruneStaleToolOutputs(history, 1)

	if len(pruned) != len(history) {
		t.Fatalf("expected pruned length %d; got %d", len(history), len(pruned))
	}
	if reclaimed <= 0 {
		t.Fatalf("expected reclaimed chars > 0; got %d", reclaimed)
	}

	// Verify invariant 1: System prompt untouched
	if pruned[0].Content != history[0].Content {
		t.Errorf("system prompt was altered")
	}

	// Verify invariant 2: Older tool result pruned
	if !strings.Contains(pruned[3].Content, "[Output pruned:") {
		t.Errorf("expected pruned[3] to be pruned; got %q", pruned[3].Content)
	}
	if strings.Contains(pruned[3].Content, "fmt.Println") {
		t.Errorf("expected pruned[3] verbose output to be removed")
	}

	// Verify invariant 3: Recent turn (index 7) preserved verbatim
	if pruned[7].Content != history[7].Content {
		t.Errorf("recent turn result (pruned[7]) was altered")
	}
}

func TestCompactHistory(t *testing.T) {
	history := []Message{
		{Role: "system", Content: "System Prompt Grounding"},
		{Role: "user", Content: "Build feature X."},
		// Turn 1
		{Role: "assistant", Content: "Reading code.\n<action name=\"read_window\">...</action>"},
		{Role: "user", Content: "<action_result>code</action_result>"},
		// Turn 2
		{Role: "assistant", Content: "Editing code.\n<action name=\"replace_file\">...</action>"},
		{Role: "user", Content: "<action_result>success</action_result>"},
		// Turn 3 (Recent, preserveRecent = 1)
		{Role: "assistant", Content: "Running tests.\n<action name=\"run_test\">go test ./...</action>"},
		{Role: "user", Content: "<action_result>PASS</action_result>"},
	}

	summary := "1. Inspected codebase.\n2. Replaced bug in feature X.\n3. Verified compilation."
	compacted := CompactHistory(history, summary, 1)

	// Compacted should have:
	// [0] System Prompt
	// [1] Initial User Prompt
	// [2] Summary Ledger (<conversation_summary>)
	// [3] Assistant Acknowledgment
	// [4] Recent Assistant Turn (Turn 3)
	// [5] Recent User Result (Turn 3)
	if len(compacted) != 6 {
		t.Fatalf("expected compacted length 6; got %d", len(compacted))
	}

	// Invariant 1: System prompt preserved
	if compacted[0].Content != "System Prompt Grounding" {
		t.Errorf("system prompt altered: %q", compacted[0].Content)
	}

	// Invariant 2: User prompt preserved
	if compacted[1].Content != "Build feature X." {
		t.Errorf("initial user prompt altered: %q", compacted[1].Content)
	}

	// Summary ledger properly injected
	if !strings.Contains(compacted[2].Content, "<conversation_summary>") || !strings.Contains(compacted[2].Content, "Replaced bug in feature X.") {
		t.Errorf("summary ledger missing in compacted[2]: %q", compacted[2].Content)
	}

	// Recent turn preserved intact
	if compacted[4].Content != history[6].Content {
		t.Errorf("recent assistant turn not preserved: %q", compacted[4].Content)
	}
	if compacted[5].Content != history[7].Content {
		t.Errorf("recent user result not preserved: %q", compacted[5].Content)
	}
}

func TestSession_CompactionIntegration(t *testing.T) {
	client := NewClient("http://127.0.0.1:8080")
	s := NewSession(client, t.TempDir())

	s.AppendUserMessage("Solve task Y")
	s.AppendAssistantMessage("<action name=\"read_window\"><path>main.go</path></action>")
	s.AppendActionResult(strings.Repeat("line content in file\n", 40), nil)

	s.AppendAssistantMessage("<action name=\"replace_file\"><path>main.go</path></action>")
	s.AppendActionResult("Replaced block", nil)

	s.AppendAssistantMessage("<action name=\"run_test\">go test</action>")
	s.AppendActionResult("PASS", nil)

	// Micro-pruning on session
	reclaimed := s.PruneToolOutputs(1)
	if reclaimed <= 0 {
		t.Fatalf("expected s.PruneToolOutputs to reclaim tokens, got %d", reclaimed)
	}

	// Verify system prompt invariant still intact
	if !strings.Contains(s.History[0].Content, "You are") {
		t.Errorf("system prompt was corrupted after PruneToolOutputs")
	}

	// Macro-compaction on session
	s.CompactHistory("Summary of earlier work", 1)
	if len(s.History) < 4 {
		t.Fatalf("expected compacted history length >= 4, got %d", len(s.History))
	}
	if !strings.Contains(s.History[2].Content, "<conversation_summary>") {
		t.Errorf("expected conversation_summary in history[2], got %q", s.History[2].Content)
	}
}
