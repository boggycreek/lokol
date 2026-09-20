# ADR 0002: Hardware Tiering and Constrained VRAM Profiles

## Status
Accepted

## Context
A major goal of `quik` is to run effectively across varied local hardware profiles:
- Primary Development Rig: NVIDIA RTX 3060 (12GB VRAM, sm_86, ~66 t/s).
- Constrained Target Rig: Lenovo ThinkPad X1 Extreme Gen 1 with NVIDIA GTX 1650 Max-Q (4GB VRAM, Turing NVENC/compute sm_75, limited bandwidth).
- Fallback: Apple Silicon (Metal) and CPU-only (AVX2 / AVX-512).

4GB VRAM is an extreme constraint for modern coding LLMs. A 7B parameter Q4_K_M model requires ~4.5GB VRAM just for weights, making it impossible to fit fully on a 4GB GPU without CPU layer offloading.

## Decision
`quik` will implement an automated **Hardware Prober** (`pkg/probe`) and a tiered **Model Sizing Matrix** (`pkg/model`):

1. **Hardware Tiering Matrix**:
   - **Tier 1 (High VRAM >= 12GB)**:
     - Target: RTX 3060 (12GB) and higher.
     - Model: `Qwen2.5-Coder-7B-Instruct-Q4_K_M` (or 14B Q4 with offload).
     - Context: 64,000 tokens with Q8_0 KV quantization (pure VRAM).
     - Features: High concurrency, prompt caching, speculative decoding candidate.
   - **Tier 2 (Mid VRAM 6GB - 10GB)**:
     - Target: RTX 2060/3050/4050 (6GB - 8GB).
     - Model: `Qwen2.5-Coder-7B-Instruct-Q4_K_M` with 16k–32k context, or `Qwen2.5-Coder-3B` with 64k context.
   - **Tier 3 (Constrained GPU 4GB - 6GB, Dedicated Compute Mode)**:
     - Target: GTX 1650 Max-Q (4GB VRAM) / ThinkPad X1 Extreme Gen 1.
     - Configuration: Intel UHD 630 iGPU drives Wayland/X11 display; NVIDIA dGPU is 100% headless for compute.
     - Model: `Qwen2.5-Coder-3B-Instruct-Q4_K_M` (~1.9 GB weights).
     - Context & KV Cache: **32,768 tokens (32k) with Q8_0 KV quantization** (~1.05 GB VRAM).
     - Total VRAM Consumption: ~2.95 GB VRAM offloaded 100% to GPU with zero CPU memory swapping, leaving ~1.0 GB safe headroom.
     - Generation Speed: Expected 40–50+ tokens/second on Turing CUDA cores.
   - **Tier 4 (CPU Fallback < 4GB)**:
     - Target: Pure host RAM execution with AVX2 quantization.

2. **Automatic Detection**:
   The `quik probe` command inspects:
   - GPU VRAM (via NVML / nvidia-smi / sysfs).
   - Host RAM and CPU vector instruction support (`AVX2`, `AVX512`, `NEON`).
   - Recommends the exact Hugging Face GGUF repository, filename, and engine flags (`-ngl`, `-c`, `-ctk`, `-ctv`).

## Consequences
### Positive
- Predictable performance: avoids CUDA OOM crashes before engine starts.
- Makes the 4GB GTX 1650 laptop a realistic, fully supported deployment target.
