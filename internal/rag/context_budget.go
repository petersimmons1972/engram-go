package rag

import (
	"sort"

	"github.com/petersimmons1972/engram/internal/types"
)

// ContextBudget limits how many chunks are assembled into a prompt by token cost.
type ContextBudget struct {
	MaxTokens int
}

// tokenCost estimates the token cost of a string as len(s)/4, floored at 1.
func tokenCost(s string) int {
	c := len(s) / 4
	if c < 1 {
		return 1
	}
	return c
}

// Trim greedily selects results in descending-score order until MaxTokens is
// exhausted, then returns the selected results in their original input order.
// Token estimate: tokenCost(result.MatchedChunk) per chunk.
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
		cost := tokenCost(items[i].result.MatchedChunk)
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
