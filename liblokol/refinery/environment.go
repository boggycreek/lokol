// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package refinery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/boggycreek/lokol/liblokol/probe"
)

// HardwareInfo captures host compute and GPU accelerator metrics.
type HardwareInfo struct {
	CPU  string `json:"cpu,omitempty"`
	RAM  string `json:"ram,omitempty"`
	GPU  string `json:"gpu,omitempty"`
	VRAM string `json:"vram,omitempty"`
}

// GitInfo represents the detected git repository state for a workspace.
type GitInfo struct {
	IsRepo bool   `json:"is_repo"`
	Root   string `json:"root,omitempty"`
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
	Dirty  bool   `json:"dirty,omitempty"`
}

// EnvironmentInfo provides a comprehensive snapshot of the host execution environment.
type EnvironmentInfo struct {
	WorkingDirectory string            `json:"working_directory"`
	OS               string            `json:"os"`
	Arch             string            `json:"arch"`
	Shell            string            `json:"shell"`
	User             string            `json:"user"`
	Hardware         *HardwareInfo     `json:"hardware,omitempty"`
	Git              GitInfo           `json:"git"`
	Toolchains       map[string]string `json:"toolchains"`
	Files            []string          `json:"files"`
}

// GetEnvironment inspects the given working directory and host environment,
// returning a structured EnvironmentInfo snapshot.
func GetEnvironment(workDir string) (*EnvironmentInfo, error) {
	if workDir == "" {
		if d, err := os.Getwd(); err == nil {
			workDir = d
		}
	}
	if abs, err := filepath.Abs(workDir); err == nil {
		workDir = abs
	} else {
		workDir = filepath.Clean(workDir)
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "powershell.exe"
		} else {
			shell = "/bin/bash"
		}
	}

	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		username = u.Username
	}

	// Git inspection
	gitInfo := detectGitInfo(workDir)

	// Toolchain detection
	toolchains := detectToolchains()

	// Top-level workspace files
	var files []string
	if entries, err := os.ReadDir(workDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") && name != ".github" {
				continue
			}
			if e.IsDir() {
				files = append(files, name+"/")
			} else {
				files = append(files, name)
			}
			if len(files) >= 50 {
				files = append(files, "...[truncated]")
				break
			}
		}
	}

	// Hardware profile detection
	var hwInfo *HardwareInfo
	if hw, err := probe.Detect(); err == nil && hw != nil {
		hwInfo = &HardwareInfo{
			CPU:  fmt.Sprintf("%d cores", hw.CPUCores),
			RAM:  hw.HumanRAM(),
			GPU:  hw.GPUName,
			VRAM: hw.HumanVRAM(),
		}
	}

	return &EnvironmentInfo{
		WorkingDirectory: workDir,
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		Shell:            shell,
		User:             username,
		Hardware:         hwInfo,
		Git:              gitInfo,
		Toolchains:       toolchains,
		Files:            files,
	}, nil
}

// FormatJSON serializes EnvironmentInfo to formatted JSON.
func (env *EnvironmentInfo) FormatJSON() (string, error) {
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func detectGitInfo(workDir string) GitInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return GitInfo{IsRepo: false}
	}

	root := strings.TrimSpace(string(out))
	info := GitInfo{
		IsRepo: true,
		Root:   root,
	}

	// Current branch
	bCmd := exec.CommandContext(ctx, "git", "branch", "--show-current")
	bCmd.Dir = workDir
	if bOut, bErr := bCmd.Output(); bErr == nil {
		info.Branch = strings.TrimSpace(string(bOut))
	}

	// Short commit hash
	cCmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	cCmd.Dir = workDir
	if cOut, cErr := cCmd.Output(); cErr == nil {
		info.Commit = strings.TrimSpace(string(cOut))
	}

	// Dirty state
	sCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	sCmd.Dir = workDir
	if sOut, sErr := sCmd.Output(); sErr == nil {
		info.Dirty = len(bytes.TrimSpace(sOut)) > 0
	}

	return info
}

func detectToolchains() map[string]string {
	toolchains := make(map[string]string)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	probes := []struct {
		name string
		args []string
		fn   func(string) string
	}{
		{
			name: "go",
			args: []string{"version"},
			fn: func(out string) string {
				// "go version go1.24.0 linux/amd64" -> "go1.24.0"
				parts := strings.Fields(out)
				if len(parts) >= 3 {
					return parts[2]
				}
				return strings.TrimSpace(out)
			},
		},
		{
			name: "git",
			args: []string{"--version"},
			fn: func(out string) string {
				// "git version 2.43.0"
				return strings.TrimSpace(out)
			},
		},
		{
			name: "python3",
			args: []string{"--version"},
			fn: func(out string) string {
				return strings.TrimSpace(out)
			},
		},
		{
			name: "podman",
			args: []string{"--version"},
			fn: func(out string) string {
				return strings.TrimSpace(out)
			},
		},
		{
			name: "docker",
			args: []string{"--version"},
			fn: func(out string) string {
				return strings.TrimSpace(out)
			},
		},
	}

	for _, p := range probes {
		if _, err := exec.LookPath(p.name); err != nil {
			continue
		}
		cmd := exec.CommandContext(ctx, p.name, p.args...)
		if out, err := cmd.Output(); err == nil {
			val := p.fn(string(out))
			if val != "" {
				toolchains[p.name] = val
			}
		}
	}

	return toolchains
}
