# ADR 0018: Monorepo Workspace Architecture with Subproject Isolation

## Status
Accepted

## Date
2026-09-27

## Context
As `lokol` expanded from a prototype CLI into an autonomous local agent ecosystem, its functionality split across distinct architectural responsibilities:
1. **Core Autonomous Engine (`liblokol`)**: Pure headless reasoning loops, tool dispatching, streaming protocols, and context refinery primitives.
2. **Interactive Developer TUI (`cmd/lk`)**: Terminal interface built on Bubble Tea for interactive approval flows and developer ergonomics.
3. **Administrative & Headless Runner (`cmd/lokol`)**: Hardware capability probing, environment bootstrapping, version updates, and headless non-interactive execution.
4. **External Agent Facade (`cmd/lokol-mcp`)**: Standalone stdio JSON-RPC server exposing context refinery tools to foreign IDEs and agents.
5. **Future Consumers**: Planned shared library wrappers (`liblokol.so` / C ABI) and local IPC daemons.

Maintaining these concerns in a single monolithic Go module introduced severe friction:
- **Dependency & Concern Leakage**: Presentation frameworks and terminal dependencies were coupled with engine primitives, preventing clean re-use by alternate clients.
- **Shared Scratch Space Collision**: Tests and build workflows competed for shared repository-level directories, leading to race conditions and test pollution.
- **Monolithic CI & Test Bottlenecks**: Unit tests could not be isolated from integration suites requiring local inference infrastructure (`llama-server` or container engines), slowing down feedback loops.
- **Lack of Independent Lifecycles**: Subprojects could not be built, versioned, or verified independently.

## Decision

We adopt a **monorepo workspace architecture** based on the following architectural decisions:

1. **Decouple Core Engine from Presentation Consumers**:
   - `liblokol` is strictly headless and library-only, free of any terminal UI or presentation dependencies.
   - Presentation layers (`cmd/lk`, `cmd/lokol`, `cmd/lokol-mcp`) are decoupled consumer binaries that import `liblokol`.
   - The interactive chat domain belongs exclusively to `cmd/lk`. The administrative binary `cmd/lokol` handles setup, hardware probing, updates, and headless execution, without hosting or emulating interactive chat.

2. **Subproject Isolation & Autonomy**:
   - Each subproject is an independent Go module with its own build definition (`Makefile`) and ignore boundaries (`.gitignore`).
   - Development across the repository is unified via a root `go.work` workspace and an orchestrating root `Makefile`.
   - Subprojects can be built, tested, and linted in isolation without knowledge of sibling subprojects or root-level scripts.

3. **Localized Scratch Space & Zero Cross-Pollution**:
   - Subprojects manage their own localized scratch spaces (such as gitignored `./data/` and `./tmp/` subdirectories).
   - Global repository-level data directories are prohibited to prevent race conditions during concurrent test runs.

4. **Separation of Hermetic Unit Tests and Local Inference Integration Tests**:
   - Fast, hermetic unit tests execute by default via standard build targets (`make test`).
   - Integration tests that require local hardware inference engines or container runtimes are isolated behind Go build tags (`//go:build integration`) and invoked explicitly (`make integration-test`).
   - CI pipelines execute unit test suites across subprojects in parallel, ensuring fast, deterministic passes without external infrastructure dependencies.

## Consequences

### Positive
- **Clear Architectural Boundaries**: Prevents presentation frameworks and UI paradigms from polluting engine core logic.
- **Fast, Independent Feedback Loops**: CI and local workflows run subproject unit tests concurrently without contention.
- **Extensible Integration Surface**: New presentation layers, language bindings (e.g. C ABI), or sidecar daemons can be added as isolated subprojects without destabilizing existing binaries.
- **Hermetic Testing**: Default test execution remains hermetic and fast across all environments.

### Negative
- **Workspace Tooling Requirements**: Requires Go workspace (`go.work`) support across developer and CI environments.
- **Multi-Module Maintenance**: Coordinated API changes across `liblokol` and consumer binaries require updating multiple `go.mod` definitions.
