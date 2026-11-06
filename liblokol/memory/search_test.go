// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]int
	}{
		{
			name:     "empty string",
			input:    "",
			expected: map[string]int{},
		},
		{
			name:  "standard alphanumeric sentence",
			input: "The quick brown fox jumps over the lazy dog",
			expected: map[string]int{
				"the":   2,
				"quick": 1,
				"brown": 1,
				"fox":   1,
				"jumps": 1,
				"over":  1,
				"lazy":  1,
				"dog":   1,
			},
		},
		{
			name:  "punctuation, hyphens and underscores",
			input: "go-lang_pkg! hello-world... test_case? -isolated- _prefix_",
			expected: map[string]int{
				"go-lang_pkg": 1,
				"hello-world": 1,
				"test_case":   1,
				"isolated":    1,
				"prefix":      1,
			},
		},
		{
			name:     "single letter words filtered out",
			input:    "a b c I 1 2 d",
			expected: map[string]int{},
		},
		{
			name:     "only whitespace and symbols",
			input:    "   !@#$%^&*()_+=-~`{}[]:\";'<>?,./   ",
			expected: map[string]int{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenize(tc.input)
			if len(got) != len(tc.expected) {
				t.Fatalf("expected length %d, got %d (got: %+v)", len(tc.expected), len(got), got)
			}
			for k, expectedCount := range tc.expected {
				if got[k] != expectedCount {
					t.Errorf("token %q: expected count %d, got %d", k, expectedCount, got[k])
				}
			}
		})
	}
}

func TestScoreRecord(t *testing.T) {
	docRec := &MemoryRecord{
		Title:    "PostgreSQL Connection Pooling",
		Abstract: "Configures pgxpool with max connections.",
		Tags:     []string{"database", "sql"},
		Domain:   "storage",
	}
	docTerms := tokenize(docRec.Title + " " + docRec.Abstract + " database sql storage")
	df := map[string]float64{
		"postgresql": 1,
		"pooling":    1,
		"database":   2,
		"storage":    3,
	}
	totalDocs := 10.0

	t.Run("exact phrase boost in title", func(t *testing.T) {
		query := "PostgreSQL Connection"
		queryTerms := tokenize(query)
		score := scoreRecord(docRec, docTerms, query, queryTerms, df, totalDocs)

		// Base BM25 + 15.0 exact phrase in title
		if score < 15.0 {
			t.Errorf("expected score >= 15.0 for exact title match, got %f", score)
		}
	})

	t.Run("exact phrase boost in abstract", func(t *testing.T) {
		query := "max connections"
		queryTerms := tokenize(query)
		score := scoreRecord(docRec, docTerms, query, queryTerms, df, totalDocs)

		// Base BM25 + 8.0 exact phrase in abstract
		if score < 8.0 {
			t.Errorf("expected score >= 8.0 for exact abstract match, got %f", score)
		}
	})

	t.Run("tag boost", func(t *testing.T) {
		query := "database"
		queryTerms := tokenize(query)
		score := scoreRecord(docRec, docTerms, query, queryTerms, df, totalDocs)

		// Should receive tag boost (10.0) + BM25 score
		if score < 10.0 {
			t.Errorf("expected score >= 10.0 for tag match, got %f", score)
		}
	})

	t.Run("domain boost", func(t *testing.T) {
		query := "storage"
		queryTerms := tokenize(query)
		score := scoreRecord(docRec, docTerms, query, queryTerms, df, totalDocs)

		// Should receive domain boost (5.0) + BM25 score
		if score < 5.0 {
			t.Errorf("expected score >= 5.0 for domain match, got %f", score)
		}
	})

	t.Run("no matches returns zero", func(t *testing.T) {
		query := "kubernetes helm"
		queryTerms := tokenize(query)
		score := scoreRecord(docRec, docTerms, query, queryTerms, df, totalDocs)

		if score != 0.0 {
			t.Errorf("expected score 0.0 for completely unrelated query, got %f", score)
		}
	})
}

func TestStore_Search_EmptyStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	results, recs, err := store.Search(ctx, "test query", "", 5)
	if err != nil {
		t.Fatalf("unexpected error searching empty store: %v", err)
	}
	if len(results) != 0 || len(recs) != 0 {
		t.Errorf("expected empty results from empty store, got %d results, %d records", len(results), len(recs))
	}
}

func TestStore_Search_EmptyOrWhitespaceQuery(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	_, err = store.Save(ctx, &MemoryRecord{
		Category: PillarSemantic,
		Title:    "Architecture Doc",
		Abstract: "Core architecture details.",
		Summary:  "Summary of the architecture.",
	})
	if err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}

	// Empty query should fall back to List() which returns Tier 1 format
	results, recs, err := store.Search(ctx, "   ", "", 5)
	if err != nil {
		t.Fatalf("unexpected error searching whitespace query: %v", err)
	}
	if len(results) != 1 || len(recs) != 1 {
		t.Fatalf("expected 1 fallback record from List(), got %d", len(results))
	}
	if !strings.HasPrefix(results[0], "• [") {
		t.Errorf("expected Tier 1 format from List() fallback, got: %s", results[0])
	}
}

func TestStore_Search_DefaultLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 8; i++ {
		_, err := store.Save(ctx, &MemoryRecord{
			Category: PillarProcedural,
			Title:    fmt.Sprintf("Benchmark Step %d", i),
			Abstract: "Performance benchmark guide.",
			Summary:  "Steps to execute the benchmark.",
		})
		if err != nil {
			t.Fatalf("failed to save memory %d: %v", i, err)
		}
	}

	// Limit <= 0 should default to 5
	results, recs, err := store.Search(ctx, "benchmark", "", 0)
	if err != nil {
		t.Fatalf("unexpected search error: %v", err)
	}
	if len(results) != 5 || len(recs) != 5 {
		t.Errorf("expected default limit of 5, got %d results, %d records", len(results), len(recs))
	}

	// Negative limit should also default to 5
	resultsNeg, recsNeg, err := store.Search(ctx, "benchmark", "", -10)
	if err != nil {
		t.Fatalf("unexpected search error: %v", err)
	}
	if len(resultsNeg) != 5 || len(recsNeg) != 5 {
		t.Errorf("expected default limit of 5 for negative limit, got %d results, %d records", len(resultsNeg), len(recsNeg))
	}
}

func TestStore_Search_CategoryFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	_, _ = store.Save(ctx, &MemoryRecord{
		Category: PillarEpisodic,
		Title:    "Network Debugging Incident",
		Abstract: "Diagnosed packet loss on the edge router.",
		Summary:  "Resolution details for packet drop.",
	})
	_, _ = store.Save(ctx, &MemoryRecord{
		Category: PillarSemantic,
		Title:    "Network Topology Overview",
		Abstract: "Diagram of mesh topology and BGP routing.",
		Summary:  "Core topology specs.",
	})

	// Search matching both, but scoped to PillarSemantic
	results, recs, err := store.Search(ctx, "Network", PillarSemantic, 10)
	if err != nil {
		t.Fatalf("unexpected search error: %v", err)
	}
	if len(results) != 1 || len(recs) != 1 {
		t.Fatalf("expected 1 result filtered by category, got %d", len(results))
	}
	if recs[0].Category != PillarSemantic {
		t.Errorf("expected Category %s, got %s", PillarSemantic, recs[0].Category)
	}
	if recs[0].Title != "Network Topology Overview" {
		t.Errorf("expected 'Network Topology Overview', got %q", recs[0].Title)
	}
}

func TestStore_Search_RankingOrder(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	// Record A: Only abstract mention
	_, _ = store.Save(ctx, &MemoryRecord{
		Category: PillarProcedural,
		Title:    "Service Maintenance Procedures",
		Abstract: "Details about database failover and failback.",
		Summary:  "Procedure text.",
	})

	// Record B: Exact phrase match in Title and Tag match (higher relevance)
	_, _ = store.Save(ctx, &MemoryRecord{
		Category: PillarProcedural,
		Title:    "Database Failover Runbook",
		Tags:     []string{"database", "failover"},
		Abstract: "Complete runbook for switching replicas.",
		Summary:  "Execution playbook steps.",
	})

	results, recs, err := store.Search(ctx, "database failover", PillarProcedural, 5)
	if err != nil {
		t.Fatalf("unexpected search error: %v", err)
	}
	if len(recs) < 2 {
		t.Fatalf("expected at least 2 records, got %d", len(recs))
	}
	if recs[0].Title != "Database Failover Runbook" {
		t.Errorf("expected highest ranked record to be 'Database Failover Runbook', got %q", recs[0].Title)
	}
	// Verify Tier 2 format returned in results
	if !strings.HasPrefix(results[0], "=== Memory: Database Failover Runbook") {
		t.Errorf("expected Tier 2 format header, got: %s", results[0])
	}
}

func TestStore_SearchWithQMD_Mock(t *testing.T) {
	// Create a temporary directory containing a mock `qmd` script
	binDir := t.TempDir()
	mockQmd := filepath.Join(binDir, "qmd")

	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	rec1, err := store.Save(ctx, &MemoryRecord{
		Category: PillarSemantic,
		Title:    "Alpha System Architecture",
		Abstract: "Alpha specs.",
		Summary:  "Alpha summary.",
	})
	if err != nil {
		t.Fatalf("failed to save rec1: %v", err)
	}
	rec2, err := store.Save(ctx, &MemoryRecord{
		Category: PillarProcedural,
		Title:    "Beta Deployment Procedure",
		Abstract: "Beta steps.",
		Summary:  "Beta summary.",
	})
	if err != nil {
		t.Fatalf("failed to save rec2: %v", err)
	}

	t.Run("qmd CLI returns matched record IDs", func(t *testing.T) {
		// Mock script outputs the ID of rec1
		scriptContent := fmt.Sprintf("#!/bin/sh\necho 'found %s in files'\n", rec1.ID)
		if err := os.WriteFile(mockQmd, []byte(scriptContent), 0755); err != nil {
			t.Fatalf("failed to write mock qmd script: %v", err)
		}

		origPath := os.Getenv("PATH")
		defer os.Setenv("PATH", origPath)
		os.Setenv("PATH", binDir+":"+origPath)

		results, recs, err := store.Search(ctx, "alpha", "", 5)
		if err != nil {
			t.Fatalf("unexpected search error: %v", err)
		}
		if len(recs) != 1 || recs[0].ID != rec1.ID {
			t.Fatalf("expected rec1 ID %s, got %+v", rec1.ID, recs)
		}
		if !strings.Contains(results[0], "Alpha System Architecture") {
			t.Errorf("expected Tier 2 formatted output with title, got: %s", results[0])
		}
	})

	t.Run("qmd CLI fails and falls back to pure Go engine", func(t *testing.T) {
		// Script exits with non-zero code
		scriptContent := "#!/bin/sh\nexit 1\n"
		if err := os.WriteFile(mockQmd, []byte(scriptContent), 0755); err != nil {
			t.Fatalf("failed to write mock qmd script: %v", err)
		}

		origPath := os.Getenv("PATH")
		defer os.Setenv("PATH", origPath)
		os.Setenv("PATH", binDir+":"+origPath)

		// Pure Go engine should kick in seamlessly
		results, recs, err := store.Search(ctx, "Beta Deployment", "", 5)
		if err != nil {
			t.Fatalf("unexpected search error: %v", err)
		}
		if len(recs) == 0 || recs[0].ID != rec2.ID {
			t.Fatalf("expected fallback to return rec2 ID %s, got %+v", rec2.ID, recs)
		}
		if !strings.Contains(results[0], "Beta Deployment Procedure") {
			t.Errorf("expected result to format rec2, got: %s", results[0])
		}
	})

	t.Run("qmd CLI returns matches filtered by category and limit", func(t *testing.T) {
		// Mock script outputs both rec1 and rec2 IDs
		scriptContent := fmt.Sprintf("#!/bin/sh\necho '%s\n%s'\n", rec1.ID, rec2.ID)
		if err := os.WriteFile(mockQmd, []byte(scriptContent), 0755); err != nil {
			t.Fatalf("failed to write mock qmd script: %v", err)
		}

		origPath := os.Getenv("PATH")
		defer os.Setenv("PATH", origPath)
		os.Setenv("PATH", binDir+":"+origPath)

		// Filter by Procedural category -> should only return rec2
		results, recs, err := store.Search(ctx, "test", PillarProcedural, 5)
		if err != nil {
			t.Fatalf("unexpected search error: %v", err)
		}
		if len(recs) != 1 || recs[0].ID != rec2.ID {
			t.Fatalf("expected category-filtered match to return rec2, got %+v", recs)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result string, got %d", len(results))
		}

		// When limit is 1 with no category filter, only 1 result should be returned
		resultsLim, recsLim, err := store.Search(ctx, "test", "", 1)
		if err != nil {
			t.Fatalf("unexpected search error: %v", err)
		}
		if len(recsLim) != 1 || len(resultsLim) != 1 {
			t.Errorf("expected limit of 1 to be respected, got %d recs", len(recsLim))
		}
	})

	t.Run("qmd CLI returns no matching IDs falls back to pure Go engine", func(t *testing.T) {
		// Mock script outputs text that doesn't match any record ID
		scriptContent := "#!/bin/sh\necho 'nothing matched here'\n"
		if err := os.WriteFile(mockQmd, []byte(scriptContent), 0755); err != nil {
			t.Fatalf("failed to write mock qmd script: %v", err)
		}

		origPath := os.Getenv("PATH")
		defer os.Setenv("PATH", origPath)
		os.Setenv("PATH", binDir+":"+origPath)

		// Pure Go engine should be invoked and find rec1 for "Alpha"
		results, recs, err := store.Search(ctx, "Alpha", "", 5)
		if err != nil {
			t.Fatalf("unexpected search error: %v", err)
		}
		if len(recs) == 0 || recs[0].ID != rec1.ID {
			t.Fatalf("expected pure Go fallback to find rec1, got %+v", recs)
		}
		if len(results) == 0 {
			t.Errorf("expected non-empty results")
		}
	})
}
