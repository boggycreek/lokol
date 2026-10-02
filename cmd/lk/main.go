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
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/boggycreek/lokol/cmd/lk/tui"
	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/probe"
	"github.com/boggycreek/lokol/liblokol/version"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lk", flag.ContinueOnError)
	flags.SetOutput(stderr)

	engineFlag := flags.String("engine", "http://127.0.0.1:8080", "URL of local llama-server engine")
	modeFlag := flags.String("mode", "general", "Operational mode: general (default), coding, or moe (alias: -m)")
	flags.StringVar(modeFlag, "m", "general", "Operational mode (shorthand)")
	yoloFlag := flags.Bool("yolo", false, "Engage YOLO mode: autonomous action execution without approval prompts")
	verboseFlag := flags.Bool("verbose", false, "Display internal reasoning tokens and tool activity")
	flags.BoolVar(verboseFlag, "v", false, "Display internal reasoning (shorthand)")
	promptFlag := flags.String("prompt", "", "Initial prompt to execute (alias: -p)")
	flags.StringVar(promptFlag, "p", "", "Initial prompt to execute (shorthand)")
	nameFlag := flags.String("name", "", "Agent persona name (overrides persistent config)")
	operatorFlag := flags.String("operator", "", "Operator name (overrides persistent config)")
	versionFlag := flags.Bool("version", false, "Display version information and exit")

	flags.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lk [options] [prompt]\n\n")
		fmt.Fprintf(stderr, "lk is the high-velocity interactive terminal agent for lokol.\n\n")
		fmt.Fprintf(stderr, "Options:\n")
		flags.PrintDefaults()
	}

	var parseArgs []string
	if len(args) > 1 {
		parseArgs = args[1:]
	}
	if err := flags.Parse(parseArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if *versionFlag {
		fmt.Fprintf(stdout, "lk %s\n", version.FullVersion())
		return 0
	}

	mode, err := agent.ParseMode(*modeFlag)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "Error getting current working directory: %v\n", err)
		return 1
	}

	client := agent.NewClient(*engineFlag)
	session := agent.NewSessionWithMode(client, cwd, mode)

	hw, _ := probe.Detect()
	model := tui.NewWithSession(session, hw, *yoloFlag, *nameFlag, *operatorFlag)
	model.SetVerbose(*verboseFlag)

	// If prompt passed via flags or positional arguments, pre-populate
	initialPrompt := *promptFlag
	if initialPrompt == "" && flags.NArg() > 0 {
		initialPrompt = strings.Join(flags.Args(), " ")
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
		fmt.Fprintf(stderr, "Error running lk: %v\n", err)
		return 1
	}

	return 0
}
