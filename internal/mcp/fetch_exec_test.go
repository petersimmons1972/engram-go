package mcp

// Internal tests for execFetch — the testable core of handleMemoryFetch.
// Uses fakeFetcher (implements backendFetcher) to avoid needing a real Postgres.

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

type fakeFetcher struct {
	mem      *types.Memory
	chunks   []*types.Chunk
	memErr   error
	chunkErr error
}

func (f *fakeFetcher) GetMemoryByID(_ context.Context, _ string) (*types.Memory, error) {
	return f.mem, f.memErr
}

func (f *fakeFetcher) GetChunksForMemory(_ context.Context, _ string) ([]*types.Chunk, error) {
	return f.chunks, f.chunkErr
}

func TestExecFetch_SummaryDetail(t *testing.T) {
	sum := "A brief summary"
	ff := &fakeFetcher{
		mem: &types.Memory{
			ID:      "m1",
			Summary: &sum,
			Content: "full content that should not appear",
		},
	}
	result, err := execFetch(context.Background(), ff, "m1", "summary", 65536, nil)
	require.NoError(t, err)
	require.Equal(t, "m1", result["id"])
	require.Equal(t, "A brief summary", result["summary"])
	_, hasContent := result["content"]
	require.False(t, hasContent, "summary detail must not include full content")
}

func TestExecFetch_SummaryDetail_NilSummary(t *testing.T) {
	ff := &fakeFetcher{
		mem: &types.Memory{ID: "m2", Summary: nil, Content: "body"},
	}
	result, err := execFetch(context.Background(), ff, "m2", "summary", 65536, nil)
	require.NoError(t, err)
	require.Equal(t, "", result["summary"])
}

func TestExecFetch_IDNotFound(t *testing.T) {
	ff := &fakeFetcher{mem: nil}
	_, err := execFetch(context.Background(), ff, "missing", "full", 65536, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestExecFetch_FullDetailByteCapApplied(t *testing.T) {
	bigContent := string(make([]byte, 200_000))
	ff := &fakeFetcher{
		mem: &types.Memory{ID: "m3", Content: bigContent},
	}
	result, err := execFetch(context.Background(), ff, "m3", "full", 65536, nil)
	require.NoError(t, err)
	content, ok := result["content"].(string)
	require.True(t, ok)
	require.LessOrEqual(t, len(content), 65536)
	truncated, _ := result["truncated"].(bool)
	require.True(t, truncated)
}

func TestExecFetch_FullDetailUnderCap_NotTruncated(t *testing.T) {
	ff := &fakeFetcher{
		mem: &types.Memory{ID: "m4", Content: "short"},
	}
	result, err := execFetch(context.Background(), ff, "m4", "full", 65536, nil)
	require.NoError(t, err)
	require.Equal(t, "short", result["content"])
	truncated, _ := result["truncated"].(bool)
	require.False(t, truncated)
}

func TestExecFetch_ChunkDetail_AllChunks(t *testing.T) {
	ff := &fakeFetcher{
		mem: &types.Memory{ID: "m5"},
		chunks: []*types.Chunk{
			{ID: "c1", ChunkIndex: 0, ChunkText: "chunk zero"},
			{ID: "c2", ChunkIndex: 1, ChunkText: "chunk one"},
		},
	}
	result, err := execFetch(context.Background(), ff, "m5", "chunk", 65536, nil)
	require.NoError(t, err)
	chunks, ok := result["chunks"].([]*types.Chunk)
	require.True(t, ok)
	require.Len(t, chunks, 2)
}

func TestExecFetch_ChunkDetail_FilterByIDs(t *testing.T) {
	ff := &fakeFetcher{
		mem: &types.Memory{ID: "m6"},
		chunks: []*types.Chunk{
			{ID: "c1", ChunkIndex: 0, ChunkText: "zero"},
			{ID: "c2", ChunkIndex: 1, ChunkText: "one"},
			{ID: "c3", ChunkIndex: 2, ChunkText: "two"},
		},
	}
	result, err := execFetch(context.Background(), ff, "m6", "chunk", 65536, []string{"c1", "c3"})
	require.NoError(t, err)
	chunks, ok := result["chunks"].([]*types.Chunk)
	require.True(t, ok)
	require.Len(t, chunks, 2)
	ids := []string{chunks[0].ID, chunks[1].ID}
	require.ElementsMatch(t, []string{"c1", "c3"}, ids)
}
