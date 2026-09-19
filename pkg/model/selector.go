package model

import (
	"fmt"

	"github.com/boggycreek/quik/pkg/probe"
)

// Tier represents hardware classification tier.
type Tier string

const (
	Tier1HighVRAM       Tier = "Tier 1: High VRAM (>= 10 GB)"
	Tier2MidVRAM        Tier = "Tier 2: Mid VRAM (6 - 10 GB)"
	Tier3ConstrainedGPU Tier = "Tier 3: Constrained GPU (4 - 6 GB, e.g. GTX 1650)"
	Tier4CPUFallback    Tier = "Tier 4: CPU Fallback (< 4 GB)"
)

// Recommendation provides the optimal model specification and launch flags.
type Recommendation struct {
	Tier            Tier   `json:"tier"`
	ModelName       string `json:"model_name"`
	HFRepo          string `json:"hf_repo"`
	HFFile          string `json:"hf_file"`
	ContextLength   int    `json:"context_length"`
	KVCacheQuant    string `json:"kv_cache_quant"`
	GPULayers       int    `json:"gpu_layers"` // 99 for full offload
	EstimatedVRAMMB int    `json:"estimated_vram_mb"`
	Notes           string `json:"notes"`
}

// SelectOptimalModel evaluates hardware specs and recommends the optimal model configuration.
func SelectOptimalModel(p *probe.HardwareProfile) Recommendation {
	vramGiB := float64(p.VRAMBytes) / (1024 * 1024 * 1024)

	// Tier 1: >= 10 GiB VRAM (RTX 3060 12GB, RTX 4070, etc.)
	if p.HasNVIDIA && vramGiB >= 10.0 {
		return Recommendation{
			Tier:            Tier1HighVRAM,
			ModelName:       "Qwen 2.5 Coder 7B Instruct (Q4_K_M)",
			HFRepo:          "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF",
			HFFile:          "qwen2.5-coder-7b-instruct-q4_k_m.gguf",
			ContextLength:   65536,
			KVCacheQuant:    "q8_0",
			GPULayers:       99,
			EstimatedVRAMMB: 6600,
			Notes:           "100% VRAM offload. 64k pure-VRAM context with Q8_0 KV cache (~66 t/s). ~5.5 GB VRAM headroom remaining.",
		}
	}

	// Tier 2: 6 GiB to 10 GiB VRAM (RTX 2060, 3050, 4050, etc.)
	if p.HasNVIDIA && vramGiB >= 6.0 {
		return Recommendation{
			Tier:            Tier2MidVRAM,
			ModelName:       "Qwen 2.5 Coder 7B Instruct (Q4_K_M)",
			HFRepo:          "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF",
			HFFile:          "qwen2.5-coder-7b-instruct-q4_k_m.gguf",
			ContextLength:   16384,
			KVCacheQuant:    "q8_0",
			GPULayers:       99,
			EstimatedVRAMMB: 5200,
			Notes:           "100% VRAM offload. 16k context fits comfortably in 6GB-8GB VRAM cards.",
		}
	}

	// Tier 3: 4 GiB to 6 GiB VRAM (ThinkPad X1 Extreme Gen 1 w/ GTX 1650 Max-Q 4GB)
	if p.HasNVIDIA && vramGiB >= 3.5 {
		// When the dGPU is fully dedicated to compute (Intel iGPU handles X11/Wayland display),
		// we have the full ~3.9 GB available. Qwen 2.5 Coder 3B with 32k Q8_0 KV uses ~2.9 GB,
		// leaving ~1 GB safety margin.
		return Recommendation{
			Tier:            Tier3ConstrainedGPU,
			ModelName:       "Qwen 2.5 Coder 3B Instruct (Q4_K_M)",
			HFRepo:          "Qwen/Qwen2.5-Coder-3B-Instruct-GGUF",
			HFFile:          "qwen2.5-coder-3b-instruct-q4_k_m.gguf",
			ContextLength:   32768,
			KVCacheQuant:    "q8_0",
			GPULayers:       99,
			EstimatedVRAMMB: 2950,
			Notes: fmt.Sprintf("Dedicated compute dGPU profile (GTX 1650 Max-Q 4GB, Intel iGPU for display). Q4_K_M weights (~1.9 GB) + 32k Q8_0 KV cache (~1.05 GB) offloaded 100%% to VRAM. High throughput with zero CPU context spillover."),
		}
	}

	// Tier 4: CPU Fallback or Low VRAM
	return Recommendation{
		Tier:            Tier4CPUFallback,
		ModelName:       "Qwen 2.5 Coder 1.5B Instruct (Q8_0)",
		HFRepo:          "Qwen/Qwen2.5-Coder-1.5B-Instruct-GGUF",
		HFFile:          "qwen2.5-coder-1.5b-instruct-q8_0.gguf",
		ContextLength:   8192,
		KVCacheQuant:    "f16",
		GPULayers:       0,
		EstimatedVRAMMB: 0,
		Notes:           "Running on CPU with AVX2 acceleration. Low latency and light memory footprint.",
	}
}
