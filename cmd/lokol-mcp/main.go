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
	"strconv"
	"strings"
	"syscall"

	"github.com/boggycreek/lokol/pkg/mcp"
	"github.com/boggycreek/lokol/pkg/tools/refinery"
	"github.com/boggycreek/lokol/pkg/version"
)

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("lokol-mcp %s (commit: %s, built: %s)\n", version.Version, version.GitCommit, version.BuildDate)
		return
	}

	server := mcp.NewServer("lokol-mcp", version.Version)

	// Register read_outline tool
	server.RegisterTool(mcp.Tool{
		Name:        "read_outline",
		Description: "Extracts high-level structural declarations (types, interfaces, function signatures) without dumping inner function bodies.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"path": {
					Type:        "string",
					Description: "Path to the source file to inspect",
				},
			},
			Required: []string{"path"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			path := getStringArg(args, "path")
			if path == "" {
				return "", true, fmt.Errorf("missing required argument 'path'")
			}
			outline, err := refinery.ReadOutline(path)
			if err != nil {
				return "", true, err
			}
			return outline, false, nil
		},
	})

	// Register read_window tool
	server.RegisterTool(mcp.Tool{
		Name:        "read_window",
		Description: "Reads a specific slice of lines from a file (maximum 120 lines) to prevent whole-file dumping.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"path": {
					Type:        "string",
					Description: "Path to the file to read",
				},
				"start_line": {
					Type:        "integer",
					Description: "1-based starting line number (default: 1)",
				},
				"end_line": {
					Type:        "integer",
					Description: "1-based ending line number (default: start_line + 50)",
				},
			},
			Required: []string{"path"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			path := getStringArg(args, "path")
			if path == "" {
				return "", true, fmt.Errorf("missing required argument 'path'")
			}
			startLine := getIntArg(args, "start_line", 1)
			endLine := getIntArg(args, "end_line", startLine+50)

			window, err := refinery.ReadWindow(path, startLine, endLine)
			if err != nil {
				return "", true, err
			}
			return window, false, nil
		},
	})

	testVerifierHandler := func(ctx context.Context, args map[string]any) (string, bool, error) {
		cmdStr := getStringArg(args, "command")
		if cmdStr == "" {
			return "", true, fmt.Errorf("missing required argument 'command'")
		}

		res, err := refinery.RunTestVerifier(ctx, cmdStr)
		if err != nil {
			return "", true, err
		}

		if !res.Passed {
			var sb strings.Builder
			sb.WriteString(res.Summary)
			if res.ErrorOutput != "" {
				sb.WriteString("\n")
				sb.WriteString(res.ErrorOutput)
			}
			return sb.String(), false, nil
		}

		return res.Summary, false, nil
	}

	// Register test_verifier tool
	server.RegisterTool(mcp.Tool{
		Name:        "test_verifier",
		Description: "Executes a test command and strips verbose passing noise and stack traces down to clean failure assertions.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"command": {
					Type:        "string",
					Description: "Shell command to run the test suite (e.g. 'go test -v ./...')",
				},
			},
			Required: []string{"command"},
		},
		Handler: testVerifierHandler,
	})

	// Register run_test alias
	server.RegisterTool(mcp.Tool{
		Name:        "run_test",
		Description: "Executes a test command with noise filtering (alias for test_verifier).",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"command": {
					Type:        "string",
					Description: "Shell command to run the test suite (e.g. 'go test -v ./...')",
				},
			},
			Required: []string{"command"},
		},
		Handler: testVerifierHandler,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "lokol-mcp error: %v\n", err)
		os.Exit(1)
	}
}

func getStringArg(args map[string]any, key string) string {
	val, ok := args[key]
	if !ok || val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return strings.TrimSpace(s)
	}
	return fmt.Sprintf("%v", val)
}

func getIntArg(args map[string]any, key string, defaultVal int) int {
	val, ok := args[key]
	if !ok || val == nil {
		return defaultVal
	}
	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}
