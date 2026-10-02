---
name: lokol-inference-and-hardware
description: >-
  Hardware probing, constrained VRAM management, MoE sparse expert offloading, CPU AVX2 offload, and workload modes.
  Use when configuring model profiles, memory allocation, or inference engine parameters.
---

# Inference & Hardware Tiering

## Constrained VRAM & The Single-Slot Rule
- **The Single-Slot Rule**: Lokol dedicates 100% of GPU VRAM to a single inference slot (`-np 1`) ([ADR 0002](../adr/0002-hardware-tiering-and-constrained-vram.md)).
- **Zero Host RAM Thrashing**: On consumer hardware (4GB–12GB VRAM), splitting slots leads to KV cache spillover into system RAM, collapsing generation throughput from ~30 tok/s to <2 tok/s.
- **Hardware Tiering Policy**:
  - **Tier 1 (12GB+ VRAM, e.g. RTX 3060/4070)**: 7B–14B models (Q4_K_M / Q5_K_M) with 32k–64k context windows.
  - **Tier 2 (8GB VRAM, e.g. RTX 3070/4060)**: 7B–8B models (Q4_K_M) with 16k–32k context windows.
  - **Tier 3 (4GB–6GB VRAM, e.g. GTX 1650)**: 3B models (Q4_K_M) or compact MoE architectures with 8k–16k context windows.

## Dynamic Mixture of Experts (MoE) Routing
- **Sparse Activation on Local Hardware**: High total parameter counts (e.g. 14B–16B) with low active parameter counts (2.4B–3B per token) ([ADR 0008](../adr/0008-dynamic-moe-expert-routing.md)).
- **Selective Layer & Expert Offload**: Attention layers and frequently activated expert weights are pinned in GPU VRAM (`-ngl`), while remaining experts reside in host RAM via `mmap` to prevent VRAM exhaustion.

## CPU AVX2 Offload Strategy for Auxiliary Decision Models
- **100% VRAM Reservation**: Generative LLM KV cache requires full GPU VRAM ([ADR 0020](../adr/0020-cpu-avx2-offload-strategy-for-auxiliary-decision-models.md)).
- **Auxiliary Decision Models on CPU**: Secondary models (such as Laya semantic judges or prompt complexity routers) execute strictly on host CPU cores using vectorized AVX2/AVX-512 instructions, completing inferences in ~30ms without GPU contention.

## Workload Modes vs Tool Access
- **Decoupled Universal Tooling**: All tools remain available across all modes via the Catalog ([ADR 0024](../adr/0024-progressive-tool-disclosure-and-catalog-architecture.md)).
- **Mode as Compute/Model Profile**:
  - `general`: Conversational instruct model, balanced temperature ($T \approx 0.7$) ([ADR 0007](../adr/0007-general-purpose-default-persona.md)).
  - `coding`: High-precision code synthesis model (Qwen2.5-Coder), low temperature ($T \approx 0.1\text{--}0.2$), strict syntax grounding.
  - `architect` / `planning`: High-parameter or long-context reasoning model for project decomposition.
  - `moe`: Sparse MoE architecture leveraging selective expert offload ([ADR 0008](../adr/0008-dynamic-moe-expert-routing.md)).

## Explicit Context Clearing & Engine Slot Purge Protocol
- **Deterministic VRAM Reclamation**: Explicit context clearing (interactive `/clear`, `Ctrl+L`/`Ctrl+K`, or headless sub-task boundaries) dispatches a slot cache purge (`action=erase`) to `llama-server`, resetting KV cache token pressure to zero without restarting the engine ([ADR 0028](../adr/0028-explicit-context-clearing-and-state-retention-invariants.md)).
- **State Retention Invariants**: Preserves host environment discovery, working directory, operational mode, active persona/operator identities, and foundational tool schemas while wiping conversational trajectories and temporary read caches.
- **Explicit Reset vs Automatic Compaction**: Explicit clearing creates a true clean slate between discrete milestones; automatic compaction ([ADR 0026](../adr/0026-inference-slot-pressure-and-context-budget-regulation.md), [ADR 0029](../adr/0029-dynamic-context-compaction-and-attention-pruning.md)) governs in-flight execution under token window pressure.

## Dynamic Context Compaction & Attention Pruning
- **Dual-Tier Reduction Pipeline**: Progressive compaction balancing token savings with attention retention ([ADR 0029](../adr/0029-dynamic-context-compaction-and-attention-pruning.md)).
  - **Tier 1 (Micro-Pruning at 60%–75% slot pressure)**: Truncates obsolete inspection outputs (`read_window`, `read_outline`, verbose command logs) into compact stubs once successors succeed.
  - **Tier 2 (Macro-Compaction at ≥75% slot pressure)**: Condenses older turn history into a structured semantic ledger while strictly preserving prompt grounding and recent turns.
- **Attention Preservation**: Protects small local models (3B–8B) from attention dilution and "lost in the middle" degradation during multi-turn refactoring.

Further reading:
- [ADR 0002 — Hardware Probing and Tiering Matrix](../adr/0002-hardware-tiering-and-constrained-vram.md)
- [ADR 0007 — General-Purpose Assistant as Default Operational Persona](../adr/0007-general-purpose-default-persona.md)
- [ADR 0008 — Dynamic Mixture of Experts (MoE) Routing for Constrained VRAM](../adr/0008-dynamic-moe-expert-routing.md)
- [ADR 0020 — CPU AVX2 Offload Strategy for Auxiliary Decision Models](../adr/0020-cpu-avx2-offload-strategy-for-auxiliary-decision-models.md)
- [ADR 0024 — Progressive Tool Disclosure and Catalog Architecture](../adr/0024-progressive-tool-disclosure-and-catalog-architecture.md)
- [ADR 0026 — Inference Slot Pressure and Context Budget Regulation](../adr/0026-inference-slot-pressure-and-context-budget-regulation.md)
- [ADR 0028 — Explicit Context Clearing and State Retention Invariants](../adr/0028-explicit-context-clearing-and-state-retention-invariants.md)
- [ADR 0029 — Dynamic Context Compaction and Attention Pruning Architecture](../adr/0029-dynamic-context-compaction-and-attention-pruning.md)
