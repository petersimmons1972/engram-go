package search_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func TestToHandles_HappyPath(t *testing.T) {
	sum := "A useful memory about foo"
	results := []types.SearchResult{
		{
			Memory: &types.Memory{
				ID:          "abc-1",
				Project:     "proj",
				Summary:     &sum,
				StorageMode: "focused",
				Content:     "full content here",
			},
			Score: 0.85,
		},
	}
	handles := search.ToHandles(results)
	require.Len(t, handles, 1)
	h := handles[0]
	require.Equal(t, "abc-1", h.ID)
	require.Equal(t, "proj", h.Project)
	require.Equal(t, sum, h.Summary)
	require.InDelta(t, 0.85, h.Score, 0.001)
	require.Equal(t, "focused", h.StorageMode)
	require.Equal(t, len("full content here"), h.Bytes)
	require.True(t, h.IsHandle)
	require.NotEmpty(t, h.FetchHint)
}

func TestToHandles_EmptyInput(t *testing.T) {
	require.Empty(t, search.ToHandles(nil))
	require.Empty(t, search.ToHandles([]types.SearchResult{}))
}

func TestToHandles_NilSummaryBecomesEmptyString(t *testing.T) {
	results := []types.SearchResult{
		{Memory: &types.Memory{ID: "x", Project: "p"}, Score: 0.5},
	}
	handles := search.ToHandles(results)
	require.Len(t, handles, 1)
	require.Equal(t, "", handles[0].Summary)
}

func TestToHandles_NilMemorySkipped(t *testing.T) {
	results := []types.SearchResult{
		{Memory: nil, Score: 0.9},
		{Memory: &types.Memory{ID: "y", Project: "p"}, Score: 0.3},
	}
	handles := search.ToHandles(results)
	require.Len(t, handles, 1)
	require.Equal(t, "y", handles[0].ID)
}
