# Agent Instructions

This project uses **bd** (beads) for issue tracking. Run `bd prime` for full workflow context.

> **Architecture in one line:** Issues live in a local Dolt database
> (`.beads/dolt/`); cross-machine sync uses `bd dolt push/pull` (a
> git-compatible protocol), stored under `refs/dolt/data` on your git
> remote — separate from `refs/heads/*` where your code lives.
> `.beads/issues.jsonl` is a passive export, not the wire protocol.
>
> See [sync-concepts](https://github.com/gastownhall/beads/blob/main/docs/core-concepts/sync-concepts.md)
> for the one-screen overview and anti-patterns (don't treat JSONL as the
> source of truth; don't `bd import` during normal operation; don't
> reach for third-party Dolt hosting before trying the default).

## Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work atomically
bd close <id>         # Complete work
bd dolt push          # Push beads data to remote
```

## Bead Workflow: Filing vs. Implementation Authorization

**CRITICAL RULE:** Distinguish between submitting a bead for future work and receiving permission to proceed with implementation.

1. **Submitting / Filing Work (`bd create`)**:
   - When the user asks to create or submit a bead (e.g., "New bead: ...", "File a bead for ...", "Track this requirement ..."), create the bead in `bd` with title, description, priority, and relevant labels.
   - **DO NOT** claim (`bd update <id> --claim`) or immediately begin implementing the requirements.
   - Report the created bead ID and summary, then wait for prioritization or explicit instruction.
2. **Authorization to Implement**:
   - Only claim and begin implementing code changes when the user explicitly instructs you to proceed (e.g., "claim and implement <id>", "go ahead and work on that", "let's do that now").
   - Creating a bead is a backlog/task-tracking operation, NOT authorization to execute.

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:46cd31e7 -->
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
   bd dolt push
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->

<!-- BEGIN BEADS CODEX SETUP: generated by bd setup codex -->
## Beads Issue Tracker

Use Beads (`bd`) for durable task tracking in repositories that include it. Use the `beads` skill at `.agents/skills/beads/SKILL.md` (project install) or `~/.agents/skills/beads/SKILL.md` (global install) for Beads workflow guidance, then use the `bd` CLI for issue operations.

### Quick Reference

```bash
bd ready                # Find available work
bd show <id>            # View issue details
bd update <id> --claim  # Claim work
bd close <id>           # Complete work
bd prime                # Refresh Beads context
```

### Rules

- Use `bd` for all task tracking; do not create markdown TODO lists.
- Run `bd prime` when Beads context is missing or stale. Codex 0.129.0+ can load Beads context automatically through native hooks; use `/hooks` to inspect or toggle them.
- Keep persistent project memory in Beads via `bd remember`; do not create ad hoc memory files.

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/core-concepts/sync-concepts.md for details and anti-patterns.
<!-- END BEADS CODEX SETUP -->

## Agent Evaluation Benchmark Suite & Semantic Testing

When developing agent loop enhancements, tool execution changes, or prompt alterations, use the **10-tier integration evaluation suite** in [`liblokol/test/eval_test.go`](liblokol/test/eval_test.go).

### Running Evaluations & Integration Tests

```bash
# 1. Run all subproject unit tests (fast, hermetic, no inference required)
make test

# 2. Run live integration tests across all subprojects (requires llama-server on http://127.0.0.1:8080)
make integration-test

# 3. Run the live 10-tier benchmark against the local engine (http://127.0.0.1:8080)
go test -v -tags integration ./liblokol/test -run TestEvalSuite_LiveEngine

# 4. Run the loop circuit breaker & oscillation intervention test
go test -v ./liblokol/test -run TestRunner_LoopCircuitBreaker

# 5. Run the sandboxed Podman container evaluation (scoped project purpose evaluation)
go test -v -tags integration ./cmd/lokol/test -run TestPodmanSandbox_ProjectEvaluation

# 6. Test Laya semantic judge health
uv run --python .venv tools/laya/judge.py --health
```

### Evaluation Architecture & Standards

1. **Progressive Tiers (Tiers 1–10)**:
   - **Tier 1**: JSON file creation and schema adherence (`write_file`)
   - **Tier 2**: Directory inspection and counting (`read_outline` / `exec_bash` / `write_file`)
   - **Tier 3**: Bug fix and test verification loop (`run_test` + `replace_file`)
   - **Tier 4**: Bounded window line inspection (`read_window`)
   - **Tier 5**: AST outline discovery (`read_outline`)
   - **Tier 6**: Git repository init, config, add, and commit (`exec_bash`)
   - **Tier 7**: Directory summarization grounding (`overview.md`)
   - **Tier 8**: Multi-turn feature addition (`read_outline` -> `read_window` -> `replace_file` -> `run_test`)
   - **Tier 9**: Targeted window edits in large files (`replace_file`)
   - **Tier 10**: Architectural overview with **Laya decision model** evaluation
2. **Containerized Sandboxing via Podman ([ADR-0016](doc/adr/0016-containerized-test-sandboxing-via-podman.md))**:
   - Tests evaluating agent autonomy on scoped test projects must run within a rootless Podman container (`cmd/lokol/test/podman_sandbox_test.go`).
   - **Zero Host Memory Pollution**: Host `.beads/` and memories are strictly protected by setting `BEADS_DIR=/workspace/.beads` and `GIT_CEILING_DIRECTORIES` so tools cannot discover host databases.
   - **No Root Binaries**: Never compile binaries into the repository root. Test binaries are built into `t.TempDir()` and mounted read-only.
   - **Inference Networking**: Containers use `--network=host` to access local `llama-server` on `127.0.0.1:8080`.
3. **Deterministic Ground-Truth Checks**:
   - Tests execute in isolated temporary directories (`t.TempDir()`).
   - Verifications validate physical file modifications, JSON unmarshaling, or `go test` exit codes rather than fuzzy string matching.
4. **Semantic Scoring via Laya**:
   - Subjective/semantic artifacts (e.g. `ARCHITECTURE.md` in Tier 10) are scored using Convai's Laya model (`tools/laya/judge.py`) bridged through `liblokol/test/laya_judge_test.go`.
   - Laya executes in ~33ms, returning calibrated probability (`noul`) and quality score (`score`).
5. **Extending the Evaluation Suite**:
   - To add a new tier or prompt scenario, append a `BenchmarkCase` struct to `cases` in [`liblokol/test/eval_test.go`](liblokol/test/eval_test.go):
     ```go
     BenchmarkCase{
         Name:     "TierX_ScenarioName",
         Prompt:   "Clear instruction to the agent, specifying tools to use, and ending with 'then finish.'",
         MaxTurns: 10,
         Setup:    func(t *testing.T, dir string) { /* prepare files in dir */ },
         Verify:   func(t *testing.T, dir string) error { /* assert state */ },
     }
     ```
6. **Python Dev Tooling Policy**:
   - All Python tools must live in distinct subdirectories under `./tools/*` (e.g., `./tools/laya/`).
   - Use `uv` with PEP 723 metadata (`uv run --python .venv tools/...`). Never run system-wide `pip install`.
7. **Local Developer Artifacts**:
   - Benchmark reports are automatically saved to subproject gitignored `liblokol/data/eval_results.json`.
8. **Code/Data Separation & Scenario Repeatability**:
   - Evaluation scenarios, capability probes, and regression test cases must be maintained as data files (e.g. JSONL in `tools/scenarios/scenarios.jsonl`), separated from harness execution logic.
   - Negative findings discovered during live exploration or agent sessions must be distilled into immutable, version-controlled records in the scenario dataset for repeatable CI regression tracking.
9. **Test Suites vs. Diagnostic Drivers (`tools/*_driver`)**:
   - **Subproject E2E Tests**: Pure automated, non-interactive end-to-end integration tests validating a specific subproject belong in that subproject's `test/` directory (e.g., `cmd/lokol/test/`, `cmd/lk/test/`, `liblokol/test/`) and run in CI via `make integration-test`.
   - **Operator Diagnostic Drivers**: Standalone interactive or parameter-driven diagnostic harnesses for developers and autonomous evaluation live under `tools/` and follow the `*_driver` naming pattern (e.g., `tools/core_driver/`, `tools/cli_driver/`, `tools/tui_driver/`).

## Architecture Decision Records (ADR) Standards

When authoring, amending, or proposing Architecture Decision Records in `doc/adr/`:

1. **Strictly Single-Topic ONLY**:
   - Each ADR must address exactly **one** architectural decision.
   - Never combine orthogonal concerns into a single ADR (e.g., avoid compound titles with "and" joining separate topics).
2. **Audit Existing ADRs First**:
   - Always check `doc/adr/` before creating a new ADR to verify whether the architectural decision has already been accepted.
   - Do not create new ADRs for developer tooling, test harnesses, or submodules that are simply concrete implementations of already accepted architectural decisions (e.g., evaluations are covered by ADR 0013, decision models by ADR 0014, auxiliary tooling by ADR 0015).
3. **What and Why, No How**:
   - Express only the context/motivation (why), the architectural decision (what), and the trade-offs (consequences).
   - **NEVER include perishable implementation details that go stale**:
     - No code snippets, programming language types, structs, or function signatures.
     - No transient microbenchmarks or execution latency numbers.
     - No hardcoded source file paths, internal package names, or transient CLI flag names.
     - No tool schemas, payload XML tags, or network endpoint routes.

