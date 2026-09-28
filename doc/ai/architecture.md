---
name: lokol-architecture
description: >-
  Architectural overview of the Lokol engine, pure Go runtime, presentation decoupling (SessionCore),
  managed llama-server isolation, and monorepo workspace orchestration.
  Use when designing or modifying core engine loops, session management, or presentation harnesses.
---

# Architecture & Engine Decoupling

## Pure Go Systems Runtime & Process Isolation
- **Language**: Pure Go exclusively for all production runtime code ([ADR 0001](../adr/0001-implementation-language-and-architecture.md), [ADR 0021](../adr/0021-zero-python-production-runtime-dependency-policy.md)).
- **Engine Process Isolation**: Communicates with `llama-server` over local HTTP/SSE (`http://127.0.0.1:8080`). Direct CGO/FFI bindings are prohibited to isolate GPU crashes (CUDA OOM) from the agent runtime process.
- **XDG Specification Compliance**: Configuration, persistent memory, and temporary data reside strictly under `$XDG_CONFIG_HOME`, `$XDG_DATA_HOME`, and `$XDG_STATE_HOME` ([ADR 0006](../adr/0006-xdg-base-directory-specification.md)).

## Decoupled Core Agent Engine (`SessionCore`)
- **Single Source of Truth**: The core autonomous loop, history buffer, and action dispatcher reside in `liblokol/agent` ([ADR 0017](../adr/0017-decoupled-core-agent-engine-from-presentation.md)).
- **Presentation Neutrality**: Interactive TUI (`cmd/lk`), administrative CLI (`cmd/lokol`), and diagnostic drivers (`tools/core_driver`) compose the identical `SessionCore` interface:
  - `AppendUserMessage(content string)`: Handles prompt ingestion and JIT intent routing.
  - `StreamTurn(...)`: Streams model tokens with slot-aware interrupt handling.
  - `ExecuteAction(ctx, act)`: Dispatches parsed actions through centralized catalog execution.

## Monorepo Workspace Structure (`go.work`)
- **Subproject Isolation**: Orchestrated via a root `go.work` multi-module workspace ([ADR 0018](../adr/0018-monorepo-workspace-architecture-with-subproject-isolation.md)):
  - `liblokol/`: Core engine library and SDK with zero external runtime dependencies.
  - `cmd/lk/`: Interactive Bubble Tea terminal user interface (`bin/lk`) ([ADR 0004](../adr/0004-terminal-interface-strategy.md)).
  - `cmd/lokol/`: Administrative operator CLI (`bin/lokol`).
  - `cmd/lokol-mcp/`: Standalone stdio JSON-RPC Model Context Protocol server ([ADR 0012](../adr/0012-in-repo-mcp-facades.md)).
  - `tools/core_driver/`: Headless in-process session harness for diagnostic evaluation.

Further reading:
- [ADR 0001 — Implementation Language and Engine Architecture](../adr/0001-implementation-language-and-architecture.md)
- [ADR 0004 — Terminal Interface Strategy](../adr/0004-terminal-interface-strategy.md)
- [ADR 0006 — XDG Base Directory Standard Adoption](../adr/0006-xdg-base-directory-specification.md)
- [ADR 0017 — Decoupled Core Agent Engine from Presentation Layers](../adr/0017-decoupled-core-agent-engine-from-presentation.md)
- [ADR 0018 — Monorepo Workspace Architecture with Subproject Isolation](../adr/0018-monorepo-workspace-architecture-with-subproject-isolation.md)
- [ADR 0021 — Zero-Python Production Runtime Dependency Policy](../adr/0021-zero-python-production-runtime-dependency-policy.md)
