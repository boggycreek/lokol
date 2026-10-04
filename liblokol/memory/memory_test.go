// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package memory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boggycreek/lokol/liblokol/memory"
)

func TestMemory_FivePillarsPartitioning(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := memory.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// 1. Episodic
	episodic, err := store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarEpisodic,
		Title:    "Fixed Regex Action Parser Edge Cases",
		Abstract: "Resolved unclosed tags and nested code fences in action parser.",
		Summary:  "Added 20+ regression test cases and verified all edge cases pass.",
	})
	if err != nil {
		t.Fatalf("failed to save episodic memory: %v", err)
	}
	if !strings.Contains(episodic.Path, filepath.Join(tmpDir, "episodic")) {
		t.Errorf("episodic memory not partitioned into episodic dir: %s", episodic.Path)
	}

	// 2. Semantic
	semantic, err := store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarSemantic,
		Domain:   "architecture",
		Title:    "Monorepo Decoupling Pattern",
		Abstract: "Presentation layers decoupled from liblokol engine.",
		Summary:  "SessionCore is the single source of truth across TUI and headless CLI.",
	})
	if err != nil {
		t.Fatalf("failed to save semantic memory: %v", err)
	}
	if !strings.Contains(semantic.Path, filepath.Join(tmpDir, "semantic", "architecture")) {
		t.Errorf("semantic memory not partitioned into semantic/architecture dir: %s", semantic.Path)
	}

	// 3. Procedural
	procedural, err := store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarProcedural,
		Title:    "Release Tagging Playbook",
		Abstract: "Steps to tag and cut a release.",
		Summary:  "Run make test, push dolt data, create git tag with semver.",
	})
	if err != nil {
		t.Fatalf("failed to save procedural memory: %v", err)
	}
	if !strings.Contains(procedural.Path, filepath.Join(tmpDir, "procedural")) {
		t.Errorf("procedural memory not in procedural dir: %s", procedural.Path)
	}

	// 4. Prospective
	prospective, err := store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarProspective,
		Title:    "Trigger Slot Compaction",
		Abstract: "When slot occupancy exceeds 85%, invoke compaction.",
		Summary:  "Trigger CompactorFunc on SlotGovernor warning.",
	})
	if err != nil {
		t.Fatalf("failed to save prospective memory: %v", err)
	}
	if !strings.Contains(prospective.Path, filepath.Join(tmpDir, "prospective")) {
		t.Errorf("prospective memory not in prospective dir: %s", prospective.Path)
	}

	// 5. Evaluative
	evaluative, err := store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarEvaluative,
		Title:    "Zero Python Production Dependency Rule",
		Abstract: "No runtime Python dependencies in liblokol or binaries.",
		Summary:  "Production code must compile to 100% pure Go static binaries.",
	})
	if err != nil {
		t.Fatalf("failed to save evaluative memory: %v", err)
	}
	if !strings.Contains(evaluative.Path, filepath.Join(tmpDir, "evaluative")) {
		t.Errorf("evaluative memory not in evaluative dir: %s", evaluative.Path)
	}
}

func TestMemory_ThreeTierProgressiveDisclosure(t *testing.T) {
	rec := &memory.MemoryRecord{
		ID:       "mem-sema-20261003-jwt-flow",
		Title:    "OAuth2 JWT Validation",
		Category: memory.PillarSemantic,
		Domain:   "auth",
		Tags:     []string{"security", "jwt", "tokens"},
		Abstract: "Validates bearer tokens against cached public keys.",
		Summary:  "Executive Summary:\nRequests must pass BearerAuth middleware.\n\nActionable Rules:\n1. Check header.\n2. Verify signature.",
		Details:  "Technical Background:\nJWKS keys are refreshed every 300 seconds from https://auth.internal/keys.\n\nCode Sample:\n```go\nfunc verify(t string) error\n```",
		ExtraData: map[string]any{
			"jwks_url": "https://auth.internal/keys",
			"ttl_sec":  300,
		},
		CreatedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC),
	}

	// Tier 1: Signpost (~20 tokens)
	t1 := rec.FormatTier1()
	tokens1 := memory.EstimateTokens(t1)
	if !strings.Contains(t1, "[mem-sema-20261003-jwt-flow]") || !strings.Contains(t1, "Validates bearer tokens") {
		t.Errorf("unexpected Tier 1 formatting: %s", t1)
	}
	if tokens1 > 40 {
		t.Errorf("Tier 1 exceeded token budget (~20 tokens expected, got %d)", tokens1)
	}

	// Tier 2: Concise Digest (~100-200 tokens)
	t2 := rec.FormatTier2()
	tokens2 := memory.EstimateTokens(t2)
	if !strings.Contains(t2, "=== Memory: OAuth2 JWT Validation") || !strings.Contains(t2, "Actionable Rules") {
		t.Errorf("unexpected Tier 2 formatting: %s", t2)
	}
	if strings.Contains(t2, "Technical Background") || strings.Contains(t2, "jwks_url") {
		t.Errorf("Tier 2 should NOT disclose Tier 3 details or companion data")
	}
	if tokens2 > 250 {
		t.Errorf("Tier 2 exceeded token budget (~100-200 tokens expected, got %d)", tokens2)
	}

	// Tier 3: Deep Disclosure & Appendix
	t3 := rec.FormatTier3()
	if !strings.Contains(t3, "Technical Background") || !strings.Contains(t3, "jwks_url") {
		t.Errorf("Tier 3 should include technical details and companion data, got: %s", t3)
	}
}

func TestMemory_BoundaryContainment(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := memory.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// Allowed path inside root
	if err := store.CheckMemoryPathWithinBounds(filepath.Join(tmpDir, "semantic", "test.md")); err != nil {
		t.Errorf("expected inside path to pass, got: %v", err)
	}

	// Escaping paths
	escapes := []string{
		filepath.Join(tmpDir, "..", "secret.txt"),
		"/etc/passwd",
		"../../outside.md",
	}

	for _, p := range escapes {
		if err := store.CheckMemoryPathWithinBounds(p); err == nil {
			t.Errorf("expected path %q to be blocked by boundary containment", p)
		}
	}
}

func TestMemory_AtomicSaveWithCompanionDetails(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := memory.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	saved, err := store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarSemantic,
		Domain:   "networking",
		Title:    "Tailscale Subnet Router",
		Abstract: "Routes 10.0.0.0/24 subnet over Tailscale mesh.",
		Summary:  "Core Takeaway: advertise-routes=10.0.0.0/24 enabled.",
		Details:  "Configuration file located at /etc/tailscale/routes.conf",
		ExtraData: map[string]any{
			"route": "10.0.0.0/24",
			"mtu":   1280,
		},
	})
	if err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}

	// Verify both .md and .details.json exist on disk
	mdFile := saved.Path
	jsonFile := strings.TrimSuffix(mdFile, ".md") + ".details.json"

	if _, err := os.Stat(mdFile); err != nil {
		t.Errorf("expected markdown file %s to exist: %v", mdFile, err)
	}
	if _, err := os.Stat(jsonFile); err != nil {
		t.Errorf("expected companion details file %s to exist: %v", jsonFile, err)
	}

	// Retrieve with depth="summary" (Tier 2)
	summaryOut, _, err := store.Get(ctx, saved.ID, "summary")
	if err != nil {
		t.Fatalf("Get summary failed: %v", err)
	}
	if strings.Contains(summaryOut, "1280") {
		t.Errorf("depth='summary' should not disclose companion JSON details")
	}

	// Retrieve with depth="full" (Tier 3)
	fullOut, _, err := store.Get(ctx, saved.ID, "full")
	if err != nil {
		t.Fatalf("Get full failed: %v", err)
	}
	if !strings.Contains(fullOut, "1280") || !strings.Contains(fullOut, "routes.conf") {
		t.Errorf("depth='full' missing companion details: %s", fullOut)
	}
}

func TestMemory_HybridSearchPureGo(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := memory.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	_, _ = store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarProcedural,
		Title:    "Docker Build Playbook",
		Tags:     []string{"container", "docker", "build"},
		Abstract: "Build rootless containers with podman.",
		Summary:  "Use podman build -t myapp . without root privileges.",
	})

	_, _ = store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarSemantic,
		Domain:   "database",
		Title:    "PostgreSQL Connection Pooling",
		Tags:     []string{"postgres", "sql", "pool"},
		Abstract: "Configures pgxpool with max 25 connections.",
		Summary:  "Set max_conns to 25 and idle_timeout to 5 minutes.",
	})

	_, _ = store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarEvaluative,
		Title:    "Avoid System Pip Installs",
		Tags:     []string{"python", "uv", "tooling"},
		Abstract: "Never run pip install system-wide.",
		Summary:  "All Python tools must run in virtual environments managed by uv.",
	})

	// Search 1: by keyword "postgres"
	results1, recs1, err := store.Search(ctx, "postgres pooling", "", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results1) == 0 || recs1[0].Title != "PostgreSQL Connection Pooling" {
		t.Errorf("expected PostgreSQL result first, got: %v", recs1)
	}

	// Search 2: by tag "uv"
	results2, recs2, err := store.Search(ctx, "uv virtualenv", "", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results2) == 0 || recs2[0].Title != "Avoid System Pip Installs" {
		t.Errorf("expected Python uv result first, got: %v", recs2)
	}

	// Search 3: category filtered
	results3, recs3, err := store.Search(ctx, "build", memory.PillarProcedural, 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results3) != 1 || recs3[0].Category != memory.PillarProcedural {
		t.Errorf("expected 1 procedural result, got %d", len(results3))
	}
}

func TestMemory_SummarizeEpisodic(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := memory.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	today := time.Now().UTC()

	_, _ = store.Save(ctx, &memory.MemoryRecord{
		Category:  memory.PillarEpisodic,
		Title:     "Refactored Action Parser",
		Abstract:  "Streamlined regex matches for JSON fallbacks.",
		CreatedAt: today,
	})

	_, _ = store.Save(ctx, &memory.MemoryRecord{
		Category:  memory.PillarEpisodic,
		Title:     "Connected Compactor to SlotGovernor",
		Abstract:  "Context compaction triggers at 85% utilization.",
		CreatedAt: today.Add(1 * time.Hour),
	})

	summary, err := store.SummarizeEpisodic(ctx, "day", today)
	if err != nil {
		t.Fatalf("SummarizeEpisodic failed: %v", err)
	}

	if !strings.Contains(summary, "Total Events Recorded: 2") {
		t.Errorf("expected 2 events in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "Refactored Action Parser") || !strings.Contains(summary, "Connected Compactor") {
		t.Errorf("summary missing event titles: %s", summary)
	}
}

func TestMemory_PayloadLimits(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := memory.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	ctx := context.Background()

	// 1. Oversized title
	_, err = store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarSemantic,
		Title:    strings.Repeat("T", memory.MaxTitleLength+1),
		Abstract: "Valid abstract",
		Summary:  "Valid summary",
	})
	if err == nil || !strings.Contains(err.Error(), "title exceeds maximum limit") {
		t.Errorf("expected title limit error, got: %v", err)
	}

	// 2. Oversized abstract
	_, err = store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarSemantic,
		Title:    "Valid Title",
		Abstract: strings.Repeat("A", memory.MaxAbstractBytes+1),
		Summary:  "Valid summary",
	})
	if err == nil || !strings.Contains(err.Error(), "abstract exceeds maximum limit") {
		t.Errorf("expected abstract limit error, got: %v", err)
	}

	// 3. Oversized summary
	_, err = store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarSemantic,
		Title:    "Valid Title",
		Abstract: "Valid abstract",
		Summary:  strings.Repeat("S", memory.MaxSummaryBytes+1),
	})
	if err == nil || !strings.Contains(err.Error(), "summary exceeds maximum limit") {
		t.Errorf("expected summary limit error, got: %v", err)
	}

	// 4. Oversized details
	_, err = store.Save(ctx, &memory.MemoryRecord{
		Category: memory.PillarSemantic,
		Title:    "Valid Title",
		Abstract: "Valid abstract",
		Summary:  "Valid summary",
		Details:  strings.Repeat("D", memory.MaxDetailsBytes+1),
	})
	if err == nil || !strings.Contains(err.Error(), "details exceeds maximum limit") {
		t.Errorf("expected details limit error, got: %v", err)
	}

	// 5. Oversized companion extra_data
	hugeMap := map[string]any{
		"blob": strings.Repeat("X", memory.MaxExtraDataBytes+10),
	}
	_, err = store.Save(ctx, &memory.MemoryRecord{
		Category:  memory.PillarSemantic,
		Title:     "Valid Title",
		Abstract:  "Valid abstract",
		Summary:   "Valid summary",
		ExtraData: hugeMap,
	})
	if err == nil || !strings.Contains(err.Error(), "companion details exceed maximum limit") {
		t.Errorf("expected extra_data limit error, got: %v", err)
	}
}

func TestMemory_TemporaryFileBoundsValidation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := memory.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	ctx := context.Background()

	// Inject path traversal into ID
	_, err = store.Save(ctx, &memory.MemoryRecord{
		ID:       "../../../../outside_jail",
		Category: memory.PillarSemantic,
		Title:    "Traversal Attack",
		Abstract: "Should be rejected",
		Summary:  "Should be rejected",
	})
	if err == nil {
		t.Fatalf("expected boundary escape error for traversal ID, got nil")
	}
	if !strings.Contains(err.Error(), "escapes authorized memory root") {
		t.Errorf("expected boundary violation error, got: %v", err)
	}
}

func TestMemory_ConcurrentSearchAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := memory.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	ctx := context.Background()

	// Seed some initial records
	for i := 0; i < 20; i++ {
		_, err := store.Save(ctx, &memory.MemoryRecord{
			Category: memory.PillarSemantic,
			Title:    strings.Repeat("TermA ", 3) + " Record",
			Abstract: "Initial abstract with keyword TermB",
			Summary:  "Initial summary with keyword TermC",
		})
		if err != nil {
			t.Fatalf("failed to seed: %v", err)
		}
	}

	done := make(chan bool)
	errChan := make(chan error, 10)

	// Spawn concurrent searches
	for i := 0; i < 5; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					_, _, err := store.Search(ctx, "TermA TermB", "", 5)
					if err != nil {
						errChan <- err
						return
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}

	// Concurrent writes
	for i := 0; i < 10; i++ {
		_, err := store.Save(ctx, &memory.MemoryRecord{
			Category: memory.PillarProcedural,
			Title:    "Concurrent Record",
			Abstract: "Concurrent abstract",
			Summary:  "Concurrent summary",
		})
		if err != nil {
			t.Fatalf("concurrent save failed: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	close(done)

	select {
	case err := <-errChan:
		t.Fatalf("concurrent search failed: %v", err)
	default:
	}
}

