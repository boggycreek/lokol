# ADR 0005: Hybrid Evaluations and Fast Routing with TypeSafe AI (Jev / "not-a-llm")

## Status
Proposed (Research & Exploration)

## Context
In agentic coding loops on constrained local hardware (such as 7B on an RTX 3060 or 3B on a ThinkPad GTX 1650 Max-Q), invoking the generative LLM for every small decision introduces unnecessary latency and context consumption. Small models are also prone to over-generating or hallucinating when asked binary evaluation questions like:
- "Did the test pass or fail?"
- "Is this bash command destructive / requiring human approval?"
- "Does the proposed code diff satisfy the contract requirements?"
- "Should we stop the loop or execute another step?"

Using a full generative LLM (autoregressive token-by-token generation) for pure classification, confidence scoring, or boolean routing consumes 1,000–3,000 tokens of KV cache and takes 2–5 seconds per step.

TypeSafe AI recently released **Jev** (their "not-a-llm" / "System One" machine-native engine, founded by Diogo Almeida, co-inventor of RLHF). Jev is a non-autoregressive decision engine that returns typed outputs (choices, confidence scores, probabilities) with sub-100ms latency and 100x lower cost/compute footprint.

## Research Questions
1. How can `quik` leverage non-autoregressive decision models alongside local generative models?
2. Can we run a local small classifier / cross-encoder (or API call to Jev) as an inline fast-evaluator?

## Proposed Architecture: The System 1 / System 2 Agent Split

```
User / Task Contract
         │
         ▼
┌─────────────────────────────────┐
│   Fast Decision Engine (Jev)    │  <-- Sub-100ms, non-autoregressive,
│   or Local Cross-Encoder        │      probabilistic typed output
└────────────────┬────────────────┘
                 │
        Decision / Route:
        ├─ [Trivial check] -> Execute directly or report PASS/FAIL
        ├─ [Safety Gate]    -> Risk Score > 0.85 -> Escalate to user approval
        └─ [Complex Code]  -> Dispatch bounded prompt to Local 7B/3B Generator
                                          │
                                          ▼
                         ┌─────────────────────────────────┐
                         │ Local Generative Worker (Qwen)  │
                         │ Ingests code & writes diff      │
                         └────────────────┬────────────────┘
                                          │
                                          ▼
                         ┌─────────────────────────────────┐
                         │ Fast Evaluator (Jev / Contract) │
                         │ Evaluates diff against criteria │
                         │ Output: {pass: true, conf: 0.94}│
                         └─────────────────────────────────┘
```

## Potential Primitives for `quik`
1. **Tool Safety & YOLO Classifier**:
   - Primitive: `Choice[SafeAutoExec, RequireUserConfirm, Reject]`
   - Latency: <100ms
   - Prevents running `rm -rf`, `git reset --hard`, or network commands automatically in YOLO mode without burning generative tokens.
2. **Acceptance Criteria Verification (Hybrid Eval)**:
   - Input: Test stdout + Git diff + Task Contract.
   - Primitive: `Score[0.0 - 1.0]` + `Choice[Pass, NeedsFix, DeadEnd]`
   - Eliminates model self-evaluation bias (where the generative model falsely claims its broken code works).

## Implementation Path
- Create a lightweight evaluator interface in Go (`pkg/eval/evaluator.go`).
- Allow pluggable backends:
  - `jev`: HTTP client to TypeSafe AI API (`console.typesafe.ai`).
  - `local_bge`: Local cross-encoder / BGE reranker on `http://127.0.0.1:8082`.
  - `rule`: Fast deterministic regex/exit-code fallback.
