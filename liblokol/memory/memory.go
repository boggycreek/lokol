// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package memory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Pillar represents one of the five cognitive memory pillars.
type Pillar string

const (
	PillarEpisodic    Pillar = "episodic"    // Temporal autobiographical progression with daily/weekly roll-ups
	PillarSemantic    Pillar = "semantic"    // Time-invariant domain facts, architectures, and specs
	PillarProcedural  Pillar = "procedural"  // Actionable 'how-to' runbooks and execution playbooks
	PillarProspective Pillar = "prospective" // Condition-action triggers and deferred intentions
	PillarEvaluative  Pillar = "evaluative"  // Normative guardrails, anti-patterns, and user preferences
)

// ValidPillars lists all authorized cognitive pillars.
var ValidPillars = []Pillar{
	PillarEpisodic,
	PillarSemantic,
	PillarProcedural,
	PillarProspective,
	PillarEvaluative,
}

// IsValidPillar checks if a pillar name is recognized.
func IsValidPillar(p string) bool {
	switch Pillar(strings.ToLower(strings.TrimSpace(p))) {
	case PillarEpisodic, PillarSemantic, PillarProcedural, PillarProspective, PillarEvaluative:
		return true
	default:
		return false
	}
}

// MemoryRecord represents a persistent memory item structured for 3-tier progressive disclosure.
type MemoryRecord struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Category  Pillar         `json:"category"`
	Domain    string         `json:"domain,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	Abstract  string         `json:"abstract"`           // Tier 1: 1-sentence abstract (~20 tokens)
	Summary   string         `json:"summary"`            // Tier 2: Executive summary & actionable rules (~100-200 tokens)
	Details   string         `json:"details,omitempty"`   // Tier 3: Full technical background, code samples
	ExtraData map[string]any `json:"extra_data,omitempty"`// Companion .details.json payload
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Path      string         `json:"path,omitempty"`
}

// EstimateTokens calculates an approximate token count using the ~4 chars/token heuristic.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	tokens := len(text) / 4
	if tokens == 0 {
		return 1
	}
	return tokens
}

// FormatTier1 formats the memory as a Tier 1 signpost (~20 tokens).
// Ideal for index listings, scan results, and catalog discovery.
func (m *MemoryRecord) FormatTier1() string {
	tagsStr := ""
	if len(m.Tags) > 0 {
		tagsStr = fmt.Sprintf(" [%s]", strings.Join(m.Tags, ", "))
	}
	domainStr := ""
	if m.Domain != "" {
		domainStr = fmt.Sprintf(" (%s)", m.Domain)
	}
	return fmt.Sprintf("• [%s] %s%s: %s%s", m.ID, m.Title, domainStr, m.Abstract, tagsStr)
}

// FormatTier2 formats the memory as a Tier 2 concise digest (~100-200 tokens).
// Default return for recall_memory and standard conversational context retrieval.
func (m *MemoryRecord) FormatTier2() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("=== Memory: %s [%s] ===\n", m.Title, m.ID))
	b.WriteString(fmt.Sprintf("Category: %s", m.Category))
	if m.Domain != "" {
		b.WriteString(fmt.Sprintf(" | Domain: %s", m.Domain))
	}
	if len(m.Tags) > 0 {
		b.WriteString(fmt.Sprintf(" | Tags: %s", strings.Join(m.Tags, ", ")))
	}
	b.WriteString(fmt.Sprintf("\nAbstract: %s\n\n", m.Abstract))
	b.WriteString(m.Summary)
	return strings.TrimSpace(b.String())
}

// FormatTier3 formats the full memory disclosure (Tier 3), including technical details
// and companion JSON metadata.
func (m *MemoryRecord) FormatTier3() string {
	var b strings.Builder
	b.WriteString(m.FormatTier2())
	b.WriteString("\n\n")

	if m.Details != "" {
		b.WriteString("--- Technical Background & Appendix ---\n")
		b.WriteString(m.Details)
		b.WriteString("\n\n")
	}

	if len(m.ExtraData) > 0 {
		b.WriteString("--- Companion Data (.details.json) ---\n")
		jsonData, err := json.MarshalIndent(m.ExtraData, "", "  ")
		if err == nil {
			b.WriteString(string(jsonData))
			b.WriteString("\n")
		}
	}

	return strings.TrimSpace(b.String())
}

// ToMarkdown serializes the memory record into Markdown with YAML frontmatter.
func (m *MemoryRecord) ToMarkdown() string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("id: %s\n", m.ID))
	b.WriteString(fmt.Sprintf("title: %q\n", m.Title))
	b.WriteString(fmt.Sprintf("category: %s\n", m.Category))
	if m.Domain != "" {
		b.WriteString(fmt.Sprintf("domain: %s\n", m.Domain))
	}
	if len(m.Tags) > 0 {
		b.WriteString("tags:\n")
		for _, tag := range m.Tags {
			b.WriteString(fmt.Sprintf("  - %s\n", tag))
		}
	}
	b.WriteString(fmt.Sprintf("abstract: %q\n", m.Abstract))
	b.WriteString(fmt.Sprintf("created_at: %s\n", m.CreatedAt.UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("updated_at: %s\n", m.UpdatedAt.UTC().Format(time.RFC3339)))
	b.WriteString("---\n\n")

	b.WriteString(fmt.Sprintf("# %s\n\n", m.Title))
	b.WriteString("## Summary\n\n")
	b.WriteString(strings.TrimSpace(m.Summary))
	b.WriteString("\n\n")

	if m.Details != "" {
		b.WriteString("## Details\n\n")
		b.WriteString(strings.TrimSpace(m.Details))
		b.WriteString("\n")
	}

	return b.String()
}

// ParseMarkdown parses a Markdown document with YAML front matter into a MemoryRecord.
func ParseMarkdown(content string) (*MemoryRecord, error) {
	rec := &MemoryRecord{
		Tags: make([]string, 0),
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return rec, nil
	}

	bodyStartIdx := 0
	if strings.TrimSpace(lines[0]) == "---" {
		fmEndIdx := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				fmEndIdx = i
				break
			}
		}

		if fmEndIdx > 0 {
			parseMemoryFrontMatter(lines[1:fmEndIdx], rec)
			bodyStartIdx = fmEndIdx + 1
		}
	}

	parseMemoryBody(lines[bodyStartIdx:], rec)
	return rec, nil
}

func parseMemoryFrontMatter(lines []string, rec *MemoryRecord) {
	inTags := false

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "- ") {
			if inTags {
				tag := strings.Trim(strings.TrimSpace(line[2:]), `"'`)
				rec.Tags = append(rec.Tags, tag)
			}
			continue
		}

		inTags = false
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(line[:colonIdx]))
		val := strings.TrimSpace(line[colonIdx+1:])
		val = strings.Trim(val, `"'`)

		switch key {
		case "id":
			rec.ID = val
		case "title":
			rec.Title = val
		case "category":
			rec.Category = Pillar(strings.ToLower(val))
		case "domain":
			rec.Domain = val
		case "abstract":
			rec.Abstract = val
		case "created_at":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				rec.CreatedAt = t
			}
		case "updated_at":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				rec.UpdatedAt = t
			}
		case "tags":
			if val != "" {
				val = strings.Trim(val, "[]")
				parts := strings.Split(val, ",")
				for _, p := range parts {
					clean := strings.Trim(strings.TrimSpace(p), `"'`)
					if clean != "" {
						rec.Tags = append(rec.Tags, clean)
					}
				}
			} else {
				inTags = true
			}
		}
	}
}

func parseMemoryBody(lines []string, rec *MemoryRecord) {
	currentSection := ""
	var summaryBuilder strings.Builder
	var detailsBuilder strings.Builder

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)

		if strings.HasPrefix(line, "# ") && rec.Title == "" {
			rec.Title = strings.TrimSpace(line[2:])
			continue
		}

		if strings.HasPrefix(line, "## ") {
			heading := strings.ToLower(strings.TrimSpace(line[3:]))
			switch {
			case strings.Contains(heading, "summary") || strings.Contains(heading, "digest"):
				currentSection = "summary"
			case strings.Contains(heading, "detail") || strings.Contains(heading, "appendix") || strings.Contains(heading, "background"):
				currentSection = "details"
			default:
				currentSection = ""
			}
			continue
		}

		switch currentSection {
		case "summary":
			summaryBuilder.WriteString(rawLine + "\n")
		case "details":
			detailsBuilder.WriteString(rawLine + "\n")
		default:
			if rec.Summary == "" && line != "" && !strings.HasPrefix(line, "#") {
				summaryBuilder.WriteString(rawLine + "\n")
			}
		}
	}

	if rec.Summary == "" {
		rec.Summary = strings.TrimSpace(summaryBuilder.String())
	}
	if rec.Details == "" {
		rec.Details = strings.TrimSpace(detailsBuilder.String())
	}
}

// GenerateID produces a deterministic, human-readable slugged ID for a new memory.
func GenerateID(pillar Pillar, title string) string {
	ts := time.Now().UTC().Format("20060102")
	cleanTitle := strings.ToLower(strings.TrimSpace(title))
	var slug strings.Builder
	dash := false

	for _, r := range cleanTitle {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			slug.WriteRune(r)
			dash = true
		} else if dash {
			slug.WriteRune('-')
			dash = false
		}
		if slug.Len() >= 25 {
			break
		}
	}

	slugStr := strings.Trim(slug.String(), "-")
	if slugStr == "" {
		slugStr = "note"
	}

	return fmt.Sprintf("mem-%s-%s-%s", string(pillar)[:4], ts, slugStr)
}
