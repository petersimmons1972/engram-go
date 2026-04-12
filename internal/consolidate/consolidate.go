// Package consolidate implements sleep-consolidation strategies for Engram memories.
// Feature 3: Sleep Consolidation Daemon.
package consolidate

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

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

	// LLM contradiction detection (opt-in).
	// When LLMContradictionDetection is true and OllamaURL is non-empty,
	// DetectContradictions runs a second pass using the local Ollama model to
	// classify high-similarity pairs that the heuristic missed. This catches
	// competing affirmative claims ("model is X" vs "model is Y") that carry no
	// negation, version, or temporal markers.
	LLMContradictionDetection bool
	OllamaURL                 string
	OllamaModel               string
	// LLMMaxCalls caps how many Ollama calls are made per DetectContradictions
	// cycle to bound latency. Zero or negative means use the default (10).
	LLMMaxCalls int

	// AutoSupersede opts into automatic supersession: when a contradiction is
	// detected and one memory is >24 h newer than the other, create a
	// "supersedes" edge from the newer to the older and soft-delete the older.
	// Opt-in because it is a destructive action — callers must set this
	// explicitly; the default (false) leaves both memories active.
	AutoSupersede bool
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

// uncaughtPair is a high-similarity memory pair that the heuristic did not
// flag as a contradiction. It is queued for the optional LLM second pass.
type uncaughtPair struct {
	sourceID string
	targetID string
	textA    string
	textB    string
	strength float64
}

// DetectContradictions scans all memory pairs with cosine similarity >= minSimilarity
// and creates a "contradicts" edge when the text content signals opposing claims.
// It skips pairs that already have a "contradicts" relationship edge.
// Returns the number of new contradicts edges created.
//
// The primary detection is pattern-based (no LLM). See isContradiction for the
// three heuristics. When called from RunAll with LLMContradictionDetection=true,
// pairs that the heuristic misses are passed to a local Ollama model for a
// second opinion. Use DetectContradictions directly (without RunOptions) when
// the LLM pass is not needed.
func (r *Runner) DetectContradictions(ctx context.Context, minSimilarity float64, limit int) (int, error) {
	return r.detectContradictions(ctx, minSimilarity, limit, RunOptions{})
}

// detectContradictions is the internal implementation of DetectContradictions
// that accepts RunOptions so RunAll can pass LLM settings without changing the
// public API signature.
func (r *Runner) detectContradictions(ctx context.Context, minSimilarity float64, limit int, opts RunOptions) (int, error) {
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

	// uncaught collects pairs that passed the similarity threshold but were not
	// flagged by the heuristic. They are candidates for the LLM second pass.
	var uncaught []uncaughtPair

	for memID, chunk := range memChunks {
		// Load existing contradicts edges — only skip pairs that already have a
		// "contradicts" edge. We intentionally do NOT skip pairs with "relates_to"
		// because InferRelationships runs first in RunAll and creates relates_to for
		// the same high-similarity pairs we need to check.
		rels, err := r.backend.GetRelationships(ctx, r.project, memID)
		if err != nil {
			return created, fmt.Errorf("consolidate: DetectContradictions: GetRelationships(%s): %w", memID, err)
		}
		alreadyContradicts := make(map[string]bool, len(rels))
		for _, rel := range rels {
			if rel.RelType != types.RelTypeContradicts {
				continue
			}
			if rel.SourceID == memID {
				alreadyContradicts[rel.TargetID] = true
			} else {
				alreadyContradicts[rel.SourceID] = true
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
			if alreadyContradicts[hit.MemoryID] {
				processed[key] = true
				continue
			}

			// Heuristic pass — pattern-based, no LLM.
			if !isContradiction(chunk.ChunkText, hit.ChunkText) {
				// Queue for LLM second pass if enabled.
				if opts.LLMContradictionDetection && opts.OllamaURL != "" {
					uncaught = append(uncaught, uncaughtPair{
						sourceID: memID,
						targetID: hit.MemoryID,
						textA:    chunk.ChunkText,
						textB:    hit.ChunkText,
						strength: similarity,
					})
				}
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

	// LLM second pass — classify pairs that the heuristic missed.
	// Best-effort: errors from individual LLM calls are logged and skipped so
	// a transient Ollama failure does not abort the entire consolidation run.
	if opts.LLMContradictionDetection && opts.OllamaURL != "" && len(uncaught) > 0 {
		maxCalls := opts.LLMMaxCalls
		if maxCalls <= 0 {
			maxCalls = 10
		}
		llmCalls := 0
		for _, pair := range uncaught {
			if llmCalls >= maxCalls {
				break
			}
			isContra, err := ClassifyContradictionLLM(ctx, pair.textA, pair.textB, opts.OllamaURL, opts.OllamaModel)
			llmCalls++ // count every attempt — erroring calls still consume latency budget
			if err != nil {
				// Best-effort: skip this pair, keep going.
				continue
			}
			if !isContra {
				continue
			}
			rel := &types.Relationship{
				ID:       types.NewMemoryID(),
				SourceID: pair.sourceID,
				TargetID: pair.targetID,
				RelType:  types.RelTypeContradicts,
				Strength: pair.strength,
				Project:  r.project,
			}
			if err := r.backend.StoreRelationship(ctx, rel); err != nil {
				return created, fmt.Errorf("consolidate: DetectContradictions: LLM pass StoreRelationship: %w", err)
			}
			created++
		}
	}

	return created, nil
}

// supersedeThreshold is the minimum age gap between two contradicting memories
// required for AutoSupersede to act. Pairs closer than this are ambiguous —
// they may have been recorded in the same session from different sources —
// so we leave them for human review rather than silently discarding one.
const supersedeThreshold = 24 * time.Hour

// AutoSupersede resolves contradictions where one memory is unambiguously newer
// than the other. For every "contradicts" edge in this project it:
//
//  1. Fetches both memories.
//  2. Skips the pair if either memory is already soft-deleted.
//  3. Skips the pair if the age gap between CreatedAt timestamps is <= 24 h.
//  4. Creates a "supersedes" edge from the newer memory to the older (skips if
//     the edge already exists — StoreRelationship is ON CONFLICT DO UPDATE, so
//     an existing supersedes edge is simply a no-op here).
//  5. Soft-deletes the older memory with reason "superseded by <newerID>".
//
// Returns the number of memories that were superseded (soft-deleted) this run.
func (r *Runner) AutoSupersede(ctx context.Context) (int, error) {
	// Collect all memory IDs for this project so we can look up their edges.
	allIDs, err := r.backend.GetAllMemoryIDs(ctx, r.project)
	if err != nil {
		return 0, fmt.Errorf("consolidate: AutoSupersede: GetAllMemoryIDs: %w", err)
	}

	// Walk every memory, gather all contradicts edges.  Use a canonical pair key
	// to avoid processing the same (A,B) pair twice when both A and B are in the
	// allIDs map.
	seenPairs := make(map[pairKey]bool)
	superseded := 0

	for memID := range allIDs {
		rels, err := r.backend.GetRelationships(ctx, r.project, memID)
		if err != nil {
			return superseded, fmt.Errorf("consolidate: AutoSupersede: GetRelationships(%s): %w", memID, err)
		}

		for _, rel := range rels {
			if rel.RelType != types.RelTypeContradicts {
				continue
			}

			key := canonical(rel.SourceID, rel.TargetID)
			if seenPairs[key] {
				continue
			}
			seenPairs[key] = true

			// Fetch both memories.  GetMemory only returns active (valid_to IS NULL)
			// records, so a nil result means the memory has already been soft-deleted.
			memA, err := r.backend.GetMemory(ctx, rel.SourceID)
			if err != nil {
				return superseded, fmt.Errorf("consolidate: AutoSupersede: GetMemory(%s): %w", rel.SourceID, err)
			}
			memB, err := r.backend.GetMemory(ctx, rel.TargetID)
			if err != nil {
				return superseded, fmt.Errorf("consolidate: AutoSupersede: GetMemory(%s): %w", rel.TargetID, err)
			}

			// Skip if either memory has already been soft-deleted.
			if memA == nil || memB == nil {
				continue
			}

			// Determine which is newer.  Use absolute difference so the direction
			// of the contradicts edge does not affect the outcome.
			diff := memA.CreatedAt.Sub(memB.CreatedAt)
			if diff < 0 {
				diff = -diff
			}
			if diff <= supersedeThreshold {
				// Too close in time — ambiguous, leave for human review.
				continue
			}

			// Assign newer / older.
			var newer, older *types.Memory
			if memA.CreatedAt.After(memB.CreatedAt) {
				newer, older = memA, memB
			} else {
				newer, older = memB, memA
			}

			// Create the supersedes edge (newer → older).  StoreRelationship uses
			// ON CONFLICT DO UPDATE so calling it on an already-existing edge is safe.
			supRel := &types.Relationship{
				ID:       types.NewMemoryID(),
				SourceID: newer.ID,
				TargetID: older.ID,
				RelType:  types.RelTypeSupersedes,
				Strength: 1.0,
				Project:  r.project,
			}
			if err := r.backend.StoreRelationship(ctx, supRel); err != nil {
				return superseded, fmt.Errorf("consolidate: AutoSupersede: StoreRelationship: %w", err)
			}

			// Soft-delete the older memory.
			reason := "superseded by " + newer.ID
			if _, err := r.backend.SoftDeleteMemory(ctx, r.project, older.ID, reason); err != nil {
				return superseded, fmt.Errorf("consolidate: AutoSupersede: SoftDeleteMemory(%s): %w", older.ID, err)
			}
			superseded++
		}
	}

	return superseded, nil
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

	// Pass the full RunOptions to detectContradictions so the LLM second pass
	// receives the Ollama URL and model without a public API change.
	contradictions, err := r.detectContradictions(ctx, minSim, limit, opts)
	if err != nil {
		return nil, fmt.Errorf("consolidate: RunAll: DetectContradictions: %w", err)
	}

	superseded := 0
	if opts.AutoSupersede {
		superseded, err = r.AutoSupersede(ctx)
		if err != nil {
			return nil, fmt.Errorf("consolidate: RunAll: AutoSupersede: %w", err)
		}
	}

	return map[string]any{
		"inferred_relationships":  inferred,
		"detected_contradictions": contradictions,
		"auto_superseded":         superseded,
	}, nil
}
