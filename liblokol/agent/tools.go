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

	"github.com/boggycreek/lokol/liblokol/catalog"
	"github.com/boggycreek/lokol/liblokol/regulator"
	"github.com/boggycreek/lokol/liblokol/refinery"
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

func resolveSafePath(path string, workDir ...string) (string, error) {
	wd := "."
	if len(workDir) > 0 && workDir[0] != "" {
		wd = workDir[0]
	}
	return regulator.CheckPathWithinBounds(wd, path)
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

	targetPath, err := resolveSafePath(input.Path, workDir...)
	if err != nil {
		return "", fmt.Errorf("boundary check failed for %s: %w", input.Path, err)
	}

	if fi, statErr := os.Stat(targetPath); statErr == nil && fi.IsDir() {
		return "", fmt.Errorf("%q is a directory, not a file: cannot replace text in a directory. Use <action name=\"find_files\"><pattern>*</pattern></action> to discover files", input.Path)
	}

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

	targetPath, err := resolveSafePath(input.Path, workDir...)
	if err != nil {
		return "", fmt.Errorf("boundary check failed for %s: %w", input.Path, err)
	}

	if fi, statErr := os.Stat(targetPath); statErr == nil && fi.IsDir() {
		return "", fmt.Errorf("%q is an existing directory, not a file. Cannot overwrite a directory with write_file", input.Path)
	}

	dir := filepath.Dir(targetPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// TOCTOU mitigation: verify target file is not a symlink pointing outside bounds
	if fi, err := os.Lstat(targetPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		wd := "."
		if len(workDir) > 0 && workDir[0] != "" {
			wd = workDir[0]
		}
		if _, err := regulator.CheckPathWithinBounds(wd, targetPath); err != nil {
			return "", fmt.Errorf("symlink target escapes workspace bounds: %w", err)
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
	targetPath, err := resolveSafePath(path, workDir...)
	if err != nil {
		return "", fmt.Errorf("boundary check failed for %s: %w", path, err)
	}
	return refinery.ReadOutline(targetPath)
}

// ExecuteReadWindow returns a bounded range of lines for a file.
func ExecuteReadWindow(ctx context.Context, payload string, workDir ...string) (string, error) {
	input, err := refinery.ParseReadWindowPayload(payload)
	if err != nil {
		return "", err
	}
	targetPath, err := resolveSafePath(input.Path, workDir...)
	if err != nil {
		return "", fmt.Errorf("boundary check failed for %s: %w", input.Path, err)
	}
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

// ExecuteGetEnvironment inspects the execution environment and returns formatted JSON.
func ExecuteGetEnvironment(ctx context.Context, payload string, workDir ...string) (string, error) {
	wd := ""
	if len(workDir) > 0 && workDir[0] != "" {
		wd = workDir[0]
	}
	if p := extractTagContent(payload, "path"); p != "" {
		resolved, err := resolveSafePath(strings.TrimSpace(p), wd)
		if err != nil {
			return "", fmt.Errorf("boundary check failed for %s: %w", p, err)
		}
		wd = resolved
	}
	envInfo, err := refinery.GetEnvironment(wd)
	if err != nil {
		return "", err
	}
	return envInfo.FormatJSON()
}

// ExecuteFindFiles discovers files matching pattern within workDir, respecting .gitignore.
func ExecuteFindFiles(ctx context.Context, payload string, workDir ...string) (string, error) {
	wd := ""
	if len(workDir) > 0 && workDir[0] != "" {
		wd = workDir[0]
	}
	input, err := refinery.ParseFindFilesPayload(payload)
	if err != nil {
		return "", err
	}
	targetDir := wd
	if input.Path != "" {
		resolved, err := resolveSafePath(input.Path, wd)
		if err != nil {
			return "", fmt.Errorf("boundary check failed for %s: %w", input.Path, err)
		}
		targetDir = resolved
	}
	return refinery.FindFiles(input.Pattern, targetDir, input.MaxResults)
}

// ExecuteSearchCode performs bounded regex or literal search across workDir, respecting .gitignore.
func ExecuteSearchCode(ctx context.Context, payload string, workDir ...string) (string, error) {
	wd := ""
	if len(workDir) > 0 && workDir[0] != "" {
		wd = workDir[0]
	}
	input, err := refinery.ParseSearchCodePayload(payload)
	if err != nil {
		return "", err
	}
	targetDir := wd
	if input.Path != "" {
		resolved, err := resolveSafePath(input.Path, wd)
		if err != nil {
			return "", fmt.Errorf("boundary check failed for %s: %w", input.Path, err)
		}
		targetDir = resolved
		input.Path = "" // already resolved into targetDir
	}
	return refinery.SearchCode(targetDir, *input)
}

// ExecuteGitDiffSummary provides structured and bounded inspection of repository working changes.
func ExecuteGitDiffSummary(ctx context.Context, payload string, workDir ...string) (string, error) {
	wd := ""
	if len(workDir) > 0 && workDir[0] != "" {
		wd = workDir[0]
	}
	input, err := refinery.ParseGitDiffSummaryPayload(payload)
	if err != nil {
		return "", err
	}
	targetDir := wd
	if input.Path != "" {
		resolved, err := resolveSafePath(input.Path, wd)
		if err != nil {
			return "", fmt.Errorf("boundary check failed for %s: %w", input.Path, err)
		}
		targetDir = resolved
	}
	return refinery.GitDiffSummary(ctx, targetDir, *input)
}

// DispatchAction executes an action against the host system or tool suite using the central tool catalog.
// It serves as the single source of truth for tool invocation across headless,
// TUI, and any future presentation layers adhering to ADR-0024.
func DispatchAction(ctx context.Context, act *Action, workDir string) (string, error) {
	if act == nil {
		return "", fmt.Errorf("action is nil")
	}

	tool, exists := catalog.DefaultRegistry.Get(act.Name)
	if !exists {
		return "", fmt.Errorf("unknown action: %s. Use <action name=\"tool_help\"><tool>all</tool></action> to see available tools", act.Name)
	}

	return tool.Execute(ctx, act.Command, workDir)
}

// TargetSummary returns a concise description or target path for the action.
func (a *Action) TargetSummary() string {
	if a == nil {
		return ""
	}
	switch a.Name {
	case "tool_help":
		if t := extractTagContent(a.Command, "tool"); t != "" {
			return t
		}
		return strings.TrimSpace(a.Command)
	case "replace_file":
		if input, _ := ParseReplaceFileInput(a.Command); input != nil {
			return input.Path
		}
		return "file"
	case "write_file":
		if input, _ := ParseWriteFileInput(a.Command); input != nil {
			return input.Path
		}
		return "file"
	case "read_outline", "read_window":
		p := strings.TrimSpace(extractTagContent(a.Command, "path"))
		if p != "" {
			return p
		}
		return strings.TrimSpace(a.Command)
	case "exec_bash", "run_test":
		return strings.TrimSpace(a.Command)
	case "get_environment":
		return "environment"
	case "find_files":
		if input, _ := refinery.ParseFindFilesPayload(a.Command); input != nil && input.Pattern != "" {
			return input.Pattern
		}
		return strings.TrimSpace(a.Command)
	case "search_code":
		if input, _ := refinery.ParseSearchCodePayload(a.Command); input != nil && input.Pattern != "" {
			return input.Pattern
		}
		return strings.TrimSpace(a.Command)
	case "git_diff_summary":
		if input, _ := refinery.ParseGitDiffSummaryPayload(a.Command); input != nil && input.Path != "" {
			return input.Path
		}
		return "working state"
	case "task_finish":
		return strings.TrimSpace(a.Command)
	default:
		return a.Command
	}
}

// VerboseDescription returns a formatted presentation banner for the action.
func (a *Action) VerboseDescription() string {
	if a == nil {
		return ""
	}
	switch a.Name {
	case "exec_bash":
		return fmt.Sprintf("⚡ Executing: %s", a.Command)
	case "replace_file":
		return fmt.Sprintf("⚡ Editing: %s", a.TargetSummary())
	case "write_file":
		return fmt.Sprintf("⚡ Writing: %s", a.TargetSummary())
	case "read_outline":
		return fmt.Sprintf("⚡ Reading Outline: %s", a.TargetSummary())
	case "read_window":
		return fmt.Sprintf("⚡ Reading Window: %s", a.TargetSummary())
	case "run_test":
		return fmt.Sprintf("⚡ Verifying Tests: %s", a.TargetSummary())
	case "get_environment":
		return "⚡ Inspecting Environment"
	case "find_files":
		return fmt.Sprintf("⚡ Finding Files: %s", a.TargetSummary())
	case "search_code":
		return fmt.Sprintf("⚡ Searching Code: %s", a.TargetSummary())
	case "git_diff_summary":
		return fmt.Sprintf("⚡ Diff Summary: %s", a.TargetSummary())
	case "task_finish":
		return fmt.Sprintf("⚡ Finishing Task: %s", a.TargetSummary())
	default:
		return fmt.Sprintf("⚡ Executing %s: %s", a.Name, a.TargetSummary())
	}
}


