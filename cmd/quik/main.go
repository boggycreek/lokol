package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/boggycreek/quik/pkg/agent"
	"github.com/boggycreek/quik/pkg/model"
	"github.com/boggycreek/quik/pkg/probe"
	"github.com/boggycreek/quik/pkg/tui"
	tea "github.com/charmbracelet/bubbletea"
)

const version = "0.1.0"

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
			case "version":
				fmt.Printf("quik version %s\n", version)
				return
			}
		}
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
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
	case "version":
		fmt.Printf("quik version %s\n", version)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("quik - High Performance Local-First Coding Agent Engine")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  quik [-p | --prompt] \"<prompt>\"      Run agent in headless mode directly")
	fmt.Println("  quik chat  [--engine=...] [--yolo]   Start interactive Bubble Tea TUI agent session")
	fmt.Println("  quik exec  [--engine=...] <prompt>   Run autonomous agent in headless mode")
	fmt.Println("  quik probe [--simulate-vram-gib=X]   Probe host capabilities and compute optimal model tier")
	fmt.Println("  quik version                         Display version")
}

func runExec(engineURL string, maxTurns int, prompt string) {
	client := agent.NewClient(engineURL)
	runner := &agent.Runner{
		Client:   client,
		MaxTurns: maxTurns,
		YOLO:     true,
		OnOutput: func(role, content string) {
			switch role {
			case "token":
				fmt.Print(content)
			case "exec_bash":
				fmt.Printf("\n⚡ Executing: %s\n", content)
			case "result":
				fmt.Println("✓ Done")
			case "error":
				fmt.Printf("✗ %s\n", content)
			case "finish":
				fmt.Printf("\n✅ Complete: %s\n", content)
			}
		},
	}

	_, err := runner.Run(context.Background(), prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nAgent error: %v\n", err)
		os.Exit(1)
	}
}

func runChat(engineURL string, yolo bool) {
	hw, err := probe.Detect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: probe error: %v\n", err)
	}

	client := agent.NewClient(engineURL)
	m := tui.New(client, hw, yolo)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func runProbe(simVRAM float64) {
	fmt.Println("==================================================")
	fmt.Println("   quik System Hardware Capability Probe")
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
	fmt.Printf("GPU Offload    : %d layers (Est VRAM: ~%d MB)\n", rec.GPULayers, rec.EstimatedVRAMMB)
	fmt.Println("--------------------------------------------------")
	fmt.Printf("Notes: %s\n", rec.Notes)
	fmt.Println("==================================================")
}
