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

// FailedFederatedProject captures one federated Recall failure for error
// attribution and API-level metadata.
type FailedFederatedProject struct {
	Project string
	Error   string
}

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
	results, _, err := RecallAcrossEnginesWithEventsAndOpts(ctx, engines, query, topK, detail, RecallOpts{}, true)
	return results, err
}

// RecallAcrossEnginesWithEventsAndOpts is RecallAcrossEnginesWithEvents with
// explicit RecallOpts propagation so federated callers can preserve the same
// response-mode and post-processing semantics as single-project recall.
//
// On a successful recall (even partial), it returns merged results plus metadata
// for engines that failed to execute. If every engine fails, results is nil and
// err is non-nil.
func RecallAcrossEnginesWithEventsAndOpts(ctx context.Context, engines []*SearchEngine, query string, topK int, detail string, opts RecallOpts, recordEvents bool) ([]types.SearchResult, []FailedFederatedProject, error) {
	if len(engines) == 0 {
		return nil, nil, nil
	}
	if len(engines) == 1 {
		if recordEvents {
			results, _, err := engines[0].RecallWithEventAndOpts(ctx, query, topK, detail, opts)
			return results, nil, err
		}
		results, err := engines[0].RecallWithOpts(ctx, query, topK, detail, opts)
		return results, nil, err
	}

	type fanResult struct {
		results []types.SearchResult
		err     error
		project string
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
			ch <- fanResult{results: res, err: err, project: eng.Project()}
		}()
	}

	// Close ch once all goroutines have sent.
	go func() {
		wg.Wait()
		close(ch)
	}()

	// Collect, dedup by ID (keep highest score), then sort.
	seen := make(map[string]types.SearchResult, len(engines)*topK)
	failed := make([]FailedFederatedProject, 0, len(engines))
	failedCount := 0
	for fan := range ch {
		if fan.err != nil {
			failedCount++
			failed = append(failed, FailedFederatedProject{Project: fan.project, Error: fan.err.Error()})
			// Best-effort: log and skip failing engines rather than aborting the whole call.
			slog.Warn("federation: engine recall failed", "project", fan.project, "err", fan.err)
			continue
		}
		for _, r := range fan.results {
			if existing, ok := seen[r.Memory.ID]; !ok || federatedScoreGreater(r.Score, existing.Score) {
				seen[r.Memory.ID] = r
			}
		}
	}
	if failedCount == len(engines) {
		return nil, failed, fmt.Errorf("federated recall failed across %d engines", failedCount)
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
	if len(failed) > 0 {
		return merged, failed, nil
	}
	return merged, nil, nil
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
