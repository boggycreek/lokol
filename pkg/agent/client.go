package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Message represents a chat message in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SystemPrompt provides lean, deterministic instructions tailored for 7B/3B models.
const SystemPrompt = `You are quik, an ultra-fast local coding agent.
Solve coding tasks deterministically by inspecting files, writing code, and testing.

Available Action Formats:
1. To run a bash command (e.g. tests, builds, git, inspection):
<action name="exec_bash">
command here
</action>

2. To replace exact text in an existing file (PREFERRED for editing code):
<action name="replace_file">
<path>relative/path/to/file</path>
<target>
exact lines to replace
</target>
<replacement>
new replacement lines
</replacement>
</action>

3. To create or overwrite a whole file:
<action name="write_file">
<path>relative/path/to/file</path>
<content>
file content here
</content>
</action>

4. When your task is complete:
<action name="task_finish">
summary of completed task
</action>

Rules:
1. Always state your intent briefly before taking an action.
2. Only output ONE action per response.
3. For modifying code: ALWAYS prefer <action name="replace_file">. Never use git apply with fake line numbers.
4. Do NOT repeat a command or edit that has already succeeded. Check <action_result>.
5. Verify changes with commands (e.g. go test, go build).
6. When finished, call task_finish.`

// Client communicates with the local llama-server instance.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
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
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

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

// ParseAction extracts <action name="...">...</action> from agent text.
func ParseAction(text string) *Action {
	startTag := "<action name=\""
	idx := strings.Index(text, startTag)
	if idx == -1 {
		return nil
	}

	thought := strings.TrimSpace(text[:idx])
	rest := text[idx+len(startTag):]
	quoteIdx := strings.Index(rest, "\">")
	if quoteIdx == -1 {
		return nil
	}

	name := rest[:quoteIdx]
	payload := rest[quoteIdx+2:]

	endTag := "</action>"
	endIdx := strings.Index(payload, endTag)
	var command string
	if endIdx != -1 {
		command = strings.TrimSpace(payload[:endIdx])
	} else {
		// Stop token or generation end may have caught it without the literal closing tag
		command = strings.TrimSpace(payload)
	}

	return &Action{
		Name:         name,
		Command:      command,
		CleanThought: thought,
	}
}

// ExecuteBash runs a command locally and returns combined stdout/stderr.
func ExecuteBash(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	out, err := cmd.CombinedOutput()
	outputStr := string(out)
	if len(outputStr) > 4000 {
		outputStr = outputStr[:4000] + "\n...[output truncated by quik for context hygiene]"
	}
	return outputStr, err
}

// SlotStatus represents the real-time context and processing metrics from llama-server /slots.
type SlotStatus struct {
	ID              int  `json:"id"`
	NCtx            int  `json:"n_ctx"`
	NPromptTokens   int  `json:"n_prompt_tokens"`
	IsProcessing    bool `json:"is_processing"`
	NextTokenRemain int  `json:"-"`
	NDecoded        int  `json:"-"`
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

	return status, nil
}

