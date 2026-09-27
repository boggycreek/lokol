# ADR 0015: Auxiliary Developer Tooling Isolation via Virtual Environments

## Status
Accepted

## Date
2026-09-27

## Context
While the core agent runtime and binary are strictly implemented in pure Go ([ADR-0001](0001-implementation-language-and-architecture.md)), advanced developer tooling, evaluation harnesses, and ML model judges ([ADR-0014](0014-non-autoregressive-decision-model-judging.md)) often require secondary runtime environments such as Python.

Unmanaged auxiliary scripts in a compiled systems repository introduce friction:
- Global package installations conflict with host package managers and break in externally managed environments.
- Ad-hoc scripts scattered across source or test directories cause dependency conflicts and codebase clutter.
- Storing transient test data, logs, and evaluation run artifacts inside source directories risks accidental repository pollution.

We need a clear architectural policy for managing auxiliary developer tooling, scripts, and local developer artifacts without polluting the core codebase.

## Decision
We establish a standard architectural policy for all auxiliary developer tooling and test artifacts:

### 1. Segregation Under Dedicated Tool Subdirectories
All non-Go developer tools, evaluators, and maintenance scripts must live in distinct subdirectories under a dedicated tooling directory. No auxiliary scripts or virtual environments are allowed in the repository root or Go package source directories.

### 2. Isolated Virtual Environment Management
All auxiliary tooling dependencies must be managed through isolated, ephemeral virtual environments:
- Tool scripts declare explicit dependency contracts within their own tooling boundary.
- Global package installations are strictly prohibited.
- Virtual environments and bytecode caches are excluded from version control.

### 3. Dedicated Transient Artifact Directory
Evaluation test runs, benchmark histories, and diagnostic logs are stored in a dedicated, gitignored developer data directory at the repository root:
- The transient data directory is excluded from version control.
- Test and benchmark runners may write structured reports to this directory to provide persistent history across iterations without dirtying the working tree.

## Consequences

### Positive
- **Clean Core Ecosystem**: The core runtime remains pristine, fast to build, and free of foreign runtime artifacts.
- **Hermetic & Reproducible**: Guarantees reproducible virtual environment management and package resolution without host system side effects.
- **Zero Accidental Commits**: All developer output artifacts, test run dumps, and virtual environments are reliably excluded from version control.

### Negative / Trade-offs
- Developers who execute auxiliary evaluation tiers locally must maintain the auxiliary runtime toolchain.
