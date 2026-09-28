// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/boggycreek/lokol/liblokol/catalog"
)

// Message represents a chat message in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// HostEnvironment represents the active host environment details injected into the system prompt.
type HostEnvironment struct {
	Cwd   string
	OS    string
	Shell string
	Files []string
}

// DetectHostEnvironment resolves the active host environment using the given working directory.
// If workDir is empty, it defaults to the current process working directory.
// cwd is always resolved to an absolute path to prevent hallucinated directory locations.
func DetectHostEnvironment(workDir string) HostEnvironment {
	cwd := workDir
	if cwd == "" {
		if d, err := os.Getwd(); err == nil {
			cwd = d
		}
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	} else {
		cwd = filepath.Clean(cwd)
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "powershell.exe"
		} else {
			shell = "/bin/bash"
		}
	}

	var files []string
	if entries, err := os.ReadDir(cwd); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") && name != ".github" {
				continue
			}
			if e.IsDir() {
				files = append(files, name+"/")
			} else {
				files = append(files, name)
			}
			if len(files) >= 50 {
				files = append(files, "...[truncated]")
				break
			}
		}
	}

	return HostEnvironment{
		Cwd:   cwd,
		OS:    runtime.GOOS,
		Shell: shell,
		Files: files,
	}
}

// FormatEnvironmentTag formats the host environment into the XML tag expected by the model.
func (env HostEnvironment) FormatEnvironmentTag() string {
	cwd := env.Cwd
	if cwd == "" {
		if d, err := os.Getwd(); err == nil {
			cwd = d
		}
	}
	if cwd == "" {
		cwd = "."
	}
	osName := env.OS
	if osName == "" {
		osName = runtime.GOOS
	}
	shell := env.Shell
	if shell == "" {
		if osName == "windows" {
			shell = "powershell.exe"
		} else {
			shell = "/bin/bash"
		}
	}
	var filesBlock string
	if len(env.Files) > 0 {
		filesBlock = fmt.Sprintf("\n<files>%s</files>", strings.Join(env.Files, ", "))
	}

	return fmt.Sprintf("<environment>\n<cwd>%s</cwd>\n<os>%s</os>\n<shell>%s</shell>%s\n</environment>", cwd, osName, shell, filesBlock)
}

// SystemPromptBase provides lean instructions tailored for 7B/3B models adhering to ADR-0024.
// Use BuildSystemPrompt or BuildSystemPromptWithEnv to produce the final prompt with host environment context.
var SystemPromptBase = catalog.DefaultRegistry.FormatBasePrompt("coding")

// SystemPrompt is the base invariant system prompt (identity, rules, and tool execution protocol).
// For host-environment grounding and codebase context, prefer BuildSystemPrompt or BuildSystemPromptWithEnv.
var SystemPrompt = "You are lokol, a local-first autonomous coding agent.\nSolve coding tasks by inspecting files, writing code, and testing.\n\n" + SystemPromptBase


// BuildSystemPromptWithEnv returns the full system prompt with host environment context injected.
func BuildSystemPromptWithEnv(env HostEnvironment) string {
	return fmt.Sprintf("You are lokol, a local-first autonomous coding agent.\nSolve coding tasks by inspecting files, writing code, and testing.\n\n%s\n\n%s",
		env.FormatEnvironmentTag(),
		SystemPromptBase,
	)
}

// BuildSystemPrompt constructs the system prompt enforcing the strict prompt hierarchy:
// Tier 1: Invariant System Prompt with host environment grounding (or base invariant if workDir is empty)
// Tier 2: Codebase / Refinery context (project guidelines, AGENTS.md, repo conventions)
//
// Arguments:
//   - BuildSystemPrompt() -> returns base SystemPrompt
//   - BuildSystemPrompt("") -> returns base SystemPrompt
//   - BuildSystemPrompt(codebaseContext) -> returns Tier 1 SystemPrompt + Tier 2 codebaseContext (when input is guidelines/XML)
//   - BuildSystemPrompt(workDir) -> returns Tier 1 SystemPrompt grounded with workDir host environment
//   - BuildSystemPrompt(workDir, codebaseContext) -> returns Tier 1 SystemPrompt grounded with workDir + Tier 2 codebaseContext
func BuildSystemPrompt(args ...string) string {
	if len(args) == 0 {
		return SystemPrompt
	}

	if len(args) == 1 {
		input := args[0]
		if input == "" {
			return SystemPrompt
		}
		// If input is codebase guidelines / XML context (e.g. contains newlines or XML tags)
		if strings.Contains(input, "<project_guidelines") || strings.Contains(input, "<codebase_context>") || strings.Contains(input, "\n") {
			input = strings.TrimSpace(input)
			return fmt.Sprintf("%s\n\n<codebase_context>\n%s\n</codebase_context>", SystemPrompt, input)
		}
		// Otherwise, input is treated as a working directory
		return BuildSystemPromptWithEnv(DetectHostEnvironment(input))
	}

	// Two or more arguments: args[0] = workDir, args[1] = codebaseContext
	workDir := args[0]
	codebaseContext := strings.TrimSpace(args[1])

	base := SystemPrompt
	if workDir != "" {
		base = BuildSystemPromptWithEnv(DetectHostEnvironment(workDir))
	}
	if codebaseContext == "" {
		return base
	}
	return fmt.Sprintf("%s\n\n<codebase_context>\n%s\n</codebase_context>", base, codebaseContext)
}

// BuildInitialHistory creates the initial message list strictly following the prompt hierarchy:
// (1) Invariant System Prompt + (2) Codebase / Refinery context as messages[0]
// (3) Fluid conversational history initial user turn as messages[1]
func BuildInitialHistory(codebaseContext, initialPrompt string) []Message {
	return []Message{
		{Role: "system", Content: BuildSystemPrompt(codebaseContext)},
		{Role: "user", Content: initialPrompt},
	}
}

// Client communicates with the local llama-server instance.
type Client struct {
	BaseURL     string
	HTTPClient  *http.Client
	cachedModel string
}

// NewClient creates a new client pointing to the inference engine.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// StreamChatRequest holds the payload for the llama-server OpenAI-compatible endpoint.
type StreamChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
	CachePrompt bool      `json:"cache_prompt"`
}

type ChatChunkResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// StreamResponse streams tokens from llama-server and sends chunks through tokenChan.
func (c *Client) StreamResponse(ctx context.Context, history []Message, tokenChan chan<- string) (string, error) {
	reqBody := StreamChatRequest{
		Model:       "local-model",
		Messages:    history,
		Stream:      true,
		Temperature: 0.1,
		MaxTokens:   4096,
		CachePrompt: true,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	doneChan := make(chan struct{})
	defer close(doneChan)

	// Monitor context cancellation to explicitly abort the active slot in llama-server
	go func() {
		select {
		case <-ctx.Done():
			abortCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = c.AbortActiveSlots(abortCtx)
		case <-doneChan:
		}
	}()

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to contact engine at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("engine returned status %d: %s", resp.StatusCode, string(body))
	}

	var fullContent strings.Builder
	reader := bufio.NewReader(resp.Body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fullContent.String(), err
		}

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "[DONE]" {
			break
		}

		var chunk ChatChunkResponse
		if err := json.Unmarshal([]byte(dataStr), &chunk); err == nil {
			if len(chunk.Choices) > 0 {
				token := chunk.Choices[0].Delta.Content
				if token != "" {
					fullContent.WriteString(token)
					tokenChan <- token
				}
			}
		}
	}

	return fullContent.String(), nil
}

// Action represents a parsed agent tool call.
type Action struct {
	Name         string
	Command      string
	CleanThought string // Prose explanation before the action tag
}

var actionRegex = regexp.MustCompile(`(?s)(?:` + "```" + `(?:xml)?\s*)?<action\s+name=["']?([a-zA-Z0-9_-]+)["']?\s*>(.*?)(?:</action>|` + "```" + `|$)`)

// ParseAction extracts <action name="...">...</action> or fallback JSON actions from agent text.
func ParseAction(text string) *Action {
	loc := actionRegex.FindStringSubmatchIndex(text)
	if loc != nil {
		thought := strings.TrimSpace(text[:loc[0]])
		thought = strings.TrimSuffix(thought, "```xml")
		thought = strings.TrimSuffix(thought, "```")
		thought = strings.TrimSpace(thought)

		name := text[loc[2]:loc[3]]
		command := strings.TrimSpace(text[loc[4]:loc[5]])
		command = strings.TrimSuffix(command, "```")
		command = strings.TrimSpace(command)

		return &Action{
			Name:         name,
			Command:      command,
			CleanThought: thought,
		}
	}

	// Fallback: Check if model generated JSON action like {"name": "...", ...}
	jsonIdx := strings.Index(text, "```json")
	var jsonBlock string
	var thought string
	if jsonIdx != -1 {
		thought = strings.TrimSpace(text[:jsonIdx])
		rest := text[jsonIdx+7:]
		endJson := strings.Index(rest, "```")
		if endJson != -1 {
			jsonBlock = strings.TrimSpace(rest[:endJson])
		} else {
			jsonBlock = strings.TrimSpace(rest)
		}
	} else if strings.HasPrefix(strings.TrimSpace(text), "{") {
		jsonBlock = strings.TrimSpace(text)
	}

	if jsonBlock != "" {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(jsonBlock), &raw); err == nil {
			if name, ok := raw["name"].(string); ok {
				// Reconstruct XML payload according to action type
				var cmd string
				switch name {
				case "read_outline":
					if p, ok := raw["path"].(string); ok {
						cmd = fmt.Sprintf("<path>%s</path>", p)
					}
				case "read_window":
					p, _ := raw["path"].(string)
					start, _ := raw["start"].(float64)
					end, _ := raw["end"].(float64)
					cmd = fmt.Sprintf("<path>%s</path>\n<start>%d</start>\n<end>%d</end>", p, int(start), int(end))
				case "replace_file":
					p, _ := raw["path"].(string)
					t, _ := raw["target"].(string)
					r, _ := raw["replacement"].(string)
					cmd = fmt.Sprintf("<path>%s</path>\n<target>%s</target>\n<replacement>%s</replacement>", p, t, r)
				case "write_file":
					p, _ := raw["path"].(string)
					c, _ := raw["content"].(string)
					cmd = fmt.Sprintf("<path>%s</path>\n<content>%s</content>", p, c)
				case "exec_bash", "run_test":
					if c, ok := raw["command"].(string); ok {
						cmd = c
					}
				case "task_finish":
					if s, ok := raw["summary"].(string); ok {
						cmd = s
					}
				}
				if cmd != "" {
					return &Action{
						Name:         name,
						Command:      cmd,
						CleanThought: thought,
					}
				}
			}
		}
	}

	return nil
}

// ExecuteBash runs a command locally and returns combined stdout/stderr.
func ExecuteBash(ctx context.Context, command string, workDir ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if len(workDir) > 0 && workDir[0] != "" {
		cwd, err := filepath.Abs(workDir[0])
		if err == nil {
			cmd.Dir = cwd
			// Sandbox execution: prevent git and beads from walking up to host parent directories
			// and polluting host repositories or beads memories.
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("BEADS_DIR=%s", filepath.Join(cwd, ".beads")),
				fmt.Sprintf("GIT_CEILING_DIRECTORIES=%s", filepath.Dir(cwd)),
			)
		} else {
			cmd.Dir = workDir[0]
		}
	}
	out, err := cmd.CombinedOutput()
	outputStr := string(out)
	if len(outputStr) > 4000 {
		outputStr = outputStr[:4000] + "\n...[output truncated by lokol for context hygiene]"
	}
	return outputStr, err
}

// SlotStatus represents the real-time context and processing metrics from llama-server /slots.
type SlotStatus struct {
	ID              int    `json:"id"`
	NCtx            int    `json:"n_ctx"`
	NPromptTokens   int    `json:"n_prompt_tokens"`
	IsProcessing    bool   `json:"is_processing"`
	NextTokenRemain int    `json:"-"`
	NDecoded        int    `json:"-"`
	ModelName       string `json:"-"`
}

// GetLoadedModel queries the engine's /v1/models endpoint to resolve the loaded model identifier.
func (c *Client) GetLoadedModel(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/v1/models", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("models endpoint returned %d", resp.StatusCode)
	}

	var res struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if len(res.Data) > 0 && res.Data[0].ID != "" {
		return res.Data[0].ID, nil
	}
	if len(res.Models) > 0 && res.Models[0].Name != "" {
		return res.Models[0].Name, nil
	}
	return "", fmt.Errorf("no models found")
}

// GetSlotStatus queries the llama-server /slots endpoint to get real-time context token usage.
func (c *Client) GetSlotStatus(ctx context.Context) (*SlotStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/slots", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("slots endpoint returned %d", resp.StatusCode)
	}

	var rawSlots []struct {
		ID            int  `json:"id"`
		NCtx          int  `json:"n_ctx"`
		NPromptTokens int  `json:"n_prompt_tokens"`
		IsProcessing  bool `json:"is_processing"`
		NextToken     []struct {
			NRemain  int `json:"n_remain"`
			NDecoded int `json:"n_decoded"`
		} `json:"next_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawSlots); err != nil {
		return nil, err
	}

	if len(rawSlots) == 0 {
		return nil, fmt.Errorf("no slots returned")
	}

	status := &SlotStatus{
		ID:            rawSlots[0].ID,
		NCtx:          rawSlots[0].NCtx,
		NPromptTokens: rawSlots[0].NPromptTokens,
		IsProcessing:  rawSlots[0].IsProcessing,
	}
	if len(rawSlots[0].NextToken) > 0 {
		status.NextTokenRemain = rawSlots[0].NextToken[0].NRemain
		status.NDecoded = rawSlots[0].NextToken[0].NDecoded
	}

	if c.cachedModel == "" {
		modelCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		if mName, err := c.GetLoadedModel(modelCtx); err == nil && mName != "" {
			c.cachedModel = mName
		}
		cancel()
	}
	status.ModelName = c.cachedModel

	return status, nil
}

// SlotInfo captures the high-level slot state from GET /slots.
type SlotInfo struct {
	ID            int  `json:"id"`
	NCtx          int  `json:"n_ctx"`
	NPromptTokens int  `json:"n_prompt_tokens"`
	IsProcessing  bool `json:"is_processing"`
}

// GetSlots queries GET /slots and returns status for all slots on the server.
func (c *Client) GetSlots(ctx context.Context) ([]SlotInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/slots", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("slots endpoint returned %d", resp.StatusCode)
	}

	var rawSlots []SlotInfo
	if err := json.NewDecoder(resp.Body).Decode(&rawSlots); err != nil {
		return nil, err
	}
	return rawSlots, nil
}

// AbortSlot sends an action=erase request to the specified slot in llama-server to release it.
// It also severs idle transport connections to ensure any hung socket read unblocks.
func (c *Client) AbortSlot(ctx context.Context, slotID int) error {
	if c.HTTPClient != nil {
		c.HTTPClient.CloseIdleConnections()
	}

	url := fmt.Sprintf("%s/slots/%d?action=erase", c.BaseURL, slotID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotImplemented {
		return nil
	}
	return fmt.Errorf("abort slot %d returned status %d", slotID, resp.StatusCode)
}

// AbortActiveSlots discovers all slots via GET /slots and aborts any that are active or processing.
// If GET /slots returns an error or empty list, it proactively aborts slot 0.
func (c *Client) AbortActiveSlots(ctx context.Context) error {
	if c.HTTPClient != nil {
		c.HTTPClient.CloseIdleConnections()
	}

	slots, err := c.GetSlots(ctx)
	if err == nil && len(slots) > 0 {
		var lastErr error
		for _, s := range slots {
			if s.IsProcessing || s.ID == 0 {
				if err := c.AbortSlot(ctx, s.ID); err != nil {
					lastErr = err
				}
			}
		}
		return lastErr
	}
	return c.AbortSlot(ctx, 0)
}

