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
	"path/filepath"
	"text/tabwriter"

	"github.com/boggycreek/lokol/liblokol/agent"
)

func main() {
	var (
		interactive   = flag.Bool("interactive", false, "Start interactive REPL directly driving SessionCore (alias: -i)")
		interactiveSh = flag.Bool("i", false, "Start interactive REPL (shorthand)")
		scenarioFile  = flag.String("scenarios", "", "Path to scenarios JSONL file (default: tools/scenarios/scenarios.jsonl)")
		scenarioID    = flag.String("scenario", "all", "Specific scenario ID to run, or 'all' (alias: -s)")
		scenarioIDSh  = flag.String("s", "all", "Specific scenario ID to run (shorthand)")
		repeat        = flag.Int("repeat", 1, "Number of times to run each scenario for repeatability testing (alias: -r)")
		repeatSh      = flag.Int("r", 1, "Number of repetitions (shorthand)")
		engineURL     = flag.String("engine", "http://127.0.0.1:8080", "Local inference engine URL (alias: -e)")
		engineURLSh   = flag.String("e", "http://127.0.0.1:8080", "Engine URL (shorthand)")
		modeStr       = flag.String("mode", "general", "Operational mode: general, coding, or moe (alias: -m)")
		modeStrSh     = flag.String("m", "general", "Operational mode (shorthand)")
		fileBeads     = flag.Bool("file-beads", false, "Automatically file bug beads in Beads (bd create) for failing scenarios")
		verbose       = flag.Bool("verbose", false, "Verbose output (alias: -v)")
		verboseSh     = flag.Bool("v", false, "Verbose output (shorthand)")
		listScenarios = flag.Bool("list", false, "List available scenarios from scenarios file (alias: -l)")
		listScenSh    = flag.Bool("l", false, "List scenarios (shorthand)")
	)

	flag.Parse()

	// Consolidate shorthand flags
	isInteractive := *interactive || *interactiveSh
	isVerbose := *verbose || *verboseSh
	isList := *listScenarios || *listScenSh
	activeScenario := *scenarioID
	if activeScenario == "all" && *scenarioIDSh != "all" {
		activeScenario = *scenarioIDSh
	}
	activeEngine := *engineURL
	if activeEngine == "http://127.0.0.1:8080" && *engineURLSh != "http://127.0.0.1:8080" {
		activeEngine = *engineURLSh
	}
	activeModeStr := *modeStr
	if activeModeStr == "general" && *modeStrSh != "general" {
		activeModeStr = *modeStrSh
	}
	numRepeats := *repeat
	if numRepeats == 1 && *repeatSh != 1 {
		numRepeats = *repeatSh
	}

	workDir, _ := os.Getwd()
	repoRoot := findRepoRoot(workDir)

	client := agent.NewClient(activeEngine)
	initialMode, err := agent.ParseMode(activeModeStr)
	if err != nil {
		initialMode = agent.ModeGeneral
	}

	// 1. Interactive Mode
	if isInteractive {
		if err := RunInteractiveREPL(client, workDir, initialMode); err != nil {
			fmt.Fprintf(os.Stderr, "REPL error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Resolve scenarios file
	scPath := *scenarioFile
	if scPath == "" {
		candidate := filepath.Join(repoRoot, "tools", "scenarios", "scenarios.jsonl")
		legacyCandidate := filepath.Join(repoRoot, "tools", "probe", "scenarios.jsonl")
		if _, err := os.Stat(candidate); err == nil {
			scPath = candidate
		} else if _, err := os.Stat(legacyCandidate); err == nil {
			scPath = legacyCandidate
		} else {
			scPath = filepath.Join(workDir, "scenarios.jsonl")
		}
	}

	scenarios, err := LoadScenarios(scPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading scenarios from %s: %v\n", scPath, err)
		os.Exit(1)
	}

	// 2. List Mode
	if isList {
		printScenarioList(scenarios)
		return
	}

	// Filter scenarios to run
	var toRun []Scenario
	if activeScenario == "all" {
		toRun = scenarios
	} else {
		for _, s := range scenarios {
			if s.ID == activeScenario {
				toRun = append(toRun, s)
				break
			}
		}
		if len(toRun) == 0 {
			fmt.Fprintf(os.Stderr, "Unknown scenario ID %q. Run with -l to view available scenarios.\n", activeScenario)
			os.Exit(1)
		}
	}

	fmt.Println("================================================================================")
	fmt.Printf("⚡ lokol In-Process Core Driver (SessionCore)\n")
	fmt.Printf("Engine: %s | Scenarios: %d | Repeats: %d | Beads Autoloader: %v\n",
		activeEngine, len(toRun), numRepeats, *fileBeads)
	fmt.Println("================================================================================")

	runner := NewScenarioRunner(client, workDir, repoRoot, *fileBeads, isVerbose)
	var allResults []*ScenarioResult

	for rep := 1; rep <= numRepeats; rep++ {
		if numRepeats > 1 {
			fmt.Printf("\n--- Iteration %d/%d ---\n", rep, numRepeats)
		}
		for _, sc := range toRun {
			fmt.Printf("\n[Running] %s (%s)...\n", sc.ID, sc.Name)
			res := runner.RunScenario(context.Background(), sc)
			allResults = append(allResults, res)

			if res.Passed {
				fmt.Printf("  ✓ PASS (took %.2fs | %d turns | %d tokens) • Laya: %.2f (%s)\n",
					res.Duration.Seconds(), res.Turns, res.TokensDecoded, res.LayaNoul, res.LayaQuality)
			} else {
				fmt.Printf("  ✗ FAIL (took %.2fs | %d turns | %d tokens)\n",
					res.Duration.Seconds(), res.Turns, res.TokensDecoded)
				for _, r := range res.FailureReasons {
					fmt.Printf("    - %s\n", r)
				}
				if res.BeadID != "" {
					fmt.Printf("    - Filed Bead: %s\n", res.BeadID)
				}
			}
		}
	}

	// Print Summary Table
	fmt.Println("\n================================================================================")
	fmt.Println("Summary of Core Driver Execution:")
	fmt.Println("================================================================================")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Scenario ID\tResult\tDuration\tTurns\tTokens\tLaya\tBead")
	fmt.Fprintln(w, "-----------\t------\t--------\t-----\t------\t----\t----")

	allPassed := true
	for _, r := range allResults {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
			allPassed = false
		}
		beadStr := r.BeadID
		if beadStr == "" {
			beadStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%.2fs\t%d\t%d\t%.2f\t%s\n",
			r.Scenario.ID, status, r.Duration.Seconds(), r.Turns, r.TokensDecoded, r.LayaNoul, beadStr)
	}
	_ = w.Flush()

	if !allPassed {
		os.Exit(1)
	}
}

func printScenarioList(scenarios []Scenario) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Scenario ID\tMode\tTimeout\tName\tDescription")
	fmt.Fprintln(w, "-----------\t----\t-------\t----\t-----------")
	for _, s := range scenarios {
		fmt.Fprintf(w, "%s\t%s\t%.0fs\t%s\t%s\n", s.ID, s.Mode, s.MaxWallClockSec, s.Name, s.Description)
	}
	_ = w.Flush()
}

func findRepoRoot(start string) string {
	curr := start
	for {
		if _, err := os.Stat(filepath.Join(curr, "go.work")); err == nil {
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			return start
		}
		curr = parent
	}
}
