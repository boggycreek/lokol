---
adr: "0020"
title: "CPU AVX2 Offload Strategy for Auxiliary Decision Models"
topic: "Hardware & Inference"
theme: "THEME-INFERENCE"
status: "accepted"
version: "v0.1.0-alpha"
as_built: true
tags:
  - cpu-offload
  - avx2
  - hardware
  - vram-preservation
  - decision-models
executive_summary: "Directs auxiliary decision models and evaluation judges to execute exclusively on host CPU cores via AVX2/AVX-512 instructions, reserving 100% of GPU VRAM for the primary generative LLM context."
---

# ADR 0020: CPU AVX2 Offload Strategy for Auxiliary Decision Models

## Status
Accepted

## Date
2026-09-27

## Context
Local AI workflows on consumer hardware operate under strict GPU memory constraints:
- Consumer GPUs frequently possess between 4 GB and 12 GB of VRAM (e.g., GTX 1650, RTX 3060, RTX 4060).
- Our hardware tiering policy ([ADR-0002](0002-hardware-tiering-and-constrained-vram.md)) allocates 100% of available VRAM to a single dedicated engine slot (`-np 1`) to maximize the generative model's KV cache and context window (up to 64k tokens) with zero host RAM spillover.
- Introducing auxiliary neural models—such as safety guardrails, prompt routing classifiers, or semantic decision judges—onto the GPU would trigger immediate VRAM competition, cache eviction, or out-of-memory (OOM) failures on entry-level hardware.
- Unlike generative language models that generate hundreds of sequential tokens autoregressively, decision models (such as Laya / ModernBERT) process inputs in a **single non-autoregressive forward pass**.

We need an execution strategy for auxiliary evaluators and decision models that introduces zero VRAM footprint and does not perturb the primary inference engine.

## Decision
We establish **CPU Offloading** as the architectural policy for all auxiliary decision models, risk evaluators, and semantic judges:

```mermaid
flowchart TD
    subgraph GPUCompute ["GPU Compute Space (100% Dedicated)"]
        LLM["Generative LLM (Qwen / Llama)"]
        KVCache["64k KV Cache Pool (-np 1)"]
    end

    subgraph HostCPU ["Host CPU Space (Zero VRAM Footprint)"]
        ProdScorer["Production Risk Scorer (Deterministic Heuristics)"]
        OfflineJudge["Offline Semantic Judge (Laya / AVX2 Accelerated)"]
        MemoryIndexer["Vector & Embedding Indexers"]
    end

    GPUCompute -.->|Zero VRAM Competition| HostCPU
    HostCPU -->|Sub-millisecond or ~30ms Pass| Result["Deterministic Decision / Score"]
```

### 1. Dedicated Hardware Partitioning
- **Primary Generative Engine (`llama-server`)**: Owns 100% of GPU compute and VRAM.
- **Production Action-Gating Evaluator**: In-process runtime safety checks and command risk scoring execute on host CPU as deterministic heuristics ([ADR-0021](0021-zero-python-production-runtime-dependency-policy.md) and [ADR-0023](0023-functional-regulator-action-gating-pipeline.md)), requiring zero neural model overhead.
- **Offline Evaluation & Semantic Judging**: Auxiliary neural decision models (such as Laya used in developer benchmark suites per [ADR-0014](0014-non-autoregressive-decision-model-judging.md)) execute on host CPU utilizing vector instruction sets (AVX2 / AVX-512 on x86_64, NEON on ARM64) to prevent GPU memory contention with the live engine under test.
- **Future Production Decision Models**: If specialized neural decision models or local embedding transformers are added to the runtime in the future, they are strictly bounded to CPU execution.

### 2. Latency Budget vs VRAM Trade-off
- Production heuristic risk scoring executes in microseconds on CPU.
- For offline or auxiliary neural models, a single non-autoregressive forward pass of a 100M–300M parameter encoder model executes in **~25ms to 45ms** on consumer CPUs under AVX2 acceleration.
- In contrast to multi-second generative token generation, CPU evaluation preserves 100% of GPU memory headroom without introducing meaningful loop latency.

## Consequences

### Positive
- **Zero VRAM Contention**: Eliminates VRAM fragmentation, model swapping, and context cache eviction on constrained GPUs (e.g., ThinkPad GTX 1650 4GB).
- **Guaranteed Concurrency**: Primary generation and safety evaluation can run simultaneously without blocking GPU compute streams.
- **Broad Portability**: AVX2 vector instructions are standard across virtually all x86_64 processors produced over the last decade, with equivalent vector parity on Apple Silicon / ARM64.

### Negative / Trade-offs
- Systems lacking AVX2 vector extensions fall back to standard scalar CPU operations, increasing forward pass latency to ~150–200ms.
