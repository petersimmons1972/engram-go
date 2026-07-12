package atom

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/metrics"
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
	Extract(ctx context.Context, sessionText string, sessionDate ...time.Time) ([]Atom, error)
}

// ClaudeExtractor uses a Claude language model to extract typed atoms from
// freeform session text. It focuses on preference/profile/status-state signals
// expressed in casual language that keyword-based extractors miss.
type ClaudeExtractor struct {
	client ClaudeCompleter
}

// NewClaudeExtractor returns a ClaudeExtractor backed by client.
func NewClaudeExtractor(client ClaudeCompleter) *ClaudeExtractor {
	return &ClaudeExtractor{client: client}
}

// maxSessionChars is the maximum size of each model extraction window.
const maxSessionChars = 6000

// extractionSystem is the system prompt for preference-focused atom extraction.
// Milestone 1 targets casual-language preferences — statements like "I usually
// prefer X", "I don't really like Y", "I tend to go with Z" — which PatternPreferenceExtractor
// misses because they don't match keyword anchors.
const extractionSystem = `You are a precise preference and state extraction assistant.
Given a passage of conversational text, identify typed atoms — minimal, self-contained
beliefs or preferences stated by the user.

Focus especially on PREFERENCES expressed in casual language:
  "I usually prefer...", "I don't really like...", "I tend to go with...",
  "I'm not a fan of...", "my favourite is...", "I'd rather have...",
  "I always choose...", "I like X better than Y", "I hate X", "I love X"

These casual phrasings are easy to miss — capture them even when they are
stated indirectly or embedded in a longer sentence.

Also capture stable PROFILE facts and STATUS CHANGES when they are stated plainly:
  "I'm vegetarian", "I work nights", "I'm based in Seattle",
  "I started a new job", "I moved to Boston", "I switched to Android"

Return ONLY a JSON array of atom objects — no prose, no markdown fences — in this exact schema:
[
  {
    "atom_type": "<preference|profile|status_change|fact|event|attribute|relationship>",
    "subject":   "<who or what the atom is about>",
    "predicate": "<the property or relationship>",
    "value":     "<the stated value, choice, or belief>",
    "statement": "<canonical NL sentence, e.g. 'Alice prefers dark chocolate over milk chocolate.'>",
    "scope":     "<global | session:<id> | entity:<id>>",
    "confidence": <0.0–1.0>,
    "source_span": "<optional verbatim quote or char range>",
    "event_date": "<optional event date, ISO YYYY-MM-DD>"
  }
]

Rules:
- atom_type must be exactly one of: preference, profile, status_change, fact, event, attribute, relationship.
- use profile for relatively stable user descriptors (diet, location, profession, timezone, routine).
- use status_change for explicit updates or transitions in user state ("started", "moved", "switched", "stopped").
- Attribute facts to the actual speaker or subject. Facts about roleplay personas, characters the user asks you to play,
  personas assigned to the assistant via role prompts ("You are <name/role>...", "staying in character"),
  article or story subjects, or hypothetical people are NOT user facts — skip them rather than assigning them to "the user".
- Preserve tense and quantifiers: one-off events and plans stay episodic; do not generalise them into routines,
  preferences, or profile facts; only repeated or explicitly stated habits become standing preferences or routines.
- When a one-off user event or plan is useful, use atom_type event and retain its time anchor in the statement
  (for example, "On <date>, the user did X once") and session scope when available. If its timing or subject is
  ambiguous, skip it. Conservative extraction is better than an unsupported habitual claim.
- For event and status_change atoms, set event_date to YYYY-MM-DD only when the source states or clearly implies
  the event's own date. Keep that same date anchor in statement. Omit event_date rather than guessing.
- subject should be the first-person actor ("the user") or a named entity.
- Normalise subject to "the user" for first-person statements.
- statement must be a complete, standalone sentence (no pronouns requiring external context).
- confidence should reflect how explicit the preference is (explicit "I love X" → 0.9+; hedged "maybe X" → 0.5).
- scope: use "global" unless there is a clear session or entity anchor.
- If nothing can be extracted, return an empty array: [].
- Do NOT invent atoms that are not supported by the text.`

// atomResponse is the per-atom JSON object returned by Claude.
type atomResponse struct {
	Type       string  `json:"atom_type"`
	Subject    string  `json:"subject"`
	Predicate  string  `json:"predicate"`
	Value      string  `json:"value"`
	Statement  string  `json:"statement"`
	Scope      string  `json:"scope"`
	Confidence float64 `json:"confidence"`
	SourceSpan string  `json:"source_span"`
	EventDate  string  `json:"event_date"`
}

// Extract calls Claude once per message-aligned window and unions the results.
// sessionDate is the authoritative assertion time supplied by the ingest path.
// No real LLM call is made in tests — inject a mock via ClaudeCompleter.
func (e *ClaudeExtractor) Extract(ctx context.Context, sessionText string, sessionDates ...time.Time) ([]Atom, error) {
	var sessionDate time.Time
	if len(sessionDates) > 0 {
		sessionDate = sessionDates[0].UTC()
	}

	windows := sessionWindows(sessionText, maxSessionChars)
	atoms := make([]Atom, 0)
	seen := make(map[exactAtomKey]struct{})
	for i, window := range windows {
		windowAtoms, err := e.extractWindow(ctx, window, sessionDate)
		if err != nil {
			return nil, fmt.Errorf("atom extraction: window %d of %d: %w", i+1, len(windows), err)
		}
		for _, candidate := range windowAtoms {
			key := exactKey(candidate)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			atoms = append(atoms, candidate)
		}
	}
	return atoms, nil
}

func (e *ClaudeExtractor) extractWindow(ctx context.Context, sessionText string, sessionDate time.Time) ([]Atom, error) {
	prompt := "Extract typed atoms (focus on preferences, profile facts, and status changes) from the following session text:\n\n" + sessionText

	raw, err := e.client.Complete(ctx, extractionSystem, prompt,
		"claude-sonnet-4-6", "claude-opus-4-6", 0, 2048)
	if err != nil {
		return nil, fmt.Errorf("atom extraction: claude call failed: %w", err)
	}

	raw, err = extractAtomJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("atom extraction: failed to parse response JSON: %w", err)
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
		applyEventTime(&a, r.EventDate, sessionDate)
		atoms = append(atoms, a)
	}
	return atoms, nil
}

type exactAtomKey struct {
	typeName  string
	subject   string
	predicate string
	value     string
	statement string
}

func exactKey(a Atom) exactAtomKey {
	return exactAtomKey{
		typeName:  a.Type,
		subject:   a.Subject,
		predicate: a.Predicate,
		value:     a.Value,
		statement: a.Statement,
	}
}

func sessionWindows(text string, maxRunes int) []string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}

	windows := make([]string, 0, (len(runes)+maxRunes-1)/maxRunes)
	for len(runes) > maxRunes {
		end := maxRunes
		if boundaryEnd := lastMessageBoundary(runes[:maxRunes]); boundaryEnd >= maxRunes/2 {
			end = boundaryEnd
		}
		windows = append(windows, string(runes[:end]))
		runes = runes[end:]
	}
	windows = append(windows, string(runes))
	return windows
}

func lastMessageBoundary(runes []rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] != '\n' || i+1 == len(runes) {
			continue
		}
		nextLine := strings.ToLower(string(runes[i+1:]))
		for _, prefix := range []string{
			"user:", "human:", "assistant:", "system:", "tool:",
			"[user]", "[human]", "[assistant]", "[system]", "[tool]",
		} {
			if strings.HasPrefix(nextLine, prefix) {
				return i + 1
			}
		}
	}
	return 0
}

func applyEventTime(a *Atom, rawEventDate string, sessionDate time.Time) {
	if !sessionDate.IsZero() {
		observedAt := sessionDate
		a.ObservedAt = &observedAt
	}
	if a.Type != TypeEvent && a.Type != TypeStatusChange {
		return
	}
	if rawEventDate == "" {
		if !sessionDate.IsZero() {
			validFrom := dateOnly(sessionDate)
			a.ValidFrom = &validFrom
		}
		return
	}

	eventDate, reason := parseEventDate(rawEventDate, sessionDate)
	if reason != "" {
		metrics.AtomEventDateRejections.WithLabelValues(reason).Inc()
		slog.Warn("atom extraction: rejected event_date", "event_date", rawEventDate, "reason", reason)
		return
	}
	a.ValidFrom = &eventDate
}

func parseEventDate(raw string, observedAt time.Time) (time.Time, string) {
	if len(raw) != len(time.DateOnly) || (raw[4] != '-' && raw[4] != '/') || raw[7] != raw[4] {
		return time.Time{}, "invalid_format"
	}
	normalized := strings.ReplaceAll(raw, "/", "-")
	parsed, err := time.Parse(time.DateOnly, normalized)
	if err != nil {
		return time.Time{}, "invalid_format"
	}
	parsed = dateOnly(parsed)
	if parsed.Before(time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		return time.Time{}, "before_1990"
	}
	if !observedAt.IsZero() && parsed.After(observedAt.AddDate(1, 0, 0)) {
		return time.Time{}, "more_than_one_year_future"
	}
	return parsed, ""
}

func dateOnly(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func extractAtomJSON(raw string) (string, error) {
	raw = strings.TrimSpace(stripMarkdownFences(stripThinkBlocks(raw)))
	if raw == "" {
		return "", fmt.Errorf("no JSON object or array found in response")
	}
	if json.Valid([]byte(raw)) {
		return raw, nil
	}
	if jsonText, ok := firstBalancedJSON(raw); ok {
		return jsonText, nil
	}
	if strings.HasPrefix(raw, "[") || strings.HasPrefix(raw, "{") {
		return raw, nil
	}
	return "", fmt.Errorf("no JSON object or array found in response")
}

func stripThinkBlocks(raw string) string {
	const (
		openTag  = "<think>"
		closeTag = "</think>"
	)
	for {
		lower := strings.ToLower(raw)
		start := strings.Index(lower, openTag)
		if start == -1 {
			return raw
		}
		closeStart := strings.Index(lower[start+len(openTag):], closeTag)
		if closeStart == -1 {
			return raw
		}
		end := start + len(openTag) + closeStart + len(closeTag)
		raw = raw[:start] + raw[end:]
	}
}

func stripMarkdownFences(raw string) string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func firstBalancedJSON(raw string) (string, bool) {
	for start := 0; start < len(raw); start++ {
		if raw[start] != '[' && raw[start] != '{' {
			continue
		}
		if jsonText, ok := balancedJSONFrom(raw, start); ok && json.Valid([]byte(jsonText)) {
			return jsonText, true
		}
	}
	return "", false
}

func balancedJSONFrom(raw string, start int) (string, bool) {
	stack := make([]byte, 0, 4)
	inString := false
	escaped := false

	for i := start; i < len(raw); i++ {
		c := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '[':
			stack = append(stack, ']')
		case '{':
			stack = append(stack, '}')
		case ']', '}':
			if len(stack) == 0 || c != stack[len(stack)-1] {
				return "", false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return strings.TrimSpace(raw[start : i+1]), true
			}
		}
	}
	return "", false
}

// ExtractionPrompt returns the system prompt used for atom extraction.
// Exported so callers (e.g. run.go generation) can inspect it without
// re-constructing a ClaudeExtractor.
func ExtractionPrompt() string {
	return extractionSystem
}
