// Package consolidate implements sleep-consolidation strategies for Engram memories.
// Feature 3: Sleep Consolidation Daemon.
package consolidate

import (
	"context"
	"fmt"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/types"
)

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

// RunAll executes all configured consolidation strategies and returns a stats map.
// The stats map always includes "inferred_relationships" (count of new edges created).
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

	return map[string]any{
		"inferred_relationships": inferred,
	}, nil
}
