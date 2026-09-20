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
	ext := filepath.Ext(path)
	if ext == ".go" {
		return outlineGoFile(path)
	}
	// Generic regex fallback for other languages (Python, JS, Rust)
	return outlineGenericFile(path)
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
		sb.WriteString("(No top-level functions or types detected)\n")
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

// RunTestVerifier executes test commands and strips verbose stack traces down to
// actionable assertion errors and line numbers.
func RunTestVerifier(ctx context.Context, command string) (*TestResult, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	var combinedBuf bytes.Buffer
	cmd.Stdout = &combinedBuf
	cmd.Stderr = &combinedBuf

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
		return nil, fmt.Errorf("missing <path>")
	}
	startStr := extractTag(payload, "start")
	endStr := extractTag(payload, "end")

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
