// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package refinery

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GitDiffSummaryInput specifies parameters for bounded git working state inspection.
type GitDiffSummaryInput struct {
	Path     string
	MaxLines int
	Staged   bool
}

const (
	DefaultDiffMaxLines = 100
	HardDiffMaxLines    = 200
)

// GitDiffSummary provides a structured, token-bounded overview of repository working state.
// Includes status, compact diffstat, and bounded unified diff snippets.
func GitDiffSummary(ctx context.Context, repoDir string, input GitDiffSummaryInput) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	maxLines := input.MaxLines
	if maxLines <= 0 {
		maxLines = DefaultDiffMaxLines
	}
	if maxLines > HardDiffMaxLines {
		maxLines = HardDiffMaxLines
	}

	// 1. Verify git repository
	chkCmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	chkCmd.Dir = repoDir
	if err := chkCmd.Run(); err != nil {
		return "", fmt.Errorf("directory is not a git repository: %s", repoDir)
	}

	// 2. Get current branch or commit
	branchCmd := exec.CommandContext(ctx, "git", "branch", "--show-current")
	branchCmd.Dir = repoDir
	branchOut, _ := branchCmd.Output()
	branch := strings.TrimSpace(string(branchOut))
	if branch == "" {
		revCmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
		revCmd.Dir = repoDir
		revOut, _ := revCmd.Output()
		branch = fmt.Sprintf("HEAD (%s)", strings.TrimSpace(string(revOut)))
	}

	// 3. Get porcelain status
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1")
	statusCmd.Dir = repoDir
	statusOut, err := statusCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git status: %w", err)
	}

	var stagedFiles []string
	var unstagedFiles []string
	var untrackedFiles []string

	lines := strings.Split(string(statusOut), "\n")
	for _, l := range lines {
		l = strings.TrimRight(l, "\r\n")
		if len(l) < 4 {
			continue
		}
		stagedCode := l[0]
		unstagedCode := l[1]
		filePath := strings.TrimSpace(l[3:])
		if strings.Contains(filePath, " -> ") {
			parts := strings.Split(filePath, " -> ")
			filePath = parts[1]
		}

		if stagedCode == '?' && unstagedCode == '?' {
			untrackedFiles = append(untrackedFiles, filePath)
			continue
		}
		if stagedCode != ' ' && stagedCode != '?' {
			stagedFiles = append(stagedFiles, fmt.Sprintf("%c %s", stagedCode, filePath))
		}
		if unstagedCode != ' ' && unstagedCode != '?' {
			unstagedFiles = append(unstagedFiles, fmt.Sprintf("%c %s", unstagedCode, filePath))
		}
	}

	if len(stagedFiles) == 0 && len(unstagedFiles) == 0 && len(untrackedFiles) == 0 {
		return fmt.Sprintf("Branch: %s\nWorking directory clean (no staged, unstaged, or untracked changes).", branch), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Branch: %s\n", branch))

	if len(stagedFiles) > 0 {
		sb.WriteString(fmt.Sprintf("\nStaged changes (%d):\n", len(stagedFiles)))
		for _, f := range stagedFiles {
			sb.WriteString(fmt.Sprintf("  %s\n", f))
		}
	}

	if len(unstagedFiles) > 0 {
		sb.WriteString(fmt.Sprintf("\nUnstaged changes (%d):\n", len(unstagedFiles)))
		for _, f := range unstagedFiles {
			sb.WriteString(fmt.Sprintf("  %s\n", f))
		}
	}

	if len(untrackedFiles) > 0 {
		sb.WriteString(fmt.Sprintf("\nUntracked files (%d):\n", len(untrackedFiles)))
		displayUntracked := untrackedFiles
		if len(displayUntracked) > 10 {
			displayUntracked = displayUntracked[:10]
		}
		for _, f := range displayUntracked {
			sb.WriteString(fmt.Sprintf("  ? %s\n", f))
		}
		if len(untrackedFiles) > 10 {
			sb.WriteString(fmt.Sprintf("  [... and %d more untracked files]\n", len(untrackedFiles)-10))
		}
	}

	// 4. Compact diffstat
	statArgs := []string{"diff", "--stat"}
	if input.Staged {
		statArgs = []string{"diff", "--cached", "--stat"}
	}
	statCmd := exec.CommandContext(ctx, "git", statArgs...)
	statCmd.Dir = repoDir
	statOut, _ := statCmd.Output()
	statStr := strings.TrimSpace(string(statOut))
	if statStr != "" {
		sb.WriteString("\nDiffstat:\n")
		sb.WriteString(statStr)
		sb.WriteString("\n")
	}

	// 5. Bounded Unified Diff
	diffArgs := []string{"diff", "-U2"}
	if input.Staged {
		diffArgs = []string{"diff", "--cached", "-U2"}
	}
	diffCmd := exec.CommandContext(ctx, "git", diffArgs...)
	diffCmd.Dir = repoDir
	diffOut, _ := diffCmd.Output()

	diffLines := strings.Split(strings.TrimSpace(string(diffOut)), "\n")
	if len(diffLines) > 0 && diffLines[0] != "" {
		sb.WriteString("\nUnified Diff (bounded):\n")
		count := 0
		truncated := false
		for _, dl := range diffLines {
			sb.WriteString(dl)
			sb.WriteString("\n")
			count++
			if count >= maxLines {
				truncated = true
				break
			}
		}
		if truncated {
			sb.WriteString(fmt.Sprintf("[... diff truncated at %d lines. Refine inspection with specific files or read_window]\n", maxLines))
		}
	}

	return strings.TrimSpace(sb.String()), nil
}

// ParseGitDiffSummaryPayload parses action command payload formatted as XML tags or plain string.
// <path>optional/path</path>
// <max_lines>100</max_lines>
// <staged>true|false</staged>
func ParseGitDiffSummaryPayload(payload string) (*GitDiffSummaryInput, error) {
	payload = strings.TrimSpace(payload)
	path := extractTag(payload, "path")
	stagedStr := extractTag(payload, "staged")
	maxStr := extractTag(payload, "max_lines")

	staged := false
	if stagedStr != "" {
		staged = strings.EqualFold(stagedStr, "true") || stagedStr == "1"
	}

	maxLines := DefaultDiffMaxLines
	if maxStr != "" {
		if val, err := strconv.Atoi(maxStr); err == nil && val > 0 {
			maxLines = val
		}
	}

	if path == "" && !strings.Contains(payload, "<") && payload != "" {
		// Treat raw string without tags as path
		path = payload
	}

	return &GitDiffSummaryInput{
		Path:     path,
		MaxLines: maxLines,
		Staged:   staged,
	}, nil
}
