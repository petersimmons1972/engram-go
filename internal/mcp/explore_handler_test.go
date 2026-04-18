package mcp

// Internal tests for handleMemoryExplore and parseExploreScope.
// Uses a no-op db.Backend stub + fake embedder so no PostgreSQL is required.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/entity"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// ── no-op db.Backend stub ────────────────────────────────────────────────────

// noopBackend implements db.Backend with all methods returning zero values.
// Used to create a SearchEngine without a real PostgreSQL instance.
type noopBackend struct{}

func (noopBackend) Close()                                                     {}
func (noopBackend) GetMeta(_ context.Context, _, _ string) (string, bool, error) { return "", false, nil }
func (noopBackend) SetMeta(_ context.Context, _, _, _ string) error            { return nil }
func (noopBackend) SetMetaTx(_ context.Context, _ db.Tx, _, _, _ string) error { return nil }
func (noopBackend) StoreMemory(_ context.Context, _ *types.Memory) error       { return nil }
func (noopBackend) StoreMemoryTx(_ context.Context, _ db.Tx, _ *types.Memory) error {
	return nil
}
func (noopBackend) GetMemory(_ context.Context, _ string) (*types.Memory, error) { return nil, nil }
func (noopBackend) GetMemoriesByIDs(_ context.Context, _ string, _ []string) ([]*types.Memory, error) {
	return nil, nil
}
func (noopBackend) UpdateMemory(_ context.Context, _ string, _ *string, _ []string, _ *int) (*types.Memory, error) {
	return nil, nil
}
func (noopBackend) DeleteMemory(_ context.Context, _ string) (bool, error)           { return false, nil }
func (noopBackend) DeleteMemoryAtomic(_ context.Context, _, _ string, _ bool) (bool, error) {
	return false, nil
}
func (noopBackend) MergeMemoriesAtomic(_ context.Context, _, _, _, _ string) error { return nil }
func (noopBackend) ListMemories(_ context.Context, _ string, _ db.ListOptions) ([]*types.Memory, error) {
	return nil, nil
}
func (noopBackend) TouchMemory(_ context.Context, _ string) error              { return nil }
func (noopBackend) TouchMemories(_ context.Context, _ []string) error          { return nil }
func (noopBackend) StoreChunks(_ context.Context, _ []*types.Chunk) error      { return nil }
func (noopBackend) StoreChunksTx(_ context.Context, _ db.Tx, _ []*types.Chunk) error { return nil }
func (noopBackend) GetChunksForMemory(_ context.Context, _ string) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) GetAllChunksWithEmbeddings(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) GetAllChunkTexts(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}
func (noopBackend) GetChunksForMemories(_ context.Context, _ []string) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) ChunkHashExists(_ context.Context, _, _ string) (bool, error) { return false, nil }
func (noopBackend) DeleteChunksForMemory(_ context.Context, _ string) error      { return nil }
func (noopBackend) DeleteChunksForMemoryTx(_ context.Context, _ db.Tx, _ string) error { return nil }
func (noopBackend) DeleteChunksByIDs(_ context.Context, _ []string) (int, error) { return 0, nil }
func (noopBackend) NullAllEmbeddings(_ context.Context, _ string) (int, error)   { return 0, nil }
func (noopBackend) NullAllEmbeddingsTx(_ context.Context, _ db.Tx, _ string) (int, error) {
	return 0, nil
}
func (noopBackend) GetChunksPendingEmbedding(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) UpdateChunkEmbedding(_ context.Context, _ string, _ []float32) (int, error) {
	return 0, nil
}
func (noopBackend) VectorSearch(_ context.Context, _ string, _ []float32, _ int) ([]db.VectorHit, error) {
	return nil, nil
}
func (noopBackend) ChunkEmbeddingDistance(_ context.Context, _, _ string) (float64, error) {
	return 2.0, nil
}
func (noopBackend) UpdateChunkLastMatched(_ context.Context, _ string) error  { return nil }
func (noopBackend) GetPendingEmbeddingCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (noopBackend) StoreRelationship(_ context.Context, _ *types.Relationship) error { return nil }
func (noopBackend) GetConnected(_ context.Context, _ string, _ int) ([]db.ConnectedResult, error) {
	return nil, nil
}
func (noopBackend) BoostEdgesForMemory(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (noopBackend) DecayEdgesForMemory(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (noopBackend) GetConnectionCount(_ context.Context, _, _ string) (int, error) { return 0, nil }
func (noopBackend) DecayAllEdges(_ context.Context, _ string, _, _ float64) (int, int, error) {
	return 0, 0, nil
}
func (noopBackend) DeleteRelationshipsForMemory(_ context.Context, _ string) error { return nil }
func (noopBackend) GetRelationships(_ context.Context, _, _ string) ([]types.Relationship, error) {
	return nil, nil
}
func (noopBackend) GetRelationshipsBatch(_ context.Context, _ string, _ []string) (map[string][]types.Relationship, error) {
	return nil, nil
}
func (noopBackend) GetMemoryHistory(_ context.Context, _, _ string) ([]*types.MemoryVersion, error) {
	return nil, nil
}
func (noopBackend) SoftDeleteMemory(_ context.Context, _, _, _ string) (bool, error) { return false, nil }
func (noopBackend) GetMemoriesAsOf(_ context.Context, _ string, _ time.Time, _ int) ([]*types.Memory, error) {
	return nil, nil
}
func (noopBackend) StoreRetrievalEvent(_ context.Context, _ *types.RetrievalEvent) error { return nil }
func (noopBackend) GetRetrievalEvent(_ context.Context, _ string) (*types.RetrievalEvent, error) {
	return nil, nil
}
func (noopBackend) RecordFeedback(_ context.Context, _ string, _ []string) error { return nil }
func (noopBackend) RecordFeedbackWithClass(_ context.Context, _ string, _ []string, _ string) error {
	return nil
}
func (noopBackend) AggregateMemories(_ context.Context, _, _, _ string, _ int) ([]types.AggregateRow, error) {
	return nil, nil
}
func (noopBackend) AggregateFailureClasses(_ context.Context, _ string, _ int) ([]types.AggregateRow, error) {
	return nil, nil
}
func (noopBackend) IncrementTimesRetrieved(_ context.Context, _ []string) error  { return nil }
func (noopBackend) UpdateDynamicImportance(_ context.Context, _ string, _, _ float64) error {
	return nil
}
func (noopBackend) SetNextReviewAt(_ context.Context, _ string, _ time.Time) error { return nil }
func (noopBackend) DecayStaleImportance(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (noopBackend) PruneStaleMemories(_ context.Context, _ string, _ float64, _ int) (int, error) {
	return 0, nil
}
func (noopBackend) PruneColdDocuments(_ context.Context, _ string, _ float64, _ int) (int, error) {
	return 0, nil
}
func (noopBackend) FTSSearch(_ context.Context, _, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	return nil, nil
}
func (noopBackend) RebuildFTS(_ context.Context) error                               { return nil }
func (noopBackend) GetStats(_ context.Context, _ string) (*types.MemoryStats, error) { return nil, nil }
func (noopBackend) ListAllProjects(_ context.Context) ([]string, error)              { return nil, nil }
func (noopBackend) GetAllMemoryIDs(_ context.Context, _ string) (map[string]struct{}, error) {
	return nil, nil
}
func (noopBackend) GetMemoryTypeMap(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil
}
func (noopBackend) GetMemoriesPendingSummary(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}
func (noopBackend) StoreSummary(_ context.Context, _, _ string) error { return nil }
func (noopBackend) GetPendingSummaryCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (noopBackend) ClearSummaries(_ context.Context, _ string) (int, error) { return 0, nil }
func (noopBackend) GetMemoriesMissingHash(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}
func (noopBackend) UpdateMemoryHash(_ context.Context, _, _ string) error { return nil }
func (noopBackend) ExistsWithContentHash(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (noopBackend) GetIntegrityStats(_ context.Context, _ string) (db.IntegrityStats, error) {
	return db.IntegrityStats{}, nil
}
func (noopBackend) StartEpisode(_ context.Context, _, _ string) (*types.Episode, error) {
	return nil, nil
}
func (noopBackend) EndEpisode(_ context.Context, _, _ string) error    { return nil }
func (noopBackend) ListEpisodes(_ context.Context, _ string, _ int) ([]*types.Episode, error) {
	return nil, nil
}
func (noopBackend) RecallEpisode(_ context.Context, _ string) ([]*types.Memory, error) {
	return nil, nil
}
func (noopBackend) Begin(_ context.Context) (db.Tx, error) { return nil, nil }
func (noopBackend) SearchChunksWithinMemory(_ context.Context, _ []float32, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) StoreDocument(_ context.Context, _, _ string) (string, error) { return "", nil }
func (noopBackend) GetDocument(_ context.Context, _ string) (string, error)      { return "", nil }
func (noopBackend) SetMemoryDocumentID(_ context.Context, _, _ string) error     { return nil }

func (noopBackend) UpsertEntity(_ context.Context, _ *entity.Entity) (string, error) {
	return "", nil
}
func (noopBackend) GetEntitiesByProject(_ context.Context, _ string) ([]entity.Entity, error) {
	return nil, nil
}
func (noopBackend) EnqueueExtractionJob(_ context.Context, _, _ string) error { return nil }
func (noopBackend) ClaimExtractionJobs(_ context.Context, _ string, _ int) ([]db.ExtractionJob, error) {
	return nil, nil
}
func (noopBackend) CompleteExtractionJob(_ context.Context, _ string, _ error) error { return nil }

var _ db.Backend = noopBackend{}

// ── fake embedder ─────────────────────────────────────────────────────────────

type noopEmbedder struct{}

func (noopEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, 384), nil
}
func (noopEmbedder) Name() string    { return "noop" }
func (noopEmbedder) Dimensions() int { return 384 }

var _ embed.Client = noopEmbedder{}

// ── helpers ───────────────────────────────────────────────────────────────────

// newTestNoopPool builds an EnginePool backed by a noopBackend + noopEmbedder.
// Suitable for unit tests that do not require real search results.
func newTestNoopPool(t *testing.T) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, noopBackend{}, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// ── TestHandleMemoryExplore_EmptyQuestion ────────────────────────────────────

// TestHandleMemoryExplore_EmptyQuestion verifies that the handler returns a
// tool error (isError:true content) when question is empty, without making any
// backend or Claude API calls.
func TestHandleMemoryExplore_EmptyQuestion(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":  "test",
		"question": "",
	}

	result, err := handleMemoryExplore(context.Background(), pool, req, Config{})
	require.NoError(t, err, "handler must not return a Go error for empty question")
	require.NotNil(t, result)
	require.True(t, result.IsError, "expected tool-error result for empty question")
}

// ── TestHandleMemoryExplore_HappyPath ────────────────────────────────────────

// TestHandleMemoryExplore_HappyPath registers a minimal Claude API stub, calls
// the handler with a valid question and project, and asserts a non-error response
// with a non-empty answer field.
func TestHandleMemoryExplore_HappyPath(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			// Scoring call: high confidence, stop immediately.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"content": []map[string]string{{"type": "text", "text": `{"confidence":0.9,"gaps":[],"refined_query":null,"stop":true}`}},
				"usage":   map[string]int{"input_tokens": 10, "output_tokens": 5},
			})
		} else {
			// Synthesis call.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"content": []map[string]string{{"type": "text", "text": "The answer is 42."}},
				"usage":   map[string]int{"input_tokens": 10, "output_tokens": 5},
			})
		}
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":  "test",
		"question": "what is the answer?",
	}

	result, err := handleMemoryExplore(context.Background(), pool, req, Config{
		claudeClient: c,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "unexpected tool error: %+v", result.Content)

	require.NotEmpty(t, result.Content, "expected at least one content item")
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &out))
	answer, ok := out["answer"].(string)
	require.True(t, ok, "answer field missing or wrong type")
	require.NotEmpty(t, answer, "answer must be non-empty")
}

// ── TestParseExploreScope_PopulatedScope ─────────────────────────────────────

// TestParseExploreScope_PopulatedScope verifies that all scope fields are
// parsed correctly from a fully-populated args map.
func TestParseExploreScope_PopulatedScope(t *testing.T) {
	since := "2024-01-01T00:00:00Z"
	until := "2024-06-30T23:59:59Z"

	args := map[string]any{
		"scope": map[string]any{
			"tags":       []any{"important", "project-a"},
			"episode_id": "ep-abc123",
			"since":      since,
			"until":      until,
		},
	}

	scope := parseExploreScope(args)

	require.Equal(t, []string{"important", "project-a"}, scope.Tags)
	require.Equal(t, "ep-abc123", scope.EpisodeID)

	require.NotNil(t, scope.Since, "Since must be parsed")
	wantSince, err := time.Parse(time.RFC3339, since)
	require.NoError(t, err)
	require.True(t, scope.Since.Equal(wantSince), "Since mismatch: got %v, want %v", scope.Since, wantSince)

	require.NotNil(t, scope.Until, "Until must be parsed")
	wantUntil, err := time.Parse(time.RFC3339, until)
	require.NoError(t, err)
	require.True(t, scope.Until.Equal(wantUntil), "Until mismatch: got %v, want %v", scope.Until, wantUntil)
}
