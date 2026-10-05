// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/boggycreek/lokol/liblokol/ipc"
	"github.com/boggycreek/lokol/liblokol/version"
)

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lokol-daemon", flag.ContinueOnError)
	flags.SetOutput(stderr)

	sockPath := flags.String("socket", "", "Path to UNIX domain socket (default: /tmp/lokol.sock)")
	tcpAddr := flags.String("listen", "", "TCP listen address (e.g. 127.0.0.1:4444)")
	engineURL := flags.String("engine", "http://127.0.0.1:8080", "URL of local llama-server engine")
	workDir := flags.String("work-dir", ".", "Default working directory for sessions")
	mode := flags.String("mode", "coding", "Default operational mode (coding, general, moe)")
	yolo := flags.Bool("yolo", false, "Autonomous mode (bypass interactive action approval gates)")
	maxTurns := flags.Int("max-turns", 20, "Default max turns per prompt")
	showVersion := flags.Bool("version", false, "Display version and exit")

	if err := flags.Parse(args[1:]); err != nil {
		return 1
	}

	if *showVersion {
		fmt.Fprintf(stdout, "lokol-daemon %s (commit: %s, built: %s)\n", version.Version, version.GitCommit, version.BuildDate)
		return 0
	}

	network := "unix"
	address := *sockPath
	if *tcpAddr != "" {
		network = "tcp"
		address = *tcpAddr
	} else if address == "" {
		address = filepath.Join(os.TempDir(), "lokol.sock")
	}

	srv := ipc.NewServer(ipc.ServerConfig{
		EngineURL:       *engineURL,
		DefaultWorkDir:  *workDir,
		DefaultYOLO:     *yolo,
		DefaultMode:     *mode,
		DefaultMaxTurns: *maxTurns,
	})

	if err := srv.Listen(network, address); err != nil {
		fmt.Fprintf(stderr, "Error listening on %s (%s): %v\n", address, network, err)
		return 1
	}
	defer srv.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(stdout, "lokol-daemon %s listening on %s (%s)...\n", version.Version, srv.Addr().String(), network)
	if *yolo {
		fmt.Fprintln(stdout, "⚡ Running in YOLO autonomous mode (approval gates disabled)")
	} else {
		fmt.Fprintln(stdout, "🛡️ Running in interactive mode (approval gates enabled)")
	}

	if err := srv.Serve(ctx); err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
		fmt.Fprintf(stderr, "Server error: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "lokol-daemon stopped gracefully.")
	return 0
}
