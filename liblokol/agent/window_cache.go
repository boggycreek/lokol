// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CachedWindowEntry holds metadata for a previously read file window.
type CachedWindowEntry struct {
	Path      string
	StartLine int
	EndLine   int
	ModTime   time.Time
	Size      int64
	ReadAt    time.Time
}

// WindowReadCache detects and suppresses redundant read_window calls on unchanged files (lokol-kih.4).
type WindowReadCache struct {
	mu      sync.RWMutex
	entries map[string][]CachedWindowEntry // key: canonical path
}

// NewWindowReadCache creates an initialized WindowReadCache.
func NewWindowReadCache() *WindowReadCache {
	return &WindowReadCache{
		entries: make(map[string][]CachedWindowEntry),
	}
}

// CheckRedundant checks whether the requested line window of path has already been read in this session
// and remains unchanged on disk.
func (c *WindowReadCache) CheckRedundant(path string, startLine, endLine int, workDir string) (bool, string, error) {
	if c == nil {
		return false, "", nil
	}

	targetPath, err := resolveSafePath(path, workDir)
	if err != nil {
		return false, "", err
	}

	fi, err := os.Stat(targetPath)
	if err != nil {
		return false, "", err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	key := filepath.Clean(targetPath)
	cachedList, exists := c.entries[key]
	if !exists {
		return false, "", nil
	}

	for _, entry := range cachedList {
		// Verify file has not been modified since the cached read
		if entry.ModTime.Equal(fi.ModTime()) && entry.Size == fi.Size() {
			// Check if requested window is covered by previously read window
			if entry.StartLine <= startLine && endLine <= entry.EndLine {
				notice := fmt.Sprintf("[Notice: File window %s (lines %d-%d) has already been read in this session and is unchanged on disk. Content was not re-sent to conserve context tokens. Refer to the earlier read_window result in your context, or proceed with modifying the file or synthesizing your answer.]", path, startLine, endLine)
				return true, notice, nil
			}
		}
	}

	return false, "", nil
}

// RecordRead records a successful read_window invocation for trajectory deduplication.
func (c *WindowReadCache) RecordRead(path string, startLine, endLine int, workDir string) {
	if c == nil {
		return
	}

	targetPath, err := resolveSafePath(path, workDir)
	if err != nil {
		return
	}

	fi, err := os.Stat(targetPath)
	if err != nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := filepath.Clean(targetPath)
	c.entries[key] = append(c.entries[key], CachedWindowEntry{
		Path:      targetPath,
		StartLine: startLine,
		EndLine:   endLine,
		ModTime:   fi.ModTime(),
		Size:      fi.Size(),
		ReadAt:    time.Now(),
	})
}

// Invalidate clears cached entries for the specified file (e.g. after write_file or replace_file).
func (c *WindowReadCache) Invalidate(path string, workDir string) {
	if c == nil {
		return
	}

	targetPath, err := resolveSafePath(path, workDir)
	if err != nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, filepath.Clean(targetPath))
}

// Clear removes all cached read windows.
func (c *WindowReadCache) Clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string][]CachedWindowEntry)
}
