// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package refinery

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SearchCodeInput specifies parameters for bounded source code search.
type SearchCodeInput struct {
	Pattern    string
	Path       string
	IsRegex    bool
	MaxResults int
}

const (
	DefaultSearchMaxResults = 30
	HardSearchCap           = 50
	MaxLineDisplayLength    = 140
	MaxSearchFileSize       = 2 * 1024 * 1024 // 2MB
)

// SearchCode performs a bounded regex or literal search across workspace files.
// Respects .gitignore and default exclusions, bounds result count and line length.
func SearchCode(baseDir string, input SearchCodeInput) (string, error) {
	if strings.TrimSpace(input.Pattern) == "" {
		return "", fmt.Errorf("search pattern cannot be empty")
	}

	maxResults := input.MaxResults
	if maxResults <= 0 {
		maxResults = DefaultSearchMaxResults
	}
	if maxResults > HardSearchCap {
		maxResults = HardSearchCap
	}

	var regex *regexp.Regexp
	var err error
	if input.IsRegex {
		regex, err = regexp.Compile(input.Pattern)
		if err != nil {
			return "", fmt.Errorf("invalid regular expression %q: %w", input.Pattern, err)
		}
	}
	lowerPattern := strings.ToLower(input.Pattern)

	searchRoot := baseDir
	if input.Path != "" {
		cleanSub := filepath.Clean(input.Path)
		if filepath.IsAbs(cleanSub) {
			rel, err := filepath.Rel(baseDir, cleanSub)
			if err != nil || strings.HasPrefix(rel, "..") {
				return "", fmt.Errorf("path %q is outside workspace directory", input.Path)
			}
			searchRoot = cleanSub
		} else {
			searchRoot = filepath.Join(baseDir, cleanSub)
		}
	}

	info, err := os.Stat(searchRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("search directory does not exist: %s", input.Path)
		}
		return "", err
	}

	gi := loadGitIgnore(baseDir)

	var matches []string
	truncated := false

	// If searchRoot is a single file:
	if !info.IsDir() {
		relPath, err := filepath.Rel(baseDir, searchRoot)
		if err != nil {
			relPath = searchRoot
		}
		fileMatches, err := searchFile(searchRoot, relPath, input.IsRegex, regex, lowerPattern, maxResults)
		if err != nil {
			return "", err
		}
		if len(fileMatches) == 0 {
			return fmt.Sprintf("No matches found for %q in %s", input.Pattern, relPath), nil
		}
		return formatSearchOutput(fileMatches, input.Pattern, len(fileMatches) >= maxResults), nil
	}

	err = filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}

		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			relPath = path
		}

		name := d.Name()
		if d.IsDir() {
			if defaultIgnoredDirs[name] {
				return filepath.SkipDir
			}
			if relPath != "." && gi.matches(relPath) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip if matched by gitignore
		if gi.matches(relPath) {
			return nil
		}

		// Filter out non-regular files
		if !d.Type().IsRegular() {
			return nil
		}

		// Filter out binary files
		if isBinaryOrOversized(path) {
			return nil
		}

		remaining := maxResults - len(matches)
		if remaining <= 0 {
			truncated = true
			return fs.SkipAll
		}

		fileMatches, err := searchFile(path, relPath, input.IsRegex, regex, lowerPattern, remaining)
		if err != nil {
			return nil // skip unreadable file
		}

		matches = append(matches, fileMatches...)
		if len(matches) >= maxResults {
			truncated = true
			return fs.SkipAll
		}

		return nil
	})

	if err != nil && err != fs.SkipAll {
		return "", fmt.Errorf("walk error: %w", err)
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No matches found for %q", input.Pattern), nil
	}

	sort.Strings(matches)
	return formatSearchOutput(matches, input.Pattern, truncated), nil
}

func searchFile(filePath, displayPath string, isRegex bool, regex *regexp.Regexp, lowerPattern string, maxCount int) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var results []string
	scanner := bufio.NewScanner(f)
	// Allow scanning up to 64KB lines
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 64*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		matched := false
		if isRegex {
			matched = regex.MatchString(line)
		} else {
			matched = strings.Contains(strings.ToLower(line), lowerPattern)
		}

		if matched {
			displayLine := strings.TrimSpace(line)
			if len(displayLine) > MaxLineDisplayLength {
				displayLine = displayLine[:MaxLineDisplayLength] + "..."
			}
			results = append(results, fmt.Sprintf("%s:%d: %s", displayPath, lineNum, displayLine))
			if len(results) >= maxCount {
				break
			}
		}
	}

	return results, scanner.Err()
}

func isBinaryOrOversized(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() > MaxSearchFileSize {
		return true
	}

	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF && n == 0 {
		return false
	}

	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}

func formatSearchOutput(matches []string, pattern string, truncated bool) string {
	var b strings.Builder
	for i, m := range matches {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m)
	}

	if truncated {
		b.WriteString(fmt.Sprintf("\n[... truncated at %d matches. Refine pattern or path to narrow results]", len(matches)))
	}
	return b.String()
}

// ParseSearchCodePayload parses action command payload formatted as XML tags or plain string.
// <pattern>search_term</pattern>
// <path>optional/path</path>
// <regex>true|false</regex>
// <max_results>30</max_results>
func ParseSearchCodePayload(payload string) (*SearchCodeInput, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, fmt.Errorf("empty search_code command")
	}

	pattern := extractTag(payload, "pattern")
	path := extractTag(payload, "path")
	regexStr := extractTag(payload, "regex")
	if regexStr == "" {
		regexStr = extractTag(payload, "is_regex")
	}
	maxStr := extractTag(payload, "max_results")

	// If no <pattern> tag, treat entire payload as pattern
	if pattern == "" {
		pattern = payload
	}

	isRegex := false
	if regexStr != "" {
		isRegex = strings.EqualFold(regexStr, "true") || regexStr == "1"
	}

	maxResults := DefaultSearchMaxResults
	if maxStr != "" {
		if val, err := strconv.Atoi(maxStr); err == nil && val > 0 {
			maxResults = val
		}
	}

	return &SearchCodeInput{
		Pattern:    pattern,
		Path:       path,
		IsRegex:    isRegex,
		MaxResults: maxResults,
	}, nil
}
