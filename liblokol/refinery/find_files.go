// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package refinery

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// FindFilesInput specifies parameters for bounded file discovery.
type FindFilesInput struct {
	Pattern    string
	Path       string
	MaxResults int
}

var defaultIgnoredDirs = map[string]bool{
	".git":         true,
	".beads":       true,
	"node_modules": true,
	".venv":        true,
	"vendor":       true,
	"__pycache__":  true,
	".idea":        true,
	".vscode":      true,
	".antigravity": true,
	"dist":         true,
	"build":        true,
	".cache":       true,
}

type gitIgnoreMatcher struct {
	patterns []string
}

func loadGitIgnore(dir string) *gitIgnoreMatcher {
	m := &gitIgnoreMatcher{}
	path := filepath.Join(dir, ".gitignore")
	f, err := os.Open(path)
	if err != nil {
		return m
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSuffix(line, "/")
		m.patterns = append(m.patterns, line)
	}
	return m
}

func (m *gitIgnoreMatcher) matches(relPath string) bool {
	if len(m.patterns) == 0 {
		return false
	}
	base := filepath.Base(relPath)
	slashPath := filepath.ToSlash(relPath)

	for _, pat := range m.patterns {
		pat = filepath.ToSlash(pat)
		if strings.HasPrefix(pat, "/") {
			patTrim := strings.TrimPrefix(pat, "/")
			if matched, _ := filepath.Match(patTrim, slashPath); matched {
				return true
			}
			if matched, _ := filepath.Match(patTrim+"/*", slashPath); matched {
				return true
			}
		} else {
			if matched, _ := filepath.Match(pat, base); matched {
				return true
			}
			if matched, _ := filepath.Match(pat, slashPath); matched {
				return true
			}
			if matched, _ := filepath.Match("*/"+pat, slashPath); matched {
				return true
			}
			if matched, _ := filepath.Match(pat+"/*", slashPath); matched {
				return true
			}
		}
	}
	return false
}

// FindFiles discovers files matching a pattern starting from dir.
// It respects .gitignore, excludes noisy metadata directories, sorts results,
// and enforces a hard cap (default: 50, max: 100) to protect the context window.
func FindFiles(pattern string, dir string, maxResultsOpt ...int) (string, error) {
	if dir == "" {
		dir = "."
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	maxResults := 50
	if len(maxResultsOpt) > 0 && maxResultsOpt[0] > 0 {
		maxResults = maxResultsOpt[0]
	}
	if maxResults > 100 {
		maxResults = 100
	}

	gi := loadGitIgnore(absDir)
	cleanPattern := strings.TrimSpace(pattern)
	patternLower := strings.ToLower(cleanPattern)

	var matches []string

	err = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip unreadable paths without terminating walk
		}

		rel, relErr := filepath.Rel(absDir, path)
		if relErr != nil || rel == "." {
			return nil
		}

		name := d.Name()

		if d.IsDir() {
			if defaultIgnoredDirs[name] {
				return filepath.SkipDir
			}
			if gi.matches(rel) {
				return filepath.SkipDir
			}
			return nil
		}

		// File filtering
		if gi.matches(rel) {
			return nil
		}

		slashRel := filepath.ToSlash(rel)
		if fileMatchesPattern(cleanPattern, patternLower, name, slashRel) {
			matches = append(matches, slashRel)
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to search directory %s: %w", dir, err)
	}

	sort.Strings(matches)
	totalMatches := len(matches)

	if totalMatches == 0 {
		if cleanPattern == "" || cleanPattern == "*" {
			return fmt.Sprintf("No files found in %s", dir), nil
		}
		return fmt.Sprintf("No files matching %q found in %s", cleanPattern, dir), nil
	}

	truncated := false
	if totalMatches > maxResults {
		matches = matches[:maxResults]
		truncated = true
	}

	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m)
		sb.WriteString("\n")
	}

	if truncated {
		sb.WriteString(fmt.Sprintf("\n[Showing %d of %d matches. Refine pattern to narrow search.]", maxResults, totalMatches))
	} else {
		plural := ""
		if totalMatches > 1 {
			plural = "s"
		}
		sb.WriteString(fmt.Sprintf("\n(%d file%s)", totalMatches, plural))
	}

	return sb.String(), nil
}

func fileMatchesPattern(pattern, patternLower, name, slashRel string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}

	baseLower := strings.ToLower(name)
	relLower := strings.ToLower(slashRel)

	// 1. Exact or glob match on filename
	if matched, _ := filepath.Match(patternLower, baseLower); matched {
		return true
	}

	// 2. Glob match on full relative path (e.g. "pkg/*/*.go")
	if matched, _ := filepath.Match(patternLower, relLower); matched {
		return true
	}

	// 3. If pattern has no wildcards, allow intuitive substring matching
	if !strings.ContainsAny(pattern, "*?[]") {
		if strings.Contains(baseLower, patternLower) || strings.Contains(relLower, patternLower) {
			return true
		}
	}

	return false
}

// ParseFindFilesPayload parses XML arguments for a find_files action.
// Expected formats:
// <pattern>*.go</pattern>
// <path>pkg</path>
// <max_results>25</max_results>
// Or plain pattern text inside the action.
func ParseFindFilesPayload(payload string) (*FindFilesInput, error) {
	pattern := extractTag(payload, "pattern")
	path := extractTag(payload, "path")
	if path == "" {
		path = extractTag(payload, "dir")
	}
	maxStr := extractTag(payload, "max_results")

	if pattern == "" && !strings.Contains(payload, "<") {
		pattern = strings.TrimSpace(payload)
	}

	maxResults := 50
	if maxStr != "" {
		if n, err := strconv.Atoi(maxStr); err == nil && n > 0 {
			maxResults = n
		}
	}

	return &FindFilesInput{
		Pattern:    pattern,
		Path:       path,
		MaxResults: maxResults,
	}, nil
}
