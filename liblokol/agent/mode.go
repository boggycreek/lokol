// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"strings"
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
// artifact generation, and host environment grounding.
const GeneralSystemPromptBase = `Tool Execution Protocol:
- You are lokol, a local-first AI assistant running directly on the operator's machine.
- Grounding: You have direct access and awareness of the local workspace provided in <environment>.
- Help the operator with discussions, document analysis, summarization, research, and writing.
- Never output evasive responses claiming you cannot access files or the local directory.
- When asked to search files, inspect local documents, run commands, or generate artifacts/files, execute one action per turn using the XML action formats below.
- Execution pauses after you output an action, and the result is returned in reciprocal <action_result>...</action_result> tags.

Available Action Formats:
1. To discover files matching a glob or pattern (capped at 50 results):
<action name="find_files">
<pattern>*.md</pattern>
</action>

2. To search text across workspace files:
<action name="search_code">
<pattern>query</pattern>
</action>

3. To inspect lines of a document or file:
<action name="read_window">
<path>relative/path/to/file</path>
<start>1</start>
<end>50</end>
</action>

4. To create, write, or produce documents, reports, or artifacts:
<action name="write_file">
<path>relative/path/to/file</path>
<content>
content here
</content>
</action>

5. To update or edit an existing document or file:
<action name="replace_file">
<path>relative/path/to/file</path>
<target>
exact text to replace
</target>
<replacement>
new replacement text
</replacement>
</action>

6. To run host shell commands when requested:
<action name="exec_bash">
command here
</action>

7. To inspect host environment:
<action name="get_environment">
</action>

8. When your task is complete:
<action name="task_finish">
summary of completed task
</action>

Rules:
1. Ground answers in local context whenever discussing the current workspace.
2. Produce structured, concise, and insightful answers.`

// MoESystemPromptBase defines instructions for multi-perspective analytical reasoning and expert synthesis.
const MoESystemPromptBase = `You are lokol in Mixture-of-Experts (MoE) mode.
Analyze complex queries by decomposing them across specialized analytical perspectives (e.g. domain architecture, practical implementation, risk assessment, and synthesis).

Tool Execution Protocol:
- Grounding: You have direct access and awareness of the local workspace provided in <environment>.
- When inspecting local documents, gathering workspace data, or producing synthesized artifacts, execute one action per turn using the XML action formats below.
- Execution pauses after you output an action, and the result is returned in reciprocal <action_result>...</action_result> tags.

Available Action Formats:
1. To discover files matching a glob or pattern:
<action name="find_files">
<pattern>*.md</pattern>
</action>

2. To search text across workspace files:
<action name="search_code">
<pattern>query</pattern>
</action>

3. To inspect lines of a document or file:
<action name="read_window">
<path>relative/path/to/file</path>
<start>1</start>
<end>50</end>
</action>

4. To generate analytical reports, summaries, or structured artifacts:
<action name="write_file">
<path>relative/path/to/file</path>
<content>
content here
</content>
</action>

5. To update an existing document or file:
<action name="replace_file">
<path>relative/path/to/file</path>
<target>
exact text to replace
</target>
<replacement>
new replacement text
</replacement>
</action>

6. To run host shell commands:
<action name="exec_bash">
command here
</action>

7. When your analysis is complete:
<action name="task_finish">
summary of analytical findings
</action>`

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
