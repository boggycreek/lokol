// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package catalog

import (
	"context"
)

// Tool defines the contract for an agent capability adhering to ADR-0024.
type Tool interface {
	// Name returns the unique action identifier matching XML <action name="...">.
	Name() string

	// Summary returns a concise one-line description of what the tool does.
	Summary() string

	// Description returns a detailed explanation of parameters and operational constraints.
	Description() string

	// SchemaXML returns the canonical XML action format template.
	SchemaXML() string

	// Examples returns realistic usage examples.
	Examples() []string

	// Tags returns keyword tags for intent classification (e.g. "git", "test", "search").
	Tags() []string

	// IsFoundational returns true if this tool belongs in the invariant base system prompt.
	IsFoundational() bool

	// Execute runs the tool against the workspace with the provided XML payload.
	Execute(ctx context.Context, payload string, workDir ...string) (string, error)
}

// SimpleTool is a standard implementation of the Tool interface.
type SimpleTool struct {
	NameVal         string
	SummaryVal      string
	DescriptionVal  string
	SchemaXMLVal    string
	ExamplesVal     []string
	TagsVal         []string
	FoundationalVal bool
	ExecuteFn       func(ctx context.Context, payload string, workDir ...string) (string, error)
}

func (t *SimpleTool) Name() string           { return t.NameVal }
func (t *SimpleTool) Summary() string        { return t.SummaryVal }
func (t *SimpleTool) Description() string    { return t.DescriptionVal }
func (t *SimpleTool) SchemaXML() string      { return t.SchemaXMLVal }
func (t *SimpleTool) Examples() []string     { return t.ExamplesVal }
func (t *SimpleTool) Tags() []string         { return t.TagsVal }
func (t *SimpleTool) IsFoundational() bool   { return t.FoundationalVal }

func (t *SimpleTool) Execute(ctx context.Context, payload string, workDir ...string) (string, error) {
	if t.ExecuteFn != nil {
		return t.ExecuteFn(ctx, payload, workDir...)
	}
	return payload, nil
}
