# ADR 0014: Non-Autoregressive Decision Models (Laya) for Semantic Evaluation

## Status
Accepted

## Date
2026-09-27

## Context
Certain software engineering tasks cannot be verified by compiler passes or exit codes alone. When an agent generates architecture documentation, summarizes a system's control flow, or explains a module interface, determining whether the output is faithful to the codebase requires semantic evaluation.

The conventional industry approach ("LLM-as-a-judge") uses a secondary generative chat model (e.g. GPT-4 or a local 14B/32B model) to evaluate the primary model's output. However, in local, constrained development environments:
1. **High Latency & VRAM Competition**: Running a secondary generative model consumes significant GPU memory and takes 3–10 seconds per evaluation turn, bottlenecking the test suite.
2. **Generative Variance & Instability**: Autoregressive token generation introduces temperature variance, verbose formatting issues, and hallucination in the judge itself.
3. **Flaky String Extraction**: Parsing judge scores out of free-form conversational prose requires brittle regular expressions.

We require a fast, deterministic, non-autoregressive evaluator to assess semantic correctness and output fidelity without employing a chat LLM.

## Decision
We adopt **Convai's Laya** decision model (`convaiinnovations/laya`) as the semantic evaluation engine for the `lokol` integration test harness.

```mermaid
flowchart LR
    subgraph TestHarness ["Go Evaluation Suite (test/eval_test.go)"]
        AgentOutput["Agent Artifact\n(e.g., ARCHITECTURE.md)"]
        ContextPayload["Ground Truth Code Context\n(user.go, auth.go, db.go)"]
        Bridge["Go Bridge (test/laya_judge_test.go)"]
    end

    subgraph LayaEngine ["Laya Decision Model (tools/laya/judge.py)"]
        DecisionAgent["Non-Autoregressive ModernBERT\n(convaiinnovations/laya)"]
        NoulQuestion["Binary Probabilistic Query\n('noul': 0.0 - 1.0)"]
        ScoreQuestion["Ordinal Quality Scoring\n('score': 0 - 4)"]
    end

    AgentOutput --> Bridge
    ContextPayload --> Bridge
    Bridge -->|JSON payload over stdin| DecisionAgent
    DecisionAgent --> NoulQuestion
    DecisionAgent --> ScoreQuestion
    NoulQuestion -->|Typed P(True) >= 0.5| Bridge
    ScoreQuestion -->|Typed Quality Level| Bridge
```

### 1. Non-Autoregressive Decision Architecture
Laya uses a bidirectional ModernBERT backbone to evaluate typed decision queries in a single forward pass (~33ms):
- **Zero Token Generation**: Does not generate text tokens autoregressively.
- **Typed Questions**:
  - `noul`: Returns calibrated probability $P(\text{True}) \in [0.0, 1.0]$ for whether the output satisfies instructions.
  - `score`: Evaluates ordinal criteria (e.g., `unacceptable`, `poor`, `acceptable`, `good`, `flawless`) mapped to numerical expectation.

### 2. Standalone Bridge & Invocation (`tools/laya/judge.py`)
The evaluator is implemented as a standalone script using `uv` (PEP 723 metadata):
- Accepts input state and query instructions via JSON over standard input or CLI flags.
- Returns structured JSON to `stdout` with `passed` boolean, `probability`, and `quality_level`.
- Exits with return code 0 on pass and 2 on score below threshold, enabling direct command-line or test integration.

### 3. Go-to-Python Bridge (`test/laya_judge_test.go`)
The Go test suite interfaces with Laya through `runLayaJudge()`:
- Automatically detects if `uv` and the Python virtual environment are operational (`checkLayaAvailable`).
- Serializes evaluation contexts and queries to the Laya subprocess.
- Applies threshold gates (e.g., $P \ge 0.50$ and Quality $\ge 2.0$) alongside deterministic keyword checks.

## Consequences

### Positive
- **Deterministic & Repeatable**: Semantic evaluation produces calibrated probabilities rather than erratic token completions.
- **Sub-100ms Evaluation**: Forward pass evaluation completes in ~33ms, enabling rapid regression testing during normal `go test` runs.
- **Lightweight Resource Footprint**: Runs efficiently on CPU or GPU without interfering with the primary generative model running in `llama-server`.
- **Elimination of Self-Evaluation Bias**: Completely separates the decision/verification system from the agent's generative weights.

### Negative / Trade-offs
- Initial run requires downloading the ModernBERT model weights from Hugging Face (~500MB, cached locally thereafter).
- Relies on Python runtime and `uv` installed on the host machine (tests fall back cleanly to lexical verification if Laya is absent).
