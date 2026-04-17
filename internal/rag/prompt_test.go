package rag_test

import (
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"

	"github.com/petersimmons1972/engram/internal/rag"
)

// makeResult is a test helper that builds a SearchResult with the given
// excerpt text, memory ID, score, and creation timestamp.
func makeResult(id, excerpt string, score float64, createdAt time.Time) types.SearchResult {
	return types.SearchResult{
		Memory: &types.Memory{
			ID:        id,
			Content:   excerpt,
			CreatedAt: createdAt,
		},
		Score:        score,
		MatchedChunk: excerpt,
	}
}

// TestAssemblePrompt_ContainsQuestion verifies that the assembled prompt
// includes the caller's question verbatim.
func TestAssemblePrompt_ContainsQuestion(t *testing.T) {
	question := "What is a nanosecond?"
	chunk := makeResult("mem-001", "Light travels 11.8 inches in one nanosecond.", 0.9, time.Now())

	result := rag.AssemblePrompt(question, []types.SearchResult{chunk})

	require.Contains(t, result, question,
		"assembled prompt must contain the original question")
}

// TestAssemblePrompt_NumberedCitations verifies that the prompt contains
// numbered citation markers [1] and [2] when two chunks are supplied.
func TestAssemblePrompt_NumberedCitations(t *testing.T) {
	chunks := []types.SearchResult{
		makeResult("mem-001", "First excerpt.", 0.9, time.Now()),
		makeResult("mem-002", "Second excerpt.", 0.8, time.Now()),
	}

	result := rag.AssemblePrompt("Tell me something", chunks)

	require.True(t, strings.Contains(result, "[1]"),
		"prompt must contain citation marker [1]")
	require.True(t, strings.Contains(result, "[2]"),
		"prompt must contain citation marker [2]")
}

// TestAssemblePrompt_ContainsTimestamp verifies that the year from the chunk's
// CreatedAt timestamp appears somewhere in the assembled prompt.
func TestAssemblePrompt_ContainsTimestamp(t *testing.T) {
	ts := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	chunk := makeResult("mem-001", "Some memory content.", 0.85, ts)

	result := rag.AssemblePrompt("When was this stored?", []types.SearchResult{chunk})

	require.Contains(t, result, "2024",
		"assembled prompt must include the year from the chunk's CreatedAt")
}

// TestAssemblePrompt_EmptyChunks verifies graceful handling when no chunks
// are provided: the question must still appear in the output.
func TestAssemblePrompt_EmptyChunks(t *testing.T) {
	question := "Is there anything here?"

	result := rag.AssemblePrompt(question, []types.SearchResult{})

	require.Contains(t, result, question,
		"assembled prompt must contain the question even with no chunks")
}

// TestBuildCitations_MapsFields verifies that BuildCitations correctly maps
// two SearchResults to Citation values with correct ranks, IDs, scores,
// excerpts, and timestamps.
func TestBuildCitations_MapsFields(t *testing.T) {
	ts1 := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	ts2 := time.Date(2025, time.June, 15, 12, 0, 0, 0, time.UTC)

	chunks := []types.SearchResult{
		makeResult("mem-aaa", "First excerpt text.", 0.95, ts1),
		makeResult("mem-bbb", "Second excerpt text.", 0.75, ts2),
	}

	citations := rag.BuildCitations(chunks)

	require.Len(t, citations, 2, "BuildCitations must return one Citation per input chunk")

	// First citation — rank 1
	require.Equal(t, 1, citations[0].Rank, "first citation must have Rank=1")
	require.Equal(t, "mem-aaa", citations[0].MemoryID)
	require.Equal(t, "First excerpt text.", citations[0].Excerpt)
	require.InDelta(t, 0.95, citations[0].Score, 1e-9)
	require.Equal(t, ts1, citations[0].Timestamp)

	// Second citation — rank 2
	require.Equal(t, 2, citations[1].Rank, "second citation must have Rank=2")
	require.Equal(t, "mem-bbb", citations[1].MemoryID)
	require.Equal(t, "Second excerpt text.", citations[1].Excerpt)
	require.InDelta(t, 0.75, citations[1].Score, 1e-9)
	require.Equal(t, ts2, citations[1].Timestamp)
}

// TestBuildCitations_Empty verifies that an empty input yields an empty
// (non-nil) slice — not a nil return and not a panic.
func TestBuildCitations_Empty(t *testing.T) {
	citations := rag.BuildCitations([]types.SearchResult{})

	require.NotNil(t, citations, "BuildCitations must return a non-nil slice for empty input")
	require.Len(t, citations, 0, "BuildCitations must return an empty slice for empty input")
}
