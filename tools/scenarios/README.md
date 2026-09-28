# Scenarios Dataset Repository

This directory serves as the centralized, version-controlled repository for evaluation scenarios, capability probes, and regression test suites across the `lokol` project.

Per [ADR-0013](../../doc/adr/0013-graded-multi-tier-agent-evaluations.md), [ADR-0015](../../doc/adr/0015-auxiliary-tooling-isolation-via-uv.md), and [ADR-0022](../../doc/adr/0022-colocated-unit-testing-and-integration-hierarchy.md), **scenario data is strictly separated from test harness execution logic**. Execution engines (`tools/core_driver`, `tools/probe`, future `tools/cli_driver`, and subproject tests) read these datasets to execute deterministic assertions, hardware telemetry supervisory loops, and semantic Laya evaluations.

Unlike developer run-logs and benchmark reports (which are gitignored in local `*/data/` directories), scenario datasets are **tracked in git as authoritative ground truth**.

---

## Files & Partitioning

| File | Purpose | Primary Runner |
| :--- | :--- | :--- |
| [`scenarios.jsonl`](scenarios.jsonl) | Canonical capability probe & regression suite | `tools/core_driver`, `tools/probe` |

As the test corpus expands, scenarios may be partitioned into purpose-specific datasets:
- `capability_probes.jsonl`: Fundamental agent self-awareness, grounding, and recovery.
- `regressions.jsonl`: Closed-loop regressions derived from resolved Beads bug reports.
- `coding_evals.jsonl`: Multi-turn code generation, refactoring, and test-driven fixes.

---

## JSONL Schema Specification

Each line in a scenario dataset is a standalone JSON object adhering to this schema:

```json
{
  "id": "probe_directory_recovery",
  "name": "Directory Inspection Tool Recovery",
  "prompt": "Inspect what is in the current directory.",
  "mode": "general",
  "max_wall_clock_sec": 20.0,
  "max_gpu_pegged_sec": 10.0,
  "laya_instructions": "The agent should discover workspace files using find_files or get_environment, and must not loop attempting to read_window on a directory.",
  "laya_threshold": 0.5,
  "forbidden_substrings": ["read .: is a directory"],
  "required_substrings": [],
  "description": "Verifies that the agent picks find_files or recovers cleanly from directory inspections without repeating invalid actions."
}
```

### Field Definitions

| Field | Type | Description |
| :--- | :--- | :--- |
| `id` | `string` | Unique alphanumeric identifier (`[a-z0-9_]+`) used for CLI targeting (`-s <id>`). |
| `name` | `string` | Human-readable title for console summaries and reports. |
| `prompt` | `string` | Initial operator instruction passed to the agent. |
| `mode` | `string` | Agent operational mode (`general`, `coding`, or `moe`). |
| `max_wall_clock_sec`| `float` | Hard wall-clock timeout enforced by the supervisory watchdog. |
| `max_gpu_pegged_sec`| `float` | Maximum consecutive seconds of GPU saturation permitted before intervention. |
| `laya_instructions` | `string` | Natural language evaluation criteria evaluated by the Laya decision model. |
| `laya_threshold`    | `float` | Minimum calibrated acceptance probability (`noul >= threshold`). |
| `forbidden_substrings` | `[]string` | Exact substrings that fail the run if present in stdout/logs (e.g. error traces). |
| `required_substrings`  | `[]string` | Exact substrings that must appear in stdout/logs for verification to pass. |
| `description`       | `string` | Detailed rationale and intended architectural invariant tested. |

---

## Authoring Guidelines

1. **Deterministic Ground-Truth Checks**: Pair semantic Laya criteria with deterministic `forbidden_substrings` (e.g., OS errors, cloud AI refusal boilerplate, unhandled panics) to catch hard regressions instantly.
2. **Reproducibility**: Prompts must be self-contained or explicitly state workspace assumptions.
3. **Closed-Loop Bead Regression Tracking**: When fixing loop defects or agent probe failures, verify that a corresponding scenario entry exists in this directory to prevent future regressions.
