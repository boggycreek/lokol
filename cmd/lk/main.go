// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/boggycreek/lokol/cmd/lk/tui"
	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/version"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	engineFlag := flag.String("engine", "http://127.0.0.1:8080", "URL of local llama-server engine")
	yoloFlag := flag.Bool("yolo", false, "Engage YOLO mode: autonomous action execution without approval prompts")
	verboseFlag := flag.Bool("verbose", false, "Display internal reasoning tokens and tool activity")
	flag.BoolVar(verboseFlag, "v", false, "Display internal reasoning (shorthand)")
	promptFlag := flag.String("prompt", "", "Initial prompt to execute (alias: -p)")
	flag.StringVar(promptFlag, "p", "", "Initial prompt to execute (shorthand)")
	versionFlag := flag.Bool("version", false, "Display version information and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lk [options] [prompt]\n\n")
		fmt.Fprintf(os.Stderr, "lk is the high-velocity interactive terminal coding agent for lokol.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *versionFlag {
		fmt.Printf("lk %s\n", version.FullVersion())
		os.Exit(0)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current working directory: %v\n", err)
		os.Exit(1)
	}

	client := agent.NewClient(*engineFlag)
	session := agent.NewSession(client, cwd)

	model := tui.NewModel(session, *yoloFlag, *verboseFlag)

	// If prompt passed via flags or positional arguments, pre-populate
	initialPrompt := *promptFlag
	if initialPrompt == "" && flag.NArg() > 0 {
		initialPrompt = strings.Join(flag.Args(), " ")
	}
	if initialPrompt != "" {
		model = model.WithInitialPrompt(initialPrompt)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running lk: %v\n", err)
		os.Exit(1)
	}
}
