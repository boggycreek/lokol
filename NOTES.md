# Machine Specific Notes & Target Environments (NOTES.md)

This document tracks target machine specifications, operational environments, and testing configurations for `quik`. While the core project code, ADRs, and README remain generalized for arbitrary Linux/POSIX hardware, this file records real-world benchmarks and device quirks.

---

## 1. Primary Workstation (Development & Reference Rig)

- **Host OS**: Linux (Ubuntu-based / Debian derivative, x86_64)
- **CPU**: AMD / Intel 12-Core, AVX2 and AVX-512 supported
- **System RAM**: ~256 GB Host RAM
- **GPU**: NVIDIA GeForce RTX 3060 12GB (Ampere `sm_86`)
- **VRAM Total**: 12,288 MiB (~12.0 GB)
- **Role**: High-throughput Tier 1 development environment.
- **Model Configuration**:
  - Model: `Qwen2.5-Coder-7B-Instruct-Q4_K_M.gguf` (~4.5 GB weights)
  - KV Cache: 65,536 tokens (64k), `q8_0` quantized (~1.85 GB)
  - GPU Offload: 100% VRAM offload (`-ngl 99`)
  - Total VRAM Consumption: ~6.56 GB (~5.7 GB free headroom)
  - Generation Throughput: ~66 tokens/second

---

## 2. Constrained Target Rig (ThinkPad X1 Extreme Gen 1)

- **Model**: Lenovo ThinkPad X1 Extreme Gen 1
- **Host OS**: Pop!_OS (Linux x86_64)
- **Display Server**: Intel UHD Graphics 630 (iGPU)
- **Discrete GPU**: NVIDIA GeForce GTX 1650 Max-Q 4GB VRAM (Turing `sm_75`)
- **OS Hybrid Graphics Configuration**:
  - Pop!_OS uses `system76-power` for GPU switching.
  - Setting: `system76-power graphics hybrid`
  - Under hybrid mode, Intel UHD 630 powers the GNOME/COSMIC desktop, Wayland/X11, display outputs, and browser hardware acceleration.
  - The NVIDIA GTX 1650 Max-Q remains completely headless and dedicated 100% to CUDA compute with **0 MiB initial display VRAM usage**.
- **Model Configuration (Tier 3 Dedicated Compute Mode)**:
  - Model: `Qwen2.5-Coder-3B-Instruct-Q4_K_M.gguf` (~1.90 GB weights)
  - KV Cache: 32,768 tokens (32k), `q8_0` quantized (~1.05 GB)
  - GPU Offload: 100% VRAM offload (`-ngl 99`)
  - Total VRAM Consumption: ~2.95 GB VRAM
  - Safe Headroom: ~1.05 GB free VRAM (prevents OOM crashes without swapping to system RAM over PCIe)
  - Expected Throughput: ~40–55 tokens/second

---

## 3. Generalization Rules for `quik`

To ensure `quik` remains portable beyond these specific machines:
1. **Dynamic VRAM Headroom Deduction**:
   - `pkg/probe` will query current VRAM usage before launch. If `nvidia-smi` shows active Xorg/Wayland usage on the GPU, `quik` automatically treats the GPU as a "shared display" device and docks 1.5 GB from the usable budget.
   - If `used_vram < 100 MiB` (e.g. Pop!_OS hybrid mode or headless server), `quik` unlocks the "dedicated compute" profile.
2. **Fallback Ladder**:
   - 10GB+ VRAM -> 7B (64k context)
   - 6GB–10GB VRAM -> 7B (16k–32k context) or 3B (64k context)
   - 4GB VRAM (dedicated) -> 3B (32k context)
   - 4GB VRAM (shared display) -> 3B (16k context) or 1.5B (32k context)
   - <4GB / No GPU -> 1.5B (8k context) via CPU AVX2
