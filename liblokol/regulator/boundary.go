// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator

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

	// Security: Detect and reject single- or multi-layer URL-encoded traversal attempts (%2e%2e%2f, %252e%252e%252f)
	if strings.Contains(cleanedTarget, "%") {
		cur := cleanedTarget
		for i := 0; i < 10; i++ {
			unescaped, err := url.QueryUnescape(cur)
			if err != nil || unescaped == cur {
				break
			}
			if strings.Contains(unescaped, "..") {
				return "", fmt.Errorf("malformed path: encoded traversal sequence detected (%q)", targetPath)
			}
			cur = unescaped
		}
		cleanedTarget = cur
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

// ExtractTagContent parses the inner text of an XML tag, supporting attributes and whitespace (lokol-gml.4).
// Performance optimization: Uses zero-regex string searching to avoid compiling regexes on every call.
func ExtractTagContent(payload, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"

	startIdx := -1
	if idx := strings.Index(payload, startTag); idx != -1 {
		startIdx = idx + len(startTag)
	} else {
		prefix := "<" + tag
		offset := 0
		for {
			idx := strings.Index(payload[offset:], prefix)
			if idx == -1 {
				return ""
			}
			pos := offset + idx
			afterTag := pos + len(prefix)
			if afterTag < len(payload) {
				nextChar := payload[afterTag]
				if nextChar == '>' {
					startIdx = afterTag + 1
					break
				}
				if nextChar == ' ' || nextChar == '\t' || nextChar == '\n' || nextChar == '\r' {
					closingBracket := strings.Index(payload[afterTag:], ">")
					if closingBracket != -1 {
						if closingBracket > 0 && payload[afterTag+closingBracket-1] == '/' {
							return ""
						}
						startIdx = afterTag + closingBracket + 1
						break
					}
				}
			}
			offset = pos + len(prefix)
		}
	}

	if startIdx == -1 {
		return ""
	}

	endIdx := strings.Index(payload[startIdx:], endTag)
	if endIdx == -1 {
		return ""
	}

	return strings.TrimSpace(payload[startIdx : startIdx+endIdx])
}

// ResolveActionPath returns the canonical target path for an action candidate,
// prioritizing an explicit XML <path> tag within Command, falling back to Path (lokol-nok.2).
func ResolveActionPath(action ActionCandidate) string {
	if action.Command != "" {
		if p := strings.TrimSpace(ExtractTagContent(action.Command, "path")); p != "" {
			return p
		}
	}
	return strings.TrimSpace(action.Path)
}

// ValidateFilesystemBounds inspects an action and confirms all referenced paths are within the workspace (lokol-gml.4).
func ValidateFilesystemBounds(workDir string, actionName string, targetPath string, command string) error {
	candidate := ActionCandidate{Name: actionName, Command: command, Path: targetPath}
	path := ResolveActionPath(candidate)

	// If both XML <path> and targetPath (action.Path) are specified and differ,
	// validate targetPath as well to prevent differential bypass (lokol-nok.2).
	if command != "" && targetPath != "" {
		xmlPath := strings.TrimSpace(ExtractTagContent(command, "path"))
		if xmlPath != "" && xmlPath != strings.TrimSpace(targetPath) {
			if _, err := CheckPathWithinBounds(workDir, targetPath); err != nil {
				return err
			}
		}
	}

	switch actionName {
	case "write_file", "replace_file", "read_window", "read_outline":
		if path == "" {
			return fmt.Errorf("action %s missing target file path", actionName)
		}
		_, err := CheckPathWithinBounds(workDir, path)
		return err

	case "find_files", "search_code":
		// Enforce boundary check if either XML <path> or targetPath is provided (lokol-nok.1)
		if path != "" {
			_, err := CheckPathWithinBounds(workDir, path)
			return err
		}
		return nil

	case "git_diff_summary", "get_environment", "run_test":
		if path != "" {
			_, err := CheckPathWithinBounds(workDir, path)
			return err
		}
		return nil

	default:
		return nil
	}
}
