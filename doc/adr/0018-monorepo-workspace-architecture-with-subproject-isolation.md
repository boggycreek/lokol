# ADR 0018: Monorepo Workspace Architecture with Subproject Isolation

## Status
Accepted

## Date
2026-09-27

## Context
As `lokol` evolves from an early prototype into a production-grade local autonomous AI platform, its component surface spans multiple functional domains:
1. **Core Autonomous Engine (`liblokol`)**: Pure headless agent loop, prompt composition, token streaming, context cancellation, tool dispatching, context refinery primitives, and MCP server protocol.
2. **Interactive Developer TUI (`lk`)**: Dedicated, high-speed terminal interface built on Bubble Tea, providing token velocity HUD, interactive action approvals, viewport wrapping, and streamlined developer workflow.
3. **Administrative & Configuration CLI (`lokol`)**: System management binary providing hardware capability probing (`lokol probe`), dependency and weight setup (`lokol setup`), release updating (`lokol update`), and headless execution.
4. **Standalone MCP Facade (`lokol-mcp`)**: Dedicated stdio JSON-RPC server exposing deterministic context refinery tools to foreign agents and IDEs.
5. **Future Consumers**: In-process shared libraries (`liblokol.so` for SDL/C/Rust clients via ADR-0017) and local IPC daemons (`lokol-daemon`).

Maintaining all components in a single monolithic Go module introduces several structural drawbacks:
- Presentation code (e.g. Bubble Tea dependencies) mixes with core engine primitives.
- Integration tests risk polluting global directories (such as a shared `./data` or `./tmp`).
- Shared state risks subtle coupling and circular dependencies across components.
- Subproject builds cannot be run or packaged independently.

We need a monorepo workspace architecture that enforces strict component boundaries, independent dependency management, and total artifact isolation.

## Decision
We organize the repository as a **Go Monorepo Workspace** (`go.work`) with self-contained subprojects:

```
lokol/
├── go.work                         # Workspace linking subprojects during development
├── Makefile                        # Root orchestrator delegating to subproject Makefiles
├── .gitignore                      # Universal repository ignore rules
│
├── liblokol/                       # Subproject 1: Core Headless Agent SDK
│   ├── go.mod                      # module github.com/boggycreek/lokol/liblokol
│   ├── Makefile                    # Targets: build, test, clean, lint
│   ├── .gitignore                  # Ignores local ./data/, ./tmp/, coverage
│   ├── pkg/
│   │   ├── agent/                  # Session, Client, Runner, DispatchAction
│   │   ├── refinery/               # find_files, search_code, git_diff_summary, read_window, etc.
│   │   ├── mcp/                    # MCP server primitives & JSON-RPC
│   │   ├── probe/                  # Hardware & GPU capability probing
│   │   ├── model/                  # Hardware sizing & model matrix
│   │   ├── setup/                  # Environment & dependency verification
│   │   ├── update/                 # Release updater
│   │   └── version/                # Build version metadata
│   └── test/                       # Core engine & refinery unit/integration tests
│
├── cmd/
│   ├── lk/                         # Subproject 2: Interactive TUI Launcher
│   │   ├── go.mod                  # module github.com/boggycreek/lokol/cmd/lk
│   │   ├── Makefile                # Produces bin/lk
│   │   ├── .gitignore              # Ignores local ./data/, ./tmp/
│   │   ├── main.go                 # Fast TUI entrypoint
│   │   ├── pkg/tui/                # Bubble Tea model, view, HUD, styles
│   │   └── test/                   # TUI presentation & approval flow tests
│   │
│   ├── lokol/                      # Subproject 3: Admin & Configuration CLI
│   │   ├── go.mod                  # module github.com/boggycreek/lokol/cmd/lokol
│   │   ├── Makefile                # Produces bin/lokol
│   │   ├── .gitignore              # Ignores local ./data/, ./tmp/
│   │   ├── main.go                 # Subcommands: probe, setup, update, exec, version
│   │   └── test/                   # CLI command tests
│   │
│   └── lokol-mcp/                  # Subproject 4: Standalone MCP Server
│       ├── go.mod                  # module github.com/boggycreek/lokol/cmd/lokol-mcp
│       ├── Makefile                # Produces bin/lokol-mcp
│       ├── .gitignore              # Ignores local ./data/, ./tmp/
│       ├── main.go                 # JSON-RPC stdio server entrypoint
│       └── test/                   # MCP protocol pipe tests
```

### Architectural Principles

1. **Subproject Self-Containment**:
   - Every subproject has its own `go.mod`, its own `Makefile`, and its own `.gitignore`.
   - Each subproject can be built (`make build`), tested (`make test`), and cleaned (`make clean`) directly from its own directory without relying on root context.

2. **Dedicated Local Artifacts & Zero Cross-Pollution**:
   - Each subproject uses its own gitignored `./data/` or `./tmp/` directory for developer logs, test database instances, and scratch artifacts.
   - Subprojects are strictly prohibited from writing to or expecting a global shared root `./data/` or `./tmp/` directory.

3. **Presentation & Core Decoupling**:
   - `liblokol` has zero terminal or UI dependencies (no Bubble Tea, lipgloss, or curses).
   - `cmd/lk` and `cmd/lokol` consume `liblokol` as clean module clients.
   - Separation of `lk` (fast interactive agent) from `lokol` (system administration and configuration).

4. **Independent Test Harnesses**:
   - Presentation tests in `cmd/lk/test` mock the agent core via `agent.SessionCore`.
   - Engine and refinery tests in `liblokol/test` test state transitions, streaming, and tool execution without presentation dependencies.

## Consequences

### Positive
- **Clear Separation of Concerns**: Developers working on UI rendering in `cmd/lk` cannot accidentally leak UI concerns into `liblokol`.
- **Fast Build Times**: Changes to TUI code do not trigger recompilations or test re-runs for `liblokol`.
- **Zero Test Collisions**: Running tests across subprojects concurrently is completely race-free due to localized `./data` and `./tmp` directories.
- **Maximal Extensibility**: Paves the way for foreign language bindings (`bindings/c/` via ADR-0017) and headless daemons (`cmd/lokol-daemon`) as independent subprojects.

### Negative
- Requires maintaining multiple `go.mod` files and a root `go.work` file.
- Relative module replacement in `go.work` requires developers to run Go 1.18+ with workspace support (standard across modern toolchains).
