// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package probe

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// HardwareProfile captures the detected capabilities of the local host.
type HardwareProfile struct {
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	CPUCores       int    `json:"cpu_cores"`
	HasAVX2        bool   `json:"has_avx2"`
	HasAVX512      bool   `json:"has_avx512"`
	SystemRAMBytes uint64 `json:"system_ram_bytes"`
	GPUName        string `json:"gpu_name"`
	VRAMBytes      uint64 `json:"vram_bytes"`
	HasNVIDIA      bool   `json:"has_nvidia"`
	HasAppleMetal  bool   `json:"has_apple_metal"`
}

// HumanVRAM returns formatted VRAM in GiB.
func (p *HardwareProfile) HumanVRAM() string {
	if p.VRAMBytes == 0 {
		return "0 GiB (No dedicated VRAM)"
	}
	return fmt.Sprintf("%.2f GiB", float64(p.VRAMBytes)/(1024*1024*1024))
}

// HumanRAM returns formatted System RAM in GiB.
func (p *HardwareProfile) HumanRAM() string {
	return fmt.Sprintf("%.2f GiB", float64(p.SystemRAMBytes)/(1024*1024*1024))
}

// Detect gathers hardware capabilities from the host system.
func Detect() (*HardwareProfile, error) {
	profile := &HardwareProfile{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}

	// 1. Detect CPU vector instruction sets (Linux)
	detectCPUFeatures(profile)

	// 2. Detect System RAM
	detectSystemRAM(profile)

	// 3. Detect GPU capabilities
	detectGPU(profile)

	return profile, nil
}

func detectCPUFeatures(p *HardwareProfile) {
	if runtime.GOOS != "linux" {
		return
	}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return
	}
	content := string(data)
	if strings.Contains(content, "avx2") {
		p.HasAVX2 = true
	}
	if strings.Contains(content, "avx512") {
		p.HasAVX512 = true
	}
}

func detectSystemRAM(p *HardwareProfile) {
	if runtime.GOOS != "linux" {
		return
	}
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				kb, err := strconv.ParseUint(parts[1], 10, 64)
				if err == nil {
					p.SystemRAMBytes = kb * 1024
				}
			}
			break
		}
	}
}

func detectGPU(p *HardwareProfile) {
	// Check nvidia-smi first
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		if len(lines) > 0 && lines[0] != "" {
			parts := strings.Split(lines[0], ",")
			if len(parts) >= 2 {
				p.GPUName = strings.TrimSpace(parts[0])
				mib, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
				if err == nil {
					p.VRAMBytes = mib * 1024 * 1024
					p.HasNVIDIA = true
					return
				}
			}
		}
	}

	// Apple Silicon Metal detection
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		p.HasAppleMetal = true
		p.GPUName = "Apple Silicon (Unified Memory)"
		// On Apple Silicon, unified memory acts as VRAM
		p.VRAMBytes = p.SystemRAMBytes
	}
}
