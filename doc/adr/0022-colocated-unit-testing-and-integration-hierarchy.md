# ADR 0022: Colocated Unit Testing and Integration Hierarchy

## Status
Accepted

## Date
2026-09-27

## Context
During initial project bootstrapping, test files were centralized in dedicated `test/` subdirectories within each subproject (such as `liblokol/test/`, `cmd/lk/test/`, `cmd/lokol/test/`, and `cmd/lokol-mcp/test/`). While this approach kept package source directories free of test files, it created several engineering and architectural challenges:

1. **Divergence from Go Idioms**: The standard and widely accepted custom in Go is to place unit test files (`*_test.go`) directly alongside the source files they test within the same package directory.
2. **Obscured Test Coverage & Friction**: Colocated unit tests provide immediate visibility into test coverage and behavioral assertions during feature development and refactoring. Storing unit tests in a remote directory discouraged continuous localized testing (e.g., `go test ./liblokol/guardrail/...`).
3. **Blurred Boundary Between Unit and Integration Tests**: Mixing fast, hermetic unit tests with heavy integration benchmarks (such as the 10-tier evaluation suite, live inference tests, and Podman sandboxes) inside the same `test/` folder created ambiguity around test dependencies, execution time, and build tagging.

## Decision

We establish a clear hierarchy separating **colocated unit tests** from **dedicated integration and evaluation suites**:

### 1. Colocated Unit Tests (`*_test.go` Alongside Source)
- All hermetic unit tests must reside directly in the directory of the package they test:
  - `liblokol/agent/*_test.go` alongside `liblokol/agent/*.go`
  - `liblokol/guardrail/*_test.go` alongside `liblokol/guardrail/*.go`
  - `liblokol/mcp/*_test.go` alongside `liblokol/mcp/*.go`
  - `liblokol/refinery/*_test.go` alongside `liblokol/refinery/*.go`
  - `liblokol/setup/*_test.go` alongside `liblokol/setup/*.go`
  - `liblokol/update/*_test.go` alongside `liblokol/update/*.go`
  - `cmd/lk/tui/*_test.go` alongside `cmd/lk/tui/*.go`
  - `cmd/lk/cli_test.go` alongside `cmd/lk/main.go`
  - `cmd/lokol/cli_test.go` alongside `cmd/lokol/main.go`
  - `cmd/lokol-mcp/mcp_test.go` alongside `cmd/lokol-mcp/main.go`
- **Testing Style**:
  - Prefer the black-box `package <pkg>_test` convention to test exported public APIs cleanly without circular import risks.
  - Internal package unit testing (`package <pkg>`) is permitted when verifying unexported state transitions or algorithm internals.
- **Hermetic Guarantee**: Colocated unit tests must be hermetic, fast (sub-second execution), and completely free of external runtime dependencies (no network, no live inference server, no container daemon).

### 2. Dedicated Integration & Sandbox Hierarchy (`test/` Subdirectories)
- Dedicated `test/` subdirectories (`liblokol/test/`, `cmd/lokol/test/`) are reserved **exclusively** for high-level integration, evaluation benchmarks, and sandboxed test environments:
  - `liblokol/test/integration_test.go`: Live inference loops communicating with local `llama-server` endpoints.
  - `liblokol/test/eval_test.go`: 10-tier autonomous benchmark evaluation suite.
  - `liblokol/test/laya_judge_test.go`: Semantic scoring using Convai's Laya decision model.
  - `cmd/lokol/test/podman_sandbox_test.go`: Sandboxed container evaluations per ADR-0016.
- **Build Tag Enforcement**: Any test in a `test/` directory requiring external infrastructure, local inference engines, or container runtimes must include `//go:build integration`.
- Subprojects containing only unit and CLI tests (such as `cmd/lk` and `cmd/lokol-mcp`) must not maintain empty or redundant `test/` directories.

### 3. Build & CI Target Separation
- Standard testing (`make test` or `go test ./...`) traverses the package tree and executes all colocated unit tests hermetically and concurrently in CI.
- Integration testing (`make integration-test` or `go test -tags integration ./liblokol/test`) is invoked explicitly when target inference environments are provisioned.

## Consequences

### Positive
- **Idiomatic Go Alignment**: Conforms to standard Go conventions and tooling expectations (`go test ./<subpackage>/...`).
- **Enhanced Developer Ergonomics**: Unit tests are immediately discoverable and maintainable side-by-side with package code.
- **Clean Architectural Separation**: Dedicated `test/` directories are no longer catch-alls; they represent explicit, heavy integration and evaluation boundaries.
- **Deterministic CI Execution**: Fast unit test suites across all monorepo modules run reliably without external service dependencies.

### Negative
- Source package directories contain both implementation files and test files (standard across the Go ecosystem).
