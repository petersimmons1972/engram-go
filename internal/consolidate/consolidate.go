// Package consolidate implements sleep-consolidation strategies for Engram memories.
// Feature 3: Sleep Consolidation Daemon.
package consolidate

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/types"
)

// versionRe matches version strings like v1.2, v10.3, 2.0, 1.2.3, etc.
var versionRe = regexp.MustCompile(`v?\d+\.\d+(?:\.\d+)*`)

// negationPhrases are token-level signals that one claim negates another.
var negationPhrases = []string{
	"not ", "no ", "never ", "does not", "do not", "is not", "are not",
	"cannot", "can't", "don't", "doesn't", "isn't", "won't", "wasn't",
	"weren't", "didn't", "hasn't", "haven't", "hadn't",
}

// pastTenseMarkers indicate a claim is describing a historical (no-longer-current) state.
var pastTenseMarkers = []string{
	"was ", "were ", "previously ", "used to ", "had been ", "no longer ",
}

// presentTenseMarkers indicate a claim is describing the current state.
var presentTenseMarkers = []string{
	" is ", " are ", " now ", "currently ", "today ",
}

// isContradiction returns true when contentA and contentB are likely contradictory.
// Three heuristics are checked in order. Any single match is sufficient.
//
// Heuristics (false negatives preferred over false positives):
//  1. Negation opposition: texts share significant vocabulary, but one contains a
//     negation phrase that the other lacks.
//  2. Version conflict: both texts reference the same entity with different version numbers.
//  3. Temporal supersession: one text uses past-tense markers while the other uses
//     present-tense markers on what appears to be the same subject.
func isContradiction(contentA, contentB string) bool {
	a := strings.ToLower(contentA)
	b := strings.ToLower(contentB)

	// Heuristic 1 — negation opposition.
	// Require both texts to share at least 3 significant words before checking
	// negation asymmetry, to keep the false-positive rate low.
	if sharedWordCount(a, b) >= 3 {
		aNeg := containsAny(a, negationPhrases)
		bNeg := containsAny(b, negationPhrases)
		if aNeg != bNeg {
			return true
		}
	}

	// Heuristic 2 — version conflict.
	aVers := versionRe.FindAllString(a, -1)
	bVers := versionRe.FindAllString(b, -1)
	if len(aVers) > 0 && len(bVers) > 0 {
		// Same surrounding context (shared words) but different version numbers.
		if sharedWordCount(stripVersions(a), stripVersions(b)) >= 3 && !sameVersionSet(aVers, bVers) {
			return true
		}
	}

	// Heuristic 3 — temporal supersession.
	// One text uses past-tense markers; the other uses present-tense markers.
	aPast := containsAny(a, pastTenseMarkers)
	bPast := containsAny(b, pastTenseMarkers)
	aPresent := containsAny(a, presentTenseMarkers)
	bPresent := containsAny(b, presentTenseMarkers)
	if (aPast && bPresent && !bPast) || (bPast && aPresent && !aPast) {
		// Only flag as contradictory if they also share vocabulary — otherwise any
		// historical sentence paired with any current sentence would trigger.
		if sharedWordCount(a, b) >= 3 {
			return true
		}
	}

	return false
}

// sharedWordCount returns the number of distinct words longer than 3 characters
// that appear in both a and b. Short stop-words are excluded to avoid trivial matches.
func sharedWordCount(a, b string) int {
	aWords := significantWords(a)
	count := 0
	bWords := significantWords(b)
	for w := range aWords {
		if bWords[w] {
			count++
		}
	}
	return count
}

// significantWords returns the set of lower-case words longer than 3 characters
// in s, split on whitespace and punctuation.
func significantWords(s string) map[string]bool {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !('a' <= r && r <= 'z') && !('0' <= r && r <= '9')
	})
	m := make(map[string]bool, len(words))
	for _, w := range words {
		if len(w) > 3 {
			m[w] = true
		}
	}
	return m
}

// containsAny returns true if s contains any of the substrings in list.
func containsAny(s string, list []string) bool {
	for _, phrase := range list {
		if strings.Contains(s, phrase) {
			return true
		}
	}
	return false
}

// stripVersions removes all version strings from s.
func stripVersions(s string) string {
	return versionRe.ReplaceAllString(s, "")
}

// sameVersionSet returns true if every version in a also appears in b and vice versa.
func sameVersionSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	setA := make(map[string]bool, len(a))
	for _, v := range a {
		setA[v] = true
	}
	for _, v := range b {
		if !setA[v] {
			return false
		}
	}
	return true
}

// RunOptions controls which consolidation strategies run and their parameters.
type RunOptions struct {
	InferRelationshipsMinSimilarity float64
	InferRelationshipsLimit         int
}

// Runner executes consolidation strategies against a single project.
type Runner struct {
	backend db.Backend
	project string
	embedder embed.Client
}

// NewRunner creates a Runner for the given project.
func NewRunner(backend db.Backend, project string, embedder embed.Client) *Runner {
	return &Runner{backend: backend, project: project, embedder: embedder}
}

// pairKey is a canonical unordered pair of memory IDs (smaller ID first).
type pairKey struct{ a, b string }

func canonical(a, b string) pairKey {
	if a < b {
		return pairKey{a, b}
	}
	return pairKey{b, a}
}

// InferRelationships creates relates_to edges between memories whose stored chunk
// embeddings are nearest neighbors with cosine similarity >= minSimilarity.
// It skips pairs that already have any relationship. Returns the number of new edges created.
func (r *Runner) InferRelationships(ctx context.Context, minSimilarity float64, limit int) (int, error) {
	chunks, err := r.backend.GetAllChunksWithEmbeddings(ctx, r.project, limit)
	if err != nil {
		return 0, fmt.Errorf("consolidate: GetAllChunksWithEmbeddings: %w", err)
	}

	// One embedding per memory — use the first chunk encountered per memory ID.
	memChunks := make(map[string]*types.Chunk, len(chunks))
	for _, c := range chunks {
		if _, ok := memChunks[c.MemoryID]; !ok {
			memChunks[c.MemoryID] = c
		}
	}

	// processed tracks canonical (a,b) pairs already handled this run to avoid
	// creating (A→B) and (B→A) for the same pair.
	processed := make(map[pairKey]bool)
	created := 0

	for memID, chunk := range memChunks {
		// Load existing connections so we don't duplicate them.
		rels, err := r.backend.GetRelationships(ctx, r.project, memID)
		if err != nil {
			return created, fmt.Errorf("consolidate: GetRelationships(%s): %w", memID, err)
		}
		connected := make(map[string]bool, len(rels))
		for _, rel := range rels {
			if rel.SourceID == memID {
				connected[rel.TargetID] = true
			} else {
				connected[rel.SourceID] = true
			}
		}

		hits, err := r.backend.VectorSearch(ctx, r.project, chunk.Embedding, limit)
		if err != nil {
			return created, fmt.Errorf("consolidate: VectorSearch(%s): %w", memID, err)
		}

		for _, hit := range hits {
			if hit.MemoryID == memID {
				continue
			}
			key := canonical(memID, hit.MemoryID)
			if processed[key] {
				continue
			}
			// cosine distance: 0 = identical, 2 = opposite → similarity = 1 - distance.
			similarity := 1.0 - hit.Distance
			if similarity < minSimilarity {
				processed[key] = true
				continue
			}
			if connected[hit.MemoryID] {
				processed[key] = true
				continue
			}
			rel := &types.Relationship{
				ID:       types.NewMemoryID(),
				SourceID: memID,
				TargetID: hit.MemoryID,
				RelType:  types.RelTypeRelatesTo,
				Strength: similarity,
				Project:  r.project,
			}
			if err := r.backend.StoreRelationship(ctx, rel); err != nil {
				return created, fmt.Errorf("consolidate: StoreRelationship: %w", err)
			}
			processed[key] = true
			created++
		}
	}

	return created, nil
}

// DetectContradictions scans all memory pairs with cosine similarity >= minSimilarity
// and creates a "contradicts" edge when the text content signals opposing claims.
// It skips pairs that already have any relationship edge (same guard as InferRelationships).
// Returns the number of new contradicts edges created.
//
// Contradiction detection is purely pattern-based (no LLM). See isContradiction for
// the three heuristics. False negatives are acceptable; false positives are not.
func (r *Runner) DetectContradictions(ctx context.Context, minSimilarity float64, limit int) (int, error) {
	chunks, err := r.backend.GetAllChunksWithEmbeddings(ctx, r.project, limit)
	if err != nil {
		return 0, fmt.Errorf("consolidate: DetectContradictions: GetAllChunksWithEmbeddings: %w", err)
	}

	// One chunk per memory — use the first chunk encountered per memory ID.
	memChunks := make(map[string]*types.Chunk, len(chunks))
	for _, c := range chunks {
		if _, ok := memChunks[c.MemoryID]; !ok {
			memChunks[c.MemoryID] = c
		}
	}

	processed := make(map[pairKey]bool)
	created := 0

	for memID, chunk := range memChunks {
		// Load existing connections — skip any pair that already has an edge.
		rels, err := r.backend.GetRelationships(ctx, r.project, memID)
		if err != nil {
			return created, fmt.Errorf("consolidate: DetectContradictions: GetRelationships(%s): %w", memID, err)
		}
		connected := make(map[string]bool, len(rels))
		for _, rel := range rels {
			if rel.SourceID == memID {
				connected[rel.TargetID] = true
			} else {
				connected[rel.SourceID] = true
			}
		}

		hits, err := r.backend.VectorSearch(ctx, r.project, chunk.Embedding, limit)
		if err != nil {
			return created, fmt.Errorf("consolidate: DetectContradictions: VectorSearch(%s): %w", memID, err)
		}

		for _, hit := range hits {
			if hit.MemoryID == memID {
				continue
			}
			key := canonical(memID, hit.MemoryID)
			if processed[key] {
				continue
			}
			similarity := 1.0 - hit.Distance
			if similarity < minSimilarity {
				processed[key] = true
				continue
			}
			if connected[hit.MemoryID] {
				processed[key] = true
				continue
			}

			// Only create the edge when the text content signals opposing claims.
			if !isContradiction(chunk.ChunkText, hit.ChunkText) {
				processed[key] = true
				continue
			}

			rel := &types.Relationship{
				ID:       types.NewMemoryID(),
				SourceID: memID,
				TargetID: hit.MemoryID,
				RelType:  types.RelTypeContradicts,
				Strength: similarity,
				Project:  r.project,
			}
			if err := r.backend.StoreRelationship(ctx, rel); err != nil {
				return created, fmt.Errorf("consolidate: DetectContradictions: StoreRelationship: %w", err)
			}
			processed[key] = true
			created++
		}
	}

	return created, nil
}

// RunAll executes all configured consolidation strategies and returns a stats map.
// The stats map includes "inferred_relationships" and "detected_contradictions" counts.
func (r *Runner) RunAll(ctx context.Context, opts RunOptions) (map[string]any, error) {
	limit := opts.InferRelationshipsLimit
	if limit <= 0 {
		limit = 500
	}
	minSim := opts.InferRelationshipsMinSimilarity

	inferred, err := r.InferRelationships(ctx, minSim, limit)
	if err != nil {
		return nil, fmt.Errorf("consolidate: RunAll: %w", err)
	}

	contradictions, err := r.DetectContradictions(ctx, minSim, limit)
	if err != nil {
		return nil, fmt.Errorf("consolidate: RunAll: DetectContradictions: %w", err)
	}

	return map[string]any{
		"inferred_relationships":  inferred,
		"detected_contradictions": contradictions,
	}, nil
}
