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

	"github.com/boggycreek/lokol/liblokol/mcp"
	"github.com/boggycreek/lokol/liblokol/refinery"
	"github.com/boggycreek/lokol/liblokol/version"
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

	// Register get_environment tool
	server.RegisterTool(mcp.Tool{
		Name:        "get_environment",
		Description: "Inspects the host execution environment, returning working directory, operating system, git status, and available development toolchains.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"path": {
					Type:        "string",
					Description: "Optional target directory to inspect (defaults to current working directory)",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			path := getStringArg(args, "path")
			envInfo, err := refinery.GetEnvironment(path)
			if err != nil {
				return "", true, err
			}
			jsonStr, err := envInfo.FormatJSON()
			if err != nil {
				return "", true, err
			}
			return jsonStr, false, nil
		},
	})

	// Register find_files tool
	server.RegisterTool(mcp.Tool{
		Name:        "find_files",
		Description: "Discovers files matching a glob or substring pattern, respecting .gitignore and enforcing a 50-file bounding limit to protect context.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"pattern": {
					Type:        "string",
					Description: "Glob or substring pattern to match (e.g. '*.go', 'test', '*.md'). Empty or '*' matches all files.",
				},
				"path": {
					Type:        "string",
					Description: "Optional base directory to search (defaults to current working directory)",
				},
				"max_results": {
					Type:        "integer",
					Description: "Maximum number of files to return (default: 50, maximum: 100)",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			pattern := getStringArg(args, "pattern")
			path := getStringArg(args, "path")
			maxResults := getIntArg(args, "max_results", 50)

			out, err := refinery.FindFiles(pattern, path, maxResults)
			if err != nil {
				return "", true, err
			}
			return out, false, nil
		},
	})

	// Register search_code tool
	server.RegisterTool(mcp.Tool{
		Name:        "search_code",
		Description: "Performs a fast, bounded regex or literal search across workspace source files respecting .gitignore, returning matching lines with path and line numbers capped at 30 results.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"pattern": {
					Type:        "string",
					Description: "Literal text or regular expression to search for across files",
				},
				"path": {
					Type:        "string",
					Description: "Optional subdirectory or file to search (defaults to current working directory)",
				},
				"is_regex": {
					Type:        "boolean",
					Description: "Whether to treat pattern as a regular expression (default: false for case-insensitive literal search)",
				},
				"max_results": {
					Type:        "integer",
					Description: "Maximum number of matching lines to return (default: 30, maximum: 50)",
				},
			},
			Required: []string{"pattern"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			pattern := getStringArg(args, "pattern")
			if pattern == "" {
				return "", true, fmt.Errorf("missing required argument 'pattern'")
			}
			path := getStringArg(args, "path")
			isRegex := getBoolArg(args, "is_regex", false)
			maxResults := getIntArg(args, "max_results", 30)

			cwd, err := os.Getwd()
			if err != nil {
				return "", true, err
			}

			out, err := refinery.SearchCode(cwd, refinery.SearchCodeInput{
				Pattern:    pattern,
				Path:       path,
				IsRegex:    isRegex,
				MaxResults: maxResults,
			})
			if err != nil {
				return "", true, err
			}
			return out, false, nil
		},
	})

	// Register git_diff_summary tool
	server.RegisterTool(mcp.Tool{
		Name:        "git_diff_summary",
		Description: "Inspects working repository state: staged changes, unstaged changes, untracked files, compact diffstat, and bounded unified diff snippets.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"path": {
					Type:        "string",
					Description: "Optional repository or subfolder path (defaults to current working directory)",
				},
				"max_lines": {
					Type:        "integer",
					Description: "Maximum lines of unified diff output (default: 100, maximum: 200)",
				},
				"staged": {
					Type:        "boolean",
					Description: "Whether to limit diff/diffstat to staged changes only (default: false)",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			path := getStringArg(args, "path")
			maxLines := getIntArg(args, "max_lines", 100)
			staged := getBoolArg(args, "staged", false)

			cwd, err := os.Getwd()
			if err != nil {
				return "", true, err
			}
			repoDir := cwd
			if path != "" {
				repoDir = path
			}

			out, err := refinery.GitDiffSummary(ctx, repoDir, refinery.GitDiffSummaryInput{
				Path:     path,
				MaxLines: maxLines,
				Staged:   staged,
			})
			if err != nil {
				return "", true, err
			}
			return out, false, nil
		},
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

func getBoolArg(args map[string]any, key string, defaultVal bool) bool {
	val, ok := args[key]
	if !ok || val == nil {
		return defaultVal
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	}
	return defaultVal
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
