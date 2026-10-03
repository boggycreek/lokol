---
adr: "0007"
title: "General-Purpose Assistant as Default Operational Persona"
topic: "Hardware & Inference"
theme: "THEME-INFERENCE"
status: "accepted"
version: "v0.1.0-alpha"
as_built: true
tags:
  - persona
  - general-purpose
  - coding-mode
  - defaults
executive_summary: "Designates general-purpose assistant as the baseline operational persona, recommending conversational instruct models by default while offering specialized coding and architect modes via explicit flags."
---

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
We make **General-Purpose Mode** the default operational persona for `lokol`, while establishing an explicit per-subcommand mode matrix:

1. **Per-Subcommand Default Mode Matrix**: Operational modes default according to the primary intent of each entrypoint:

| Subcommand / Entrypoint | Default Mode | Target Intent & Persona Behavior |
| :--- | :--- | :--- |
| `lk` (Interactive TUI) | `general` | Interactive conversational assistant with document/artifact tools. Can be switched dynamically via `/mode` or keyboard shortcut. |
| `lokol probe` | `general` | Probes hardware and recommends optimal model weights for general conversational reasoning by default (accepts mode flags to inspect coding/MoE recommendations). |
| `lokol` (Top-level prompt) | `general` | One-shot conversational or document inspection prompt execution. |
| `lokol exec` | `coding` | Non-interactive autonomous coding agent, enabling AST outline discovery, test execution gates, and repository convention ingestion. |

2. **Model Selection**: In general-purpose mode, the model selector recommends conversational, general-reasoning models (such as Llama 3.1 8B Instruct or Llama 3.2 3B Instruct) rather than code-specialized weights.
3. **Workspace Grounding & Artifact Creation**: The system prompt in `general` mode retains host environment grounding (`<environment>`) and document inspection / artifact tools (`write_file`, `replace_file`, `read_window`, `find_files`, `exec_bash`) so the agent can inspect local documents and generate artifacts. It strictly excludes software engineering gate tools (`run_test`, `read_outline`) and repository issue tracking / coding conventions (`AGENTS.md`), preserving attention window for conversational and analytical reasoning.
4. **Explicit Mode Specialization**: Specialized personas (such as `coding` and `moe`) can be explicitly selected via `--mode=coding|moe` (or `-m`), or dynamically switched in `lk` using the `/mode <name>` command.

## Consequences

### Positive
- **Broad Accessibility**: Out-of-the-box utility for non-coding tasks (prose, analysis, general chat).
- **Context Efficiency**: Zero token waste on unnecessary code tool descriptions for everyday queries.
- **Natural Responses**: High-quality natural language generation without unwanted markdown code block wrapping.

### Negative / Trade-offs
- Developers running coding tasks via interactive or top-level interfaces must specify `--mode=coding` or persist it in their local configuration.
- Probing defaults to general mode: Operators provisioning headless coding environments should explicitly probe with coding mode (e.g. `lokol probe -m coding`) to inspect recommended weights tailored for code synthesis rather than general conversation.
