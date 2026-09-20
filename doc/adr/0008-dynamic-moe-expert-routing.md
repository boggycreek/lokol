# ADR 0008: Dynamic Mixture of Experts (MoE) Routing for Constrained VRAM

## Status
Accepted

## Date
2026-09-20

## Context
Dense large language models (such as 14B or 32B models) exceed the VRAM boundaries of consumer GPUs (e.g., 8GB to 12GB on RTX 3060/4060, or 4GB on laptop dGPUs). While 7B/8B dense models fit comfortably into VRAM, their parameter capacity can be insufficient for complex multi-domain reasoning and nuanced problem decomposition.

Mixture of Experts (MoE) architectures (such as DeepSeek-V2-Lite or Qwen1.5-MoE) maintain high total parameter counts (e.g. 14B–16B total parameters) while sparsely activating only a subset of experts per token (e.g. 2.4B to 3B active parameters). 

However, running MoE models locally requires a specific memory strategy: can constrained GPUs take advantage of MoE architectures without running out of VRAM?

## Decision
We introduce a dedicated **MoE Operational Mode** (`--mode=moe`) engineered to leverage dynamic sparse expert activation:
1. **Model Selection**: When `--mode=moe` is specified, `lokol` probes available VRAM and selects an optimal sparse MoE model (e.g. DeepSeek-V2-Lite or Qwen1.5-MoE-A2.7B).
2. **Selective Layer & Expert Offload**: The engine parameters are tuned to offload attention layers and the most frequently activated expert weights into GPU VRAM (`-ngl`), while leaving non-activated or fallback experts mapped via `mmap` in host system RAM.
3. **Execution Routing**: During token generation, active expert pathways execute at GPU tensor core speeds, while sparse lookups avoid thrashing the entire model into VRAM simultaneously.

## Consequences

### Positive
- **High-Capacity Reasoning on Consumer Hardware**: Delivers reasoning capabilities superior to 7B dense models while fitting within 8GB–12GB VRAM constraints.
- **Fast Active Inference**: Sparse parameter activation maintains high token throughput compared to dense models of comparable total parameter count.

### Negative / Trade-offs
- If host RAM is slow (e.g. non-DDR5 without AVX2/AVX-512 memory bandwidth), expert switching latency can reduce generation speed when tokens route across non-VRAM experts.
