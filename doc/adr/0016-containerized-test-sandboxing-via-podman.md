# ADR 0016: Containerized Test Sandboxing via Podman for Host Memory and State Isolation

## Status
Accepted

## Date
2026-09-27

## Context
`lokol` is an autonomous coding agent capable of multi-turn directory inspection, file modification, bash command execution, and test verification.

When running integration tests or evaluations of autonomous agent behavior (such as evaluating the purpose of a repository or executing multi-step tool calls), executing directly on the host machine presents critical boundary risks:
1. **Host Memory & Issue Pollution**: Tools such as Beads (`bd`) and Git discover project state by walking upward through parent directories until finding `.beads/` or `.git/`. If an agent under test executes state-recording commands (e.g. `bd remember`, git commits, or file creation) inside a subfolder, it can unintentionally mutate the host repository's Dolt database, issue graph, or commit history.
2. **Workstation File Contamination**: Rogue binaries or compilation artifacts (such as `lokol` compiled into the repository root) can pollute git status and violate repository hygiene.
3. **Network Requirements for Local LLMs**: The agent under test must still connect to the local inference engine (`llama-server` running on `http://127.0.0.1:8080`) without granting the container access to host files or environment secrets.

We require a standardized, airtight sandboxing strategy for executing autonomous agent evaluations without any possibility of host memory or workspace contamination.

## Decision
We adopt **Podman container sandboxing** for running integration evaluations of the `lokol` agent against scoped test projects.

```
Host Workstation (RTX 3060 / Linux)
├── llama-server (http://127.0.0.1:8080)
├── Host Repository (lokol/) [UNTOUCHED]
│   └── .beads/ (Dolt DB & persistent memories)
│
└── Podman Container Sandbox (`podman run --rm`)
    ├── Network: `--network=host` (connects to llama-server)
    ├── Binary: Mount temp binary to `/usr/local/bin/lokol:ro,Z`
    ├── Workspace: Mount `t.TempDir()` to `/workspace:rw,Z`
    └── Environment Sandboxing:
        ├── `HOME=/tmp`
        ├── `BEADS_DIR=/workspace/.beads`
        └── `GIT_CEILING_DIRECTORIES=/tmp`
```

### 1. Rootless Podman Container Isolation
Agent integration benchmarks that evaluate autonomous reasoning (e.g. [`test/podman_sandbox_test.go`](../../test/podman_sandbox_test.go)) run inside an ephemeral rootless Podman container (`docker.io/library/golang:1.25-bookworm`).

### 2. Isolated Scoped Test Projects
Tests construct self-contained, scoped project workspaces (e.g. a microservice with `go.mod`, `README.md`, `main.go`, and `service.go`) within Go temporary directories (`t.TempDir()`). The directory is mounted into the container as `/workspace:rw,Z`. All file operations, reads, edits, and created files remain strictly within this container boundary.

### 3. Strict Temporary Binary Compilation
Test binaries must **never** be compiled into or written to the repository root. Tests compile `lokol` into a separate, isolated `t.TempDir()` (e.g. `binDir := t.TempDir()`), mount that binary read-only (`-v $binDir/lokol:/usr/local/bin/lokol:ro,Z`), and let the operating system automatically reap the temporary directory when tests complete.

### 4. Memory & Git Ceiling Enforcement
To guarantee that neither the containerized agent nor host subcommands touch host memories:
- Inside the container: Environment variables set `HOME=/tmp` and `BEADS_DIR=/workspace/.beads`.
- On the host runtime ([`pkg/agent/client.go`](../../pkg/agent/client.go) and [`pkg/tools/refinery/refinery.go`](../../pkg/tools/refinery/refinery.go)): `ExecuteBash` and `RunTestVerifier` inject `GIT_CEILING_DIRECTORIES` and `BEADS_DIR` bound to the active `workDir`, stopping tools from traversing upward to host `.git` or `.beads` stores.

### 5. Host Network Binding for Inference
The container uses `--network=host` to route HTTP requests directly to `http://127.0.0.1:8080`. This provides zero-latency access to the local GPU-accelerated `llama-server` instance without exposing any host filesystem mounts.

### 6. Graceful Degradation & Health Probing
If `podman` is not installed or the local `llama-server` engine is unreachable, containerized integration tests gracefully skip (`t.Skip`) so that standard offline CI and unit test suites pass without failure.

## Consequences

### Positive
- **Guaranteed Host Protection**: Host `.beads/` Dolt databases, persistent project memories, and repository files are physically inaccessible to the agent under test.
- **Clean Repository Root**: Prohibits stray binaries or test outputs from ever appearing in the project root.
- **Realistic End-to-End Validation**: Evaluates the compiled `lokol` CLI executable in an authentic Linux POSIX environment with real tools (`bash`, `git`, `go`).
- **Internalized Inference Verification**: Validates that standard output receives only clean final summaries, while intermediate tool steps route to stderr.

### Negative / Trade-offs
- Running containerized tests requires `podman` on the host machine. (Tests automatically skip if Podman is absent).
- Container startup introduces a minor runtime overhead (~1–2s per test execution).
