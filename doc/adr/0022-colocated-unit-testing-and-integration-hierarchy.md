---
adr: "0022"
title: "Colocated Unit Testing and Integration Hierarchy"
topic: "Evaluations & Verification"
theme: "THEME-QUALITY"
status: "accepted"
version: "v0.1.0-alpha"
as_built: true
tags:
  - testing
  - colocated-tests
  - test-hierarchy
  - go-idioms
executive_summary: "Standardizes idiomatic colocated unit tests (*_test.go) within subproject packages, reserving subproject test/ directories exclusively for multi-turn integration and end-to-end suites."
---

# ADR 0022: Colocated Unit Testing and Integration Hierarchy

## Status
Accepted

## Date
2026-09-27

## Context
During initial project bootstrapping, test files were centralized in dedicated `test/` subdirectories within each subproject (such as `liblokol/test/`, `cmd/lk/test/`, `cmd/lokol/test/`, and `cmd/lokol-mcp/test/`). While this approach kept package source directories free of test files, it created several engineering and architectural challenges:

1. **Divergence from Go Idioms**: The standard and widely accepted custom in Go is to place unit test files (`*_test.go`) directly alongside the source files they test within the same package directory.
2. **Obscured Test Coverage & Friction**: Colocated unit tests provide immediate visibility into test coverage and behavioral assertions during feature development and refactoring. Storing unit tests in a remote directory discouraged continuous localized testing (e.g., `go test ./liblokol/regulator/...`).
3. **Blurred Boundary Between Unit and Integration Tests**: Mixing fast, hermetic unit tests with heavy integration benchmarks (such as the 10-tier evaluation suite, live inference tests, and Podman sandboxes) inside the same `test/` folder created ambiguity around test dependencies, execution time, and build tagging.

## Decision

We establish a clear hierarchy separating **colocated unit tests** from **dedicated integration and evaluation suites**:

### 1. Colocated Unit Tests (`*_test.go` Alongside Source)
- All hermetic unit tests must reside directly in the directory of the package they test:
  - `liblokol/agent/*_test.go` alongside `liblokol/agent/*.go`
  - `liblokol/regulator/*_test.go` alongside `liblokol/regulator/*.go`
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

### 2. Subproject Integration & E2E Suites (`**/test/` Subdirectories)
- Dedicated `test/` subdirectories (`liblokol/test/`, `cmd/lokol/test/`, `cmd/lk/test/`) are reserved **exclusively** for automated, non-interactive integration benchmarks and end-to-end test suites scoped to that subproject:
  - Subproject-specific end-to-end assertions (such as container sandboxes, live server communication, or compiled CLI/TUI pass/fail regression suites) must reside within the respective subproject's `test/` directory.
  - Pure automated E2E verification suites must not reside in the general developer tooling directory if their purpose is deterministic pass/fail quality gating of a specific subproject.
- **Build Tag Enforcement**: Any test in a `test/` directory requiring external infrastructure, local inference engines, or container runtimes must include `//go:build integration`.

### 3. Operator Diagnostic & Probing Drivers (`tools/*_driver/`)
- Standalone diagnostic, probing, and interactive testing harnesses are segregated under the dedicated tooling directory and follow the `*_driver` naming pattern (e.g., `core_driver`, `cli_driver`, `tui_driver`):
  - **Operational & Interactive Role**: Unlike automated test suites in `**/test/` that execute assertions and terminate, `*_driver` utilities are active operational tools providing interactive REPLs, repetition loops for determinism evaluation, configurable telemetry monitoring, and semantic model judge integrations.
  - **Cross-Cutting Evaluation**: Drivers may orchestrate cross-boundary evaluations, ingest version-controlled capability scenario datasets, or bridge external evaluation models to stress-test agent behavior.

### 4. Build & CI Target Separation
- Standard testing (`make test` or `go test ./...`) traverses the package tree and executes all colocated unit tests hermetically and concurrently in CI.
- Integration testing (`make integration-test`) executes automated E2E and integration suites within `**/test/` directories when target inference environments are provisioned.
- Diagnostic drivers are invoked explicitly on demand by operators or autonomous benchmark workflows (`make test-core-driver`, `go run ./tools/core_driver`, etc.).

## Consequences

### Positive
- **Idiomatic Go Alignment**: Conforms to standard Go conventions and tooling expectations (`go test ./<subpackage>/...`).
- **Enhanced Developer Ergonomics**: Unit tests are immediately discoverable and maintainable side-by-side with package code.
- **Clear Architectural Taxonomy**: Subproject E2E tests have a distinct home (`**/test/`) separate from interactive developer/operator driver tools (`tools/*_driver`).
- **Clean Architectural Separation**: Dedicated `test/` directories are no longer catch-alls; they represent explicit, heavy integration and evaluation boundaries.
- **Deterministic CI Execution**: Fast unit test suites across all monorepo modules run reliably without external service dependencies.

### Negative
- Source package directories contain both implementation files and test files (standard across the Go ecosystem).
