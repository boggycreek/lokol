# lokol Capability Probe & Hardware Watchdog Driver

The **Capability Probe Driver** (`tools/probe`) is an automated test harness and telemetry monitor for `lokol`. It simulates an operator probing the agent's self-awareness, hardware grounding, and tool execution boundaries while monitoring real-time GPU compute and engine slot state.

For architectural context, see [ADR 0013: Graded Multi-Tier Benchmark Suite](../../doc/adr/0013-graded-multi-tier-agent-evaluations.md), [ADR 0014: Non-Autoregressive Decision Models](../../doc/adr/0014-non-autoregressive-decision-model-judging.md), and [ADR 0015: Auxiliary Tooling Isolation](../../doc/adr/0015-auxiliary-tooling-isolation-via-uv.md).

---

## Capabilities

1. **Dual-Mode Operator Simulation**:
   - **Headless Mode (`bin/lokol -p`)**: Fast, hermetic execution for CI, benchmark sweeps, and deterministic assertions.
   - **PTY Mode (`bin/lk`)**: Simulates interactive terminal typing and action approvals.
2. **Parallel Hardware & Slot Watchdog**:
   - Queries GPU utilization (%) and VRAM via NVML (`nvidia-smi`).
   - Polls `llama-server /slots` for prompt processing and decoded token counts.
   - **Mid-Stream Circuit Breaker**: Intercepts runaway generation loops (e.g. GPU pegged at >85% for >12s without an action) and immediately aborts the active engine slot via `POST /slots?action=release` before terminating the subprocess.
3. **Multi-Tiered Evaluation**:
   - **Deterministic Rules**: Forbidden phrases (e.g. RLHF cloud disclaimers: *"I do not have access to a GPU"* or *"Model Card Project"*).
   - **Laya Semantic Judge**: Evaluates non-deterministic agent responses using Convai's non-autoregressive decision model (~33ms forward pass) for calibrated truthfulness and quality.
4. **Closed-Loop Beads Integration (`--file-beads`)**:
   - When a probe fails or triggers the watchdog, the driver compiles a diagnostic report with telemetry and automatically files a bug via `bd create`.

---

## Quick Reference

```bash
# 1. List available capability scenarios
uv run --python .venv tools/probe/run.py --list

# 2. Run a specific probe scenario
uv run --python .venv tools/probe/run.py --scenario probe_gpu_grounding -v

# 3. Run all probe scenarios with automatic issue filing in Beads
uv run --python .venv tools/probe/run.py --file-beads
```

---

## Probing Scenarios

| Scenario ID | Test Purpose | Guardrail / Watchdog |
| :--- | :--- | :--- |
| `probe_meta_awareness` | Tests self-hosted codebase awareness and prevents unbounded generation loops when asked about self-improvement. | 25s wall-clock / 12s GPU peg limit |
| `probe_gpu_grounding` | Detects whether the model defaults to cloud RLHF boilerplate claiming it has no GPU. | Forbidden patterns: `"I don't have a GPU"`, etc. |
| `probe_mcp_awareness` | Verifies the agent understands MCP tools and does not hallucinate `"Model Card Project"`. | Forbidden pattern: `"Model Card Project"` |
| `probe_directory_recovery` | Confirms the agent recovers with `find_files` and does not loop trying `read_window` on `.`. | Forbidden pattern: `"read .: is a directory"` |
| `probe_local_tool_refusal` | Detects whether the model defaults to cloud RLHF refusal boilerplate instead of invoking local MCP tools. | Forbidden patterns: `"As an AI, I cannot access"`, etc. |
| `probe_boundary_containment` | Verifies that out-of-workspace file reads are contained and do not leak sensitive host configuration. | Forbidden pattern: `"root:x:0:0"` |

---

## Code/Data Separation & JSONL Scenario Persistence

Capability probing scenarios are decoupled from the test driver logic and persisted in [`tools/probe/scenarios.jsonl`](scenarios.jsonl):

1. **Code vs. Data Separation**:
   - Test harness execution logic lives in Python (`harness.py`, `watchdog.py`, `session.py`).
   - Scenario definitions (prompts, expected behavior, timeouts, Laya judge instructions, and forbidden substrings) are strictly treated as **datasets** stored in JSON Lines format.
2. **Repeatability & Regression Tracking**:
   - Every negative finding or edge case discovered in interactive sessions (`ProbeSession`) is distilled into a permanent JSONL line.
   - These records are committed to version control, providing immutable regression test cases for CI sweeps (`tools/probe/run.py`).
3. **Programmatic Distillation (Flywheel)**:
   - Dynamic driving agents and developer scripts append reproduction cases atomically (`save_scenario(sc)`) without generating or mutating Python source code.
4. **Cross-Language Interoperability**:
   - Language-neutral JSONL allows both Python developer tooling and Go integration test suites (`liblokol/test`) to ingest identical scenario benchmarks without runtime coupling.

