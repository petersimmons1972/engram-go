package rag_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/rag"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// Token estimate used throughout: len(result.MatchedChunk) / 4

// TestContextBudget_KeepsHighestScoring verifies that when the budget fits the
// top 2 highest-scoring chunks but not all 3, exactly those 2 are returned in
// their original rank (input-index) order.
func TestContextBudget_KeepsHighestScoring(t *testing.T) {
	results := []types.SearchResult{
		{MatchedChunk: "aaaaaaaaaa", Score: 0.9},         // 10 chars = 2 tokens  (idx 0)
		{MatchedChunk: "bbbbbbbbbbbbbbbbbbbb", Score: 0.5}, // 20 chars = 5 tokens  (idx 1)
		{MatchedChunk: "cccccccc", Score: 0.8},            // 8 chars  = 2 tokens  (idx 2)
	}
	// Tokens: idx0=2, idx1=5, idx2=2. Budget=6: idx0+idx1=7>6, idx0+idx2=4<=6.
	// Top 2 by score that fit: idx0(0.9) + idx2(0.8) = 4 tokens <= 6.
	b := rag.ContextBudget{MaxTokens: 6}
	got := b.Trim(results)
	require.Len(t, got, 2)
	// Rank order preserved: idx0 before idx2.
	require.Equal(t, results[0].MatchedChunk, got[0].MatchedChunk)
	require.Equal(t, results[2].MatchedChunk, got[1].MatchedChunk)
}

// TestContextBudget_OverflowDropsLowest verifies that when only 1 chunk fits
// the budget, the highest-scoring chunk is kept and the others are dropped.
func TestContextBudget_OverflowDropsLowest(t *testing.T) {
	// idx0: 40 chars = 10 tokens, score=0.3
	// idx1: 8 chars  = 2 tokens,  score=0.9
	// idx2: 20 chars = 5 tokens,  score=0.6
	// Budget = 3 tokens: only idx1 (2 tokens) fits individually.
	results := []types.SearchResult{
		{MatchedChunk: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Score: 0.3}, // 40 chars
		{MatchedChunk: "bbbbbbbb", Score: 0.9},                                  // 8 chars
		{MatchedChunk: "cccccccccccccccccccc", Score: 0.6},                      // 20 chars
	}
	b := rag.ContextBudget{MaxTokens: 3}
	got := b.Trim(results)
	require.Len(t, got, 1)
	require.Equal(t, results[1].MatchedChunk, got[0].MatchedChunk)
}

// TestContextBudget_ZeroBudget verifies that a MaxTokens of 0 returns an empty
// slice regardless of input.
func TestContextBudget_ZeroBudget(t *testing.T) {
	results := []types.SearchResult{
		{MatchedChunk: "hello", Score: 0.9},
	}
	b := rag.ContextBudget{MaxTokens: 0}
	got := b.Trim(results)
	require.Empty(t, got)
}

// TestContextBudget_SingleChunk verifies that a single chunk that fits within
// the budget is returned as-is.
func TestContextBudget_SingleChunk(t *testing.T) {
	// "helloworld" = 10 chars = 2 tokens; budget = 5.
	results := []types.SearchResult{
		{MatchedChunk: "helloworld", Score: 0.7},
	}
	b := rag.ContextBudget{MaxTokens: 5}
	got := b.Trim(results)
	require.Len(t, got, 1)
	require.Equal(t, results[0].MatchedChunk, got[0].MatchedChunk)
}

// TestContextBudget_PreservesRankOrder verifies that when all chunks fit in the
// budget, the output order matches the input rank order (not sorted by score).
func TestContextBudget_PreservesRankOrder(t *testing.T) {
	// All 3 fit: 2+2+2 = 6 tokens <= 20 budget.
	// Scores are deliberately non-monotonic to catch sorting bugs.
	results := []types.SearchResult{
		{MatchedChunk: "aaaaaaaa", Score: 0.5}, // idx 0, 2 tokens
		{MatchedChunk: "bbbbbbbb", Score: 0.9}, // idx 1, 2 tokens
		{MatchedChunk: "cccccccc", Score: 0.1}, // idx 2, 2 tokens
	}
	b := rag.ContextBudget{MaxTokens: 20}
	got := b.Trim(results)
	require.Len(t, got, 3)
	require.Equal(t, results[0].MatchedChunk, got[0].MatchedChunk)
	require.Equal(t, results[1].MatchedChunk, got[1].MatchedChunk)
	require.Equal(t, results[2].MatchedChunk, got[2].MatchedChunk)
}

// TestContextBudget_EmptyInput verifies that an empty input slice produces an
// empty output slice without panicking.
func TestContextBudget_EmptyInput(t *testing.T) {
	b := rag.ContextBudget{MaxTokens: 100}
	got := b.Trim([]types.SearchResult{})
	require.Empty(t, got)
}
