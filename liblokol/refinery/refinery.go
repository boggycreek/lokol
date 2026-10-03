// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package refinery

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ReadWindowInput specifies parameters for bounded line reading.
type ReadWindowInput struct {
	Path      string
	StartLine int
	EndLine   int
}

// ReadWindow reads a specific slice of lines from a file, preventing whole-file dumping.
// Maximum allowable window is 120 lines to protect the KV cache.
func ReadWindow(path string, startLine, endLine int) (string, error) {
	if startLine <= 0 {
		startLine = 1
	}
	if endLine < startLine {
		endLine = startLine + 50
	}
	if endLine-startLine > 120 {
		endLine = startLine + 120
	}

	fi, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", path, err)
	}
	if fi.IsDir() {
		cleanPath := filepath.Clean(path)
		cwd, _ := os.Getwd()
		cleanCwd := filepath.Clean(cwd)
		if cleanPath == "." || cleanPath == "./" || cleanPath == cleanCwd {
			return "", fmt.Errorf("current directory is a directory, not a file. To discover files in this workspace, use <action name=\"find_files\"><pattern>*</pattern></action>. To inspect a file, provide a file path")
		}
		rel, err := filepath.Rel(cleanCwd, cleanPath)
		displayPath := path
		if err == nil && !strings.HasPrefix(rel, "..") {
			displayPath = rel
		}
		return "", fmt.Errorf("%q is a directory, not a file. To discover files in this directory, use <action name=\"find_files\"><path>%s</path><pattern>*</pattern></action>. To inspect a file, provide a file path", displayPath, displayPath)
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentLine := 1
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("// --- %s (Lines %d-%d) ---\n", path, startLine, endLine))
	for scanner.Scan() {
		if currentLine >= startLine && currentLine <= endLine {
			sb.WriteString(fmt.Sprintf("%4d | %s\n", currentLine, scanner.Text()))
		}
		if currentLine > endLine {
			break
		}
		currentLine++
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading file %s: %w", path, err)
	}

	return sb.String(), nil
}

// OutlineItem represents a symbol outline in a file.
type OutlineItem struct {
	Kind      string // package, type, interface, func, method
	Name      string
	Signature string
	Line      int
}

// ReadOutline extracts high-level structural declarations (types, interfaces, function signatures)
// without dumping the inner function bodies.
func ReadOutline(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to stat file %s: %w", path, err)
	}
	cleanPath := filepath.Clean(path)
	cwd, _ := os.Getwd()
	cleanCwd := filepath.Clean(cwd)
	rel, err := filepath.Rel(cleanCwd, cleanPath)
	displayPath := path
	if err == nil && !strings.HasPrefix(rel, "..") {
		displayPath = rel
	}

	if fi.IsDir() {
		if cleanPath == "." || cleanPath == "./" || cleanPath == cleanCwd {
			return "", fmt.Errorf("current directory is a directory, not a file. To discover files in this workspace, use <action name=\"find_files\"><pattern>*</pattern></action>")
		}
		return "", fmt.Errorf("%q is a directory, not a file. To discover files in this directory, use <action name=\"find_files\"><path>%s</path><pattern>*</pattern></action>", displayPath, displayPath)
	}

	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))

	var desc string
	if d, ok := nonCodeFileExtensions[ext]; ok {
		desc = d
	} else if d, ok := nonCodeBaseNames[base]; ok {
		desc = d
	}

	if desc != "" {
		return "", fmt.Errorf("%q is a %s file, not a source code file with function or type declarations. To read its content, use <action name=\"read_window\"><path>%s</path><start>1</start><end>100</end></action>", displayPath, desc, displayPath)
	}

	if ext == ".go" {
		return outlineGoFile(path)
	}
	// Generic regex fallback for other languages (Python, JS, Rust)
	return outlineGenericFile(path)
}

var nonCodeFileExtensions = map[string]string{
	".md":       "Markdown documentation",
	".markdown": "Markdown documentation",
	".mdown":    "Markdown documentation",
	".txt":      "plain text",
	".text":     "plain text",
	".json":     "JSON data",
	".yaml":     "YAML configuration",
	".yml":      "YAML configuration",
	".toml":     "TOML configuration",
	".csv":      "CSV data",
	".tsv":      "TSV data",
	".xml":      "XML document",
	".html":     "HTML document",
	".htm":      "HTML document",
	".css":      "CSS stylesheet",
	".scss":     "SCSS stylesheet",
	".sass":     "SASS stylesheet",
	".less":     "LESS stylesheet",
	".sql":      "SQL query",
	".log":      "log",
	".ini":      "INI configuration",
	".cfg":      "configuration",
	".conf":     "configuration",
	".env":      "environment configuration",
}

var nonCodeBaseNames = map[string]string{
	"readme":       "documentation",
	"license":      "legal/license",
	"copying":      "legal/license",
	"changelog":    "changelog documentation",
	"contributing": "contributing documentation",
	"authors":      "author documentation",
	".gitignore":   "Git ignore",
	".dockerignore": "Docker ignore",
}

func outlineGoFile(path string) (string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return outlineGenericFile(path)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("// Outline: %s (package %s)\n", path, node.Name.Name))

	for _, decl := range node.Decls {
		pos := fset.Position(decl.Pos())
		// Scan declarations
		// We format top-level comments and function/type signatures
		switch d := decl.(type) {
		case interface{}:
			_ = d
		}
		_ = pos
	}

	// For clean concise outlines, line-by-line regex over Go AST is fast and robust
	return outlineGenericFile(path)
}

func outlineGenericFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 1
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("// Symbol Outline: %s\n", path))

	// Regexes to capture signatures without function bodies
	goFuncRegex := regexp.MustCompile(`^(func\s+(\([^)]+\)\s+)?([A-Za-z0-9_]+)\s*\([^)]*\)[^{]*)`)
	goTypeRegex := regexp.MustCompile(`^(type\s+[A-Za-z0-9_]+\s+(struct|interface))`)
	pyDefRegex := regexp.MustCompile(`^(class\s+[A-Za-z0-9_]+|def\s+[A-Za-z0-9_]+\([^)]*\):)`)
	jsFuncRegex := regexp.MustCompile(`^(export\s+)?(function\s+[A-Za-z0-9_]+|const\s+[A-Za-z0-9_]+\s*=\s*(\([^)]*\)|async\s*\([^)]*\))\s*=>)`)

	foundAny := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		var match string
		if m := goFuncRegex.FindString(line); m != "" {
			match = m
		} else if m := goTypeRegex.FindString(line); m != "" {
			match = m
		} else if m := pyDefRegex.FindString(line); m != "" {
			match = m
		} else if m := jsFuncRegex.FindString(line); m != "" {
			match = m
		}

		if match != "" {
			foundAny = true
			sb.WriteString(fmt.Sprintf("L%-4d: %s\n", lineNum, strings.TrimSpace(match)))
		} else if strings.HasPrefix(trimmed, "package ") || strings.HasPrefix(trimmed, "import ") {
			if strings.HasPrefix(trimmed, "package ") {
				sb.WriteString(fmt.Sprintf("L%-4d: %s\n", lineNum, trimmed))
			}
		}
		lineNum++
	}

	if !foundAny {
		sb.WriteString("(No top-level functions, classes, or types detected. Use read_window to inspect file contents.)\n")
	}

	return sb.String(), nil
}

// TestResult represents the summarized, noise-filtered outcome of a test suite run.
type TestResult struct {
	Passed      bool
	Summary     string
	FailedTests []string
	ErrorOutput string
}

// MaxTestOutputBytes is the maximum test output captured by RunTestVerifier (1MB).
const MaxTestOutputBytes = 1024 * 1024

// CappedBuffer captures up to limit bytes, safely discarding subsequent writes
// and appending an output truncation notice.
type CappedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

// NewCappedBuffer creates a new CappedBuffer with the specified byte limit.
func NewCappedBuffer(limit int) *CappedBuffer {
	if limit <= 0 {
		limit = MaxTestOutputBytes
	}
	return &CappedBuffer{limit: limit}
}

func (b *CappedBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n = len(p)
	if b.truncated {
		return n, nil
	}

	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return n, nil
	}

	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		b.truncated = true
	} else {
		b.buf.Write(p)
	}
	return n, nil
}

func (b *CappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		return b.buf.String() + fmt.Sprintf("\n[output truncated after %d bytes]", b.limit)
	}
	return b.buf.String()
}

func (b *CappedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

func (b *CappedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// RunTestVerifier executes test commands and strips verbose stack traces down to
// actionable assertion errors and line numbers.
func RunTestVerifier(ctx context.Context, command string, workDir ...string) (*TestResult, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
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
	combinedBuf := NewCappedBuffer(MaxTestOutputBytes)
	cmd.Stdout = combinedBuf
	cmd.Stderr = combinedBuf

	err := cmd.Run()
	output := combinedBuf.String()

	res := &TestResult{
		Passed: (err == nil),
	}

	if res.Passed {
		// Clean pass receipt
		res.Summary = "✓ Tests passed successfully"
		// Look for standard test summary lines
		lines := strings.Split(output, "\n")
		for _, l := range lines {
			lTrim := strings.TrimSpace(l)
			if strings.HasPrefix(lTrim, "PASS") || strings.HasPrefix(lTrim, "ok") || strings.Contains(lTrim, "passed") {
				res.Summary = fmt.Sprintf("✓ %s", lTrim)
				break
			}
		}
		return res, nil
	}

	// Filter failures
	res.Summary = "✗ Tests failed"
	var failures []string
	var failureDetails strings.Builder

	lines := strings.Split(output, "\n")
	captureBlock := false

	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		// Detect Go test failure
		if strings.HasPrefix(trimmed, "--- FAIL:") {
			failures = append(failures, trimmed)
			captureBlock = true
			failureDetails.WriteString(l + "\n")
			continue
		}
		// Detect compiler errors (e.g. ./main.go:12:4: undefined: Foo)
		if strings.Contains(l, ".go:") && (strings.Contains(l, ": syntax error") || strings.Contains(l, ": undefined:") || strings.Contains(l, ": cannot use")) {
			failures = append(failures, trimmed)
			failureDetails.WriteString(l + "\n")
			continue
		}

		if captureBlock {
			if strings.HasPrefix(trimmed, "FAIL") && !strings.HasPrefix(trimmed, "--- FAIL") {
				captureBlock = false
			} else {
				// Capture only indented error/assertion lines, stop at giant stack traces
				if strings.HasPrefix(l, "    ") && !strings.Contains(l, "runtime/") && !strings.Contains(l, "testing.go") {
					failureDetails.WriteString(l + "\n")
				}
			}
		}
	}

	res.FailedTests = failures
	if failureDetails.Len() > 0 {
		res.ErrorOutput = failureDetails.String()
	} else {
		// Fallback: take last 20 lines if custom regex didn't catch specific format
		if len(lines) > 20 {
			res.ErrorOutput = strings.Join(lines[len(lines)-20:], "\n")
		} else {
			res.ErrorOutput = output
		}
	}

	return res, nil
}

// ParseReadWindowPayload parses XML payload for read_window
// <path>path/to/file</path>
// <start>10</start>
// <end>50</end>
func ParseReadWindowPayload(payload string) (*ReadWindowInput, error) {
	path := extractTag(payload, "path")
	if path == "" {
		path = extractTag(payload, "file")
	}
	if path == "" {
		return nil, fmt.Errorf("missing <path>")
	}
	startStr := extractTag(payload, "start")
	if startStr == "" {
		startStr = extractTag(payload, "start_line")
	}
	endStr := extractTag(payload, "end")
	if endStr == "" {
		endStr = extractTag(payload, "end_line")
	}

	start := 1
	end := 50
	if startStr != "" {
		if s, err := strconv.Atoi(startStr); err == nil {
			start = s
		}
	}
	if endStr != "" {
		if e, err := strconv.Atoi(endStr); err == nil {
			end = e
		}
	}

	return &ReadWindowInput{
		Path:      path,
		StartLine: start,
		EndLine:   end,
	}, nil
}

func extractTag(xml, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"
	s := strings.Index(xml, startTag)
	if s == -1 {
		return ""
	}
	s += len(startTag)
	e := strings.Index(xml[s:], endTag)
	if e == -1 {
		return ""
	}
	return strings.TrimSpace(xml[s : s+e])
}

// LoadCodebaseContext discovers repository guidelines and context following ADR 0009:
// 1. ./AGENTS.md
// 2. ./CLAUDE.md
// 3. .github/AGENTS.md
// The context is formatted inside <project_guidelines> and capped to a safe token budget.
func LoadCodebaseContext(workspaceDir string) string {
	if workspaceDir == "" {
		workspaceDir = "."
	}

	candidates := []string{
		filepath.Join(workspaceDir, "AGENTS.md"),
		filepath.Join(workspaceDir, "CLAUDE.md"),
		filepath.Join(workspaceDir, ".github", "AGENTS.md"),
	}

	var foundPath string
	var content []byte
	for _, cand := range candidates {
		data, err := os.ReadFile(cand)
		if err == nil && len(bytes.TrimSpace(data)) > 0 {
			foundPath = filepath.Base(cand)
			content = bytes.TrimSpace(data)
			break
		}
	}

	if len(content) == 0 {
		return ""
	}

	// Truncate to safe context budget (e.g. 4000 characters) to avoid blowing KV cache
	const maxContextChars = 4000
	text := string(content)
	if len(text) > maxContextChars {
		text = text[:maxContextChars] + "\n...[truncated for context hygiene]"
	}

	return fmt.Sprintf("<project_guidelines source=\"%s\">\n%s\n</project_guidelines>", foundPath, text)
}

