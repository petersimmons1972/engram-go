package atom

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ClaudeCompleter is the narrow interface satisfied by *claude.Client.
// Declared here so the atom package does not import the claude package directly,
// keeping the dependency direction clean. Mirrors the identical interface in
// internal/entity/extract.go.
type ClaudeCompleter interface {
	Complete(ctx context.Context, system, prompt, executorModel, advisorModel string, advisorMaxUses, maxTokens int) (string, error)
}

// Extractor extracts atoms from session text.
type Extractor interface {
	Extract(ctx context.Context, sessionText string) ([]Atom, error)
}

// ClaudeExtractor uses a Claude language model to extract typed atoms from
// freeform session text. It is preference-focused (Milestone 1): the prompt
// emphasises casual-language preferences that keyword-based extractors miss.
type ClaudeExtractor struct {
	client ClaudeCompleter
}

// NewClaudeExtractor returns a ClaudeExtractor backed by client.
func NewClaudeExtractor(client ClaudeCompleter) *ClaudeExtractor {
	return &ClaudeExtractor{client: client}
}

// maxSessionChars truncates session text before sending to the model.
// Keeps atom prompts within the token budget; mirrors entity.maxContentChars.
const maxSessionChars = 6000

// extractionSystem is the system prompt for preference-focused atom extraction.
// Milestone 1 targets casual-language preferences — statements like "I usually
// prefer X", "I don't really like Y", "I tend to go with Z" — which PatternPreferenceExtractor
// misses because they don't match keyword anchors.
const extractionSystem = `You are a precise preference and fact extraction assistant.
Given a passage of conversational text, identify typed atoms — minimal, self-contained
beliefs or preferences stated by the user.

Focus especially on PREFERENCES expressed in casual language:
  "I usually prefer...", "I don't really like...", "I tend to go with...",
  "I'm not a fan of...", "my favourite is...", "I'd rather have...",
  "I always choose...", "I like X better than Y", "I hate X", "I love X"

These casual phrasings are easy to miss — capture them even when they are
stated indirectly or embedded in a longer sentence.

Return ONLY a JSON array of atom objects — no prose, no markdown fences — in this exact schema:
[
  {
    "atom_type": "<preference|fact|event|attribute|relationship>",
    "subject":   "<who or what the atom is about>",
    "predicate": "<the property or relationship>",
    "value":     "<the stated value, choice, or belief>",
    "statement": "<canonical NL sentence, e.g. 'Alice prefers dark chocolate over milk chocolate.'>",
    "scope":     "<global | session:<id> | entity:<id>>",
    "confidence": <0.0–1.0>,
    "source_span": "<optional verbatim quote or char range>"
  }
]

Rules:
- atom_type must be exactly one of: preference, fact, event, attribute, relationship.
- subject should be the first-person actor ("the user") or a named entity.
- Normalise subject to "the user" for first-person statements.
- statement must be a complete, standalone sentence (no pronouns requiring external context).
- confidence should reflect how explicit the preference is (explicit "I love X" → 0.9+; hedged "maybe X" → 0.5).
- scope: use "global" unless there is a clear session or entity anchor.
- If nothing can be extracted, return an empty array: [].
- Do NOT invent atoms that are not supported by the text.`

// atomResponse is the per-atom JSON object returned by Claude.
type atomResponse struct {
	Type        string  `json:"atom_type"`
	Subject     string  `json:"subject"`
	Predicate   string  `json:"predicate"`
	Value       string  `json:"value"`
	Statement   string  `json:"statement"`
	Scope       string  `json:"scope"`
	Confidence  float64 `json:"confidence"`
	SourceSpan  string  `json:"source_span"`
}

// Extract calls Claude and parses the JSON result into Atom slices.
// Session text is silently truncated to maxSessionChars before sending.
// No real LLM call is made in tests — inject a mock via ClaudeCompleter.
func (e *ClaudeExtractor) Extract(ctx context.Context, sessionText string) ([]Atom, error) {
	if len([]rune(sessionText)) > maxSessionChars {
		sessionText = string([]rune(sessionText)[:maxSessionChars])
	}

	prompt := "Extract typed atoms (focus on preferences) from the following session text:\n\n" + sessionText

	raw, err := e.client.Complete(ctx, extractionSystem, prompt,
		"claude-sonnet-4-6", "claude-opus-4-6", 0, 2048)
	if err != nil {
		return nil, fmt.Errorf("atom extraction: claude call failed: %w", err)
	}

	raw = strings.TrimSpace(raw)
	// Claude sometimes wraps JSON in markdown code fences — strip them.
	if strings.HasPrefix(raw, "```") {
		if idx := strings.Index(raw, "\n"); idx != -1 {
			raw = raw[idx+1:]
		}
		if idx := strings.LastIndex(raw, "```"); idx != -1 {
			raw = raw[:idx]
		}
		raw = strings.TrimSpace(raw)
	}

	var resp []atomResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("atom extraction: failed to parse response JSON: %w", err)
	}

	atoms := make([]Atom, 0, len(resp))
	for _, r := range resp {
		a := Atom{
			Type:           r.Type,
			Subject:        r.Subject,
			Predicate:      r.Predicate,
			Value:          r.Value,
			Statement:      r.Statement,
			Scope:          r.Scope,
			Confidence:     r.Confidence,
			ProvenanceSpan: r.SourceSpan,
		}
		if a.Scope == "" {
			a.Scope = ScopeGlobal
		}
		if a.Confidence == 0 {
			a.Confidence = 1.0
		}
		if !a.IsValid() {
			continue // skip malformed atoms silently
		}
		atoms = append(atoms, a)
	}
	return atoms, nil
}

// ExtractionPrompt returns the system prompt used for atom extraction.
// Exported so callers (e.g. run.go generation) can inspect it without
// re-constructing a ClaudeExtractor.
func ExtractionPrompt() string {
	return extractionSystem
}
