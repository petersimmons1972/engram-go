package entity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Extractor extracts entities and relations from text.
type Extractor interface {
	Extract(ctx context.Context, content string) ([]Entity, []Relation, error)
}

// ClaudeCompleter is the narrow interface satisfied by *claude.Client.
// Declared here so the entity package does not import the claude package directly,
// keeping the dependency direction clean.
type ClaudeCompleter interface {
	Complete(ctx context.Context, system, prompt, executorModel, advisorModel string, advisorMaxUses, maxTokens int) (string, error)
}

// ClaudeExtractor uses a Claude language model to extract entities and
// relations from freeform text. It truncates input to 4 000 characters before
// sending so that extraction prompts stay well within the token budget.
type ClaudeExtractor struct {
	client ClaudeCompleter
}

// NewClaudeExtractor returns a ClaudeExtractor backed by client.
func NewClaudeExtractor(client ClaudeCompleter) *ClaudeExtractor {
	return &ClaudeExtractor{client: client}
}

const maxContentChars = 4000

const extractionSystem = `You are an entity and relation extraction assistant.
Given a passage of text, identify the key named entities (people, organizations,
technologies, concepts, projects) and the relationships between them.

Return ONLY a JSON object — no prose, no markdown fences — in this exact schema:
{
  "entities": [
    {"name": "<canonical name>", "aliases": ["<alt name>", ...]}
  ],
  "relations": [
    {
      "source_name": "<entity name>",
      "target_name": "<entity name>",
      "rel_type": "relates_to|depends_on|caused_by|supersedes",
      "strength": <0.0–1.0>
    }
  ]
}

Rules:
- Use the most specific canonical name you can identify.
- Only include relations whose source and target appear in the entities list.
- If there is nothing to extract, return {"entities":[],"relations":[]}.`

// extractionResponse is the JSON structure returned by Claude.
type extractionResponse struct {
	Entities []struct {
		Name    string   `json:"name"`
		Aliases []string `json:"aliases"`
	} `json:"entities"`
	Relations []struct {
		SourceName string  `json:"source_name"`
		TargetName string  `json:"target_name"`
		RelType    string  `json:"rel_type"`
		Strength   float64 `json:"strength"`
	} `json:"relations"`
}

// Extract calls Claude and parses the JSON result into Entity and Relation slices.
// Content is silently truncated to 4 000 characters before sending.
func (e *ClaudeExtractor) Extract(ctx context.Context, content string) ([]Entity, []Relation, error) {
	if len([]rune(content)) > maxContentChars {
		content = string([]rune(content)[:maxContentChars])
	}

	prompt := "Extract entities and relations from the following text:\n\n" + content

	raw, err := e.client.Complete(ctx, extractionSystem, prompt,
		"claude-sonnet-4-6", "claude-opus-4-6", 0, 1024)
	if err != nil {
		return nil, nil, fmt.Errorf("entity extraction: claude call failed: %w", err)
	}

	// Claude sometimes wraps JSON in markdown code fences — strip them.
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		// Remove opening fence (```json or ```)
		if idx := strings.Index(raw, "\n"); idx != -1 {
			raw = raw[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(raw, "```"); idx != -1 {
			raw = raw[:idx]
		}
		raw = strings.TrimSpace(raw)
	}

	var resp extractionResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, nil, fmt.Errorf("entity extraction: failed to parse response JSON: %w", err)
	}

	entities := make([]Entity, 0, len(resp.Entities))
	for _, re := range resp.Entities {
		aliases := re.Aliases
		if aliases == nil {
			aliases = []string{}
		}
		entities = append(entities, Entity{
			Name:    re.Name,
			Aliases: aliases,
		})
	}

	relations := make([]Relation, 0, len(resp.Relations))
	for _, rr := range resp.Relations {
		relations = append(relations, Relation{
			SourceName: rr.SourceName,
			TargetName: rr.TargetName,
			RelType:    rr.RelType,
			Strength:   rr.Strength,
		})
	}

	return entities, relations, nil
}
