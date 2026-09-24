// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/boggycreek/lokol/pkg/agent"
	"github.com/boggycreek/lokol/pkg/model"
	"github.com/boggycreek/lokol/pkg/probe"
	"github.com/boggycreek/lokol/pkg/setup"
	"github.com/boggycreek/lokol/pkg/tui"
	"github.com/boggycreek/lokol/pkg/update"
	"github.com/boggycreek/lokol/pkg/version"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Top-level flags
	promptFlag := flag.String("prompt", "", "Run prompt directly in headless mode (alias: -p)")
	flag.StringVar(promptFlag, "p", "", "Run prompt directly in headless mode (shorthand)")
	topEngine := flag.String("engine", "http://127.0.0.1:8080", "URL of local llama-server engine")
	topYOLO := flag.Bool("yolo", false, "Engage YOLO mode: autonomous bash execution without interactive approval")
	topMaxTurns := flag.Int("max-turns", 15, "Max turns for agent loop in headless mode")

	// Custom usage func
	flag.Usage = printUsage

	// Subcommands
	probeCmd := flag.NewFlagSet("probe", flag.ExitOnError)
	simVRAM := probeCmd.Float64("simulate-vram-gib", 0, "Simulate a specific VRAM amount in GiB (e.g. 4.0 for GTX 1650)")

	chatCmd := flag.NewFlagSet("chat", flag.ExitOnError)
	engineURL := chatCmd.String("engine", "http://127.0.0.1:8080", "URL of local llama-server engine")
	yolo := chatCmd.Bool("yolo", false, "Engage YOLO mode: autonomous bash execution without interactive approval")

	execCmd := flag.NewFlagSet("exec", flag.ExitOnError)
	execEngine := execCmd.String("engine", "http://127.0.0.1:8080", "URL of local llama-server engine")
	execMaxTurns := execCmd.Int("max-turns", 15, "Max turns for agent loop")

	setupCmd := flag.NewFlagSet("setup", flag.ExitOnError)
	setupDownload := setupCmd.Bool("download-model", false, "Automatically download recommended GGUF weights if missing")
	setupInstallLlama := setupCmd.Bool("install-llama", false, "Automatically download or compile llama.cpp and llama-server if missing")
	setupSimVRAM := setupCmd.Float64("simulate-vram-gib", 0, "Simulate a specific VRAM amount in GiB")

	updateCmd := flag.NewFlagSet("update", flag.ExitOnError)
	updatePre := updateCmd.Bool("pre", false, "Allow updating to unstable pre-release versions")
	updateCmd.BoolVar(updatePre, "prerelease", false, "Allow updating to unstable pre-release versions (alias)")
	updateTargetVer := updateCmd.String("version", "", "Target specific semver version to install (e.g. v0.1.0-alpha.1)")
	updateList := updateCmd.Bool("list", false, "List available releases from GitHub without installing")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Check if top-level flags like -p, --prompt, -h, --help were passed
	if strings.HasPrefix(os.Args[1], "-") {
		_ = flag.CommandLine.Parse(os.Args[1:])
		if *promptFlag != "" {
			runExec(*topEngine, *topMaxTurns, *promptFlag)
			return
		}
		// If remaining arguments exist after parsing flags
		if flag.NArg() > 0 {
			switch flag.Arg(0) {
			case "setup":
				_ = setupCmd.Parse(flag.Args()[1:])
				runSetup(*setupDownload, *setupSimVRAM, *setupInstallLlama)
				return
			case "probe":
				_ = probeCmd.Parse(flag.Args()[1:])
				runProbe(*simVRAM)
				return
			case "chat":
				_ = chatCmd.Parse(flag.Args()[1:])
				runChat(*engineURL, *yolo || *topYOLO)
				return
			case "exec":
				_ = execCmd.Parse(flag.Args()[1:])
				prompt := strings.Join(execCmd.Args(), " ")
				if prompt == "" {
					fmt.Fprintln(os.Stderr, "Error: prompt required for exec")
					os.Exit(1)
				}
				runExec(*execEngine, *execMaxTurns, prompt)
				return
			case "update":
				_ = updateCmd.Parse(flag.Args()[1:])
				runUpdate(*updatePre, *updateTargetVer, *updateList)
				return
			case "version":
				fmt.Printf("lokol %s (commit: %s, built: %s)\n", version.Version, version.GitCommit, version.BuildDate)
				return
			}
		}
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "setup":
		_ = setupCmd.Parse(os.Args[2:])
		runSetup(*setupDownload, *setupSimVRAM, *setupInstallLlama)
	case "probe":
		_ = probeCmd.Parse(os.Args[2:])
		runProbe(*simVRAM)
	case "chat":
		_ = chatCmd.Parse(os.Args[2:])
		runChat(*engineURL, *yolo)
	case "exec":
		_ = execCmd.Parse(os.Args[2:])
		prompt := strings.Join(execCmd.Args(), " ")
		if prompt == "" {
			fmt.Fprintln(os.Stderr, "Error: prompt required for exec")
			os.Exit(1)
		}
		runExec(*execEngine, *execMaxTurns, prompt)
	case "update":
		_ = updateCmd.Parse(os.Args[2:])
		runUpdate(*updatePre, *updateTargetVer, *updateList)
	case "version":
		fmt.Printf("lokol %s (commit: %s, built: %s)\n", version.Version, version.GitCommit, version.BuildDate)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("lokol - Local-first autonomous AI agent for consumer GPUs")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  lokol setup  [--download-model] [--install-llama] Bootstrap environment, probe hardware & check dependencies")
	fmt.Println("  lokol probe  [--simulate-vram-gib=X]              Probe host capabilities and compute optimal model tier")
	fmt.Println("  lokol chat   [--engine=...] [--yolo]              Start interactive Bubble Tea TUI agent session")
	fmt.Println("  lokol exec   [--engine=...] <prompt>              Run autonomous agent in headless mode")
	fmt.Println("  lokol update [--pre] [--version=vX]               Update to latest release from GitHub (or specific version)")
	fmt.Println("  lokol update --list                               List all published releases available on GitHub")
	fmt.Println("  lokol [-p | --prompt] \"<prompt>\"                 Run agent in headless mode directly")
	fmt.Println("  lokol version                                    Display version")
}

func runUpdate(allowPre bool, targetVersion string, listOnly bool) {
	err := update.Run(update.Options{
		Prerelease: allowPre,
		Version:    targetVersion,
		ListOnly:   listOnly,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Update error: %v\n", err)
		os.Exit(1)
	}
}

func runSetup(downloadModel bool, simVRAM float64, installLlama bool) {
	_, err := setup.Run(setup.Options{
		DownloadModel: downloadModel,
		SimulateVRAM:  simVRAM,
		InstallLlama:  installLlama,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Setup error: %v\n", err)
		os.Exit(1)
	}
}

func runExec(engineURL string, maxTurns int, prompt string) {
	workDir, _ := os.Getwd()
	client := agent.NewClient(engineURL)
	runner := &agent.Runner{
		Client:   client,
		MaxTurns: maxTurns,
		YOLO:     true,
		WorkDir:  workDir,
		OnOutput: func(role, content string) {
			switch role {
			case "token":
				fmt.Print(content)
			case "exec_bash":
				fmt.Printf("\n⚡ Executing: %s\n", content)
			case "replace_file":
				fmt.Printf("\n⚡ Editing: %s\n", content)
			case "write_file":
				fmt.Printf("\n⚡ Writing: %s\n", content)
			case "read_outline":
				fmt.Printf("\n⚡ Reading Outline: %s\n", content)
			case "read_window":
				fmt.Printf("\n⚡ Reading Window: %s\n", content)
			case "run_test":
				fmt.Printf("\n⚡ Verifying Tests: %s\n", content)
			case "task_finish", "finish":
				fmt.Printf("\n✅ Complete: %s\n", content)
			}
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	_, err := runner.Run(ctx, prompt)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			fmt.Println("\n[Execution interrupted by signal. Slot released.]")
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runChat(engineURL string, yolo bool) {
	hw, err := probe.Detect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: probe error: %v\n", err)
	}

	workDir, _ := os.Getwd()
	client := agent.NewClient(engineURL)
	m := tui.New(client, hw, yolo, workDir)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func runProbe(simVRAM float64) {
	fmt.Println("==================================================")
	fmt.Println("   lokol System Hardware Capability Probe")
	fmt.Println("==================================================")

	hw, err := probe.Detect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error probing hardware: %v\n", err)
		os.Exit(1)
	}

	if simVRAM > 0 {
		fmt.Printf("[SIMULATION MODE] Overriding detected VRAM with: %.2f GiB\n", simVRAM)
		hw.VRAMBytes = uint64(simVRAM * 1024 * 1024 * 1024)
		hw.HasNVIDIA = true
		hw.GPUName = fmt.Sprintf("Simulated GPU (%.1f GiB VRAM)", simVRAM)
	}

	fmt.Printf("OS / Arch      : %s / %s\n", hw.OS, hw.Arch)
	fmt.Printf("CPU Cores      : %d\n", hw.CPUCores)
	fmt.Printf("AVX2 / AVX512  : %t / %t\n", hw.HasAVX2, hw.HasAVX512)
	fmt.Printf("System RAM     : %s\n", hw.HumanRAM())
	if hw.GPUName != "" {
		fmt.Printf("GPU Detected   : %s\n", hw.GPUName)
		fmt.Printf("VRAM Total     : %s\n", hw.HumanVRAM())
	} else {
		fmt.Println("GPU Detected   : None (CPU Only)")
	}
	fmt.Println("--------------------------------------------------")

	rec := model.SelectOptimalModel(hw)
	fmt.Printf("Target Tier    : %s\n", rec.Tier)
	fmt.Printf("Optimal Model  : %s\n", rec.ModelName)
	fmt.Printf("HuggingFace    : %s / %s\n", rec.HFRepo, rec.HFFile)
	fmt.Printf("Max Context    : %d tokens (KV Cache Quant: %s)\n", rec.ContextLength, rec.KVCacheQuant)
	fmt.Printf("Engine Slots   : %d slot (-np %d, 100%% VRAM allocated to active agent)\n", rec.ParallelSlots, rec.ParallelSlots)
	fmt.Printf("GPU Offload    : %d layers (Est VRAM: ~%d MB)\n", rec.GPULayers, rec.EstimatedVRAMMB)
	fmt.Println("--------------------------------------------------")
	fmt.Printf("Notes: %s\n", rec.Notes)
	fmt.Println("==================================================")
}

