package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// ExecuteReplaceFile performs an exact in-place string replacement in the specified file.
func ExecuteReplaceFile(ctx context.Context, payload string) (string, error) {
	input, err := ParseReplaceFileInput(payload)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(input.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", input.Path, err)
	}

	content := string(data)
	if !strings.Contains(content, input.Target) {
		// Provide helpful feedback to the agent
		return "", fmt.Errorf("target string not found in %s. Please inspect the file to verify exact whitespace and contents", input.Path)
	}

	// Replace exactly one instance
	newContent := strings.Replace(content, input.Target, input.Replacement, 1)

	if err := os.WriteFile(input.Path, []byte(newContent), 0644); err != nil {
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
	content := extractTagContent(payload, "content")

	if path == "" {
		return nil, fmt.Errorf("missing <path> in write_file action")
	}

	return &WriteFileInput{
		Path:    strings.TrimSpace(path),
		Content: content,
	}, nil
}

// ExecuteWriteFile writes full content to the specified file, creating parent directories if needed.
func ExecuteWriteFile(ctx context.Context, payload string) (string, error) {
	input, err := ParseWriteFileInput(payload)
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(input.Path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	if err := os.WriteFile(input.Path, []byte(input.Content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", input.Path, err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(input.Content), input.Path), nil
}

func extractTagContent(xml, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"

	startIdx := strings.Index(xml, startTag)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(startTag)

	endIdx := strings.Index(xml[startIdx:], endTag)
	if endIdx == -1 {
		return ""
	}

	val := xml[startIdx : startIdx+endIdx]
	// If the value starts and ends with a newline, trim only the leading and trailing newline from XML formatting
	val = strings.TrimPrefix(val, "\n")
	val = strings.TrimSuffix(val, "\n")
	return val
}
