// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package model_test

import (
	"strings"
	"testing"

	"github.com/boggycreek/lokol/liblokol/model"
	"github.com/boggycreek/lokol/liblokol/probe"
)

func TestSelectOptimalModel_AppleMetal(t *testing.T) {
	tests := []struct {
		name         string
		vramGiB      float64
		expectedTier model.Tier
		expectedNGL  int
	}{
		{"Metal 16GB (Tier 1)", 16.0, model.Tier1HighVRAM, 99},
		{"Metal 8GB (Tier 2)", 8.0, model.Tier2MidVRAM, 99},
		{"Metal 4GB (Tier 3)", 4.0, model.Tier3ConstrainedGPU, 99},
		{"Metal 2GB (Tier 4 Fallback)", 2.0, model.Tier4CPUFallback, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &probe.HardwareProfile{
				OS:            "darwin",
				Arch:          "arm64",
				HasAppleMetal: true,
				VRAMBytes:     uint64(tt.vramGiB * 1024 * 1024 * 1024),
			}

			rec := model.SelectOptimalModel(p)
			if rec.Tier != tt.expectedTier {
				t.Errorf("got tier %q, want %q", rec.Tier, tt.expectedTier)
			}
			if rec.GPULayers != tt.expectedNGL {
				t.Errorf("got GPULayers %d, want %d", rec.GPULayers, tt.expectedNGL)
			}
			if tt.expectedNGL > 0 && !strings.Contains(rec.Notes, "Metal") {
				t.Errorf("expected Metal reference in notes, got: %s", rec.Notes)
			}
		})
	}
}

func TestSelectOptimalModel_ModesWithMetal(t *testing.T) {
	p := &probe.HardwareProfile{
		OS:            "darwin",
		Arch:          "arm64",
		HasAppleMetal: true,
		VRAMBytes:     16 * 1024 * 1024 * 1024, // 16GB
	}

	// 1. Coding Mode
	recCoding := model.SelectOptimalModelForMode(p, model.ModeCoding)
	if recCoding.Tier != model.Tier1HighVRAM {
		t.Errorf("coding mode: expected Tier 1, got %s", recCoding.Tier)
	}
	if !strings.Contains(recCoding.ModelName, "Qwen 2.5 Coder") {
		t.Errorf("coding mode: expected Qwen 2.5 Coder, got %s", recCoding.ModelName)
	}
	if recCoding.GPULayers != 99 {
		t.Errorf("coding mode: expected GPULayers=99, got %d", recCoding.GPULayers)
	}
	if !strings.Contains(recCoding.Notes, "Apple Metal") {
		t.Errorf("coding mode: expected Apple Metal notes, got: %s", recCoding.Notes)
	}

	// 2. MoE Mode
	recMoE := model.SelectOptimalModelForMode(p, model.ModeMoE)
	if recMoE.Tier != model.Tier1HighVRAM {
		t.Errorf("moe mode: expected Tier 1, got %s", recMoE.Tier)
	}
	if !strings.Contains(recMoE.ModelName, "MoE") {
		t.Errorf("moe mode: expected MoE model, got %s", recMoE.ModelName)
	}
	if recMoE.GPULayers != 99 {
		t.Errorf("moe mode: expected GPULayers=99, got %d", recMoE.GPULayers)
	}

	// 3. General Mode
	recGeneral := model.SelectOptimalModelForMode(p, model.ModeGeneral)
	if recGeneral.Tier != model.Tier1HighVRAM {
		t.Errorf("general mode: expected Tier 1, got %s", recGeneral.Tier)
	}
	if !strings.Contains(recGeneral.ModelName, "Llama") {
		t.Errorf("general mode: expected Llama model, got %s", recGeneral.ModelName)
	}
	if recGeneral.GPULayers != 99 {
		t.Errorf("general mode: expected GPULayers=99, got %d", recGeneral.GPULayers)
	}
}

func TestSelectOptimalModel_NVIDIA(t *testing.T) {
	p := &probe.HardwareProfile{
		OS:        "linux",
		Arch:      "amd64",
		HasNVIDIA: true,
		VRAMBytes: 12 * 1024 * 1024 * 1024, // 12GB
	}

	rec := model.SelectOptimalModel(p)
	if rec.Tier != model.Tier1HighVRAM {
		t.Errorf("expected Tier 1, got %s", rec.Tier)
	}
	if rec.GPULayers != 99 {
		t.Errorf("expected GPULayers=99, got %d", rec.GPULayers)
	}

	// CPU fallback when no GPU detected
	pCPU := &probe.HardwareProfile{
		OS:   "linux",
		Arch: "amd64",
	}
	recCPU := model.SelectOptimalModel(pCPU)
	if recCPU.Tier != model.Tier4CPUFallback {
		t.Errorf("expected Tier 4 CPU fallback, got %s", recCPU.Tier)
	}
	if recCPU.GPULayers != 0 {
		t.Errorf("expected GPULayers=0 for CPU fallback, got %d", recCPU.GPULayers)
	}
}
