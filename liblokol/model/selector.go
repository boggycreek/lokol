// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package model

import (
	"fmt"
	"strings"

	"github.com/boggycreek/lokol/liblokol/probe"
)

// Mode represents operational persona for model recommendations.
type Mode string

const (
	ModeGeneral Mode = "general"
	ModeCoding  Mode = "coding"
	ModeMoE     Mode = "moe"
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
	ParallelSlots   int    `json:"parallel_slots"` // -np 1 to allocate 100% of VRAM KV pool to the active agent
	GPULayers       int    `json:"gpu_layers"`     // 99 for full offload
	EstimatedVRAMMB int    `json:"estimated_vram_mb"`
	Notes           string `json:"notes"`
}

// SelectOptimalModel evaluates hardware specs and recommends the optimal model configuration for default mode.
func SelectOptimalModel(p *probe.HardwareProfile) Recommendation {
	return SelectOptimalModelForMode(p, ModeGeneral)
}

// SelectOptimalModelForMode evaluates hardware specs and recommends the optimal model configuration for the requested mode.
func SelectOptimalModelForMode(p *probe.HardwareProfile, mode Mode) Recommendation {
	vramGiB := float64(p.VRAMBytes) / (1024 * 1024 * 1024)

	switch Mode(strings.ToLower(string(mode))) {
	case ModeCoding:
		return selectCodingModel(p, vramGiB)
	case ModeMoE:
		return selectMoEModel(p, vramGiB)
	case ModeGeneral:
		fallthrough
	default:
		return selectGeneralModel(p, vramGiB)
	}
}

func selectGeneralModel(p *probe.HardwareProfile, vramGiB float64) Recommendation {
	// Tier 1: >= 10 GiB VRAM
	if p.HasNVIDIA && vramGiB >= 10.0 {
		return Recommendation{
			Tier:            Tier1HighVRAM,
			ModelName:       "Meta Llama 3.1 8B Instruct (Q4_K_M)",
			HFRepo:          "bartowski/Meta-Llama-3.1-8B-Instruct-GGUF",
			HFFile:          "Meta-Llama-3.1-8B-Instruct-Q4_K_M.gguf",
			ContextLength:   65536,
			KVCacheQuant:    "q8_0",
			ParallelSlots:   1,
			GPULayers:       99,
			EstimatedVRAMMB: 6800,
			Notes:           "General conversational and analytical assistant. 100% VRAM offload with dedicated slot (-np 1) maximizing available VRAM for natural language attention.",
		}
	}

	// Tier 2: 6 GiB to 10 GiB VRAM
	if p.HasNVIDIA && vramGiB >= 6.0 {
		return Recommendation{
			Tier:            Tier2MidVRAM,
			ModelName:       "Meta Llama 3.1 8B Instruct (Q4_K_M)",
			HFRepo:          "bartowski/Meta-Llama-3.1-8B-Instruct-GGUF",
			HFFile:          "Meta-Llama-3.1-8B-Instruct-Q4_K_M.gguf",
			ContextLength:   32768,
			KVCacheQuant:    "q4_0",
			ParallelSlots:   1,
			GPULayers:       99,
			EstimatedVRAMMB: 5600,
			Notes:           "100% VRAM offload with Q4_0 KV cache and single dedicated slot (-np 1). 32k context fits cleanly in 6GB-8GB VRAM cards with zero host RAM spillover.",
		}
	}

	// Tier 3: 4 GiB to 6 GiB VRAM (GTX 1650 4GB)
	if p.HasNVIDIA && vramGiB >= 3.5 {
		return Recommendation{
			Tier:            Tier3ConstrainedGPU,
			ModelName:       "Meta Llama 3.2 3B Instruct (Q4_K_M)",
			HFRepo:          "bartowski/Llama-3.2-3B-Instruct-GGUF",
			HFFile:          "Llama-3.2-3B-Instruct-Q4_K_M.gguf",
			ContextLength:   32768,
			KVCacheQuant:    "q4_0",
			ParallelSlots:   1,
			GPULayers:       99,
			EstimatedVRAMMB: 2500,
			Notes:           "Dedicated compute profile for constrained 4GB GPUs. High throughput conversational reasoning with zero CPU context spillover.",
		}
	}

	// Tier 4: CPU Fallback or Low VRAM
	return Recommendation{
		Tier:            Tier4CPUFallback,
		ModelName:       "Meta Llama 3.2 1B Instruct (Q8_0)",
		HFRepo:          "bartowski/Llama-3.2-1B-Instruct-GGUF",
		HFFile:          "Llama-3.2-1B-Instruct-Q8_0.gguf",
		ContextLength:   8192,
		KVCacheQuant:    "f16",
		ParallelSlots:   1,
		GPULayers:       0,
		EstimatedVRAMMB: 0,
		Notes:           "Running on CPU with AVX2 acceleration and single slot (-np 1). Lightweight conversational footprint.",
	}
}

func selectCodingModel(p *probe.HardwareProfile, vramGiB float64) Recommendation {
	// Tier 1: >= 10 GiB VRAM
	if p.HasNVIDIA && vramGiB >= 10.0 {
		return Recommendation{
			Tier:            Tier1HighVRAM,
			ModelName:       "Qwen 2.5 Coder 7B Instruct (Q4_K_M)",
			HFRepo:          "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF",
			HFFile:          "qwen2.5-coder-7b-instruct-q4_k_m.gguf",
			ContextLength:   65536,
			KVCacheQuant:    "q8_0",
			ParallelSlots:   1,
			GPULayers:       99,
			EstimatedVRAMMB: 6600,
			Notes:           "100% VRAM offload. Single dedicated slot (-np 1) maximizes available VRAM for KV cache (~66 t/s). ~5.5 GB VRAM headroom remaining.",
		}
	}

	// Tier 2: 6 GiB to 10 GiB VRAM
	if p.HasNVIDIA && vramGiB >= 6.0 {
		return Recommendation{
			Tier:            Tier2MidVRAM,
			ModelName:       "Qwen 2.5 Coder 7B Instruct (Q4_K_M)",
			HFRepo:          "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF",
			HFFile:          "qwen2.5-coder-7b-instruct-q4_k_m.gguf",
			ContextLength:   32768,
			KVCacheQuant:    "q4_0",
			ParallelSlots:   1,
			GPULayers:       99,
			EstimatedVRAMMB: 5400,
			Notes:           "100% VRAM offload with Q4_0 KV cache and single dedicated slot (-np 1). 32k context fits cleanly in 6GB-8GB VRAM cards with zero host RAM spillover.",
		}
	}

	// Tier 3: 4 GiB to 6 GiB VRAM
	if p.HasNVIDIA && vramGiB >= 3.5 {
		return Recommendation{
			Tier:            Tier3ConstrainedGPU,
			ModelName:       "Qwen 2.5 Coder 3B Instruct (Q4_K_M)",
			HFRepo:          "Qwen/Qwen2.5-Coder-3B-Instruct-GGUF",
			HFFile:          "qwen2.5-coder-3b-instruct-q4_k_m.gguf",
			ContextLength:   32768,
			KVCacheQuant:    "q4_0",
			ParallelSlots:   1,
			GPULayers:       99,
			EstimatedVRAMMB: 2450,
			Notes:           fmt.Sprintf("Dedicated compute dGPU profile (GTX 1650 Max-Q 4GB). Q4_K_M weights (~1.9 GB) + 32k Q4_0 KV cache (~0.55 GB) offloaded 100%% to VRAM with dedicated slot (-np 1)."),
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
		ParallelSlots:   1,
		GPULayers:       0,
		EstimatedVRAMMB: 0,
		Notes:           "Running on CPU with AVX2 acceleration and single slot (-np 1). Low latency and light memory footprint.",
	}
}

func selectMoEModel(p *probe.HardwareProfile, vramGiB float64) Recommendation {
	if p.HasNVIDIA && vramGiB >= 6.0 {
		return Recommendation{
			Tier:            Tier1HighVRAM,
			ModelName:       "Qwen 1.5 MoE A2.7B Chat (Q4_K_M)",
			HFRepo:          "Qwen/Qwen1.5-MoE-A2.7B-Chat-GGUF",
			HFFile:          "qwen1.5-moe-a2.7b-chat-q4_k_m.gguf",
			ContextLength:   32768,
			KVCacheQuant:    "q4_0",
			ParallelSlots:   1,
			GPULayers:       99,
			EstimatedVRAMMB: 4800,
			Notes:           "Dynamic Mixture of Experts mode: 14B total parameters with 2.7B active per token. Offloads active expert paths to VRAM for high-capacity reasoning within consumer GPU bounds.",
		}
	}

	if p.HasNVIDIA && vramGiB >= 3.5 {
		return Recommendation{
			Tier:            Tier3ConstrainedGPU,
			ModelName:       "Qwen 1.5 MoE A2.7B Chat (Q3_K_M)",
			HFRepo:          "Qwen/Qwen1.5-MoE-A2.7B-Chat-GGUF",
			HFFile:          "qwen1.5-moe-a2.7b-chat-q3_k_m.gguf",
			ContextLength:   16384,
			KVCacheQuant:    "q4_0",
			ParallelSlots:   1,
			GPULayers:       99,
			EstimatedVRAMMB: 3600,
			Notes:           "MoE sparse expert routing optimized for 4GB VRAM. Fast active expert execution with memory-mapped host RAM expert fallback.",
		}
	}

	return Recommendation{
		Tier:            Tier4CPUFallback,
		ModelName:       "Qwen 1.5 MoE A2.7B Chat (Q3_K_M)",
		HFRepo:          "Qwen/Qwen1.5-MoE-A2.7B-Chat-GGUF",
		HFFile:          "qwen1.5-moe-a2.7b-chat-q3_k_m.gguf",
		ContextLength:   8192,
		KVCacheQuant:    "f16",
		ParallelSlots:   1,
		GPULayers:       0,
		EstimatedVRAMMB: 0,
		Notes:           "CPU MoE execution with AVX2 expert routing and single slot (-np 1).",
	}
}
