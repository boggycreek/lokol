// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package catalog

import (
	"context"
	"fmt"
	"strings"
)

// IntentRouter evaluates user prompts and dynamically selects specialized tools
// for pre-flight injection adhering to ADR-0020, ADR-0021, and ADR-0024.
type IntentRouter struct {
	registry *Registry
}

// NewIntentRouter constructs an IntentRouter backed by the given tool registry.
func NewIntentRouter(reg *Registry) *IntentRouter {
	if reg == nil {
		reg = DefaultRegistry
	}
	return &IntentRouter{registry: reg}
}

// DefaultRouter is the default intent router backed by DefaultRegistry.
var DefaultRouter = NewIntentRouter(DefaultRegistry)

// RouteIntent analyzes the user prompt and returns matching specialized tools.
func (r *IntentRouter) RouteIntent(ctx context.Context, prompt string) []Tool {
	// Suppress tool injection on pure conversational feedback or acknowledgments (lokol-kih.6)
	if IsConversationalFeedback(prompt) {
		return nil
	}

	// Suppress specialized tools on context introspection and meta-queries (lokol-kih.1)
	if IsContextMetaQuery(prompt) {
		return nil
	}

	lower := strings.ToLower(prompt)
	words := strings.Fields(lower)

	specialized := r.registry.ListSpecialized()
	matched := make([]Tool, 0)
	seen := make(map[string]bool)

	for _, tool := range specialized {
		if seen[tool.Name()] {
			continue
		}

		// Exact tool name mentioned
		if strings.Contains(lower, strings.ToLower(tool.Name())) {
			matched = append(matched, tool)
			seen[tool.Name()] = true
			continue
		}

		// Tag and phrase matching
		if r.matchesIntent(lower, words, tool) {
			matched = append(matched, tool)
			seen[tool.Name()] = true
		}
	}

	return matched
}

// IsConversationalFeedback checks if the user prompt is conversational feedback, praise, greetings,
// or evaluative remarks that should be answered directly without triggering tool invocations (lokol-kih.6).
func IsConversationalFeedback(prompt string) bool {
	p := strings.TrimSpace(strings.ToLower(prompt))
	if p == "" {
		return false
	}

	// Action words and explicit questions indicate actionable intent, not pure conversational feedback
	actionWords := []string{
		"?", "please", "can you", "could you", "would you",
		"write", "create", "edit", "replace", "fix", "update", "delete", "remove",
		"run", "exec", "test", "build", "compile", "find", "search", "read",
		"show", "list", "inspect", "diff", "outline", "summarize", "explain",
	}
	for _, word := range actionWords {
		if strings.Contains(p, word) {
			return false
		}
	}

	feedbackPhrases := []string{
		"nice", "great", "cool", "awesome", "good job", "well done", "nice work",
		"looks good", "sounds good", "lgtm", "perfect",
		"thanks", "thank you", "thx", "appreciate it",
		"glad your regulator flagged", "glad that was flagged", "glad that was caught", "glad it caught that", "glad to hear",
		"ok", "okay", "got it", "understood", "acknowledged", "no problem", "sure",
		"hello", "hi", "hey", "good morning", "good evening", "good night",
	}

	for _, phrase := range feedbackPhrases {
		if p == phrase || strings.HasPrefix(p, phrase+".") || strings.HasPrefix(p, phrase+"!") || strings.HasPrefix(p, phrase+",") {
			return true
		}
		if strings.Contains(p, phrase) && len(strings.Fields(p)) <= 10 {
			return true
		}
	}

	return false
}

// IsProjectSummaryQuery detects requests asking what the project/codebase does or to summarize it (lokol-kih.8).
func IsProjectSummaryQuery(prompt string) bool {
	p := strings.TrimSpace(strings.ToLower(prompt))
	if p == "" {
		return false
	}

	summaryPatterns := []string{
		"what can you tell me about the current project",
		"what does the project do",
		"what does this project do",
		"summarize that the project does",
		"summarize what the project does",
		"summarize the project",
		"summarize the current project",
		"summarize this project",
		"summarize the codebase",
		"summarize this codebase",
		"tell me about this project",
		"tell me about the current project",
		"tell me about this codebase",
		"tell me about this repository",
		"explain this repository",
		"explain this project",
		"explain the project",
		"overview of the project",
		"project overview",
		"what is this project",
		"what is this repository",
		"what is this codebase",
	}

	for _, pat := range summaryPatterns {
		if strings.Contains(p, pat) {
			return true
		}
	}

	return false
}

// IsContextMetaQuery detects queries asking about the agent's internal context window, system prompt, or session state (lokol-kih.1).
func IsContextMetaQuery(prompt string) bool {
	p := strings.TrimSpace(strings.ToLower(prompt))
	if p == "" {
		return false
	}

	metaPatterns := []string{
		"show me your context",
		"show your context",
		"what is your context",
		"what's in your context",
		"what is in your context",
		"whats in your context",
		"see your context",
		"inspect your context",
		"show your system prompt",
		"show me your system prompt",
		"what is your system prompt",
		"what are your instructions",
		"show me your instructions",
		"what instructions do you have",
		"what are your system instructions",
		"what mode are you in",
		"show your mode",
		"which mode are you in",
		"active mode",
		"what is your persona",
		"what are your active tools",
		"what tools do you have",
		"session state",
		"context window",
	}

	for _, pat := range metaPatterns {
		if strings.Contains(p, pat) {
			return true
		}
	}

	return false
}

func (r *IntentRouter) matchesIntent(lower string, words []string, tool Tool) bool {
	switch tool.Name() {
	case "git_diff_summary":
		return strings.Contains(lower, "git ") || strings.Contains(lower, "diff") ||
			strings.Contains(lower, "uncommitted") || strings.Contains(lower, "working tree") ||
			strings.Contains(lower, "commit ") || strings.Contains(lower, "branch")

	case "run_test":
		return strings.Contains(lower, "test") || strings.Contains(lower, "tests") ||
			strings.Contains(lower, "verify") || strings.Contains(lower, "failing") ||
			strings.Contains(lower, "pass") || strings.Contains(lower, "assertion")

	case "exec_bash":
		return strings.Contains(lower, "packages in this repository") || strings.Contains(lower, "go list") ||
			strings.Contains(lower, "run command") || strings.Contains(lower, "bash") ||
			strings.Contains(lower, "shell") || strings.Contains(lower, "exec") ||
			strings.Contains(lower, "terminal") || strings.Contains(lower, "compile") ||
			strings.Contains(lower, "install")

	case "search_code":
		return strings.Contains(lower, "search code") || strings.Contains(lower, "grep") ||
			strings.Contains(lower, "find pattern") || strings.Contains(lower, "find references") ||
			strings.Contains(lower, "search for") || strings.Contains(lower, "where is")

	case "read_outline":
		return strings.Contains(lower, "outline") || strings.Contains(lower, "ast") ||
			strings.Contains(lower, "signatures") || strings.Contains(lower, "declarations") ||
			strings.Contains(lower, "symbols") || strings.Contains(lower, "structure of")

	case "write_file":
		// Only trigger write_file when explicitly requested to create a brand new file
		return strings.Contains(lower, "create a new file") || strings.Contains(lower, "create file") ||
			strings.Contains(lower, "write a new file") || strings.Contains(lower, "save new file")

	case "get_environment":
		return strings.Contains(lower, "environment") || strings.Contains(lower, "toolchains") ||
			strings.Contains(lower, "mcp tools") || strings.Contains(lower, "host os") ||
			strings.Contains(lower, "check your environment") || strings.Contains(lower, "gpu") ||
			strings.Contains(lower, "hardware") || strings.Contains(lower, "cpu")
	}

	// Fallback to checking tool tags against prompt words
	for _, tag := range tool.Tags() {
		for _, w := range words {
			if w == tag {
				return true
			}
		}
	}

	return false
}

// FormatInjectedTools formats the XML specifications for injected tools into a lean block.
func (r *IntentRouter) FormatInjectedTools(tools []Tool) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n<available_specialized_tools>\n")
	sb.WriteString("The following specialized tools are available for this request:\n\n")
	for i, t := range tools {
		sb.WriteString(fmt.Sprintf("%d. %s:\n%s\n\n", i+1, t.Summary(), t.SchemaXML()))
	}
	sb.WriteString("</available_specialized_tools>\n")
	return sb.String()
}
