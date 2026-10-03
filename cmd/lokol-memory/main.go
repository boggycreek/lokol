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
	"time"

	"github.com/boggycreek/lokol/liblokol/config"
	"github.com/boggycreek/lokol/liblokol/mcp"
	"github.com/boggycreek/lokol/liblokol/memory"
	"github.com/boggycreek/lokol/liblokol/version"
)

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lokol-memory", flag.ContinueOnError)
	flags.SetOutput(stderr)
	versionFlag := flags.Bool("version", false, "Print version and exit")
	memoryRoot := flags.String("memory-root", "", "Custom persistent memory root directory (defaults to XDG memory dir)")

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
		fmt.Fprintf(stdout, "lokol-memory %s (commit: %s, built: %s)\n", version.Version, version.GitCommit, version.BuildDate)
		return 0
	}

	root := *memoryRoot
	if root == "" {
		root = config.MemoryDir()
	}

	store, err := memory.NewStore(root)
	if err != nil {
		fmt.Fprintf(stderr, "lokol-memory store init error: %v\n", err)
		return 1
	}

	server := mcp.NewServer("lokol-memory", version.Version)
	registerMemoryTools(server, store)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := server.Serve(ctx, stdin, stdout); err != nil && err != context.Canceled {
		fmt.Fprintf(stderr, "lokol-memory error: %v\n", err)
		return 1
	}

	return 0
}

func registerMemoryTools(server *mcp.Server, store *memory.Store) {
	// 1. store_memory
	server.RegisterTool(mcp.Tool{
		Name:        "store_memory",
		Description: "Stores a persistent memory record into one of five cognitive pillars (episodic, semantic, procedural, prospective, evaluative) with 3-tier progressive disclosure.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"category": {
					Type:        "string",
					Description: "Cognitive pillar: episodic, semantic, procedural, prospective, or evaluative.",
				},
				"title": {
					Type:        "string",
					Description: "Short descriptive title of the memory.",
				},
				"abstract": {
					Type:        "string",
					Description: "Tier 1: 1-sentence signpost abstract (~20 tokens).",
				},
				"summary": {
					Type:        "string",
					Description: "Tier 2: Executive summary, key takeaways, and actionable rules (~100-200 tokens).",
				},
				"details": {
					Type:        "string",
					Description: "Tier 3: Detailed technical background, command outputs, or code samples (optional).",
				},
				"domain": {
					Type:        "string",
					Description: "Domain namespace for semantic memories (e.g. auth, db, ui) (optional).",
				},
				"tags": {
					Type:        "string",
					Description: "Comma-separated list or JSON array of search tags (optional).",
				},
				"extra_data": {
					Type:        "object",
					Description: "Structured companion JSON payload saved to .details.json (optional).",
				},
			},
			Required: []string{"category", "title", "abstract", "summary"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			categoryStr, _ := args["category"].(string)
			if categoryStr == "" {
				return "Error: category parameter is required", true, nil
			}
			if !memory.IsValidPillar(categoryStr) {
				return fmt.Sprintf("Error: invalid category '%s'. Must be one of: episodic, semantic, procedural, prospective, evaluative", categoryStr), true, nil
			}

			title, _ := args["title"].(string)
			if title == "" {
				return "Error: title parameter is required", true, nil
			}
			abstract, _ := args["abstract"].(string)
			if abstract == "" {
				return "Error: abstract parameter is required (1-sentence Tier 1 signpost)", true, nil
			}
			summary, _ := args["summary"].(string)
			if summary == "" {
				return "Error: summary parameter is required (Tier 2 concise digest)", true, nil
			}

			details, _ := args["details"].(string)
			domain, _ := args["domain"].(string)

			// Parse tags
			var tags []string
			if rawTags, ok := args["tags"]; ok {
				switch val := rawTags.(type) {
				case string:
					for _, t := range strings.Split(val, ",") {
						clean := strings.TrimSpace(t)
						if clean != "" {
							tags = append(tags, clean)
						}
					}
				case []any:
					for _, item := range val {
						if s, ok := item.(string); ok && s != "" {
							tags = append(tags, strings.TrimSpace(s))
						}
					}
				}
			}

			// Parse extra data
			var extra map[string]any
			if rawExtra, ok := args["extra_data"]; ok {
				if m, ok := rawExtra.(map[string]any); ok {
					extra = m
				}
			}

			rec := &memory.MemoryRecord{
				Category:  memory.Pillar(strings.ToLower(categoryStr)),
				Title:     title,
				Domain:    domain,
				Tags:      tags,
				Abstract:  abstract,
				Summary:   summary,
				Details:   details,
				ExtraData: extra,
			}

			saved, err := store.Save(ctx, rec)
			if err != nil {
				return fmt.Sprintf("Failed to store memory: %v", err), true, nil
			}

			return fmt.Sprintf("Memory stored successfully:\nID: %s\nPillar: %s\nPath: %s\nTier 1 Signpost: %s",
				saved.ID, saved.Category, saved.Path, saved.FormatTier1()), false, nil
		},
	})

	// 2. recall_memory (returns Tier 2 concise digests)
	server.RegisterTool(mcp.Tool{
		Name:        "recall_memory",
		Description: "Searches persistent memories via hybrid BM25 and vector semantic search, returning bounded Tier 2 concise digests (~100-200 tokens each).",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"query": {
					Type:        "string",
					Description: "Search query terms or concept keywords.",
				},
				"category": {
					Type:        "string",
					Description: "Optional filter: episodic, semantic, procedural, prospective, or evaluative.",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum results to return (default: 5).",
				},
			},
			Required: []string{"query"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			query, _ := args["query"].(string)
			if query == "" {
				return "Error: query parameter is required", true, nil
			}

			categoryStr, _ := args["category"].(string)
			limit := 5
			if l, ok := args["limit"].(float64); ok && l > 0 {
				limit = int(l)
			}

			results, _, err := store.Search(ctx, query, memory.Pillar(categoryStr), limit)
			if err != nil {
				return fmt.Sprintf("Search error: %v", err), true, nil
			}

			if len(results) == 0 {
				return fmt.Sprintf("No memories found matching query %q.", query), false, nil
			}

			var b strings.Builder
			b.WriteString(fmt.Sprintf("Recalled %d memories (Tier 2 Concise Digests):\n\n", len(results)))
			for _, r := range results {
				b.WriteString(r)
				b.WriteString("\n\n")
			}

			return strings.TrimSpace(b.String()), false, nil
		},
	})

	// 3. list_memories (returns Tier 1 signposts)
	server.RegisterTool(mcp.Tool{
		Name:        "list_memories",
		Description: "Lists persistent memories as lightweight Tier 1 signposts (~20 tokens per item) for index scanning.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"category": {
					Type:        "string",
					Description: "Optional filter by cognitive pillar (episodic, semantic, procedural, prospective, evaluative).",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum items to list (default: 20).",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			categoryStr, _ := args["category"].(string)
			limit := 20
			if l, ok := args["limit"].(float64); ok && l > 0 {
				limit = int(l)
			}

			signposts, _, err := store.List(ctx, memory.Pillar(categoryStr), limit)
			if err != nil {
				return fmt.Sprintf("List error: %v", err), true, nil
			}

			if len(signposts) == 0 {
				return "No memories found.", false, nil
			}

			var b strings.Builder
			b.WriteString(fmt.Sprintf("Persistent Memories (Tier 1 Index - %d items):\n", len(signposts)))
			for _, sp := range signposts {
				b.WriteString(sp)
				b.WriteString("\n")
			}

			return strings.TrimSpace(b.String()), false, nil
		},
	})

	// 4. get_memory
	server.RegisterTool(mcp.Tool{
		Name:        "get_memory",
		Description: "Retrieves a specific memory by ID with controlled progressive disclosure depth ('summary' for Tier 2, 'full' for Tier 3).",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"id": {
					Type:        "string",
					Description: "Unique memory ID.",
				},
				"depth": {
					Type:        "string",
					Description: "Disclosure depth: 'summary' (default, Tier 2 digest) or 'full' (Tier 3 full technical background and companion data).",
				},
			},
			Required: []string{"id"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return "Error: id parameter is required", true, nil
			}

			depth, _ := args["depth"].(string)
			if depth == "" {
				depth = "summary"
			}

			content, _, err := store.Get(ctx, id, depth)
			if err != nil {
				if errors.Is(err, memory.ErrNotFound) {
					return fmt.Sprintf("Memory ID %q not found.", id), true, nil
				}
				return fmt.Sprintf("Failed to get memory: %v", err), true, nil
			}

			return content, false, nil
		},
	})

	// 5. summarize_episodic
	server.RegisterTool(mcp.Tool{
		Name:        "summarize_episodic",
		Description: "Rolls up episodic progression notes into a daily, weekly, or monthly structured progress summary.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDoc{
				"period": {
					Type:        "string",
					Description: "Aggregation timeframe: 'day' (default), 'week', or 'month'.",
				},
				"date": {
					Type:        "string",
					Description: "Target date in YYYY-MM-DD format (defaults to today).",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, bool, error) {
			period, _ := args["period"].(string)
			if period == "" {
				period = "day"
			}

			targetDate := time.Now().UTC()
			if dateStr, ok := args["date"].(string); ok && dateStr != "" {
				if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
					targetDate = parsed
				}
			}

			summary, err := store.SummarizeEpisodic(ctx, period, targetDate)
			if err != nil {
				return fmt.Sprintf("Summarize episodic error: %v", err), true, nil
			}

			return summary, false, nil
		},
	})
}
