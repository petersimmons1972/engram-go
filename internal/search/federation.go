package search

import (
	"context"
	"sort"

	"github.com/petersimmons1972/engram/internal/types"
)

// RecallAcrossEngines fans out a recall query to multiple project engines concurrently,
// merges all results, deduplicates by memory ID, and returns the topK results sorted by
// composite score descending. If topK <= 0, all results are returned.
//
// This is the engine-layer primitive for Cross-Project Knowledge Federation (Feature 4).
// The MCP handler wires project name → engine via EnginePool and calls this function.
func RecallAcrossEngines(ctx context.Context, engines []*SearchEngine, query string, topK int, detail string) ([]types.SearchResult, error) {
	if len(engines) == 0 {
		return nil, nil
	}
	if len(engines) == 1 {
		return engines[0].Recall(ctx, query, topK, detail)
	}

	type fanResult struct {
		results []types.SearchResult
		err     error
	}
	ch := make(chan fanResult, len(engines))
	for _, eng := range engines {
		eng := eng
		go func() {
			res, err := eng.Recall(ctx, query, topK, detail)
			ch <- fanResult{res, err}
		}()
	}

	// Collect, dedup by ID (keep highest score), then sort.
	seen := make(map[string]types.SearchResult, len(engines)*topK)
	for range engines {
		fan := <-ch
		if fan.err != nil {
			// Best-effort: log and skip failing engines rather than aborting the whole call.
			continue
		}
		for _, r := range fan.results {
			if existing, ok := seen[r.Memory.ID]; !ok || r.Score > existing.Score {
				seen[r.Memory.ID] = r
			}
		}
	}

	merged := make([]types.SearchResult, 0, len(seen))
	for _, r := range seen {
		merged = append(merged, r)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })

	if topK > 0 && len(merged) > topK {
		merged = merged[:topK]
	}
	return merged, nil
}
