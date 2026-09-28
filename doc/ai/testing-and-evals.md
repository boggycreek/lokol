---
name: lokol-testing-and-evals
description: >-
  Testing hierarchy, colocated unit tests, 10-tier integration benchmark suite, Laya semantic judging on CPU,
  Podman container test sandboxing, and capability probe regression tracking.
  Use when writing unit tests, running integration evaluations, or adding capability probes.
---

# Testing & Evaluation Hierarchy

## Test Execution Hierarchy
- **Subproject Colocated Unit Tests** (`make test`): Fast, hermetic, pure Go unit tests colocated alongside source files (`*_test.go`) across all subprojects ([ADR 0022](../adr/0022-colocated-unit-testing-and-integration-hierarchy.md)). Never requires a running LLM.
- **Subproject Integration Tests** (`make integration-test`): Multi-turn non-interactive integration suites located in each subproject's `test/` directory (`cmd/lk/test/`, `cmd/lokol/test/`, `liblokol/test/`). Requires local `llama-server` on `http://127.0.0.1:8080`.
- **Diagnostic Drivers (`tools/*_driver/`)**: Standalone developer harnesses (e.g. `tools/core_driver`) for live probing and parameter-driven scenario evaluation.

## Graded 10-Tier Integration Benchmark Suite (`liblokol/test/eval_test.go`)
- Progressively tests autonomous capabilities across 10 difficulty tiers ([ADR 0013](../adr/0013-graded-multi-tier-agent-evaluations.md)):
  - **Tiers 1–3**: Tool adherence, directory inspection, bug fix & test loops (`run_test` + `replace_file`).
  - **Tiers 4–6**: Bounded reading, AST outline discovery, git init/commit sequences.
  - **Tiers 7–9**: Multi-turn feature addition, targeted window edits in large files.
  - **Tier 10**: Architectural overview evaluated via Laya decision model.

## Non-Autoregressive Decision Model Judging (Laya)
- **CPU Execution**: Scored using Convai's Laya model (`tools/laya/judge.py`) bridged through `liblokol/test/laya_judge_test.go` ([ADR 0014](../adr/0014-non-autoregressive-decision-model-judging.md)).
- **Sub-50ms Latency**: Executes in ~33ms on host CPU via AVX2 instructions without generative judge variance or GPU memory contention ([ADR 0020](../adr/0020-cpu-avx2-offload-strategy-for-auxiliary-decision-models.md)).
- **Health Check**: `uv run --python .venv tools/laya/judge.py --health`.

## Containerized Test Sandboxing via Rootless Podman
- **Host State & Beads Protection**: Multi-turn evaluations on scoped test projects must run within a rootless Podman container (`cmd/lokol/test/podman_sandbox_test.go`) ([ADR 0016](../adr/0016-containerized-test-sandboxing-via-podman.md)).
- **Zero Host Memory Pollution**: Host `.beads/` and memories are protected by setting `BEADS_DIR=/workspace/.beads` and `GIT_CEILING_DIRECTORIES` so tools cannot discover host databases.
- **Inference Networking**: Containers connect via `--network=host` to access local `llama-server` on `127.0.0.1:8080`.

## Capability Probe Suite & Regression Tracking
- **Data-Driven Scenarios**: Maintained as version-controlled data files in `tools/scenarios/scenarios.jsonl`.
- **Regression Isolation**: Negative findings discovered during live exploration or agent sessions must be distilled into immutable records in `tools/scenarios/` for automated verification via `./bin/core_driver -s all`.

Further reading:
- [ADR 0013 — Graded Multi-Tier Integration Benchmark Suite](../adr/0013-graded-multi-tier-agent-evaluations.md)
- [ADR 0014 — Non-Autoregressive Decision Models for Semantic Evaluation](../adr/0014-non-autoregressive-decision-model-judging.md)
- [ADR 0015 — Auxiliary Developer Tooling Isolation via Virtual Environments](../adr/0015-auxiliary-tooling-isolation-via-uv.md)
- [ADR 0016 — Containerized Test Sandboxing for Host Memory and State Isolation](../adr/0016-containerized-test-sandboxing-via-podman.md)
- [ADR 0020 — CPU AVX2 Offload Strategy for Auxiliary Decision Models](../adr/0020-cpu-avx2-offload-strategy-for-auxiliary-decision-models.md)
- [ADR 0022 — Colocated Unit Testing and Integration Hierarchy](../adr/0022-colocated-unit-testing-and-integration-hierarchy.md)
