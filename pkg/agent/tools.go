// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/boggycreek/lokol/pkg/tools/refinery"
)

// ReplaceFileInput specifies parameters for an exact substring replacement in a file.
type ReplaceFileInput struct {
	Path        string
	Target      string
	Replacement string
}

// ParseReplaceFileInput parses the inner XML of a replace_file action.
// Expected inner XML format:
// <path>path/to/file</path>
// <target>exact text to match</target>
// <replacement>exact replacement text</replacement>
func ParseReplaceFileInput(payload string) (*ReplaceFileInput, error) {
	path := extractTagContent(payload, "path")
	target := extractTagContent(payload, "target")
	replacement := extractTagContent(payload, "replacement")

	if path == "" {
		return nil, fmt.Errorf("missing <path> in replace_file action")
	}
	if target == "" {
		return nil, fmt.Errorf("missing <target> in replace_file action")
	}

	return &ReplaceFileInput{
		Path:        strings.TrimSpace(path),
		Target:      target,
		Replacement: replacement,
	}, nil
}

func resolvePath(path string, workDir ...string) string {
	if filepath.IsAbs(path) || len(workDir) == 0 || workDir[0] == "" {
		return path
	}
	return filepath.Join(workDir[0], path)
}

// ExecuteReplaceFile performs an exact in-place string replacement in the specified file.
func ExecuteReplaceFile(ctx context.Context, payload string, workDir ...string) (string, error) {
	// If the model mistakenly used <content> instead of <target>/<replacement> to write or create a file:
	target := extractTagContent(payload, "target")
	content := extractTagContent(payload, "content")
	if target == "" && content != "" {
		return ExecuteWriteFile(ctx, payload, workDir...)
	}

	input, err := ParseReplaceFileInput(payload)
	if err != nil {
		return "", err
	}

	targetPath := resolvePath(input.Path, workDir...)

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", input.Path, err)
	}

	content = string(data)
	contentNorm := strings.ReplaceAll(content, "\r\n", "\n")
	targetNorm := strings.ReplaceAll(input.Target, "\r\n", "\n")
	replacementNorm := strings.ReplaceAll(input.Replacement, "\r\n", "\n")

	var newContent string
	if strings.Contains(contentNorm, targetNorm) {
		newContent = strings.Replace(contentNorm, targetNorm, replacementNorm, 1)
	} else {
		// Fallback: check if trimming leading/trailing empty lines matches uniquely
		trimmedTarget := strings.Trim(targetNorm, "\n")
		trimmedReplacement := strings.Trim(replacementNorm, "\n")
		if trimmedTarget != "" && strings.Contains(contentNorm, trimmedTarget) && strings.Count(contentNorm, trimmedTarget) == 1 {
			newContent = strings.Replace(contentNorm, trimmedTarget, trimmedReplacement, 1)
		} else {
			lineCount := strings.Count(contentNorm, "\n") + 1
			return "", fmt.Errorf("target string not found in %s (%d lines in file). Please inspect the file with read_window to verify exact content, or use write_file to overwrite the file completely", input.Path, lineCount)
		}
	}

	if err := os.WriteFile(targetPath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write updated file %s: %w", input.Path, err)
	}

	return fmt.Sprintf("Successfully replaced target block in %s", input.Path), nil
}

// WriteFileInput specifies parameters for writing or creating a file.
type WriteFileInput struct {
	Path    string
	Content string
}

// ParseWriteFileInput parses the inner XML of a write_file action.
// <path>path/to/file</path>
// <content>file content here</content>
func ParseWriteFileInput(payload string) (*WriteFileInput, error) {
	path := extractTagContent(payload, "path")
	if path == "" {
		return nil, fmt.Errorf("missing <path> in write_file action")
	}

	var content string
	if strings.Contains(payload, "<content") {
		content = extractTagContent(payload, "content")
	} else {
		// Check if content was provided directly after </path> without <content> tags
		pathEnd := "</path>"
		if idx := strings.Index(payload, pathEnd); idx != -1 {
			rest := strings.TrimSpace(payload[idx+len(pathEnd):])
			if rest != "" {
				content = rest
			}
		}
	}

	if content == "" && !strings.Contains(payload, "<content") {
		return nil, fmt.Errorf("missing <content> in write_file action: file content must be enclosed inside <content>...</content> inside the action")
	}

	return &WriteFileInput{
		Path:    strings.TrimSpace(path),
		Content: content,
	}, nil
}

// ExecuteWriteFile writes full content to the specified file, creating parent directories if needed.
func ExecuteWriteFile(ctx context.Context, payload string, workDir ...string) (string, error) {
	input, err := ParseWriteFileInput(payload)
	if err != nil {
		return "", err
	}

	targetPath := resolvePath(input.Path, workDir...)

	dir := filepath.Dir(targetPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	if err := os.WriteFile(targetPath, []byte(input.Content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", input.Path, err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(input.Content), input.Path), nil
}

func extractTagContent(xml, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"

	startIdx := strings.Index(xml, startTag)
	if startIdx == -1 {
		// Also check for tag with attributes, e.g. <tag lang="...">
		startTagPrefix := "<" + tag
		idx := strings.Index(xml, startTagPrefix)
		if idx != -1 {
			closingBracket := strings.Index(xml[idx:], ">")
			if closingBracket != -1 {
				startIdx = idx + closingBracket + 1
			} else {
				return ""
			}
		} else {
			return ""
		}
	} else {
		startIdx += len(startTag)
	}

	endIdx := strings.Index(xml[startIdx:], endTag)
	if endIdx == -1 {
		// Tag was not closed explicitly; take remainder of xml up to </action> or end
		val := xml[startIdx:]
		if actEnd := strings.Index(val, "</action>"); actEnd != -1 {
			val = val[:actEnd]
		}
		val = strings.TrimPrefix(val, "\n")
		val = strings.TrimSuffix(val, "\n")
		return val
	}

	val := xml[startIdx : startIdx+endIdx]
	val = strings.TrimPrefix(val, "\n")
	val = strings.TrimSuffix(val, "\n")
	return val
}

// ExecuteReadOutline returns the outline of types and function signatures for a file.
func ExecuteReadOutline(ctx context.Context, payload string, workDir ...string) (string, error) {
	path := strings.TrimSpace(extractTagContent(payload, "path"))
	if path == "" {
		path = strings.TrimSpace(payload)
	}
	targetPath := resolvePath(path, workDir...)
	return refinery.ReadOutline(targetPath)
}

// ExecuteReadWindow returns a bounded range of lines for a file.
func ExecuteReadWindow(ctx context.Context, payload string, workDir ...string) (string, error) {
	input, err := refinery.ParseReadWindowPayload(payload)
	if err != nil {
		return "", err
	}
	targetPath := resolvePath(input.Path, workDir...)
	return refinery.ReadWindow(targetPath, input.StartLine, input.EndLine)
}

// ExecuteRunTest runs a test command and returns a noise-filtered result.
func ExecuteRunTest(ctx context.Context, command string, workDir ...string) (string, error) {
	command = strings.TrimSpace(command)
	res, err := refinery.RunTestVerifier(ctx, command, workDir...)
	if err != nil {
		return "", err
	}
	if res.Passed {
		return res.Summary, nil
	}
	return fmt.Sprintf("%s\n\nFailures:\n%s", res.Summary, res.ErrorOutput), nil
}

