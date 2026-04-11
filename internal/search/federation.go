package search

import (
	"context"
	"log/slog"
	"sort"
	"sync"

	"github.com/petersimmons1972/engram/internal/types"
)

// maxFederationConcurrency caps how many project engines are queried simultaneously
// to prevent a large EnginePool from opening O(N) parallel database connections (#119).
const maxFederationConcurrency = 4

// RecallAcrossEngines fans out a recall query to multiple project engines concurrently,
// merges all results, deduplicates by memory ID, and returns the topK results sorted by
// composite score descending. If topK <= 0, all results are returned.
//
// Concurrency is bounded to maxFederationConcurrency (#119) to prevent unbounded
// fan-out when many projects are registered.
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

	// Semaphore limits concurrent Recall calls to maxFederationConcurrency.
	sem := make(chan struct{}, maxFederationConcurrency)
	ch := make(chan fanResult, len(engines))
	var wg sync.WaitGroup

	for _, eng := range engines {
		eng := eng
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}        // acquire slot
			defer func() { <-sem }() // release slot
			// Use RecallWithEvent so Feature 5 retrieval tracking is emitted
			// for cross-project queries too (#92). The event IDs are not returned
			// to the federated caller — they're stored per-project for local feedback.
			res, _, err := eng.RecallWithEvent(ctx, query, topK, detail)
			ch <- fanResult{res, err}
		}()
	}

	// Close ch once all goroutines have sent.
	go func() {
		wg.Wait()
		close(ch)
	}()

	// Collect, dedup by ID (keep highest score), then sort.
	seen := make(map[string]types.SearchResult, len(engines)*topK)
	for fan := range ch {
		if fan.err != nil {
			// Best-effort: log and skip failing engines rather than aborting the whole call.
			slog.Warn("federation: engine recall failed", "err", fan.err)
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
