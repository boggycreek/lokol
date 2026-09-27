# ADR 0007: General-Purpose Assistant as Default Operational Persona

## Status
Accepted

## Date
2026-09-20

## Context
Initially, `lokol` was conceived strictly as an autonomous agentic coding agent, loading coding-specialized model weights (e.g. `Qwen 2.5 Coder 7B/3B`) and imposing code-specific tool schemas and system prompts on every startup.

However, many users seek an accessible, private, local assistant for daily natural language tasks: conversation, document summarization, analytical reasoning, and writing. In these common contexts:
1. Hardcoding coding-specific system prompts wastes precious context tokens.
2. Coding models often produce rigid, code-focused outputs even when prompted for prose or conversational answers.
3. Forcing code tooling on install alienates users who want a simple, fast, private chatbot.

We need to decide the baseline user persona and model selection strategy for `lokol`.

## Decision
We make **General-Purpose Mode** the default operational persona for `lokol`:
1. **Default Mode**: When invoked without explicit flags (`lk`, headless `lokol`, or via `install.sh`), the agent operates in `general` mode. Headless autonomous runs via `lokol exec` default to `coding` mode.
2. **Model Selection**: In general-purpose mode, the model selector recommends conversational, general-reasoning models (such as Llama 3.1 8B Instruct or Llama 3.2 3B Instruct) rather than code-specialized weights.
3. **Workspace Grounding & Artifact Creation**: The system prompt in `general` mode retains host environment grounding (`<environment>`) and document inspection / artifact tools (`write_file`, `replace_file`, `read_window`, `find_files`, `exec_bash`) so the agent can inspect local documents and generate artifacts. It strictly excludes software engineering gate tools (`run_test`, `read_outline`) and repository issue tracking / coding conventions (`AGENTS.md`), preserving attention window for conversational and analytical reasoning.
4. **Explicit Mode Specialization**: Specialized personas (such as `coding` and `moe`) can be explicitly selected via `--mode=coding|moe` (or `-m`), or dynamically switched in `lk` using the `/mode <name>` command.

## Consequences

### Positive
- **Broad Accessibility**: Out-of-the-box utility for non-coding tasks (prose, analysis, general chat).
- **Context Efficiency**: Zero token waste on unnecessary code tool descriptions for everyday queries.
- **Natural Responses**: High-quality natural language generation without unwanted markdown code block wrapping.

### Negative / Trade-offs
- Developers running coding tasks must pass `--mode=coding` or persist it in their local configuration.
