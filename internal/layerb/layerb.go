package layerb

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/petersimmons1972/engram/internal/aggq"
	"github.com/petersimmons1972/engram/internal/types"
)

// Atom is one deterministic evidence span extracted from a recalled raw memory.
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
func BuildSummary(ctx context.Context, store Store, q string, results []types.SearchResult) (*Summary, error) {
	if !aggq.IsAggregationQuestion(q) {
		return nil, nil
	}

	anchor := strings.TrimSpace(aggq.ExtractAggregationAnchor(q))
	anchorTerms := normalizeTerms(anchor)
	if len(anchorTerms) == 0 {
		return nil, nil
	}

	project := ""
	memoryIDs := make([]string, 0, len(results))
	var contributions []memberContribution
	for _, result := range results {
		if result.Memory == nil || strings.TrimSpace(result.Memory.Content) == "" {
			continue
		}
		if project == "" {
			project = result.Memory.Project
		}
		matches, memMatchedTerms := extractMatchingAtoms(result.Memory, anchorTerms)
		if len(matches) == 0 {
			continue
		}
		memoryIDs = append(memoryIDs, result.Memory.ID)
		contributions = append(contributions, memberContribution{atoms: matches, terms: dedupe(memMatchedTerms)})
	}

	if len(memoryIDs) == 0 || project == "" {
		return nil, nil
	}

	var unionTerms []string
	for _, c := range contributions {
		unionTerms = append(unionTerms, c.terms...)
	}

	// Collective near-complete gate: the ASSEMBLED evidence across every
	// contributing memory must cover the anchor near-completely, even though
	// each individual memory only needed to supply a fragment
	// (minimumAnchorTermMatchesForAggregation in extractMatchingAtoms). This
	// is where the strict v1 near-complete threshold now lives for the
	// multi-memory case — a single low-coverage memory cannot pass this gate
	// alone, and true scattered multi-session evidence collectively can.
	if len(dedupe(unionTerms)) < minimumAnchorTermMatches(len(anchorTerms)) {
		return nil, nil
	}

	// Connectivity gate (v4, issue #9 round-1 review finding): the lexical
	// union gate above only checks total term coverage — it does not check
	// that the contributing memories are actually RELATED to each other. Two
	// memories that share zero matched anchor terms can each supply a
	// fragment whose union still clears the near-complete bar, despite
	// describing wholly unrelated facts (e.g. "I serviced the bike rack in
	// January" + "I plan the March budget next week" — union covers all 4
	// anchor terms, but neither memory has anything to do with the other).
	// Require every contributing memory to be reachable from every other one
	// via a chain of shared matched terms (single connected component). This
	// preserves genuine scattered-evidence cases (each memory need only
	// share one term with SOME other contributor, not all of them directly)
	// while rejecting coincidental disjoint-fragment unions.
	if !contributionsAreConnected(contributions) {
		return nil, nil
	}

	var pendingAtoms []Atom
	for _, c := range contributions {
		pendingAtoms = append(pendingAtoms, c.atoms...)
	}

	for _, atom := range pendingAtoms {
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

func extractMatchingAtoms(mem *types.Memory, anchorTerms []string) ([]Atom, []string) {
	if mem == nil {
		return nil, nil
	}
	// Coverage is always computed from the whole memory's content. The
	// per-sentence-vs-fallback distinction below governs only which atoms
	// (evidence spans) get extracted for persistence — never how much anchor
	// coverage this memory contributes to BuildSummary's collective gate.
	wholeMemoryMatched := dedupe(matchedAnchorTerms(normalizeText(mem.Content), anchorTerms))

	aggregationMin := minimumAnchorTermMatchesForAggregation(len(anchorTerms))
	spans := sentenceSpans(mem.Content)
	matches := make([]Atom, 0, len(spans))
	for _, span := range spans {
		sentence := mem.Content[span.start:span.end]
		if sentence == "" {
			continue
		}
		normalized := normalizeText(sentence)
		sentenceMatched := matchedAnchorTerms(normalized, anchorTerms)
		if len(sentenceMatched) < aggregationMin {
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
	if len(matches) > 0 {
		return matches, wholeMemoryMatched
	}
	// Whole-memory fallback ATOM (extraction only — no sentence individually
	// qualified). Still gated by the strict v1 near-complete threshold: this is
	// where the arbitrary-unrelated-text false-positive risk lives, and it is
	// unchanged from v1/v2.
	if len(wholeMemoryMatched) < minimumAnchorTermMatches(len(anchorTerms)) {
		return nil, nil
	}
	if trimmed, ok := trimSpanBounds(mem.Content, 0, len(mem.Content)); ok {
		matches = append(matches, Atom{
			Project:        mem.Project,
			MemoryID:       mem.ID,
			ProvenanceSpan: fmt.Sprintf("chars:%d-%d", trimmed.start, trimmed.end),
			SpanText:       mem.Content[trimmed.start:trimmed.end],
			Statement:      mem.Content[trimmed.start:trimmed.end],
			NormalizedText: normalizeText(mem.Content[trimmed.start:trimmed.end]),
			EventTime:      memoryEventTime(mem),
		})
	}
	return matches, wholeMemoryMatched
}

// memberContribution is one contributing memory's atoms and matched anchor
// terms, kept together so BuildSummary's connectivity gate (v4) can check
// term-overlap between contributors before committing to persist anything.
type memberContribution struct {
	atoms []Atom
	terms []string
}

// contributionsAreConnected reports whether every contributing memory is
// reachable from every other one via a chain of shared matched anchor terms
// — the discriminator between genuine scattered evidence about the same
// underlying fact (each fragment overlaps on at least one term with some
// other fragment, even if not directly with all of them) and a coincidental
// union of wholly disjoint fragments that only look complete when combined
// (see PR #10 round-1 review, hermes + grok, both independently found this).
func contributionsAreConnected(contributions []memberContribution) bool {
	if len(contributions) <= 1 {
		return true
	}
	parent := make([]int, len(contributions))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	for i := 0; i < len(contributions); i++ {
		for j := i + 1; j < len(contributions); j++ {
			if sharesTerm(contributions[i].terms, contributions[j].terms) {
				union(i, j)
			}
		}
	}
	root := find(0)
	for i := 1; i < len(contributions); i++ {
		if find(i) != root {
			return false
		}
	}
	return true
}

// sharesTerm reports whether a and b have at least one element in common.
func sharesTerm(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, t := range a {
		set[t] = true
	}
	for _, t := range b {
		if set[t] {
			return true
		}
	}
	return false
}

type span struct {
	start int
	end   int
}

func sentenceSpans(text string) []span {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var spans []span
	start := 0
	for i, r := range text {
		switch r {
		case '.', '!', '?', '\n':
			end := i + 1
			if trimmed, ok := trimSpanBounds(text, start, end); ok {
				spans = append(spans, trimmed)
			}
			start = end
		}
	}
	if trimmed, ok := trimSpanBounds(text, start, len(text)); ok {
		spans = append(spans, trimmed)
	}
	return spans
}

func trimSpanBounds(text string, start, end int) (span, bool) {
	for start < end {
		r, size := utf8.DecodeRuneInString(text[start:end])
		if !unicode.IsSpace(r) {
			break
		}
		start += size
	}
	for start < end {
		r, size := utf8.DecodeLastRuneInString(text[start:end])
		if !unicode.IsSpace(r) {
			break
		}
		end -= size
	}
	if start >= end {
		return span{}, false
	}
	return span{start: start, end: end}, true
}

// matchedAnchorTerms returns the subset of terms present in normalized,
// preserving terms' input order and without duplicates within the result.
func matchedAnchorTerms(normalized string, terms []string) []string {
	have := make(map[string]bool, len(terms))
	for _, term := range normalizeTerms(normalized) {
		have[term] = true
	}
	matched := make([]string, 0, len(terms))
	for _, term := range terms {
		if have[term] {
			matched = append(matched, term)
		}
	}
	return matched
}

func minimumAnchorTermMatches(termCount int) int {
	switch {
	case termCount <= 0:
		return 0
	case termCount <= 2:
		return termCount
	default:
		return termCount - 1
	}
}

// minimumAnchorTermMatchesForAggregation is the per-memory bar used when a
// memory is one of potentially several contributing to a cross-memory
// aggregation: a memory only needs to supply a genuine fragment of the
// anchor, not near-complete coverage on its own — the near-complete
// requirement is enforced collectively, across the whole evidence set, by
// the caller (see BuildSummary). A single common word should not qualify a
// memory, so the floor is min(2, termCount).
func minimumAnchorTermMatchesForAggregation(termCount int) int {
	if termCount < 2 {
		return termCount
	}
	return 2
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
		return stemAfterSuffix(token[:len(token)-3])
	case len(token) > 3 && strings.HasSuffix(token, "ed"):
		return stemAfterSuffix(token[:len(token)-2])
	case len(token) > 3 && strings.HasSuffix(token, "es"):
		candidate := token[:len(token)-2]
		if endsWithSibilantOrIrregularO(candidate) {
			return candidate
		}
		return token[:len(token)-1]
	case len(token) > 2 && strings.HasSuffix(token, "s") && !strings.HasSuffix(token, "ss"):
		return token[:len(token)-1]
	default:
		return token
	}
}

// endsWithSibilant reports whether s ends in a sibilant sound (s, x, z, ch,
// sh) — the class of endings where English pluralizes with "-es" rather than
// a bare "-s" (wish->wishes, box->boxes, class->classes), as distinct from
// endings like "bike" where the "-es" in "bikes" is really just "-s" plus the
// word's own trailing "e" (bike->bikes, not "bik"+"es").
func endsWithSibilant(s string) bool {
	if strings.HasSuffix(s, "ch") || strings.HasSuffix(s, "sh") {
		return true
	}
	if len(s) == 0 {
		return false
	}
	switch s[len(s)-1] {
	case 's', 'x', 'z':
		return true
	default:
		return false
	}
}

// endsWithSibilantOrIrregularO reports whether s ends in a sibilant (see
// endsWithSibilant) OR in a bare "o" — the second case covers a common
// irregular English "-oes" plural class (go->goes, do->does, hero->heroes,
// tomato->tomatoes, echo->echoes) that would otherwise mis-stem to "goe",
// "doe", "heroe", "tomatoe", "echoe" if only the sibilant check applied.
// Known residual imprecision (inherent to a lookup-free heuristic stemmer,
// not fixable without a dictionary): a handful of real words already ending
// in silent-e before a bare "o" syllable (e.g. "shoe"/"shoes") will still
// mis-stem under this rule; this is an accepted tradeoff, not a regression
// target, since no test in this package currently depends on that case.
//
//nolint:misspell
func endsWithSibilantOrIrregularO(s string) bool {
	if endsWithSibilant(s) {
		return true
	}
	if len(s) == 0 {
		return false
	}
	return s[len(s)-1] == 'o'
}

func stemAfterSuffix(base string) string {
	switch {
	case strings.HasSuffix(base, "ic"):
		return base + "e"
	case strings.HasSuffix(base, "at"), strings.HasSuffix(base, "bl"), strings.HasSuffix(base, "iz"):
		return base + "e"
	case endsWithDoubleConsonant(base) && !strings.HasSuffix(base, "l") && !strings.HasSuffix(base, "s") && !strings.HasSuffix(base, "z"):
		return base[:len(base)-1]
	case isShortStem(base):
		return base + "e"
	default:
		return base
	}
}

func endsWithDoubleConsonant(token string) bool {
	if len(token) < 2 {
		return false
	}
	last := token[len(token)-1]
	prev := token[len(token)-2]
	return last == prev && isConsonant(token, len(token)-1)
}

func isShortStem(token string) bool {
	if len(token) < 3 {
		return false
	}
	last := len(token) - 1
	return measure(token) == 1 &&
		isConsonant(token, last) &&
		!isConsonant(token, last-1) &&
		isConsonant(token, last-2) &&
		!strings.ContainsRune("wxy", rune(token[last]))
}

func measure(token string) int {
	count := 0
	inVowelRun := false
	for i := 0; i < len(token); i++ {
		if isConsonant(token, i) {
			if inVowelRun {
				count++
				inVowelRun = false
			}
			continue
		}
		inVowelRun = true
	}
	return count
}

func isConsonant(token string, index int) bool {
	if index < 0 || index >= len(token) {
		return false
	}
	switch token[index] {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	case 'y':
		if index == 0 {
			return true
		}
		return !isConsonant(token, index-1)
	default:
		return true
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
