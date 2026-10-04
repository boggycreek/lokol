// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"regexp"
	"strings"
)

// PressureLevel defines the real-time context token pressure level reported by the engine.
type PressureLevel string

const (
	PressureNominal  PressureLevel = "nominal"  // < 60% of n_ctx
	PressureWarning  PressureLevel = "warning"  // 60% - 75% of n_ctx (triggers micro-pruning)
	PressureCritical PressureLevel = "critical" // >= 75% of n_ctx (triggers macro-compaction)
)

// ClassifySlotPressure categorizes slot utilization into operational pressure bands.
func ClassifySlotPressure(nPromptTokens, nCtx int) PressureLevel {
	if nCtx <= 0 {
		return PressureNominal
	}
	utilPct := (float64(nPromptTokens) / float64(nCtx)) * 100.0
	if utilPct >= 75.0 {
		return PressureCritical
	}
	if utilPct >= 60.0 {
		return PressureWarning
	}
	return PressureNominal
}

// EstimateTokenCount provides a fast character-based heuristic for token estimation (~4 chars/token).
func EstimateTokenCount(text string) int {
	if len(text) == 0 {
		return 0
	}
	// Heuristic: ~4 characters per token in code/English text
	tokens := len(text) / 4
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}

var actionResultRegex = regexp.MustCompile(`(?s)<action_result>(.*?)</action_result>`)

// PruneStaleToolOutputs performs micro-compaction by stripping verbose observation tool outputs
// (such as large read_window, read_outline, or bash directory listings) from older turns,
// replacing them with lightweight stubs while leaving reasoning, decisions, and recent turns intact.
// preserveRecent specifies how many recent turns (each turn = assistant + tool result) to preserve verbatim.
func PruneStaleToolOutputs(history []Message, preserveRecent int) ([]Message, int) {
	if len(history) <= 2 {
		return history, 0
	}

	if preserveRecent < 1 {
		preserveRecent = 1
	}

	// Calculate the index threshold beyond which messages are considered recent.
	// Each interaction turn typically consists of an assistant message followed by a tool result (2 messages).
	cutoff := len(history) - (preserveRecent * 2)
	if cutoff <= 1 {
		return history, 0
	}

	reclaimedChars := 0
	pruned := make([]Message, len(history))
	copy(pruned, history)

	// Iterate through older messages (skipping system prompt at index 0)
	for i := 1; i < cutoff; i++ {
		msg := pruned[i]
		if msg.Role != "user" || !strings.Contains(msg.Content, "<action_result>") {
			continue
		}

		// Check if content is long enough to warrant pruning (> 200 chars)
		if len(msg.Content) <= 200 {
			continue
		}

		// Determine if this is an observation tool result that can be safely collapsed
		isPrunable := strings.Contains(msg.Content, "read_window") ||
			strings.Contains(msg.Content, "read_outline") ||
			strings.Contains(msg.Content, "find_files") ||
			strings.Contains(msg.Content, "search_code") ||
			strings.Contains(msg.Content, "get_environment") ||
			strings.Contains(msg.Content, "=== Output ===") ||
			strings.Contains(msg.Content, "lines ")

		// Also prune large stdout outputs from non-failing bash runs
		if !isPrunable && strings.Contains(msg.Content, "<action_result>") && !strings.Contains(msg.Content, "[Exit error:") && !strings.Contains(msg.Content, "[Edit error:") {
			if len(msg.Content) > 500 {
				isPrunable = true
			}
		}

		if isPrunable {
			origLen := len(msg.Content)
			stub := fmt.Sprintf("<action_result>\n[Output pruned: %d chars of prior observation data compacted to conserve attention]\n</action_result>", origLen)
			pruned[i].Content = stub
			reclaimedChars += (origLen - len(stub))
		}
	}

	return pruned, reclaimedChars
}

// CompactHistory performs macro-compaction by summarizing older conversation turns
// into a structured context ledger while strictly preserving:
// 1. Tier 1 system prompt + host environment / codebase context (history[0]).
// 2. Initial user objective (history[1]).
// 3. The latest preserveRecent turns verbatim.
//
// Older intermediate turns are replaced by a single structured ledger entry.
func CompactHistory(history []Message, summaryLedger string, preserveRecent int) []Message {
	if len(history) <= 3 {
		return history
	}

	if preserveRecent < 1 {
		preserveRecent = 1
	}

	cutoff := len(history) - (preserveRecent * 2)
	// If preserving preserveRecent turns leaves no older turns to compact,
	// fall back to preserving 1 recent turn to reclaim headroom (lokol-asw).
	if cutoff <= 2 && preserveRecent > 1 {
		cutoff = len(history) - 2
	}
	// We need at least: system prompt (0), initial user prompt (1), and some older turns (2..cutoff)
	if cutoff <= 2 {
		return history
	}

	compacted := make([]Message, 0, 4+(preserveRecent*2))

	// Invariant 1: Preserve Tier 1 System Prompt with environment grounding
	compacted = append(compacted, history[0])

	// Invariant 2: Preserve original user objective
	compacted = append(compacted, history[1])

	// Macro-compaction: Insert structured ledger summarizing turns 2 through cutoff-1
	summaryBlock := fmt.Sprintf("<conversation_summary>\n%s\n</conversation_summary>", strings.TrimSpace(summaryLedger))
	compacted = append(compacted, Message{
		Role:    "user",
		Content: summaryBlock,
	})
	compacted = append(compacted, Message{
		Role:    "assistant",
		Content: "Understood. I have integrated the compacted execution summary and will continue toward the objective.",
	})

	// Invariant 3: Preserve recent turns verbatim
	for i := cutoff; i < len(history); i++ {
		compacted = append(compacted, history[i])
	}

	return compacted
}
