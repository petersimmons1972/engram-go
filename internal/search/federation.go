package search

import (
	"context"
	"fmt"
	"log/slog"
	"math"
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
	return RecallAcrossEnginesWithEventsAndOpts(ctx, engines, query, topK, detail, RecallOpts{}, true)
}

// RecallAcrossEnginesWithEvents is RecallAcrossEngines with explicit retrieval
// telemetry control. When recordEvents is false, federation suppresses
// retrieval-event writes but still follows the underlying recall mode's normal
// access-heat behavior.
func RecallAcrossEnginesWithEvents(ctx context.Context, engines []*SearchEngine, query string, topK int, detail string, recordEvents bool) ([]types.SearchResult, error) {
	return RecallAcrossEnginesWithEventsAndOpts(ctx, engines, query, topK, detail, RecallOpts{}, recordEvents)
}

// RecallAcrossEnginesWithEventsAndOpts is RecallAcrossEnginesWithEvents with
// explicit RecallOpts propagation so federated callers can preserve the same
// response-mode and post-processing semantics as single-project recall.
func RecallAcrossEnginesWithEventsAndOpts(ctx context.Context, engines []*SearchEngine, query string, topK int, detail string, opts RecallOpts, recordEvents bool) ([]types.SearchResult, error) {
	if len(engines) == 0 {
		return nil, nil
	}
	if len(engines) == 1 {
		if recordEvents {
			results, _, err := engines[0].RecallWithEventAndOpts(ctx, query, topK, detail, opts)
			return results, err
		}
		return engines[0].RecallWithOpts(ctx, query, topK, detail, opts)
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
			var res []types.SearchResult
			var err error
			if recordEvents {
				// Use RecallWithEventAndOpts so cross-project callers keep the same
				// retrieval-tracking behavior while preserving mode-specific fast paths.
				res, _, err = eng.RecallWithEventAndOpts(ctx, query, topK, detail, opts)
			} else {
				res, err = eng.RecallWithOpts(ctx, query, topK, detail, opts)
			}
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
	failed := 0
	for fan := range ch {
		if fan.err != nil {
			failed++
			// Best-effort: log and skip failing engines rather than aborting the whole call.
			slog.Warn("federation: engine recall failed", "err", fan.err)
			continue
		}
		for _, r := range fan.results {
			if existing, ok := seen[r.Memory.ID]; !ok || federatedScoreGreater(r.Score, existing.Score) {
				seen[r.Memory.ID] = r
			}
		}
	}
	if failed == len(engines) {
		return nil, fmt.Errorf("federated recall failed across %d engines", failed)
	}

	merged := make([]types.SearchResult, 0, len(seen))
	for _, r := range seen {
		merged = append(merged, r)
	}
	sort.Slice(merged, func(i, j int) bool {
		return federatedScoreGreater(merged[i].Score, merged[j].Score)
	})

	if topK > 0 && len(merged) > topK {
		merged = merged[:topK]
	}
	return merged, nil
}

func federatedScoreGreater(a, b float64) bool {
	if math.IsNaN(a) {
		return false
	}
	if math.IsNaN(b) {
		return true
	}
	return a > b
}
