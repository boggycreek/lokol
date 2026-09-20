# ADR 0007: General-Purpose Assistant as Default Operational Persona

## Status
Accepted

## Date
2026-09-20

## Context
Initially, `lokol` was conceived strictly as an autonomous agentic coding engine, loading coding-specialized model weights (e.g. `Qwen 2.5 Coder 7B/3B`) and imposing code-specific tool schemas and system prompts on every startup.

However, many users seek an accessible, private, local assistant for daily natural language tasks: conversation, document summarization, analytical reasoning, and writing. In these common contexts:
1. Hardcoding coding-specific system prompts wastes precious context tokens.
2. Coding models often produce rigid, code-focused outputs even when prompted for prose or conversational answers.
3. Forcing code tooling on install alienates users who want a simple, high-performance private chatbot.

We need to decide the baseline user persona and model selection strategy for `lokol`.

## Decision
We make **General-Purpose Mode** the default operational persona for `lokol`:
1. **Default Mode**: When invoked without explicit flags (`lokol`, `lokol chat`, or via `install.sh`), `lokol` operates in `general` mode.
2. **Model Selection**: In general-purpose mode, the model selector recommends conversational, general-reasoning models (such as Llama 3.1 8B Instruct or Mistral 7B Instruct Q4_K_M) rather than code-specialized weights.
3. **Clean Context**: The system prompt in `general` mode excludes software engineering tool definitions (such as `replace_file` or `run_test`), devoting 100% of the attention window to conversational reasoning.
4. **Explicit Mode Specialization**: Specialized personas (such as `coding` and `moe`) must be explicitly requested via `--mode=coding` or configured in `$XDG_CONFIG_HOME/lokol/config.toml`.

## Consequences

### Positive
- **Broad Accessibility**: Out-of-the-box utility for non-coding tasks (prose, analysis, general chat).
- **Context Efficiency**: Zero token waste on unnecessary code tool descriptions for everyday queries.
- **Natural Responses**: High-quality natural language generation without unwanted markdown code block wrapping.

### Negative / Trade-offs
- Developers running coding tasks must pass `--mode=coding` or persist it in their local configuration.
