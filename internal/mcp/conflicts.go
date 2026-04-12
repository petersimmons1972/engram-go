package mcp

// conflicts.go implements noise-aware recall enrichment (Step 2 of the
// Noise Resistance plan). After a normal memory_recall, callers may request
// conflicting_results by setting include_conflicts=true. This file provides
// EnrichWithConflicts, which follows "contradicts" edges in the relationship
// graph and returns the opposing memories so the caller can reason about
// contradictions explicitly.

import (
	"context"
	"log/slog"

	"github.com/petersimmons1972/engram/internal/types"
)

const maxConflictResults = 50

// ConflictReader is the minimal subset of db.Backend required by
// EnrichWithConflicts. Exporting the interface lets callers (including test
// packages) supply lightweight stubs without satisfying the full db.Backend
// interface.
type ConflictReader interface {
	GetRelationships(ctx context.Context, project, memoryID string) ([]types.Relationship, error)
	GetMemory(ctx context.Context, id string) (*types.Memory, error)
}

// EnrichWithConflicts walks the "contradicts" edges for each recalled memory
// and returns the opposing memories as ConflictingResult values.
//
// The function is best-effort: errors from the backend (GetRelationships,
// GetMemory) are silently skipped so that a transient DB problem does not
// prevent the primary recall results from being returned.
//
// Deduplication: a contradicting memory is included at most once regardless of
// how many primary results point to it.
func EnrichWithConflicts(
	ctx context.Context,
	backend ConflictReader,
	project string,
	results []types.SearchResult,
) []types.ConflictingResult {
	// Build set of primary result IDs so we never return a memory that already
	// appears in the main results — surfacing the same memory in both would
	// confuse the caller.
	primaryIDs := make(map[string]bool, len(results))
	for _, r := range results {
		if r.Memory != nil {
			primaryIDs[r.Memory.ID] = true
		}
	}

	var conflicts []types.ConflictingResult
	seen := make(map[string]bool)

	for _, r := range results {
		if r.Memory == nil {
			continue
		}

		rels, err := backend.GetRelationships(ctx, project, r.Memory.ID)
		if err != nil {
			slog.Warn("EnrichWithConflicts: GetRelationships failed",
				"memory_id", r.Memory.ID, "err", err)
			continue
		}

		for _, rel := range rels {
			if rel.RelType != types.RelTypeContradicts {
				continue
			}

			// Identify the other endpoint of the contradiction.
			otherID := rel.TargetID
			if rel.TargetID == r.Memory.ID {
				otherID = rel.SourceID
			}

			// Skip memories already in primary results or already emitted.
			if primaryIDs[otherID] || seen[otherID] {
				continue
			}
			seen[otherID] = true

			otherMem, err := backend.GetMemory(ctx, otherID)
			if err != nil {
				slog.Warn("EnrichWithConflicts: GetMemory failed",
					"memory_id", otherID, "err", err)
				continue
			}
			if otherMem == nil {
				continue
			}

			// Truncate content to 500 bytes for the matched_chunk preview.
			chunk := otherMem.Content
			if len(chunk) > 500 {
				chunk = chunk[:500]
			}

			conflicts = append(conflicts, types.ConflictingResult{
				Memory:        otherMem,
				ContradictsID: r.Memory.ID,
				Strength:      rel.Strength,
				MatchedChunk:  chunk,
			})

			if len(conflicts) >= maxConflictResults {
				return conflicts
			}
		}
	}

	return conflicts
}
