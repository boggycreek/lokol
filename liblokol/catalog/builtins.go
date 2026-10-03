// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package catalog

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/boggycreek/lokol/liblokol/regulator"
	"github.com/boggycreek/lokol/liblokol/refinery"
)

// DefaultRegistry is the pre-populated catalog of all built-in agent capabilities.
var DefaultRegistry = NewDefaultRegistry()

// NewDefaultRegistry builds and populates a Registry with all standard built-in tools.
func NewDefaultRegistry() *Registry {
	reg := NewRegistry()

	// 1. Foundational Tools (Permanently resident in the base system prompt)
	reg.Register(&SimpleTool{
		NameVal:         "find_files",
		SummaryVal:      "Discover files in workspace or inspect directory contents",
		DescriptionVal:  "Recursively searches for files matching a glob or substring pattern within the workspace or a specified subfolder. Bounded at 50 results.",
		SchemaXMLVal:    "<action name=\"find_files\">\n<pattern>*</pattern>\n</action>\n<!-- Or to inspect a subdirectory: <action name=\"find_files\"><path>subfolder</path><pattern>*</pattern></action> -->",
		ExamplesVal:     []string{"<action name=\"find_files\"><pattern>*.go</pattern></action>", "<action name=\"find_files\"><path>liblokol</path><pattern>*</pattern></action>"},
		TagsVal:         []string{"find", "files", "discover", "list", "directory", "dir", "ls"},
		FoundationalVal: true,
		ExecuteFn:       builtinFindFiles,
	})

	reg.Register(&SimpleTool{
		NameVal:         "read_window",
		SummaryVal:      "Inspect lines of a specific file",
		DescriptionVal:  "Reads a bounded line range (e.g. 1-100) from a specific file. Never call on a directory; use find_files for directories.",
		SchemaXMLVal:    "<action name=\"read_window\">\n<path>relative/path/to/file</path>\n<start>1</start>\n<end>100</end>\n</action>",
		ExamplesVal:     []string{"<action name=\"read_window\"><path>main.go</path><start>1</start><end>50</end></action>"},
		TagsVal:         []string{"read", "view", "inspect", "file", "lines"},
		FoundationalVal: true,
		ExecuteFn:       builtinReadWindow,
	})

	reg.Register(&SimpleTool{
		NameVal:         "replace_file",
		SummaryVal:      "Targeted exact substring replacement in a file",
		DescriptionVal:  "Replaces an exact block of code in a file. Must provide exact matching target text.",
		SchemaXMLVal:    "<action name=\"replace_file\">\n<path>relative/path/to/file</path>\n<target>\nexact text to match\n</target>\n<replacement>\nexact replacement text\n</replacement>\n</action>",
		ExamplesVal:     []string{"<action name=\"replace_file\">\n<path>lib/calc.go</path>\n<target>\nreturn a - b\n</target>\n<replacement>\nreturn a + b\n</replacement>\n</action>"},
		TagsVal:         []string{"replace", "edit", "modify", "patch", "fix"},
		FoundationalVal: true,
		ExecuteFn:       builtinReplaceFile,
	})

	reg.Register(&SimpleTool{
		NameVal:         "task_finish",
		SummaryVal:      "Signal task completion and provide final handover",
		DescriptionVal:  "Concludes the active agent loop and returns the final handover summary to the operator.",
		SchemaXMLVal:    "<action name=\"task_finish\">\nsummary of completed task\n</action>",
		ExamplesVal:     []string{"<action name=\"task_finish\">\nRefactored calculateSum to handle zero correctly and verified tests pass.\n</action>"},
		TagsVal:         []string{"finish", "done", "complete", "handover"},
		FoundationalVal: true,
		ExecuteFn: func(ctx context.Context, payload string, workDir ...string) (string, error) {
			return strings.TrimSpace(payload), nil
		},
	})

	reg.Register(&SimpleTool{
		NameVal:         "tool_help",
		SummaryVal:      "Inspect usage instructions and schema for specialized tools",
		DescriptionVal:  "Queries the tool catalog for usage instructions, schema templates, and examples for any specialized tool. Call with empty <tool> or 'all' to list all tools.",
		SchemaXMLVal:    "<action name=\"tool_help\">\n<tool>tool_name</tool>\n</action>",
		ExamplesVal:     []string{"<action name=\"tool_help\"><tool>git_diff_summary</tool></action>", "<action name=\"tool_help\"><tool>exec_bash</tool></action>"},
		TagsVal:         []string{"help", "tools", "describe", "usage", "info"},
		FoundationalVal: true,
		ExecuteFn: func(ctx context.Context, payload string, workDir ...string) (string, error) {
			toolName := extractTagContent(payload, "tool")
			if toolName == "" {
				toolName = strings.TrimSpace(payload)
			}
			return reg.Help(toolName)
		},
	})

	// 2. Specialized Tools (Disclosed progressively on demand or via intent routing)
	reg.Register(&SimpleTool{
		NameVal:         "exec_bash",
		SummaryVal:      "Run non-interactive shell commands",
		DescriptionVal:  "Executes a non-interactive shell command within the workspace. Output is noise-filtered.",
		SchemaXMLVal:    "<action name=\"exec_bash\">\ncommand here\n</action>",
		ExamplesVal:     []string{"<action name=\"exec_bash\">\ngo list ./...\n</action>", "<action name=\"exec_bash\">\ngit status\n</action>"},
		TagsVal:         []string{"shell", "bash", "exec", "terminal", "command", "run", "packages", "go"},
		FoundationalVal: false,
		ExecuteFn:       builtinExecBash,
	})

	reg.Register(&SimpleTool{
		NameVal:         "run_test",
		SummaryVal:      "Run workspace test suite verifier",
		DescriptionVal:  "Executes tests in the workspace and extracts concise pass/fail summaries with isolated error lines.",
		SchemaXMLVal:    "<action name=\"run_test\">\noptional/subpackage\n</action>",
		ExamplesVal:     []string{"<action name=\"run_test\">\n./liblokol/agent\n</action>"},
		TagsVal:         []string{"test", "tests", "verify", "verification", "check", "assert"},
		FoundationalVal: false,
		ExecuteFn:       builtinRunTest,
	})

	reg.Register(&SimpleTool{
		NameVal:         "search_code",
		SummaryVal:      "Search text or regex across workspace files",
		DescriptionVal:  "Performs case-insensitive regex or literal search across workspace files, respecting .gitignore.",
		SchemaXMLVal:    "<action name=\"search_code\">\n<pattern>query</pattern>\n</action>",
		ExamplesVal:     []string{"<action name=\"search_code\"><pattern>func NewSession</pattern></action>"},
		TagsVal:         []string{"search", "grep", "find_text", "code", "where"},
		FoundationalVal: false,
		ExecuteFn:       builtinSearchCode,
	})

	reg.Register(&SimpleTool{
		NameVal:         "git_diff_summary",
		SummaryVal:      "Inspect git working directory diff and status",
		DescriptionVal:  "Returns git status and concise diff summary of uncommitted changes or branch changes.",
		SchemaXMLVal:    "<action name=\"git_diff_summary\">\n<path>optional/path</path>\n</action>",
		ExamplesVal:     []string{"<action name=\"git_diff_summary\"></action>"},
		TagsVal:         []string{"git", "diff", "branch", "commit", "changes", "status"},
		FoundationalVal: false,
		ExecuteFn:       builtinGitDiffSummary,
	})

	reg.Register(&SimpleTool{
		NameVal:         "read_outline",
		SummaryVal:      "Discover code structure, functions, classes, and types (AST)",
		DescriptionVal:  "Parses code symbols and returns a high-level outline of declarations, functions, and methods.",
		SchemaXMLVal:    "<action name=\"read_outline\">\n<path>relative/path/to/file</path>\n</action>",
		ExamplesVal:     []string{"<action name=\"read_outline\"><path>liblokol/agent/session.go</path></action>"},
		TagsVal:         []string{"ast", "outline", "structure", "symbols", "signatures", "methods"},
		FoundationalVal: false,
		ExecuteFn:       builtinReadOutline,
	})

	reg.Register(&SimpleTool{
		NameVal:         "write_file",
		SummaryVal:      "Create a new file or write complete content",
		DescriptionVal:  "Creates a new file or overwrites an existing file with the provided content.",
		SchemaXMLVal:    "<action name=\"write_file\">\n<path>path/to/file</path>\n<content>\nfull content here\n</content>\n</action>",
		ExamplesVal:     []string{"<action name=\"write_file\">\n<path>hello.txt</path>\n<content>\nHello world\n</content>\n</action>"},
		TagsVal:         []string{"write", "create_file", "new_file", "save"},
		FoundationalVal: false,
		ExecuteFn:       builtinWriteFile,
	})

	reg.Register(&SimpleTool{
		NameVal:         "get_environment",
		SummaryVal:      "Inspect host OS, shell, git status, and toolchain versions",
		DescriptionVal:  "Returns a JSON summary of host environment, installed toolchains, git branch, and top-level files.",
		SchemaXMLVal:    "<action name=\"get_environment\">\n</action>",
		ExamplesVal:     []string{"<action name=\"get_environment\"></action>"},
		TagsVal:         []string{"environment", "env", "mcp", "host", "system", "toolchains"},
		FoundationalVal: false,
		ExecuteFn:       builtinGetEnvironment,
	})

	return reg
}

// -------------------------------------------------------------------------
// Builtin Execution Implementations
// -------------------------------------------------------------------------

func resolveSafePath(path string, workDir ...string) (string, error) {
	wd := "."
	if len(workDir) > 0 && workDir[0] != "" {
		wd = workDir[0]
	}
	return regulator.CheckPathWithinBounds(wd, path)
}

// ExtractTagContent parses the inner text of an XML tag, supporting attributes and whitespace (lokol-oxb.1).
func ExtractTagContent(xml, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"

	startIdx := -1
	if idx := strings.Index(xml, startTag); idx != -1 {
		startIdx = idx + len(startTag)
	} else {
		prefix := "<" + tag
		offset := 0
		for {
			idx := strings.Index(xml[offset:], prefix)
			if idx == -1 {
				return ""
			}
			pos := offset + idx
			afterTag := pos + len(prefix)
			if afterTag < len(xml) {
				nextChar := xml[afterTag]
				if nextChar == '>' {
					startIdx = afterTag + 1
					break
				}
				if nextChar == ' ' || nextChar == '\t' || nextChar == '\n' || nextChar == '\r' {
					closingBracket := strings.Index(xml[afterTag:], ">")
					if closingBracket != -1 {
						if closingBracket > 0 && xml[afterTag+closingBracket-1] == '/' {
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

	endIdx := strings.Index(xml[startIdx:], endTag)
	if endIdx == -1 {
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

var extractTagContent = ExtractTagContent

func builtinFindFiles(ctx context.Context, payload string, workDir ...string) (string, error) {
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
			return "", fmt.Errorf("boundary check failed for directory %s: %w", input.Path, err)
		}
		targetDir = resolved
	}
	return refinery.FindFiles(input.Pattern, targetDir, input.MaxResults)
}

func builtinReadWindow(ctx context.Context, payload string, workDir ...string) (string, error) {
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

func builtinReplaceFile(ctx context.Context, payload string, workDir ...string) (string, error) {
	target := extractTagContent(payload, "target")
	content := extractTagContent(payload, "content")
	if target == "" && content != "" {
		return builtinWriteFile(ctx, payload, workDir...)
	}

	path := extractTagContent(payload, "path")
	replacement := extractTagContent(payload, "replacement")
	if path == "" {
		return "", fmt.Errorf("missing <path> in replace_file action")
	}
	if target == "" {
		return "", fmt.Errorf("missing <target> in replace_file action")
	}

	targetPath, err := resolveSafePath(strings.TrimSpace(path), workDir...)
	if err != nil {
		return "", fmt.Errorf("boundary check failed for %s: %w", path, err)
	}

	if fi, statErr := os.Stat(targetPath); statErr == nil && fi.IsDir() {
		return "", fmt.Errorf("%q is a directory, not a file: cannot replace text in a directory. Use <action name=\"find_files\"><pattern>*</pattern></action> to discover files", path)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}

	fileContent := string(data)
	hasCRLF := strings.Contains(fileContent, "\r\n")
	hasBareLF := strings.Contains(strings.ReplaceAll(fileContent, "\r\n", ""), "\n")
	isUniformCRLF := hasCRLF && !hasBareLF

	targetNorm := strings.ReplaceAll(target, "\r\n", "\n")
	replacementNorm := strings.ReplaceAll(replacement, "\r\n", "\n")

	var newContent string
	replaced := false

	// If the file is uniformly CRLF, adapt target and replacement to CRLF and match directly.
	// This preserves all untouched content byte-identically without full-file re-expansion.
	if isUniformCRLF {
		targetCRLF := strings.ReplaceAll(targetNorm, "\n", "\r\n")
		replacementCRLF := strings.ReplaceAll(replacementNorm, "\n", "\r\n")
		if strings.Contains(fileContent, targetCRLF) {
			newContent = strings.Replace(fileContent, targetCRLF, replacementCRLF, 1)
			replaced = true
		}
	} else if strings.Contains(fileContent, target) {
		// Exact match in file with LF or mixed endings as-is
		newContent = strings.Replace(fileContent, target, replacement, 1)
		replaced = true
	}

	if !replaced {
		contentNorm := strings.ReplaceAll(fileContent, "\r\n", "\n")
		if strings.Contains(contentNorm, targetNorm) {
			newContent = strings.Replace(contentNorm, targetNorm, replacementNorm, 1)
		} else {
			trimmedTarget := strings.Trim(targetNorm, "\n")
			trimmedReplacement := strings.Trim(replacementNorm, "\n")
			if trimmedTarget != "" && strings.Contains(contentNorm, trimmedTarget) && strings.Count(contentNorm, trimmedTarget) == 1 {
				newContent = strings.Replace(contentNorm, trimmedTarget, trimmedReplacement, 1)
			} else {
				lineCount := strings.Count(contentNorm, "\n") + 1
				return "", fmt.Errorf("target string not found in %s (%d lines in file). Please inspect the file with read_window to verify exact content, or use write_file to overwrite the file completely", path, lineCount)
			}
		}

		if isUniformCRLF {
			newContent = strings.ReplaceAll(newContent, "\n", "\r\n")
		}
	}

	if err := os.WriteFile(targetPath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write updated file %s: %w", path, err)
	}

	return fmt.Sprintf("Successfully replaced target block in %s", path), nil
}

func builtinWriteFile(ctx context.Context, payload string, workDir ...string) (string, error) {
	path := extractTagContent(payload, "path")
	if path == "" {
		return "", fmt.Errorf("missing <path> in write_file action")
	}

	var content string
	if strings.Contains(payload, "<content") {
		content = extractTagContent(payload, "content")
	} else {
		pathEnd := "</path>"
		if idx := strings.Index(payload, pathEnd); idx != -1 {
			rest := strings.TrimSpace(payload[idx+len(pathEnd):])
			if rest != "" {
				content = rest
			}
		}
	}

	if content == "" && !strings.Contains(payload, "<content") {
		return "", fmt.Errorf("missing <content> in write_file action: file content must be enclosed inside <content>...</content> inside the action")
	}

	targetPath, err := resolveSafePath(strings.TrimSpace(path), workDir...)
	if err != nil {
		return "", fmt.Errorf("boundary check failed for %s: %w", path, err)
	}

	if fi, statErr := os.Stat(targetPath); statErr == nil && fi.IsDir() {
		return "", fmt.Errorf("%q is an existing directory, not a file. Cannot overwrite a directory with write_file", path)
	}

	dir := filepath.Dir(targetPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory structure for %s: %w", path, err)
		}
	}

	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// MaxExecOutputChars defines the maximum output budget before truncation in exec_bash (4000 chars).
const MaxExecOutputChars = 4000

// TruncateOutputAtLine cuts output at the last complete newline within maxChars
// and appends a "[truncated N more lines]" indicator.
func TruncateOutputAtLine(output string, maxChars int) string {
	if len(output) <= maxChars {
		return output
	}
	lastNL := strings.LastIndex(output[:maxChars], "\n")
	var truncated string
	var remainingLines int
	if lastNL != -1 {
		truncated = output[:lastNL]
		remainingLines = strings.Count(output[lastNL+1:], "\n")
		if !strings.HasSuffix(output, "\n") {
			remainingLines++
		}
	} else {
		truncated = output[:maxChars]
		remainingLines = strings.Count(output[maxChars:], "\n")
		if !strings.HasSuffix(output, "\n") {
			remainingLines++
		}
	}
	if remainingLines <= 0 {
		remainingLines = 1
	}
	return truncated + fmt.Sprintf("\n[truncated %d more lines]", remainingLines)
}

func builtinExecBash(ctx context.Context, command string, workDir ...string) (string, error) {
	if inner := extractTagContent(command, "command"); inner != "" {
		command = inner
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("exec_bash: empty command")
	}

	wd := "."
	if len(workDir) > 0 && workDir[0] != "" {
		wd = workDir[0]
	}

	// Static regulator safety check
	risk := regulator.InspectShellRisk(wd, command)
	if risk.Level == regulator.RiskLevelHigh || risk.Level == regulator.RiskLevelCritical {
		return "", fmt.Errorf("regulator blocked execution: %s (reason: %s)", command, risk.Reason)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", command)
	if len(workDir) > 0 && workDir[0] != "" {
		cwd, err := filepath.Abs(workDir[0])
		if err == nil {
			cmd.Dir = cwd
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("BEADS_DIR=%s", filepath.Join(cwd, ".beads")),
				fmt.Sprintf("GIT_CEILING_DIRECTORIES=%s", filepath.Dir(cwd)),
			)
		} else {
			cmd.Dir = workDir[0]
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	execErr := cmd.Run()
	output := TruncateOutputAtLine(stdout.String(), MaxExecOutputChars)
	if stderr.Len() > 0 {
		stderrStr := TruncateOutputAtLine(stderr.String(), MaxExecOutputChars)
		if output != "" {
			output += "\n"
		}
		output += stderrStr
	}

	if execErr != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after 30s: %s", command)
		}
		return output, fmt.Errorf("command exited with error: %w", execErr)
	}

	return output, nil
}

func builtinRunTest(ctx context.Context, command string, workDir ...string) (string, error) {
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

func builtinSearchCode(ctx context.Context, payload string, workDir ...string) (string, error) {
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
			return "", fmt.Errorf("boundary check failed for directory %s: %w", input.Path, err)
		}
		targetDir = resolved
		input.Path = ""
	}
	return refinery.SearchCode(targetDir, *input)
}

func builtinGitDiffSummary(ctx context.Context, payload string, workDir ...string) (string, error) {
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
			return "", fmt.Errorf("boundary check failed for path %s: %w", input.Path, err)
		}
		targetDir = resolved
		input.Path = ""
	}
	return refinery.GitDiffSummary(ctx, targetDir, *input)
}

func builtinReadOutline(ctx context.Context, payload string, workDir ...string) (string, error) {
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

func builtinGetEnvironment(ctx context.Context, payload string, workDir ...string) (string, error) {
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
