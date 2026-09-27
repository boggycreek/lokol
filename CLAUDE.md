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
# Build the lokol binary
make build

# Run unit and integration tests
make test

# Run the 10-tier graded live evaluation suite (requires llama-server at http://127.0.0.1:8080)
go test -v ./test -run TestEvalSuite_LiveEngine

# Run loop circuit breaker and repetition intervention tests
go test -v ./test -run TestRunner_LoopCircuitBreaker

# Run sandboxed Podman container evaluation (scoped project evaluation)
go test -v ./test -run TestPodmanSandbox_ProjectEvaluation

# Check Laya semantic evaluator health
uv run --python .venv tools/laya/judge.py --health
```

## Architecture Overview

- **Core Agent Loop (`pkg/agent/`)**:
  - `client.go`: Connects to `llama-server`, parses `<action name="...">` delimiters, injects `<environment>` grounding.
  - `runner.go`: Multi-turn orchestrator, turn bounds, oscillation detection, loop intervention nudges, and internalized inferences.
  - `tools.go`: File and shell tool execution (`write_file`, `replace_file`, `exec_bash`, etc.) with strict `WorkDir` isolation and git/beads ceiling barriers.
- **Context Refinery & Facades (`pkg/tools/refinery/`)**:
  - Mechanical filtering: AST outlines (`read_outline`), bounded windows <= 100 lines (`read_window`), and 1-line failure assertions (`run_test`).
- **Evaluation Harness & Laya Decision Model (`test/eval_test.go`, `tools/laya/`)**:
  - 10-tier progressive benchmark evaluating tool use, error recovery, git workflows, and multi-turn refactoring.
  - `tools/laya/judge.py` + `test/laya_judge_test.go`: Non-autoregressive decision model (`convaiinnovations/laya`) scoring semantic correctness in ~33ms.
  - Output reports stored in gitignored `data/eval_results.json`.
- **Containerized Sandbox Testing (`test/podman_sandbox_test.go`, [ADR-0016](doc/adr/0016-containerized-test-sandboxing-via-podman.md))**:
  - Ephemeral rootless Podman container sandboxing for evaluating agent autonomy on scoped test projects.
  - Total host isolation: `BEADS_DIR=/workspace/.beads` and `GIT_CEILING_DIRECTORIES` ensure zero pollution of host Dolt databases or git refs.
  - Binaries compiled strictly into `t.TempDir()` and mounted read-only, preventing rogue binaries in repository root.

## Conventions & Patterns

1. **Tool Invocation Protocol**: Actions use XML tags: `<action name="tool_name">...</action>`. Exactly one action per model turn.
2. **Deterministic Workspace Isolation & Container Sandboxing**: All tests operate within isolated temporary directories (`t.TempDir()`). Autonomous agent evaluations on scoped projects must execute in Podman container sandboxes ([ADR-0016](doc/adr/0016-containerized-test-sandboxing-via-podman.md)). Never build binaries into the repository root.
3. **Internalized Inferences**: The agent internalizes intermediate reasoning and tool progress. Standard output is reserved for clean completions, while intermediate tool steps route to stderr or transient TUI status.
4. **Python Developer Tooling**:
   - Must live in dedicated subdirectories under `./tools/*` (e.g. `./tools/laya/`).
   - Managed strictly via `uv` with PEP 723 inline script metadata. Never install to global Python.
5. **Transient Data & Artifacts**:
   - Store local developer test dumps, benchmark history, and scratch logs in `./data/` (gitignored).
6. **Non-Interactive Commands**: Always use non-interactive flags (`cp -f`, `rm -rf`, `apt-get -y`, `HOMEBREW_NO_AUTO_UPDATE=1`) to prevent hangs.
