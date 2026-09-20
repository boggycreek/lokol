# ADR 0009: Ingestion of Repository AGENTS.md in Agentic Coding Mode

## Status
Accepted

## Date
2026-09-20

## Context
When executing coding tasks, general agent system prompts cannot foresee repository-specific conventions:
- Local service endpoints and hardware constraints (e.g. `http://127.0.0.1:8080/v1`).
- Non-interactive shell execution requirements (e.g. `cp -f`, `rm -rf`).
- Project issue tracking workflows (e.g. Beads `bd` CLI commands vs ad-hoc markdown TODO lists).
- Repository test suites and linting commands.

Without project-specific alignment, the agent hallucinates generic workflows, violates repository policies, or alters files outside the intended scope.

We need a deterministic mechanism for `lokol` to inherit repository guidelines without manual prompt prefixing.

## Decision
When running in **Agentic Coding Mode** (`--mode=coding` or `lokol exec`):
1. **Discovery Hierarchy**: `lokol` scans the workspace root for instruction contracts in this order:
   1. `./AGENTS.md` (Standard project instruction contract)
   2. `./CLAUDE.md` (Legacy fallback)
   3. `.github/AGENTS.md`
2. **Context Injection**:
   - If an instruction file is present, its content is parsed and injected directly into the system prompt within a structured `<project_guidelines>` block.
   - Project-specific constraints defined in `AGENTS.md` strictly take precedence over default agent heuristics.
3. **Mode Isolation**:
   - In `general` mode, workspace `AGENTS.md` files are deliberately ignored to prevent code guidelines from polluting non-coding conversational sessions.

## Consequences

### Positive
- **Automatic Alignment**: The coding agent immediately respects repository conventions (Beads issue tracking, non-interactive shell commands, test gates) without explicit user prompting.
- **Single Source of Truth**: Repository maintainers configure agent behavior for all contributors simply by committing an `AGENTS.md` file.

### Negative / Trade-offs
- Large `AGENTS.md` files consume initial KV context tokens. (Can be mitigated in future iterations via context refinery filtering).
