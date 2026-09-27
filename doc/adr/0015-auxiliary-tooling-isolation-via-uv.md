# ADR 0015: Auxiliary Developer Tooling Isolation via uv and Segregated Subdirectories

## Status
Accepted

## Date
2026-09-27

## Context
While the core `lokol` agent runtime and binary are strictly implemented in pure Go ([ADR-0001](0001-implementation-language-and-architecture.md)), advanced developer tooling, evaluation harnesses, and ML model judges (such as Laya in [ADR-0014](0014-non-autoregressive-decision-model-judging.md)) often require Python or specialized scripting environments.

Unregulated Python scripts in a Go repository introduce friction:
- Global `pip install` commands fail or trigger PEP 668 errors on modern Linux systems (e.g. Ubuntu 24.04 externally-managed environments).
- Ad-hoc scripts scattered in root directories or test folders cause dependency conflicts and clutter.
- Storing transient test data, logs, and evaluation run artifacts inside source directories risks accidental git commits.

We need a clear architectural policy for managing auxiliary developer tooling, scripts, and local developer artifacts without polluting the Go codebase.

## Decision
We establish a standard policy for all auxiliary developer tooling and test artifacts:

```
lokol/
├── cmd/
├── pkg/
├── test/
│   ├── eval_test.go          # Evaluation suite driver (Go)
│   └── laya_judge_test.go    # Subprocess bridge to tools/laya
├── tools/                    # Dedicated home for auxiliary dev tooling
│   └── laya/
│       └── judge.py          # Standalone PEP 723 script
├── data/                     # Gitignored local developer artifacts
│   └── eval_results.json     # Persisted test run reports
└── .gitignore                # data/, .venv/, __pycache__/ ignored
```

### 1. Segregation Under `./tools/<toolname>/`
All non-Go developer tools, evaluators, and maintenance scripts must live in distinct subdirectories under `./tools/*` (e.g. `./tools/laya/`). No Python scripts or virtual environments are allowed in the repository root or package source directories.

### 2. Standardization on `uv` for Python Execution
All Python tooling must be managed through `uv`:
- Scripts declare dependencies inline using PEP 723 metadata (`# /// script ... # ///`).
- Scripts are executed using `uv run --python .venv tools/<toolname>/<script>.py`.
- No global `pip install` or `--break-system-packages` commands are used.
- The virtual environment (`.venv/`), bytecode caches (`__pycache__/`, `*.pyc`), and environments are tracked in `.gitignore`.

### 3. Gitignored `./data/` for Developer Artifacts
Evaluation test runs, benchmark histories, and diagnostic logs for local developers are stored in a dedicated `./data/` directory at the repository root:
- The `./data/` directory is gitignored.
- Benchmark drivers (such as `test/eval_test.go`) write structured JSON reports (`data/eval_results.json`) if the directory is present, providing persistent history across test iterations without dirtying git status.

## Consequences

### Positive
- **Clean Go Ecosystem**: The core Go project remains pristine, fast to build, and free of foreign runtime artifacts.
- **Hermetic & Reproducible**: `uv` guarantees fast, reproducible virtual environment management and package resolution without system-level side effects.
- **Zero Accidental Commits**: All developer output artifacts, test run dumps, and virtual environments are reliably gitignored.

### Negative / Trade-offs
- Developers who wish to run Laya semantic evaluation tiers locally must have `uv` installed on their path (the evaluation suite detects presence and skips gracefully if unavailable).
