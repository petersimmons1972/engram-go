package rag

import (
	"sort"

	"github.com/petersimmons1972/engram/internal/types"
)

// ContextBudget limits how many chunks are assembled into a prompt by token cost.
type ContextBudget struct {
	MaxTokens int
}

// Trim returns the highest-scoring subset of results that fit within MaxTokens.
// Token estimate: len(result.MatchedChunk) / 4 per chunk.
// Results are returned in their original rank order (index order from the input).
func (b ContextBudget) Trim(results []types.SearchResult) []types.SearchResult {
	if b.MaxTokens <= 0 || len(results) == 0 {
		return []types.SearchResult{}
	}

	// Build an index-aware slice so we can restore input order after selecting.
	type indexed struct {
		idx    int
		result types.SearchResult
	}

	items := make([]indexed, len(results))
	for i, r := range results {
		items[i] = indexed{idx: i, result: r}
	}

	// Sort by score descending to greedily pick highest-value chunks first.
	sort.Slice(items, func(i, j int) bool {
		return items[i].result.Score > items[j].result.Score
	})

	// Greedily accumulate until budget exhausted.
	remaining := b.MaxTokens
	selected := make([]indexed, 0, len(items))
	for i := range items {
		cost := len(items[i].result.MatchedChunk) / 4
		if cost < 1 {
			cost = 1
		}
		if cost > remaining {
			continue
		}
		remaining -= cost
		selected = append(selected, items[i])
	}

	if len(selected) == 0 {
		return []types.SearchResult{}
	}

	// Restore original input order (sort selected by original index).
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].idx < selected[j].idx
	})

	out := make([]types.SearchResult, len(selected))
	for i, item := range selected {
		out[i] = item.result
	}
	return out
}
