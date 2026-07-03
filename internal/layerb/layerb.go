package layerb

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/aggq"
	"github.com/petersimmons1972/engram/internal/types"
)

// Atom is one deterministic evidence span extracted from a recalled memory.
type Atom struct {
	Project        string     `json:"project"`
	MemoryID       string     `json:"memory_id"`
	ProvenanceSpan string     `json:"provenance_span"`
	SpanText       string     `json:"span_text"`
	Statement      string     `json:"statement"`
	NormalizedText string     `json:"normalized_text"`
	EventTime      *time.Time `json:"event_time,omitempty"`
}

// Event is one deterministic aggregation event derived from an Atom.
type Event struct {
	Project        string     `json:"project"`
	MemoryID       string     `json:"memory_id"`
	ProvenanceSpan string     `json:"provenance_span"`
	SpanText       string     `json:"span_text"`
	Anchor         string     `json:"anchor"`
	NormalizedText string     `json:"normalized_text"`
	EventTime      *time.Time `json:"event_time,omitempty"`
}

// EventRecord is the stored event shape returned by the persistence layer.
type EventRecord struct {
	MemoryID       string     `json:"memory_id"`
	ProvenanceSpan string     `json:"provenance_span"`
	SpanText       string     `json:"span_text"`
	Anchor         string     `json:"anchor"`
	NormalizedText string     `json:"normalized_text"`
	EventTime      *time.Time `json:"event_time,omitempty"`
}

// Summary is the additive Layer B payload returned to callers.
type Summary struct {
	Mode     string        `json:"mode"`
	Anchor   string        `json:"anchor"`
	Count    int           `json:"count"`
	Evidence []EventRecord `json:"evidence"`
}

// Store is the narrow persistence seam for Layer B.
type Store interface {
	UpsertLayerBAtom(ctx context.Context, atom Atom) error
	UpsertLayerBEvent(ctx context.Context, event Event) error
	ListLayerBEvents(ctx context.Context, project string, memoryIDs []string) ([]EventRecord, error)
}

var nonWordRE = regexp.MustCompile(`[^a-z0-9]+`)
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "i": true, "me": true, "my": true,
	"did": true, "do": true, "does": true, "have": true, "had": true,
	"how": true, "many": true, "much": true, "times": true, "time": true,
	"what": true, "is": true, "was": true, "were": true, "of": true,
	"to": true, "on": true, "in": true, "for": true, "and": true,
}

// BuildSummary deterministically indexes matching evidence spans from recalled
// memories and returns a count-style aggregation payload when the query is
// aggregation-shaped. Non-aggregation queries return (nil, nil).
func BuildSummary(ctx context.Context, store Store, query string, results []types.SearchResult) (*Summary, error) {
	if !aggq.IsAggregationQuestion(query) {
		return nil, nil
	}

	anchor := strings.TrimSpace(aggq.ExtractAggregationAnchor(query))
	anchorTerms := normalizeTerms(anchor)
	if len(anchorTerms) == 0 {
		return nil, nil
	}

	project := ""
	memoryIDs := make([]string, 0, len(results))
	for _, result := range results {
		if result.Memory == nil || strings.TrimSpace(result.Memory.Content) == "" {
			continue
		}
		if project == "" {
			project = result.Memory.Project
		}
		memoryIDs = append(memoryIDs, result.Memory.ID)
		matches := extractMatchingAtoms(result.Memory, anchorTerms)
		for _, atom := range matches {
			if err := store.UpsertLayerBAtom(ctx, atom); err != nil {
				return nil, fmt.Errorf("layerb upsert atom: %w", err)
			}
			event := Event{
				Project:        atom.Project,
				MemoryID:       atom.MemoryID,
				ProvenanceSpan: atom.ProvenanceSpan,
				SpanText:       atom.SpanText,
				Anchor:         strings.Join(anchorTerms, " "),
				NormalizedText: atom.NormalizedText,
				EventTime:      atom.EventTime,
			}
			if err := store.UpsertLayerBEvent(ctx, event); err != nil {
				return nil, fmt.Errorf("layerb upsert event: %w", err)
			}
		}
	}

	if len(memoryIDs) == 0 || project == "" {
		return nil, nil
	}

	records, err := store.ListLayerBEvents(ctx, project, dedupe(memoryIDs))
	if err != nil {
		return nil, fmt.Errorf("layerb list events: %w", err)
	}

	wantAnchor := strings.Join(anchorTerms, " ")
	evidence := make([]EventRecord, 0, len(records))
	for _, record := range records {
		if record.Anchor != wantAnchor {
			continue
		}
		evidence = append(evidence, record)
	}
	if len(evidence) == 0 {
		return nil, nil
	}

	slices.SortStableFunc(evidence, func(a, b EventRecord) int {
		if a.EventTime == nil && b.EventTime == nil {
			return strings.Compare(a.ProvenanceSpan, b.ProvenanceSpan)
		}
		if a.EventTime == nil {
			return 1
		}
		if b.EventTime == nil {
			return -1
		}
		if a.EventTime.Equal(*b.EventTime) {
			return strings.Compare(a.ProvenanceSpan, b.ProvenanceSpan)
		}
		if a.EventTime.Before(*b.EventTime) {
			return -1
		}
		return 1
	})

	return &Summary{
		Mode:     "count",
		Anchor:   wantAnchor,
		Count:    len(evidence),
		Evidence: evidence,
	}, nil
}

func extractMatchingAtoms(mem *types.Memory, anchorTerms []string) []Atom {
	if mem == nil {
		return nil
	}
	spans := sentenceSpans(mem.Content)
	matches := make([]Atom, 0, len(spans))
	for _, span := range spans {
		sentence := strings.TrimSpace(mem.Content[span.start:span.end])
		if sentence == "" {
			continue
		}
		normalized := normalizeText(sentence)
		if !containsAllTerms(normalized, anchorTerms) {
			continue
		}
		matches = append(matches, Atom{
			Project:        mem.Project,
			MemoryID:       mem.ID,
			ProvenanceSpan: fmt.Sprintf("chars:%d-%d", span.start, span.end),
			SpanText:       sentence,
			Statement:      sentence,
			NormalizedText: normalized,
			EventTime:      memoryEventTime(mem),
		})
	}
	return matches
}

type span struct {
	start int
	end   int
}

func sentenceSpans(text string) []span {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var spans []span
	start := 0
	for i, r := range text {
		switch r {
		case '.', '!', '?', '\n':
			end := i + 1
			if start < end {
				spans = append(spans, span{start: start, end: end})
			}
			start = end
		}
	}
	if start < len(text) {
		spans = append(spans, span{start: start, end: len(text)})
	}
	return spans
}

func containsAllTerms(normalized string, terms []string) bool {
	if len(terms) == 0 {
		return false
	}
	have := make(map[string]bool, len(terms))
	for _, term := range normalizeTerms(normalized) {
		have[term] = true
	}
	for _, term := range terms {
		if !have[term] {
			return false
		}
	}
	return true
}

func normalizeTerms(text string) []string {
	parts := strings.Fields(normalizeText(text))
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		if stopWords[part] {
			continue
		}
		part = stem(part)
		if part == "" || stopWords[part] {
			continue
		}
		normalized = append(normalized, part)
	}
	return dedupe(normalized)
}

func normalizeText(text string) string {
	return strings.TrimSpace(nonWordRE.ReplaceAllString(strings.ToLower(text), " "))
}

func stem(token string) string {
	switch {
	case len(token) > 4 && strings.HasSuffix(token, "ied"):
		return token[:len(token)-3] + "y"
	case len(token) > 4 && strings.HasSuffix(token, "ing"):
		return token[:len(token)-3]
	case len(token) > 3 && strings.HasSuffix(token, "ed"):
		return token[:len(token)-2]
	case len(token) > 3 && strings.HasSuffix(token, "es"):
		return token[:len(token)-2]
	case len(token) > 2 && strings.HasSuffix(token, "s"):
		return token[:len(token)-1]
	default:
		return token
	}
}

func dedupe(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func memoryEventTime(mem *types.Memory) *time.Time {
	if mem == nil {
		return nil
	}
	if mem.ValidFrom != nil {
		t := mem.ValidFrom.UTC()
		return &t
	}
	if mem.CreatedAt.IsZero() {
		return nil
	}
	t := mem.CreatedAt.UTC()
	return &t
}
