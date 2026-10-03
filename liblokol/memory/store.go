// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/boggycreek/lokol/liblokol/config"
)

var (
	ErrNotFound          = errors.New("memory record not found")
	ErrBoundaryViolation = errors.New("memory path escapes authorized memory root")
)

// Store manages partitioned persistent memory records with strict boundary containment
// and atomic filesystem persistence.
type Store struct {
	RootDir string
	mu      sync.RWMutex
	cache   map[string]*MemoryRecord
}

// NewStore initializes a persistent memory store. If rootDir is empty,
// defaults to the XDG Data memory directory (~/.local/share/lokol/memory).
func NewStore(rootDir string) (*Store, error) {
	if rootDir == "" {
		rootDir = config.MemoryDir()
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute memory root: %w", err)
	}

	s := &Store{
		RootDir: absRoot,
		cache:   make(map[string]*MemoryRecord),
	}

	if err := s.Init(); err != nil {
		return nil, err
	}

	return s, nil
}

// Init creates the 5 cognitive pillar directories and loads existing records.
func (s *Store) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range ValidPillars {
		dir := filepath.Join(s.RootDir, string(p))
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create memory pillar directory %s: %w", dir, err)
		}
	}

	return s.reindexLocked()
}

// CheckMemoryPathWithinBounds enforces strict workspace containment to prevent
// unauthorized reading or writing outside the memory hierarchy (ADR 0019 memo).
func (s *Store) CheckMemoryPathWithinBounds(path string) error {
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Join(s.RootDir, cleanPath)
	}

	rel, err := filepath.Rel(s.RootDir, cleanPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("%w: %s (authorized root: %s)", ErrBoundaryViolation, path, s.RootDir)
	}

	return nil
}

// Save stores a memory record, writing the Markdown document and optional
// companion .details.json file atomically to disk.
func (s *Store) Save(ctx context.Context, rec *MemoryRecord) (*MemoryRecord, error) {
	if rec == nil {
		return nil, errors.New("cannot save nil memory record")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if rec.Category == "" {
		rec.Category = PillarSemantic
	}
	if !IsValidPillar(string(rec.Category)) {
		return nil, fmt.Errorf("invalid memory category: %s", rec.Category)
	}

	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now

	if rec.ID == "" {
		rec.ID = GenerateID(rec.Category, rec.Title)
	}

	// Compute partitioned destination path based on cognitive pillar
	relDir := string(rec.Category)
	switch rec.Category {
	case PillarEpisodic:
		relDir = filepath.Join(string(rec.Category), rec.CreatedAt.Format("2006/01/02"))
	case PillarSemantic:
		domain := rec.Domain
		if domain == "" {
			domain = "general"
		}
		relDir = filepath.Join(string(rec.Category), domain)
	}

	targetDir := filepath.Join(s.RootDir, relDir)
	if err := s.CheckMemoryPathWithinBounds(targetDir); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	mdPath := filepath.Join(targetDir, rec.ID+".md")
	if err := s.CheckMemoryPathWithinBounds(mdPath); err != nil {
		return nil, err
	}

	rec.Path = mdPath
	content := rec.ToMarkdown()

	// Atomic write for Markdown record
	tmpMd := filepath.Join(targetDir, fmt.Sprintf(".%s.tmp.%d", rec.ID, time.Now().UnixNano()))
	if err := os.WriteFile(tmpMd, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write temporary memory file: %w", err)
	}
	if err := os.Rename(tmpMd, mdPath); err != nil {
		_ = os.Remove(tmpMd)
		return nil, fmt.Errorf("failed to commit memory file: %w", err)
	}

	// Atomic write for companion .details.json if extra structured data exists
	if len(rec.ExtraData) > 0 {
		jsonPath := filepath.Join(targetDir, rec.ID+".details.json")
		jsonData, err := json.MarshalIndent(rec.ExtraData, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal companion details: %w", err)
		}

		tmpJson := filepath.Join(targetDir, fmt.Sprintf(".%s.details.tmp.%d", rec.ID, time.Now().UnixNano()))
		if err := os.WriteFile(tmpJson, jsonData, 0644); err != nil {
			return nil, fmt.Errorf("failed to write temporary companion details: %w", err)
		}
		if err := os.Rename(tmpJson, jsonPath); err != nil {
			_ = os.Remove(tmpJson)
			return nil, fmt.Errorf("failed to commit companion details: %w", err)
		}
	}

	// Update in-memory cache
	s.cache[rec.ID] = rec
	return rec, nil
}

// Get retrieves a memory by its ID, returning the formatted string according to
// the requested progressive disclosure depth ("summary" for Tier 2, "full" for Tier 3).
func (s *Store) Get(ctx context.Context, id string, depth string) (string, *MemoryRecord, error) {
	s.mu.RLock()
	rec, exists := s.cache[id]
	s.mu.RUnlock()

	if !exists {
		return "", nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}

	// If depth is full, ensure companion details.json is loaded
	if strings.ToLower(depth) == "full" && len(rec.ExtraData) == 0 && rec.Path != "" {
		jsonPath := strings.TrimSuffix(rec.Path, ".md") + ".details.json"
		if data, err := os.ReadFile(jsonPath); err == nil {
			var extra map[string]any
			if err := json.Unmarshal(data, &extra); err == nil {
				rec.ExtraData = extra
			}
		}
	}

	switch strings.ToLower(depth) {
	case "full", "tier3":
		return rec.FormatTier3(), rec, nil
	default:
		return rec.FormatTier2(), rec, nil
	}
}

// List returns memories optionally filtered by category in Tier 1 format (~20 tokens each).
func (s *Store) List(ctx context.Context, category Pillar, limit int) ([]string, []*MemoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	records := make([]*MemoryRecord, 0, len(s.cache))
	for _, rec := range s.cache {
		if category == "" || rec.Category == category {
			records = append(records, rec)
		}
	}

	// Sort chronologically descending
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})

	if len(records) > limit {
		records = records[:limit]
	}

	tier1Strings := make([]string, len(records))
	for i, r := range records {
		tier1Strings[i] = r.FormatTier1()
	}

	return tier1Strings, records, nil
}

// SummarizeEpisodic aggregates daily or weekly autobiographical progress notes
// into a structured temporal overview.
func (s *Store) SummarizeEpisodic(ctx context.Context, period string, date time.Time) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if date.IsZero() {
		date = time.Now().UTC()
	}

	var matched []*MemoryRecord
	for _, rec := range s.cache {
		if rec.Category != PillarEpisodic {
			continue
		}

		switch strings.ToLower(period) {
		case "day", "daily":
			if rec.CreatedAt.Format("2006-01-02") == date.Format("2006-01-02") {
				matched = append(matched, rec)
			}
		case "week", "weekly":
			yearA, weekA := rec.CreatedAt.ISOWeek()
			yearB, weekB := date.ISOWeek()
			if yearA == yearB && weekA == weekB {
				matched = append(matched, rec)
			}
		case "month", "monthly":
			if rec.CreatedAt.Format("2006-01") == date.Format("2006-01") {
				matched = append(matched, rec)
			}
		default:
			// Default to current date match
			if rec.CreatedAt.Format("2006-01-02") == date.Format("2006-01-02") {
				matched = append(matched, rec)
			}
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.Before(matched[j].CreatedAt)
	})

	var b strings.Builder
	b.WriteString(fmt.Sprintf("=== Episodic Summary (%s: %s) ===\n", strings.ToUpper(period), date.Format("2006-01-02")))
	b.WriteString(fmt.Sprintf("Total Events Recorded: %d\n\n", len(matched)))

	if len(matched) == 0 {
		b.WriteString("No episodic events recorded for this timeframe.")
		return b.String(), nil
	}

	for _, rec := range matched {
		b.WriteString(fmt.Sprintf("- [%s] %s\n  %s\n",
			rec.CreatedAt.Format("15:04"), rec.Title, rec.Abstract))
	}

	return strings.TrimSpace(b.String()), nil
}

// reindexLocked crawls RootDir and indexes all .md memory files into cache.
func (s *Store) reindexLocked() error {
	s.cache = make(map[string]*MemoryRecord)

	err := filepath.WalkDir(s.RootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") || strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		rec, err := ParseMarkdown(string(data))
		if err != nil || rec.ID == "" {
			return nil
		}

		rec.Path = path
		s.cache[rec.ID] = rec
		return nil
	})

	return err
}
