# Architecture Decision Records (ADRs)

This directory documents the foundational architectural decisions governing the **lokol** local agentic coding ecosystem for the **v0.1.0-alpha** release.

Records are numbered serially (`0001` through `0029`) and organized by topic domain to reflect the current **as-built** architecture. Each decision record includes machine-readable YAML front matter (with standardized thematic markers, tags, and executive summaries) for consumption by automated agents and tooling.

---

## Thematic Groupings

| Theme Code | Topic Domain | Scope |
| :--- | :--- | :--- |
| **`THEME-CORE`** | Foundations & Architecture | Pure Go systems architecture, managed `llama-server` process isolation, XDG specification, presentation decoupling, monorepo workspace isolation, zero-Python production runtime policy. |
| **`THEME-INFERENCE`** | Hardware, Models & VRAM | Hardware tiering (12GB down to 4GB), single-slot 100% VRAM allocation, baseline general-purpose persona, dynamic MoE sparse expert offloading, CPU AVX2 decision model offload, explicit context clearing & slot purge. |
| **`THEME-AGENT`** | Protocols, Catalog & Governance | Strict XML deterministic action protocol, in-repo MCP server facades, `AGENTS.md` context ingestion, functional regulator action-gating pipeline, progressive tool disclosure catalog. |
| **`THEME-MEMORY`** | Memory & Retrieval | Structured persistent memory across XDG state/data, local embedded vector database for semantic indexing. |
| **`THEME-SECURITY`** | Security, Boundary & Sandboxing | Filesystem boundary containment, command risk inspection, unprivileged rootless Podman container test sandboxing. |
| **`THEME-QUALITY`** | Evaluations, Ergonomics & Quality Gates | Split Bubble Tea TUI vs headless CLI, graded 10-tier integration benchmark suite, non-autoregressive Laya decision model judging, auxiliary tooling isolation via `uv`, colocated unit testing hierarchy. |

---

## Architectural Decision Records

### Foundations & Architecture (`THEME-CORE`)
- **[ADR 0001 — Use Go with Managed llama-server Process Isolation](0001-implementation-language-and-architecture.md)**  
  *Executive Summary:* Lokol implements its core runtime and tools in pure Go, communicating with a managed `llama-server` subprocess over HTTP for deterministic process isolation and native binary execution.
- **[ADR 0006 — XDG Base Directory Standard Adoption](0006-xdg-base-directory-specification.md)**  
  *Executive Summary:* Adopts the XDG Base Directory specification for all configuration, cache, state, and persistent data paths across `lokol` and `lk`.
- **[ADR 0017 — Decoupled Core Agent Engine from Presentation Layers](0017-decoupled-core-agent-engine-from-presentation.md)**  
  *Executive Summary:* Decouples the core agent execution engine (`SessionCore`, `DispatchAction`) from terminal presentation layers, enabling headful TUI, headless CLI, and diagnostic drivers to share identical reasoning loops.
- **[ADR 0018 — Monorepo Workspace Architecture with Subproject Isolation](0018-monorepo-workspace-architecture-with-subproject-isolation.md)**  
  *Executive Summary:* Establishes a multi-module Go workspace (`go.work`) isolating `liblokol`, `cmd/lk`, `cmd/lokol`, and `cmd/lokol-mcp` with dedicated Makefiles and parallel CI matrix jobs.
- **[ADR 0021 — Zero-Python Production Runtime Dependency Policy](0021-zero-python-production-runtime-dependency-policy.md)**  
  *Executive Summary:* Mandates that all production runtime components (`liblokol`, `cmd/lk`, `cmd/lokol`, `cmd/lokol-mcp`) are written in 100% pure Go with zero runtime Python or dynamic interpreter dependencies.

### Hardware, Models & VRAM (`THEME-INFERENCE`)
- **[ADR 0002 — Hardware Probing and Tiering Matrix (12GB to 4GB GTX 1650)](0002-hardware-tiering-and-constrained-vram.md)**  
  *Executive Summary:* Establishes a three-tier hardware allocation policy (12GB+, 8GB, 4GB) dedicating 100% of GPU VRAM to a single inference slot to guarantee full context window utilization without host RAM thrashing.
- **[ADR 0007 — General-Purpose Assistant as Default Operational Persona](0007-general-purpose-default-persona.md)**  
  *Executive Summary:* Designates general-purpose assistant as the baseline operational persona, recommending conversational instruct models by default while offering specialized coding and architect modes via explicit flags.
- **[ADR 0008 — Dynamic Mixture of Experts (MoE) Routing for Constrained VRAM](0008-dynamic-moe-expert-routing.md)**  
  *Executive Summary:* Establishes a sparse MoE operational strategy that prioritizes attention layers and active experts in GPU VRAM while paging non-active experts in system RAM via `mmap`.
- **[ADR 0020 — CPU AVX2 Offload Strategy for Auxiliary Decision Models](0020-cpu-avx2-offload-strategy-for-auxiliary-decision-models.md)**  
  *Executive Summary:* Directs auxiliary decision models to execute exclusively on host CPU cores via AVX2/AVX-512 instructions, reserving 100% of GPU VRAM for the primary generative LLM context.
- **[ADR 0026 — Inference Slot Pressure and Context Budget Regulation](0026-inference-slot-pressure-and-context-budget-regulation.md)**  
  *Executive Summary:* Regulates execution cadence and triggers proactive context compaction based on inference slot memory utilization and token window pressure.
- **[ADR 0028 — Explicit Context Clearing and State Retention Invariants](0028-explicit-context-clearing-and-state-retention-invariants.md)**  
  *Executive Summary:* Establishes explicit context clearing protocols, retention invariants, and inference slot cache purges across interactive and headless execution boundaries.
- **[ADR 0029 — Dynamic Context Compaction and Attention Pruning Architecture](0029-dynamic-context-compaction-and-attention-pruning.md)**  
  *Executive Summary:* Establishes a dual-tier dynamic context compaction strategy combining deterministic observation pruning and semantic history summarization triggered by real-time inference slot pressure.

### Protocols, Catalog & Governance (`THEME-AGENT`)
- **[ADR 0003 — Deterministic Agent Protocol vs JSON Schema Tool Calling](0003-deterministic-agent-protocol.md)**  
  *Executive Summary:* Adopts a strict XML action-tag protocol over JSON Schema tool calling to ensure reliable tool invocations and eliminate schema hallucination across small local models (3B–8B).
- **[ADR 0009 — Ingestion of Repository AGENTS.md in Agentic Coding Mode](0009-agents-md-contract-ingestion.md)**  
  *Executive Summary:* Ingests repository `AGENTS.md` instructions directly into the agent context in coding mode to ensure strict adherence to workspace conventions, quality gates, and tool rules.
- **[ADR 0012 — In-Repo MCP Servers as Architectural Facades for Deterministic Tool Call Invocations](0012-in-repo-mcp-facades.md)**  
  *Executive Summary:* Provides an in-repo Model Context Protocol (MCP) server facade (`cmd/lokol-mcp`) exposing refinery operations over stdio JSON-RPC for external tools and agents.
- **[ADR 0023 — Functional Regulator Action-Gating Pipeline](0023-functional-regulator-action-gating-pipeline.md)**  
  *Executive Summary:* Defines a composable functional regulator pipeline that intercepts, inspects, and validates proposed agent actions through layered static, dynamic, and human-in-the-loop gates.
- **[ADR 0024 — Progressive Tool Disclosure and Catalog Architecture](0024-progressive-tool-disclosure-and-catalog-architecture.md)**  
  *Executive Summary:* Implements a centralized tool catalog providing an invariant ~250-token base prompt with 4 foundational primitives, disclosing specialized tools dynamically via intent routing or `tool_help` reflection.
- **[ADR 0025 — Dynamic Loop Circuit Breaking and Oscillation Governance](0025-dynamic-loop-circuit-breaking-and-oscillation-governance.md)**  
  *Executive Summary:* Establishes automated loop detection and circuit breaking within the regulator pipeline to identify repeated identical tool failures, runaway inspection loops, and state oscillation before resource exhaustion occurs.
- **[ADR 0027 — Structured Trajectory Remediation for Autonomous Recovery](0027-structured-trajectory-remediation-for-autonomous-recovery.md)**  
  *Executive Summary:* Defines a structured remediation protocol that transforms regulatory rejections and boundary denials into actionable self-correction feedback for the agent.

### Memory & Retrieval (`THEME-MEMORY`)
- **[ADR 0010 — Persistent Memory Storage Architecture in XDG State and Data Directories](0010-xdg-persistent-memory-storage.md)**  
  *Executive Summary:* Stores long-term agent memories and learned preferences across sessions using structured persistence in standard XDG state and data directories.
- **[ADR 0011 — Local Vector Database Adoption for Memory and Source Code Indexing](0011-local-vector-database-adoption.md)**  
  *Executive Summary:* Adopts an embedded, pure Go local vector database for semantic memory retrieval and code symbol indexing without external service dependencies.

### Security & Isolation (`THEME-SECURITY`)
- **[ADR 0016 — Containerized Test Sandboxing for Host Memory and State Isolation](0016-containerized-test-sandboxing-via-podman.md)**  
  *Executive Summary:* Executes agent autonomy evaluations inside unprivileged, rootless Podman containers with isolated volumes to strictly protect host task tracking (`.beads`) and file state.
- **[ADR 0019 — Filesystem Boundary Containment and Pre-Execution Permissions Pipeline](0019-filesystem-boundary-containment-and-permissions-pipeline.md)**  
  *Executive Summary:* Enforces strict workspace directory traversal containment and shell command risk inspection before any filesystem mutation or command execution occurs.

### Evaluations, Ergonomics & Quality Gates (`THEME-QUALITY`)
- **[ADR 0004 — Terminal Interface Strategy (Bubble Tea TUI vs Headless CLI)](0004-terminal-interface-strategy.md)**  
  *Executive Summary:* Splits terminal presentation into an interactive Bubble Tea TUI (`cmd/lk`) for live operator feedback and a headless non-interactive CLI (`cmd/lokol`) for scripted automation and CI.
- **[ADR 0005 — Hybrid Evaluations and Fast Routing with TypeSafe AI (Jev / "not-a-llm")](0005-typesafe-ai-hybrid-evaluations.md)**  
  *Executive Summary:* Proposes integrating TypeSafe AI non-LLM decision classifiers for fast query complexity routing and deterministic evaluation scoring.
- **[ADR 0013 — Graded Multi-Tier Integration Benchmark Suite](0013-graded-multi-tier-agent-evaluations.md)**  
  *Executive Summary:* Establishes a 10-tier graded integration benchmark suite testing agent loop mechanics, tool recovery, bounded editing, and full-project autonomy against live inference engines.
- **[ADR 0014 — Non-Autoregressive Decision Models for Semantic Evaluation](0014-non-autoregressive-decision-model-judging.md)**  
  *Executive Summary:* Integrates ultra-fast non-autoregressive decision models (Laya) on CPU to score qualitative software engineering artifacts deterministically without generative judge variance or GPU VRAM contention.
- **[ADR 0015 — Auxiliary Developer Tooling Isolation via Virtual Environments](0015-auxiliary-tooling-isolation-via-uv.md)**  
  *Executive Summary:* Isolates auxiliary developer tooling and evaluation bridges in project-local virtual environments managed strictly via `uv` and PEP 723 metadata without polluting system environments.
- **[ADR 0022 — Colocated Unit Testing and Integration Hierarchy](0022-colocated-unit-testing-and-integration-hierarchy.md)**  
  *Executive Summary:* Standardizes idiomatic colocated unit tests (`*_test.go`) within subproject packages, reserving subproject `test/` directories exclusively for multi-turn integration and end-to-end suites.
