// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package guardrail

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// CheckPathWithinBounds verifies that a given target path strictly resides within the authorized workspace.
// It handles relative paths, absolute paths, parent directory traversal (".."), and symlink dereferencing.
func CheckPathWithinBounds(workDir string, targetPath string) (string, error) {
	if workDir == "" {
		d, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve current working directory: %w", err)
		}
		workDir = d
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve absolute workspace directory: %w", err)
	}

	// Resolve symlinks on the workspace root itself if it exists
	if realWorkDir, err := filepath.EvalSymlinks(absWorkDir); err == nil {
		absWorkDir = realWorkDir
	}
	absWorkDir = filepath.Clean(absWorkDir)

	cleanedTarget := strings.TrimSpace(targetPath)
	if cleanedTarget == "" {
		return absWorkDir, nil
	}

	// Security: Reject null bytes immediately
	if strings.Contains(cleanedTarget, "\x00") {
		return "", fmt.Errorf("malformed path: null byte detected")
	}

	// Security: Detect and reject URL-encoded traversal attempts (%2e%2e%2f)
	if strings.Contains(cleanedTarget, "%") {
		if unescaped, err := url.QueryUnescape(cleanedTarget); err == nil && unescaped != cleanedTarget {
			if strings.Contains(unescaped, "..") {
				return "", fmt.Errorf("malformed path: encoded traversal sequence detected (%q)", targetPath)
			}
			cleanedTarget = unescaped
		}
	}

	// Expand ~ to user home directory if present
	if strings.HasPrefix(cleanedTarget, "~/") || cleanedTarget == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			cleanedTarget = filepath.Join(home, strings.TrimPrefix(cleanedTarget, "~"))
		}
	}

	var absTarget string
	if filepath.IsAbs(cleanedTarget) {
		absTarget = filepath.Clean(cleanedTarget)
	} else {
		absTarget = filepath.Clean(filepath.Join(absWorkDir, cleanedTarget))
	}

	// Check if target or any existing ancestor is a symlink pointing outside workDir
	evalPath := absTarget
	for {
		if realPath, err := filepath.EvalSymlinks(evalPath); err == nil {
			// If we successfully evaluated a path or symlink, replace that portion
			if evalPath != absTarget {
				relToEval, _ := filepath.Rel(evalPath, absTarget)
				absTarget = filepath.Clean(filepath.Join(realPath, relToEval))
			} else {
				absTarget = realPath
			}
			break
		}
		parent := filepath.Dir(evalPath)
		if parent == evalPath {
			break
		}
		evalPath = parent
	}

	// Verify containment: absTarget must equal absWorkDir OR start with absWorkDir + separator
	rel, err := filepath.Rel(absWorkDir, absTarget)
	if err != nil {
		return "", fmt.Errorf("path %q cannot be resolved relative to workspace %q: %w", targetPath, workDir, err)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return absTarget, fmt.Errorf("path %q traverses outside authorized workspace %q (resolved: %q)", targetPath, workDir, absTarget)
	}

	return absTarget, nil
}

// ExtractTagContent parses the inner text of an XML tag.
func ExtractTagContent(payload, tag string) string {
	openTag := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	start := strings.Index(payload, openTag)
	if start == -1 {
		return ""
	}
	start += len(openTag)
	end := strings.Index(payload[start:], closeTag)
	if end == -1 {
		return ""
	}
	return payload[start : start+end]
}

// ValidateFilesystemBounds inspects an action and confirms all referenced paths are within the workspace.
func ValidateFilesystemBounds(workDir string, actionName string, targetPath string, command string) error {
	// Always prioritize actual XML payload <path> tag if present in the command
	path := ""
	if command != "" {
		path = strings.TrimSpace(ExtractTagContent(command, "path"))
	}
	if path == "" {
		path = strings.TrimSpace(targetPath)
	}

	switch actionName {
	case "write_file", "replace_file", "read_window", "read_outline":
		if path == "" {
			return fmt.Errorf("action %s missing target file path", actionName)
		}
		_, err := CheckPathWithinBounds(workDir, path)
		return err

	case "find_files", "search_code", "git_diff_summary", "get_environment", "run_test":
		if path != "" {
			_, err := CheckPathWithinBounds(workDir, path)
			return err
		}
		return nil

	default:
		return nil
	}
}
