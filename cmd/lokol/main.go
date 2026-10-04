// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/config"
	"github.com/boggycreek/lokol/liblokol/model"
	"github.com/boggycreek/lokol/liblokol/probe"
	"github.com/boggycreek/lokol/liblokol/setup"
	"github.com/boggycreek/lokol/liblokol/spec"
	"github.com/boggycreek/lokol/liblokol/update"
	"github.com/boggycreek/lokol/liblokol/version"
)

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	// Top-level flags
	topFlags := flag.NewFlagSet("lokol", flag.ContinueOnError)
	topFlags.SetOutput(stderr)

	promptFlag := topFlags.String("prompt", "", "Run prompt directly in headless mode (alias: -p)")
	topFlags.StringVar(promptFlag, "p", "", "Run prompt directly in headless mode (shorthand)")
	topMode := topFlags.String("mode", "general", "Operational mode: general (default), coding, or moe (alias: -m)")
	topFlags.StringVar(topMode, "m", "general", "Operational mode (shorthand)")
	topName := topFlags.String("name", "", "Agent persona name (overrides persistent config)")
	topOperator := topFlags.String("operator", "", "Operator name (overrides persistent config)")
	topEngine := topFlags.String("engine", "http://127.0.0.1:8080", "URL of local llama-server engine")
	topMaxTurns := topFlags.Int("max-turns", 15, "Max turns for agent loop in headless mode")
	topVerbose := topFlags.Bool("verbose", false, "Display internal reasoning tokens and verbose tool activity")
	topFlags.BoolVar(topVerbose, "v", false, "Display internal reasoning tokens (shorthand)")
	topSpec := topFlags.String("spec", "", "Path to SPEC.md structured specification file (alias: -s)")
	topFlags.StringVar(topSpec, "s", "", "Path to SPEC.md structured specification file (shorthand)")
	topVersion := topFlags.Bool("version", false, "Display version and exit")

	// Custom usage func
	topFlags.Usage = func() {
		printUsage(stdout)
	}

	// Subcommands
	probeCmd := flag.NewFlagSet("probe", flag.ContinueOnError)
	probeCmd.SetOutput(stderr)
	simVRAM := probeCmd.Float64("simulate-vram-gib", 0, "Simulate a specific VRAM amount in GiB (e.g. 4.0 for GTX 1650)")
	probeMode := probeCmd.String("mode", "general", "Operational mode: general (default), coding, or moe (alias: -m)")
	probeCmd.StringVar(probeMode, "m", "general", "Operational mode (shorthand)")

	execCmd := flag.NewFlagSet("exec", flag.ContinueOnError)
	execCmd.SetOutput(stderr)
	execEngine := execCmd.String("engine", "http://127.0.0.1:8080", "URL of local llama-server engine")
	execMode := execCmd.String("mode", "coding", "Operational mode: coding (default for exec), general, or moe (alias: -m)")
	execCmd.StringVar(execMode, "m", "coding", "Operational mode (shorthand)")
	execName := execCmd.String("name", "", "Agent persona name (overrides persistent config)")
	execOperator := execCmd.String("operator", "", "Operator name (overrides persistent config)")
	execMaxTurns := execCmd.Int("max-turns", 15, "Max turns for agent loop")
	execVerbose := execCmd.Bool("verbose", false, "Display internal reasoning tokens and verbose tool activity")
	execCmd.BoolVar(execVerbose, "v", false, "Display internal reasoning tokens (shorthand)")
	execSpec := execCmd.String("spec", "", "Path to SPEC.md structured specification file (alias: -s)")
	execCmd.StringVar(execSpec, "s", "", "Path to SPEC.md structured specification file (shorthand)")

	setupCmd := flag.NewFlagSet("setup", flag.ContinueOnError)
	setupCmd.SetOutput(stderr)
	setupDownload := setupCmd.Bool("download-model", false, "Automatically download recommended GGUF weights if missing")
	setupInstallLlama := setupCmd.Bool("install-llama", false, "Automatically download or compile llama.cpp and llama-server if missing")
	setupSimVRAM := setupCmd.Float64("simulate-vram-gib", 0, "Simulate a specific VRAM amount in GiB")

	updateCmd := flag.NewFlagSet("update", flag.ContinueOnError)
	updateCmd.SetOutput(stderr)
	updatePre := updateCmd.Bool("pre", false, "Allow updating to unstable pre-release versions")
	updateCmd.BoolVar(updatePre, "prerelease", false, "Allow updating to unstable pre-release versions (alias)")
	updateTargetVer := updateCmd.String("version", "", "Target specific semver version to install (e.g. v0.1.0-alpha.1)")
	updateList := updateCmd.Bool("list", false, "List available releases from GitHub without installing")

	if len(args) < 2 {
		printUsage(stderr)
		return 1
	}

	// Check if top-level flags like -p, --prompt, -h, --help, --version were passed
	if strings.HasPrefix(args[1], "-") {
		if err := topFlags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 1
		}
		if *topVersion {
			fmt.Fprintf(stdout, "lokol %s (commit: %s, built: %s)\n", version.Version, version.GitCommit, version.BuildDate)
			return 0
		}
		if *promptFlag != "" || *topSpec != "" {
			m, _ := agent.ParseMode(*topMode)
			return runExec(*topEngine, *topMaxTurns, *promptFlag, *topVerbose, m, *topSpec, stdout, stderr, *topName, *topOperator)
		}
		// If remaining arguments exist after parsing flags
		if topFlags.NArg() > 0 {
			switch topFlags.Arg(0) {
			case "config":
				return runConfig(topFlags.Args()[1:], stdout, stderr)
			case "setup":
				if err := setupCmd.Parse(topFlags.Args()[1:]); err != nil {
					if errors.Is(err, flag.ErrHelp) {
						return 0
					}
					return 1
				}
				return runSetup(*setupDownload, *setupSimVRAM, *setupInstallLlama, stdout, stderr)
			case "probe":
				if err := probeCmd.Parse(topFlags.Args()[1:]); err != nil {
					if errors.Is(err, flag.ErrHelp) {
						return 0
					}
					return 1
				}
				m, _ := agent.ParseMode(*probeMode)
				return runProbe(*simVRAM, m, stdout, stderr)
			case "exec":
				if err := execCmd.Parse(topFlags.Args()[1:]); err != nil {
					if errors.Is(err, flag.ErrHelp) {
						return 0
					}
					return 1
				}
				prompt := strings.Join(execCmd.Args(), " ")
				specPath := *execSpec
				if specPath == "" {
					specPath = *topSpec
				}
				if prompt == "" && specPath == "" {
					fmt.Fprintln(stderr, "Error: prompt required for exec (or specify --spec)")
					return 1
				}
				m, _ := agent.ParseMode(*execMode)
				nameArg := *execName
				if nameArg == "" {
					nameArg = *topName
				}
				opArg := *execOperator
				if opArg == "" {
					opArg = *topOperator
				}
				return runExec(*execEngine, *execMaxTurns, prompt, *execVerbose || *topVerbose, m, specPath, stdout, stderr, nameArg, opArg)
			case "update":
				if err := updateCmd.Parse(topFlags.Args()[1:]); err != nil {
					if errors.Is(err, flag.ErrHelp) {
						return 0
					}
					return 1
				}
				return runUpdate(*updatePre, *updateTargetVer, *updateList, stdout, stderr)
			case "version":
				fmt.Fprintf(stdout, "lokol %s (commit: %s, built: %s)\n", version.Version, version.GitCommit, version.BuildDate)
				return 0
			}
		}
		printUsage(stderr)
		return 1
	}

	switch args[1] {
	case "config":
		return runConfig(args[2:], stdout, stderr)
	case "setup":
		if err := setupCmd.Parse(args[2:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 1
		}
		return runSetup(*setupDownload, *setupSimVRAM, *setupInstallLlama, stdout, stderr)
	case "probe":
		if err := probeCmd.Parse(args[2:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 1
		}
		m, _ := agent.ParseMode(*probeMode)
		return runProbe(*simVRAM, m, stdout, stderr)
	case "exec":
		if err := execCmd.Parse(args[2:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 1
		}
		prompt := strings.Join(execCmd.Args(), " ")
		if prompt == "" && *execSpec == "" {
			fmt.Fprintln(stderr, "Error: prompt required for exec (or specify --spec)")
			return 1
		}
		m, _ := agent.ParseMode(*execMode)
		return runExec(*execEngine, *execMaxTurns, prompt, *execVerbose, m, *execSpec, stdout, stderr, *execName, *execOperator)
	case "update":
		if err := updateCmd.Parse(args[2:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 1
		}
		return runUpdate(*updatePre, *updateTargetVer, *updateList, stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "lokol %s (commit: %s, built: %s)\n", version.Version, version.GitCommit, version.BuildDate)
		return 0
	default:
		printUsage(stderr)
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "lokol - Local-first autonomous AI agent for consumer GPUs")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  lokol exec   [--spec=SPEC.md] [--engine=...] [-m general|coding|moe] [--name=...] [--operator=...] [-v] [prompt]  Run autonomous agent in headless mode")
	fmt.Fprintln(w, "  lokol setup  [--download-model] [--install-llama]                  Bootstrap environment, probe hardware & check dependencies")
	fmt.Fprintln(w, "  lokol probe  [--simulate-vram-gib=X] [-m general|coding|moe]       Probe host capabilities and compute optimal model tier")
	fmt.Fprintln(w, "  lokol config [show | set-name <name> | set-operator <name>]        Inspect or update persistent agent persona configuration")
	fmt.Fprintln(w, "  lokol update [--pre] [--version=vX]                                Update to latest release from GitHub (or specific version)")
	fmt.Fprintln(w, "  lokol update --list                                                List all published releases available on GitHub")
	fmt.Fprintln(w, "  lokol [-p | --prompt] \"<prompt>\" [-s | --spec SPEC.md] [-m mode] [-v]  Run agent in headless mode directly")
	fmt.Fprintln(w, "  lokol version                                                     Display version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Interactive TUI:")
	fmt.Fprintln(w, "  lk           [--engine=...] [-m general|coding|moe] [--name=...] [--operator=...] [--yolo] [-v]  Launch interactive Bubble Tea TUI agent")
}

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "show" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(stderr, "Error loading config: %v\n", err)
			return 1
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "Error marshaling config: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(data))
		return 0
	}

	switch args[0] {
	case "set-name":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			fmt.Fprintf(stderr, "Error: missing name argument. Usage: lokol config set-name <agent-name>\n")
			return 1
		}
		name := strings.TrimSpace(args[1])
		cfg, _ := config.Load()
		cfg.AgentName = name
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(stderr, "Error saving config: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Agent persona name set to %q in %s\n", name, config.ConfigPath())
		return 0

	case "set-operator":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			fmt.Fprintf(stderr, "Error: missing operator argument. Usage: lokol config set-operator <operator-name>\n")
			return 1
		}
		op := strings.TrimSpace(args[1])
		cfg, _ := config.Load()
		cfg.OperatorName = op
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(stderr, "Error saving config: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Operator name set to %q in %s\n", op, config.ConfigPath())
		return 0

	default:
		fmt.Fprintf(stderr, "Unknown config command %q. Usage: lokol config [show | set-name <name> | set-operator <name>]\n", args[0])
		return 1
	}
}

func runUpdate(allowPre bool, targetVersion string, listOnly bool, stdout, stderr io.Writer) int {
	err := update.Run(update.Options{
		Prerelease: allowPre,
		Version:    targetVersion,
		ListOnly:   listOnly,
	})
	if err != nil {
		fmt.Fprintf(stderr, "Update error: %v\n", err)
		return 1
	}
	return 0
}

func runSetup(downloadModel bool, simVRAM float64, installLlama bool, stdout, stderr io.Writer) int {
	_, err := setup.Run(setup.Options{
		DownloadModel: downloadModel,
		SimulateVRAM:  simVRAM,
		InstallLlama:  installLlama,
	})
	if err != nil {
		fmt.Fprintf(stderr, "Setup error: %v\n", err)
		return 1
	}
	return 0
}

func runExec(engineURL string, maxTurns int, prompt string, verbose bool, mode agent.Mode, specPath string, stdout, stderr io.Writer, persona ...string) int {
	workDir, _ := os.Getwd()
	client := agent.NewClient(engineURL)
	stepCount := 0
	finished := false

	var agentName, operatorName string
	if len(persona) > 0 {
		agentName = persona[0]
	}
	if len(persona) > 1 {
		operatorName = persona[1]
	}

	var specMachine *spec.StateMachine
	if specPath != "" {
		sp, err := spec.Load(specPath)
		if err != nil {
			fmt.Fprintf(stderr, "Error loading spec %s: %v\n", specPath, err)
			return 1
		}
		if prompt == "" {
			prompt = sp.Prompt()
		} else {
			prompt = fmt.Sprintf("%s\n\nAdditional Operator Instructions:\n%s", sp.Prompt(), prompt)
		}
		if sp.MaxTurns > 0 && maxTurns == 15 {
			maxTurns = sp.MaxTurns
		}
		specMachine = spec.NewStateMachine(sp, workDir)
	}

	runner := &agent.Runner{
		Client:       client,
		MaxTurns:     maxTurns,
		YOLO:         true,
		WorkDir:      workDir,
		Mode:         mode,
		AgentName:    agentName,
		OperatorName: operatorName,
		SpecMachine:  specMachine,
		OnOutput: func(role, content string) {
			if verbose {
				switch role {
				case "token":
					fmt.Fprint(stdout, content)
				case "spec":
					fmt.Fprintf(stdout, "\n⚡ Spec Gate: %s\n", content)
				case "exec_bash":
					fmt.Fprintf(stdout, "\n⚡ Executing: %s\n", content)
				case "replace_file":
					fmt.Fprintf(stdout, "\n⚡ Editing: %s\n", content)
				case "write_file":
					fmt.Fprintf(stdout, "\n⚡ Writing: %s\n", content)
				case "read_outline":
					fmt.Fprintf(stdout, "\n⚡ Reading Outline: %s\n", content)
				case "read_window":
					fmt.Fprintf(stdout, "\n⚡ Reading Window: %s\n", content)
				case "run_test":
					fmt.Fprintf(stdout, "\n⚡ Verifying Tests: %s\n", content)
				case "get_environment":
					fmt.Fprintf(stdout, "\n⚡ Inspecting Environment: %s\n", content)
				case "task_finish", "finish":
					finished = true
					fmt.Fprintf(stdout, "\n✅ Complete: %s\n", content)
				default:
					if role != "error" && role != "result" {
						fmt.Fprintf(stdout, "\n⚡ Executing %s: %s\n", role, content)
					}
				}
				return
			}

			// Clean internalized presentation:
			// Suppress intermediate thought tokens.
			// Emit compact progress on stderr so stdout remains clean for piping.
			switch role {
			case "token", "error", "result":
				// Internalized
			case "exec_bash":
				stepCount++
				fmt.Fprintf(stderr, "⚡ [Step %d] Executing: %s\n", stepCount, content)
			case "replace_file":
				stepCount++
				fmt.Fprintf(stderr, "⚡ [Step %d] Editing: %s\n", stepCount, content)
			case "write_file":
				stepCount++
				fmt.Fprintf(stderr, "⚡ [Step %d] Writing: %s\n", stepCount, content)
			case "read_outline":
				stepCount++
				fmt.Fprintf(stderr, "⚡ [Step %d] Reading Outline: %s\n", stepCount, content)
			case "read_window":
				stepCount++
				fmt.Fprintf(stderr, "⚡ [Step %d] Reading Window: %s\n", stepCount, content)
			case "run_test":
				stepCount++
				fmt.Fprintf(stderr, "⚡ [Step %d] Verifying Tests: %s\n", stepCount, content)
			case "get_environment":
				stepCount++
				fmt.Fprintf(stderr, "⚡ [Step %d] Inspecting Environment: %s\n", stepCount, content)
			case "spec":
				fmt.Fprintf(stderr, "⚡ [Spec Gate] %s\n", content)
			case "task_finish", "finish":
				finished = true
				plural := ""
				if stepCount > 1 {
					plural = "s"
				}
				stepInfo := ""
				if stepCount > 0 {
					stepInfo = fmt.Sprintf(" (in %d step%s)", stepCount, plural)
				}
				fmt.Fprintf(stdout, "\n✅ Complete%s: %s\n", stepInfo, content)
			default:
				stepCount++
				fmt.Fprintf(stderr, "⚡ [Step %d] Executing %s: %s\n", stepCount, role, content)
			}
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	summary, err := runner.Run(ctx, prompt)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			fmt.Fprintln(stdout, "\n[Execution interrupted by signal. Slot released.]")
			return 130
		}
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	if !finished && summary != "" {
		fmt.Fprintln(stdout, summary)
	}
	return 0
}

func runProbe(simVRAM float64, mode agent.Mode, stdout, stderr io.Writer) int {
	fmt.Fprintln(stdout, "==================================================")
	fmt.Fprintln(stdout, "   lokol System Hardware Capability Probe")
	fmt.Fprintln(stdout, "==================================================")

	hw, err := probe.Detect()
	if err != nil {
		fmt.Fprintf(stderr, "Error probing hardware: %v\n", err)
		return 1
	}

	if simVRAM > 0 {
		fmt.Fprintf(stdout, "[SIMULATION MODE] Overriding detected VRAM with: %.2f GiB\n", simVRAM)
		hw.VRAMBytes = uint64(simVRAM * 1024 * 1024 * 1024)
		hw.HasNVIDIA = true
		hw.GPUName = fmt.Sprintf("Simulated GPU (%.1f GiB VRAM)", simVRAM)
	}

	fmt.Fprintf(stdout, "OS / Arch      : %s / %s\n", hw.OS, hw.Arch)
	fmt.Fprintf(stdout, "CPU Cores      : %d\n", hw.CPUCores)
	fmt.Fprintf(stdout, "AVX2 / AVX512  : %t / %t\n", hw.HasAVX2, hw.HasAVX512)
	fmt.Fprintf(stdout, "System RAM     : %s\n", hw.HumanRAM())
	if hw.GPUName != "" {
		fmt.Fprintf(stdout, "GPU Detected   : %s\n", hw.GPUName)
		fmt.Fprintf(stdout, "VRAM Total     : %s\n", hw.HumanVRAM())
	} else {
		fmt.Fprintln(stdout, "GPU Detected   : None (CPU Only)")
	}
	fmt.Fprintln(stdout, "--------------------------------------------------")

	rec := model.SelectOptimalModelForMode(hw, model.Mode(mode))
	fmt.Fprintf(stdout, "Target Mode    : %s\n", mode)
	fmt.Fprintf(stdout, "Target Tier    : %s\n", rec.Tier)
	fmt.Fprintf(stdout, "Optimal Model  : %s\n", rec.ModelName)
	fmt.Fprintf(stdout, "HuggingFace    : %s / %s\n", rec.HFRepo, rec.HFFile)
	fmt.Fprintf(stdout, "Max Context    : %d tokens (KV Cache Quant: %s)\n", rec.ContextLength, rec.KVCacheQuant)
	fmt.Fprintf(stdout, "Engine Slots   : %d slot (-np %d, 100%% VRAM allocated to active agent)\n", rec.ParallelSlots, rec.ParallelSlots)
	fmt.Fprintf(stdout, "GPU Offload    : %d layers (Est VRAM: ~%d MB)\n", rec.GPULayers, rec.EstimatedVRAMMB)
	fmt.Fprintln(stdout, "--------------------------------------------------")
	fmt.Fprintf(stdout, "Notes: %s\n", rec.Notes)
	fmt.Fprintln(stdout, "==================================================")
	return 0
}
