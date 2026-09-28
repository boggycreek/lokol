// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"strings"

	"github.com/boggycreek/lokol/liblokol/catalog"
)

// Mode represents the operational persona and tool profile of the agent.
type Mode string

const (
	// ModeGeneral is the default operational persona: a versatile local-first assistant
	// with workspace awareness, file search, document inspection, and artifact generation,
	// without software engineering guidelines or test gates.
	ModeGeneral Mode = "general"

	// ModeCoding is the autonomous agentic coding persona: full code refactoring, AST outline,
	// test-driven verification, and automatic ingestion of repository AGENTS.md guidelines.
	ModeCoding Mode = "coding"

	// ModeMoE is the dynamic Mixture of Experts persona: analytical multi-perspective
	// problem decomposition and domain expert routing with local artifact generation.
	ModeMoE Mode = "moe"
)

// ParseMode parses a mode string case-insensitively, defaulting to ModeGeneral if empty.
func ParseMode(s string) (Mode, error) {
	trimmed := strings.ToLower(strings.TrimSpace(s))
	switch trimmed {
	case "", string(ModeGeneral), "chat":
		return ModeGeneral, nil
	case string(ModeCoding), "code":
		return ModeCoding, nil
	case string(ModeMoE), "expert":
		return ModeMoE, nil
	default:
		return "", fmt.Errorf("invalid mode %q: must be 'general' (chat), 'coding' (code), or 'moe' (expert)", s)
	}
}

// GeneralSystemPromptBase defines instructions for natural conversation, document inspection,
// artifact generation, and host environment grounding adhering to ADR-0024.
var GeneralSystemPromptBase = catalog.DefaultRegistry.FormatBasePrompt("general")

// MoESystemPromptBase defines instructions for multi-perspective analytical reasoning and expert synthesis adhering to ADR-0024.
var MoESystemPromptBase = catalog.DefaultRegistry.FormatBasePrompt("moe")


// BuildSystemPromptForMode constructs the mode-specific system prompt adhering to ADR 0007 and ADR 0009:
// - All modes receive the host environment grounding tag (<environment>).
// - ModeCoding ingests repository guidelines (AGENTS.md / CLAUDE.md) into <codebase_context>.
// - ModeGeneral and ModeMoE omit AGENTS.md to protect the context window for natural language and general tasks.
func BuildSystemPromptForMode(mode Mode, env HostEnvironment, codebaseContext string) string {
	switch mode {
	case ModeCoding:
		prompt := fmt.Sprintf("You are lokol, a local-first autonomous coding agent.\nSolve coding tasks by inspecting files, writing code, and testing.\n\n%s\n\n%s",
			env.FormatEnvironmentTag(),
			SystemPromptBase,
		)
		codebaseContext = strings.TrimSpace(codebaseContext)
		if codebaseContext != "" {
			prompt = fmt.Sprintf("%s\n\n<codebase_context>\n%s\n</codebase_context>", prompt, codebaseContext)
		}
		return prompt

	case ModeMoE:
		return fmt.Sprintf("%s\n\n%s", env.FormatEnvironmentTag(), MoESystemPromptBase)

	case ModeGeneral:
		fallthrough
	default:
		return fmt.Sprintf("%s\n\n%s", env.FormatEnvironmentTag(), GeneralSystemPromptBase)
	}
}
