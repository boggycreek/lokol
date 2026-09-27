# Project Instructions for AI Agents

This file provides instructions and context for AI coding agents working on this project.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:1105d646 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/core-concepts/sync-concepts.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->


## Build & Test

```bash
# Build all subprojects (lk, lokol, lokol-mcp)
make build

# Run tests across all subprojects
make test

# Build or test specific subprojects
make -C cmd/lk build
make -C cmd/lokol build
make -C cmd/lokol-mcp build
make -C liblokol test

# Run the 10-tier graded live evaluation suite (requires llama-server at http://127.0.0.1:8080)
go test -v ./liblokol/test -run TestEvalSuite_LiveEngine

# Run loop circuit breaker and repetition intervention tests
go test -v ./liblokol/test -run TestRunner_LoopCircuitBreaker

# Run sandboxed Podman container evaluation (scoped project evaluation)
go test -v ./cmd/lokol/test -run TestPodmanSandbox_ProjectEvaluation

# Check Laya semantic evaluator health
uv run --python .venv tools/laya/judge.py --health
```

## Architecture Overview

`lokol` is structured as a Go monorepo orchestrated via `go.work`:

- **SDK & Core Agent Engine (`liblokol/`)**:
  - `agent/`: Deterministic agent loop (`runner.go`), SSE streaming client (`client.go`), action parser (`tools.go`), and session contract (`session.go`).
  - `refinery/`: Mechanical filtering: AST outlines (`read_outline`), bounded windows <= 120 lines (`read_window`), search and diff tools (`find_files`, `search_code`, `git_diff_summary`), and 1-line test failure assertions (`run_test`).
  - `mcp/`: Pure-Go Model Context Protocol stdio JSON-RPC server implementation.
  - `model/`: VRAM tiering and hardware model matrix sizing.
  - `probe/`: Hardware and GPU capability detection (NVIDIA VRAM, CPU vector flags).
  - `setup/`: Environment bootstrap and dependency checking.
  - `update/`: Self-updater fetching signed GitHub releases.
  - `version/`: Build-time version metadata injected via `-ldflags`.
  - `test/`: Integration test suite, 10-tier evaluation benchmark, and Laya bridge.
- **Interactive TUI Launcher (`cmd/lk/`)**:
  - Dedicated Bubble Tea interactive terminal UI (`bin/lk`) with live context HUD, prompt history, viewport wrapping, and hardware status.
- **Admin & Configuration CLI (`cmd/lokol/`)**:
  - Administrative CLI tool (`bin/lokol`) handling `setup`, `probe`, `exec` (headless agent runner), `update`, and `version`.
- **In-Repo MCP Server (`cmd/lokol-mcp/`)**:
  - Standalone stdio JSON-RPC MCP server binary (`bin/lokol-mcp`) exposing refinery tools to Claude Code, Codex, and Cursor.

## Conventions & Patterns

1. **Subproject Isolation**:
   - Each subproject maintains its own `Makefile` (`build`, `test`, `clean`, `lint`), local `.gitignore`, and isolated `data/` and `tmp/` directories.
   - Root `Makefile` orchestrates across subprojects (`liblokol`, `cmd/lk`, `cmd/lokol`, `cmd/lokol-mcp`).
   - Never write to a global root `./data/` or `./tmp/`.
2. **Tool Invocation Protocol**: Actions use XML tags: `<action name="tool_name">...</action>`. Exactly one action per model turn.
3. **Deterministic Workspace Isolation & Container Sandboxing**: All tests operate within isolated temporary directories (`t.TempDir()`). Autonomous agent evaluations on scoped projects must execute in Podman container sandboxes ([ADR-0016](doc/adr/0016-containerized-test-sandboxing-via-podman.md)). Never build binaries into the repository root.
4. **Internalized Inferences**: The agent internalizes intermediate reasoning and tool progress. Standard output is reserved for clean completions, while intermediate tool steps route to stderr or transient TUI status.
5. **Python Developer Tooling**:
   - Must live in dedicated subdirectories under `./tools/*` (e.g. `./tools/laya/`).
   - Managed strictly via `uv` with PEP 723 inline script metadata. Never install to global Python.
6. **Non-Interactive Commands**: Always use non-interactive flags (`cp -f`, `rm -rf`, `apt-get -y`, `HOMEBREW_NO_AUTO_UPDATE=1`) to prevent hangs.
