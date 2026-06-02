package search

// mempalace.go — LME experiment #9: MemPalace hierarchical (coarse→fine) recall.
//
// When ENGRAM_MEMPALACE_HIERARCHICAL_RECALL=true (default: false), RecallWithOpts
// uses a 2-level hierarchy instead of flat vector search:
//
//   Level 1 (coarse): embed query → find top-C nearest cluster centroids
//                     stored in memory_clusters per project.
//   Level 2 (fine):   restrict VectorSearch to chunks whose parent memory
//                     has cluster_id IN (top-C cluster IDs) → standard scoring.
//
// FLAG IS DEFAULT OFF.  Set ENGRAM_MEMPALACE_HIERARCHICAL_RECALL=1 or =true to
// enable.  Ablation: unset or set to any other value to run the baseline flat path.
//
// BACK-FILL NOTE: existing memories have cluster_id = NULL and will NOT benefit
// from hierarchical filtering until the back-fill step is run.  The flat path is
// used automatically for any project whose memories have no cluster assignments
// (FindNearestClusters returns empty → VectorSearchWithClusters falls back to flat).
//
// Bench command (after back-fill):
//
//   ENGRAM_MEMPALACE_HIERARCHICAL_RECALL=true \
//   scripts/longmemeval-pipeline.sh
//
// See also: docs/mempalace-backfill.md

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
)

// defaultTopClusters is the number of coarse-level clusters searched per recall
// when RecallOpts.TopClusters is 0.  3 covers the typical topic spread for LME
// personal-conversation data while keeping the filter tight enough to exclude
// off-topic clusters.
const defaultTopClusters = 3

// HierarchicalRecallEnabled reports whether the MemPalace hierarchical recall
// path is active.  Reads ENGRAM_MEMPALACE_HIERARCHICAL_RECALL at call time so
// that tests can toggle it with t.Setenv without restarting the process.
func HierarchicalRecallEnabled() bool {
	v := os.Getenv("ENGRAM_MEMPALACE_HIERARCHICAL_RECALL")
	return v == "true" || v == "1"
}

// clusterBackend is satisfied by db.Backend. Declared here to avoid a full
// db.Backend import in the search package (which would import db, creating a
// potential cycle). The search package already imports db, so this is fine.
type clusterBackend interface {
	FindNearestClusters(ctx context.Context, project string, queryVec []float32, topK int) ([]string, error)
	VectorSearchWithClusters(ctx context.Context, project string, queryVec []float32, limit int, clusterIDs []string, since, before *time.Time) ([]db.VectorHit, error)
}

// hierarchicalVectorSearch performs the MemPalace coarse→fine lookup:
//  1. Find the top-C nearest cluster centroids to queryVec.
//  2. Run VectorSearch restricted to chunks in those clusters.
//
// Falls back transparently to unconstrained search when:
//   - The backend doesn't expose clusterBackend (should not happen in prod).
//   - No clusters exist for the project (FindNearestClusters returns empty).
//   - topClusters <= 0 (uses defaultTopClusters).
func hierarchicalVectorSearch(
	ctx context.Context,
	backend db.Backend,
	project string,
	queryVec []float32,
	limit int,
	topClusters int,
	since, before *time.Time,
) ([]db.VectorHit, error) {
	cb, ok := backend.(clusterBackend)
	if !ok {
		// Backend doesn't implement cluster methods — fall back.
		slog.Debug("mempalace: backend does not implement clusterBackend; falling back to flat search",
			"project", project)
		return backend.VectorSearchWithDateRange(ctx, project, queryVec, limit, since, before)
	}

	if topClusters <= 0 {
		topClusters = defaultTopClusters
	}

	clusterIDs, err := cb.FindNearestClusters(ctx, project, queryVec, topClusters)
	if err != nil {
		// Non-fatal: log and fall back to flat path.
		slog.Warn("mempalace: FindNearestClusters failed; falling back to flat search",
			"project", project, "err", err)
		return backend.VectorSearchWithDateRange(ctx, project, queryVec, limit, since, before)
	}

	if len(clusterIDs) == 0 {
		// No clusters yet — project hasn't been back-filled. Use flat path silently.
		return backend.VectorSearchWithDateRange(ctx, project, queryVec, limit, since, before)
	}

	hits, err := cb.VectorSearchWithClusters(ctx, project, queryVec, limit, clusterIDs, since, before)
	if err != nil {
		slog.Warn("mempalace: VectorSearchWithClusters failed; falling back to flat search",
			"project", project, "err", err)
		return backend.VectorSearchWithDateRange(ctx, project, queryVec, limit, since, before)
	}
	slog.Debug("mempalace: hierarchical search complete",
		"project", project,
		"top_clusters", topClusters,
		"cluster_ids", clusterIDs,
		"hits", len(hits))
	return hits, nil
}
