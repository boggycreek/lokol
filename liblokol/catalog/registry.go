// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package catalog

import (
	"fmt"
	"strings"
	"sync"
)

// Registry maintains an inventory of available agent tools, partitioned into
// foundational primitives and specialized tools per ADR-0024.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	order []string
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
		order: make([]string, 0),
	}
}

// Register registers a tool in the catalog.
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = tool
}

// Get returns the named tool if registered.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	return tool, exists
}

// List returns all registered tools in registration order.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		if t, ok := r.tools[name]; ok {
			result = append(result, t)
		}
	}
	return result
}

// ListFoundational returns all foundational tools that belong in the base prompt.
func (r *Registry) ListFoundational() []Tool {
	all := r.List()
	res := make([]Tool, 0, len(all))
	for _, t := range all {
		if t.IsFoundational() {
			res = append(res, t)
		}
	}
	return res
}

// ListSpecialized returns all specialized tools that are disclosed progressively.
func (r *Registry) ListSpecialized() []Tool {
	all := r.List()
	res := make([]Tool, 0, len(all))
	for _, t := range all {
		if !t.IsFoundational() {
			res = append(res, t)
		}
	}
	return res
}

// Help formats usage documentation for the requested tool or provides a catalog directory.
func (r *Registry) Help(toolName string) (string, error) {
	name := strings.TrimSpace(strings.ToLower(toolName))

	// If no tool name was specified or "all" was requested, list the specialized directory
	if name == "" || name == "all" || name == "list" {
		var sb strings.Builder
		sb.WriteString("<tool_directory>\nAvailable Specialized Tools (inspect with <action name=\"tool_help\"><tool>NAME</tool></action>):\n")
		for _, t := range r.ListSpecialized() {
			sb.WriteString(fmt.Sprintf("• %s: %s\n", t.Name(), t.Summary()))
		}
		sb.WriteString("</tool_directory>")
		return sb.String(), nil
	}

	tool, exists := r.Get(name)
	if !exists {
		// Provide helpful steering with valid tool names
		specialized := r.ListSpecialized()
		names := make([]string, 0, len(specialized))
		for _, t := range specialized {
			names = append(names, t.Name())
		}
		return "", fmt.Errorf("unknown tool %q. Available specialized tools: %s", toolName, strings.Join(names, ", "))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<tool_documentation name=%q>\n", tool.Name()))
	sb.WriteString(fmt.Sprintf("<summary>%s</summary>\n", tool.Summary()))
	if desc := tool.Description(); desc != "" {
		sb.WriteString(fmt.Sprintf("<description>%s</description>\n", desc))
	}
	sb.WriteString("<usage>\n")
	sb.WriteString(tool.SchemaXML())
	sb.WriteString("\n</usage>\n")
	if len(tool.Examples()) > 0 {
		sb.WriteString("<examples>\n")
		for _, ex := range tool.Examples() {
			sb.WriteString(ex)
			sb.WriteString("\n")
		}
		sb.WriteString("</examples>\n")
	}
	sb.WriteString("</tool_documentation>")

	return sb.String(), nil
}

// FormatBasePrompt generates an invariant lean (~250 tokens) base system prompt adhering to ADR-0024.
func (r *Registry) FormatBasePrompt(mode string) string {
	var sb strings.Builder

	modeDesc := "a versatile local-first assistant with workspace awareness, file search, document inspection, and artifact generation"
	switch strings.ToLower(mode) {
	case "coding", "code":
		modeDesc = "an autonomous coding assistant with direct workspace access, targeted code refactoring, and test verification"
	case "moe", "expert":
		modeDesc = "in Mixture-of-Experts (MoE) mode for analytical multi-perspective problem decomposition and domain expert routing"
	}

	sb.WriteString("Tool Execution Protocol:\n")
	sb.WriteString(fmt.Sprintf("- You are lokol, %s running directly on the operator's machine.\n", modeDesc))
	sb.WriteString("- Grounding: You have direct access and awareness of the local workspace provided in <environment>.\n")
	sb.WriteString("- Never output evasive responses claiming you cannot access files or the local directory.\n")
	sb.WriteString("- Execute one action per turn using the XML action formats below. Execution pauses after you output an action, and the result is returned in reciprocal <action_result>...</action_result> tags. The local engine executes your tool automatically; never thank or acknowledge the operator for tool results.\n\n")

	sb.WriteString("Foundational Actions:\n")
	foundational := r.ListFoundational()
	for i, t := range foundational {
		sb.WriteString(fmt.Sprintf("%d. %s:\n%s\n\n", i+1, t.Summary(), t.SchemaXML()))
	}

	sb.WriteString("Specialized Tools (inspect usage via <action name=\"tool_help\"><tool>NAME</tool></action>):\n")
	isCoding := strings.ToLower(mode) == "coding" || strings.ToLower(mode) == "code"
	for _, t := range r.ListSpecialized() {
		if !isCoding && (t.Name() == "run_test" || t.Name() == "read_outline") {
			continue
		}
		sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Name(), t.Summary()))
	}

	sb.WriteString("\nRules:\n")
	sb.WriteString("1. Ground answers in local context whenever discussing the current workspace.\n")
	sb.WriteString("2. Produce structured, concise, and insightful answers.\n")
	sb.WriteString("3. Only output an action when you intend to execute it immediately. Never include example action XML blocks in your conversational response to the operator; only output an action if you want the system to run it right now.\n")
	sb.WriteString("4. Bounded Inquiries: When exploring or answering questions about the repository, use find_files to discover actual files rather than guessing filenames. Limit file reading to 1 or 2 relevant files, synthesize your findings directly, and call task_finish to conclude your response.\n")
	sb.WriteString("5. Tool Results: Outputs inside <action_result> are returned by local host tools, not provided by the operator. CRITICAL: Never start with 'Thank you' or acknowledge receipt of tool results. Always begin directly with your factual findings or next action.")

	return sb.String()
}
