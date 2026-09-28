# Lokol AI Agent Knowledge Base

This directory provides progressive disclosure of architectural principles, engineering constraints, and operational runbooks for the `lokol` repository. It is designed for optimal consumption by autonomous coding agents and doubles as agent skills with standard YAML front matter.

## Knowledge Index & Skills

- **[Architecture & Engine Decoupling](architecture.md)** (`lokol-architecture`): Pure Go runtime, managed `llama-server` process isolation, presentation decoupling (`SessionCore`), and monorepo workspace structure.
- **[Inference & Hardware Tiering](inference-and-hardware.md)** (`lokol-inference-and-hardware`): Constrained VRAM tiering (12GB down to 4GB), single-slot 100% allocation rule, sparse MoE activation, and CPU AVX2 decision model offload.
- **[Tool Catalog & Protocols](tool-catalog-and-protocols.md)** (`lokol-tool-catalog-and-protocols`): Invariant ~250-token base prompt, progressive tool disclosure catalog, deterministic XML action protocol, dynamic intent routing, and functional regulator pipeline.
- **[Testing & Evaluation Hierarchy](testing-and-evals.md)** (`lokol-testing-and-evals`): Colocated unit tests vs integration tests, 10-tier benchmark suite, Laya semantic judging on CPU, Podman sandboxing, and capability probe regression tracking.
- **[Development Workflow & PR Protocol](dev-workflow.md)** (`lokol-dev-workflow`): Branching rules, PR requirements, Beads (`bd`) task tracking protocol, and zero-Python production runtime policy.

## Core Directives for Autonomous Agents

1. **Strictly Single-Topic ADRs**: Always audit `doc/adr/` before authoring an ADR. Adhere strictly to "What and Why, No How" (no transient code snippets or perishable paths).
2. **Lean Context & Progressive Disclosure**: Maintain the invariant ~250-token base prompt; specialized tools must be disclosed dynamically via intent routing or `<action name="tool_help">`.
3. **Zero-Python Production Runtime (ADR-0021)**: Production runtime components (`liblokol`, `cmd/lk`, `cmd/lokol`, `cmd/lokol-mcp`) must remain 100% pure Go. Python is restricted exclusively to isolated developer tooling under `tools/` via `uv`.
4. **Hardware & VRAM Preservation (ADR-0002, ADR-0020)**: Reserve 100% of GPU VRAM for the primary generative model. Secondary decision models (Laya) execute strictly on host CPU cores via AVX2.
5. **Containerized Sandboxing (ADR-0016)**: Multi-turn tests evaluating agent autonomy on scoped projects must run inside unprivileged, rootless Podman containers with isolated volumes to protect host `.beads` databases.
6. **Task Tracking via Beads (`bd`)**: Distinguish between submitting a bead for future work (`bd create`) and receiving explicit permission to implement. Never implement without explicit authorization.
