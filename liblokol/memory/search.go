// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package memory

import (
	"context"
	"math"
	"os/exec"
	"sort"
	"strings"
)

// SearchResult pairs a scored memory record with its relevance score.
type SearchResult struct {
	Record *MemoryRecord
	Score  float64
}

// Search performs BM25 keyword search with exact phrase and tag boosting across memories,
// using the local QMD CLI if installed, or falling back seamlessly to an
// embedded pure-Go BM25 ranking engine.
// Results are returned in Tier 2 format (~100-200 tokens each).
func (s *Store) Search(ctx context.Context, query string, category Pillar, limit int) ([]string, []*MemoryRecord, error) {
	if limit <= 0 {
		limit = 5
	}

	// 1. Check if QMD CLI tool is available on PATH
	if qmdPath, err := exec.LookPath("qmd"); err == nil && qmdPath != "" {
		if results, recs, err := s.searchWithQMD(ctx, qmdPath, query, category, limit); err == nil && len(results) > 0 {
			return results, recs, nil
		}
	}

	// 2. Pure-Go Embedded BM25 Ranking Engine
	return s.searchPureGo(ctx, query, category, limit)
}

func (s *Store) searchWithQMD(ctx context.Context, qmdPath string, query string, category Pillar, limit int) ([]string, []*MemoryRecord, error) {
	// If QMD is present, run search scoped to root
	cmd := exec.CommandContext(ctx, qmdPath, "search", query, "--json", "--limit", "10")
	cmd.Dir = s.RootDir
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, err
	}

	// QMD outputs file paths or matches; correlate with in-memory store
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matched []*MemoryRecord
	outStr := string(out)
	for id, rec := range s.cache {
		if category != "" && rec.Category != category {
			continue
		}
		if strings.Contains(outStr, id) {
			matched = append(matched, rec)
		}
	}

	if len(matched) == 0 {
		return nil, nil, nil
	}

	if len(matched) > limit {
		matched = matched[:limit]
	}

	results := make([]string, len(matched))
	for i, r := range matched {
		results[i] = r.FormatTier2()
	}

	return results, matched, nil
}

// searchPureGo computes pure-Go BM25 term weighting with exact phrase, tag, and domain boosting.
// To prevent lock starvation and high CPU under mutex contention, records are snapshotted
// under RLock and released immediately before executing BM25 calculations.
func (s *Store) searchPureGo(ctx context.Context, query string, category Pillar, limit int) ([]string, []*MemoryRecord, error) {
	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return s.List(ctx, category, limit)
	}

	// 1. Snapshot records and term index under RLock and release lock immediately
	s.mu.RLock()
	docCount := float64(len(s.cache))
	if docCount == 0 {
		s.mu.RUnlock()
		return nil, nil, nil
	}

	type recordSnapshot struct {
		rec   *MemoryRecord
		terms map[string]int
	}

	snapshots := make([]recordSnapshot, 0, len(s.cache))
	for id, rec := range s.cache {
		if category != "" && rec.Category != category {
			continue
		}
		snapshots = append(snapshots, recordSnapshot{
			rec:   rec,
			terms: s.docTerms[id],
		})
	}
	s.mu.RUnlock()

	// 2. Compute document frequencies across the snapshot (no locks held)
	df := make(map[string]float64)
	for _, snap := range snapshots {
		if snap.terms == nil {
			continue
		}
		for qt := range queryTerms {
			if snap.terms[qt] > 0 {
				df[qt]++
			}
		}
	}

	// 3. Score candidates using pre-tokenized term maps
	var candidates []SearchResult
	for _, snap := range snapshots {
		terms := snap.terms
		if terms == nil {
			terms = tokenize(snap.rec.Title + " " + snap.rec.Title + " " + snap.rec.Abstract + " " + snap.rec.Summary + " " + strings.Join(snap.rec.Tags, " "))
		}

		score := scoreRecord(snap.rec, terms, query, queryTerms, df, docCount)
		if score > 0 {
			candidates = append(candidates, SearchResult{
				Record: snap.rec,
				Score:  score,
			})
		}
	}

	// 4. Rank descending by score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	tier2Outputs := make([]string, len(candidates))
	records := make([]*MemoryRecord, len(candidates))
	for i, c := range candidates {
		tier2Outputs[i] = c.Record.FormatTier2()
		records[i] = c.Record
	}

	return tier2Outputs, records, nil
}

func scoreRecord(rec *MemoryRecord, docTerms map[string]int, rawQuery string, queryTerms map[string]int, df map[string]float64, totalDocs float64) float64 {
	score := 0.0

	// 1. Exact phrase boost in Title / Abstract
	queryLower := strings.ToLower(rawQuery)
	if strings.Contains(strings.ToLower(rec.Title), queryLower) {
		score += 15.0
	}
	if strings.Contains(strings.ToLower(rec.Abstract), queryLower) {
		score += 8.0
	}

	// 2. Tag boosts
	for _, tag := range rec.Tags {
		tagLower := strings.ToLower(tag)
		if queryTerms[tagLower] > 0 || tagLower == queryLower {
			score += 10.0
		}
	}

	// 3. Domain boost
	if rec.Domain != "" && queryTerms[strings.ToLower(rec.Domain)] > 0 {
		score += 5.0
	}

	// 4. BM25 / TF-IDF scoring using pre-indexed docTerms
	docLen := float64(len(docTerms))
	avgDocLen := 50.0 // heuristic baseline

	k1 := 1.5
	b := 0.75

	for qTerm, qf := range queryTerms {
		tf := float64(docTerms[qTerm])
		if tf == 0 {
			continue
		}

		docFreq := df[qTerm]
		if docFreq == 0 {
			docFreq = 1
		}

		// Standard BM25 IDF formulation
		idf := math.Log(1.0 + (totalDocs-docFreq+0.5)/(docFreq+0.5))
		if idf < 0 {
			idf = 0.1
		}

		// BM25 TF component
		tfWeight := (tf * (k1 + 1.0)) / (tf + k1*(1.0-b+b*(docLen/avgDocLen)))
		score += float64(qf) * idf * tfWeight
	}

	return score
}

// tokenize splits a string into clean lowercase alphanumeric terms.
func tokenize(text string) map[string]int {
	tokens := make(map[string]int)
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-'
	})

	for _, w := range words {
		w = strings.Trim(w, "_-")
		if len(w) >= 2 {
			tokens[w]++
		}
	}

	return tokens
}
