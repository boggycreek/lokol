---
name: lokol-tool-catalog-and-protocols
description: >-
  Progressive tool disclosure catalog, deterministic XML action protocol, dynamic intent routing,
  and functional regulator action gating.
  Use when creating, registering, or invoking agent tools and action parsers.
---

# Tool Catalog & Protocols

## Deterministic XML Action Protocol
- **Action Tag Syntax**: Tools are invoked via explicit XML blocks rather than JSON Schema tool calling ([ADR 0003](../adr/0003-deterministic-agent-protocol.md)):
  ```xml
  <action name="tool_name">
  <param>value</param>
  </action>
  ```
- **Robustness on Small Models**: Small local models (3B–8B) reliably emit well-formed XML without the schema hallucinations and syntax failures common in raw JSON function calling.

## Progressive Tool Disclosure Catalog (`liblokol/catalog`)
- **Invariant Base System Prompt**: The baseline system prompt contains an invariant ~250 tokens across all modes ([ADR 0024](../adr/0024-progressive-tool-disclosure-and-catalog-architecture.md)).
- **Foundational Primitives (Permanently Resident)**:
  1. `find_files`: Recursive pattern and directory discovery.
  2. `read_window`: Bounded line range file inspection.
  3. `replace_file`: Targeted exact substring replacement.
  4. `task_finish`: Concluding turn response.
  5. `tool_help`: Discovery meta-tool reflecting XML schema and usage examples on-demand.
- **Specialized Tools (Disclosed Just-in-Time)**:
  `exec_bash`, `run_test`, `search_code`, `git_diff_summary`, `read_outline`, `get_environment`, `write_file`.
- **Dynamic Intent Routing**: Fast, pure Go `IntentRouter` parses operator prompts and injects relevant specialized tool specifications into `<available_specialized_tools>` blocks just-in-time.

## Functional Regulator Action-Gating Pipeline
- **Layered Action Governance**: Mediates tool execution through composable pipeline stages ([ADR 0023](../adr/0023-functional-regulator-action-gating-pipeline.md)):
  - **Static Gating**: Pre-execution syntax validation, filesystem boundary containment (`CheckPathWithinBounds`), and shell command blocklist scanning ([ADR 0019](../adr/0019-filesystem-boundary-containment-and-permissions-pipeline.md)).
  - **Dynamic Gating**: Loop circuit breakers detecting repetitive failed action loops, runaway reads, and ping-pong state oscillation.
  - **Human-in-the-Loop Intercepts**: Gating destructive operations for explicit operator approval in the TUI (`cmd/lk`).

Further reading:
- [ADR 0003 — Deterministic Agent Protocol vs JSON Schema Tool Calling](../adr/0003-deterministic-agent-protocol.md)
- [ADR 0009 — Ingestion of Repository AGENTS.md in Agentic Coding Mode](../adr/0009-agents-md-contract-ingestion.md)
- [ADR 0012 — In-Repo MCP Servers as Architectural Facades](../adr/0012-in-repo-mcp-facades.md)
- [ADR 0019 — Filesystem Boundary Containment and Pre-Execution Permissions Pipeline](../adr/0019-filesystem-boundary-containment-and-permissions-pipeline.md)
- [ADR 0023 — Functional Regulator Action-Gating Pipeline](../adr/0023-functional-regulator-action-gating-pipeline.md)
- [ADR 0024 — Progressive Tool Disclosure and Catalog Architecture](../adr/0024-progressive-tool-disclosure-and-catalog-architecture.md)
