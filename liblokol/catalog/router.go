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
