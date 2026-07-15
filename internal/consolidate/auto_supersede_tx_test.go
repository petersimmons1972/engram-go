package consolidate_test

// Unit tests for AutoSupersede transaction atomicity.
// Uses a hand-rolled fake backend so no PostgreSQL is needed.
// These tests verify that when SoftDeleteMemoryTx returns an error,
// the Rollback path is taken and StoreRelationshipTx's write is reverted.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/consolidate"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/entity"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// ── minimal fake Tx ───────────────────────────────────────────────────────────

type fakeTx struct {
	mu        sync.Mutex
	committed bool
	rolled    bool
}

func (t *fakeTx) Commit(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.committed = true
	return nil
}

func (t *fakeTx) Rollback(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rolled = true
	return nil
}

// ── minimal fake Backend ──────────────────────────────────────────────────────

// autoSupersedeStub is the smallest db.Backend implementation needed to drive
// AutoSupersede through the transaction path.
type autoSupersedeStub struct {
	// memories indexed by ID, returned by GetMemory.
	memories map[string]*types.Memory
	// relationships returned by GetRelationships, indexed by memoryID.
	rels map[string][]types.Relationship
	// tx is the fake transaction returned by Begin.
	tx *fakeTx
	// storedRels accumulates calls to StoreRelationshipTx (only committed if
	// Commit is called by the runner).
	mu         sync.Mutex
	storedRels []*types.Relationship
	// softDeleteTxErr, if non-nil, is returned by SoftDeleteMemoryTx.
	softDeleteTxErr error
}

func (b *autoSupersedeStub) Begin(_ context.Context) (db.Tx, error) {
	b.tx = &fakeTx{}
	return b.tx, nil
}
func (b *autoSupersedeStub) GetAllMemoryIDs(_ context.Context, _ string) (map[string]struct{}, error) {
	ids := make(map[string]struct{}, len(b.memories))
	for id := range b.memories {
		ids[id] = struct{}{}
	}
	return ids, nil
}
func (b *autoSupersedeStub) GetRelationships(_ context.Context, _, memID string) ([]types.Relationship, error) {
	return b.rels[memID], nil
}
func (b *autoSupersedeStub) GetMemory(_ context.Context, id string) (*types.Memory, error) {
	return b.memories[id], nil
}
func (b *autoSupersedeStub) StoreRelationshipTx(_ context.Context, _ db.Tx, rel *types.Relationship) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.storedRels = append(b.storedRels, rel)
	return nil
}
func (b *autoSupersedeStub) SoftDeleteMemoryTx(_ context.Context, _ db.Tx, _, _, _ string) (bool, error) {
	return false, b.softDeleteTxErr
}

// ── all the remaining interface methods as no-ops ────────────────────────────
func (b *autoSupersedeStub) Close() {}
func (b *autoSupersedeStub) GetMeta(_ context.Context, _, _ string) (string, bool, error) {
	return "", false, nil
}
func (b *autoSupersedeStub) SetMeta(_ context.Context, _, _, _ string) error            { return nil }
func (b *autoSupersedeStub) SetMetaTx(_ context.Context, _ db.Tx, _, _, _ string) error { return nil }
func (b *autoSupersedeStub) StoreMemory(_ context.Context, _ *types.Memory) error       { return nil }
func (b *autoSupersedeStub) StoreMemoryTx(_ context.Context, _ db.Tx, _ *types.Memory) error {
	return nil
}
func (b *autoSupersedeStub) GetMemoryByID(_ context.Context, _ string) (*types.Memory, error) {
	return nil, nil
}
func (b *autoSupersedeStub) GetMemoryByIDInProject(_ context.Context, _, _ string) (*types.Memory, error) {
	return nil, nil
}
func (b *autoSupersedeStub) GetMemoriesByIDs(_ context.Context, _ string, _ []string) ([]*types.Memory, error) {
	return nil, nil
}
func (b *autoSupersedeStub) UpdateMemory(_ context.Context, _ string, _ *string, _ []string, _ *int, _ *float64) (*types.Memory, error) {
	return nil, nil
}
func (b *autoSupersedeStub) DeleteMemory(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (b *autoSupersedeStub) DeleteMemoryAtomic(_ context.Context, _, _ string, _ bool) (bool, error) {
	return false, nil
}
func (b *autoSupersedeStub) MergeMemoriesAtomic(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (b *autoSupersedeStub) ListMemories(_ context.Context, _ string, _ db.ListOptions) ([]*types.Memory, error) {
	return nil, nil
}
func (b *autoSupersedeStub) TouchMemory(_ context.Context, _ string) error         { return nil }
func (b *autoSupersedeStub) TouchMemories(_ context.Context, _ []string) error     { return nil }
func (b *autoSupersedeStub) StoreChunks(_ context.Context, _ []*types.Chunk) error { return nil }
func (b *autoSupersedeStub) StoreChunksTx(_ context.Context, _ db.Tx, _ []*types.Chunk) error {
	return nil
}
func (b *autoSupersedeStub) GetChunksForMemory(_ context.Context, _ string) ([]*types.Chunk, error) {
	return nil, nil
}
func (b *autoSupersedeStub) GetAllChunksWithEmbeddings(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (b *autoSupersedeStub) GetAllChunkTexts(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}
func (b *autoSupersedeStub) GetChunksForMemories(_ context.Context, _ []string) ([]*types.Chunk, error) {
	return nil, nil
}
func (b *autoSupersedeStub) ChunkHashExists(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (b *autoSupersedeStub) DeleteChunksForMemory(_ context.Context, _ string) error { return nil }
func (b *autoSupersedeStub) DeleteChunksForMemoryTx(_ context.Context, _ db.Tx, _ string) error {
	return nil
}
func (b *autoSupersedeStub) DeleteChunksByIDs(_ context.Context, _ []string) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) NullAllEmbeddings(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) NullAllEmbeddingsTx(_ context.Context, _ db.Tx, _ string) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) CountProjectChunks(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) GetChunksPendingEmbedding(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (b *autoSupersedeStub) UpdateChunkEmbedding(_ context.Context, _ string, _ []float32) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) VectorSearch(_ context.Context, _ string, _ []float32, _ int) ([]db.VectorHit, error) {
	return nil, nil
}
func (b *autoSupersedeStub) VectorSearchWithDateRange(_ context.Context, _ string, _ []float32, _ int, _, _ *time.Time) ([]db.VectorHit, error) {
	return nil, nil
}
func (b *autoSupersedeStub) ChunkEmbeddingDistance(_ context.Context, _, _ string) (float64, error) {
	return 0, nil
}
func (b *autoSupersedeStub) UpdateChunkLastMatched(_ context.Context, _ string) error { return nil }
func (b *autoSupersedeStub) GetPendingEmbeddingCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) EnqueueChunkLeases(_ context.Context, _ []string) error { return nil }
func (b *autoSupersedeStub) StoreRelationship(_ context.Context, _ *types.Relationship) error {
	return nil
}
func (b *autoSupersedeStub) GetConnected(_ context.Context, _ string, _ int) ([]db.ConnectedResult, error) {
	return nil, nil
}
func (b *autoSupersedeStub) BoostEdgesForMemory(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) DecayEdgesForMemory(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) GetConnectionCount(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) DecayAllEdges(_ context.Context, _ string, _, _ float64) (int, int, error) {
	return 0, 0, nil
}
func (b *autoSupersedeStub) DeleteRelationshipsForMemory(_ context.Context, _ string) error {
	return nil
}
func (b *autoSupersedeStub) GetRelationshipsBatch(_ context.Context, _ string, _ []string) (map[string][]types.Relationship, error) {
	return nil, nil
}
func (b *autoSupersedeStub) GetMemoryHistory(_ context.Context, _, _ string) ([]*types.MemoryVersion, error) {
	return nil, nil
}
func (b *autoSupersedeStub) SoftDeleteMemory(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}
func (b *autoSupersedeStub) GetMemoriesAsOf(_ context.Context, _ string, _ time.Time, _ int) ([]*types.Memory, error) {
	return nil, nil
}
func (b *autoSupersedeStub) StoreRetrievalEvent(_ context.Context, _ *types.RetrievalEvent) error {
	return nil
}
func (b *autoSupersedeStub) GetRetrievalEvent(_ context.Context, _ string) (*types.RetrievalEvent, error) {
	return nil, nil
}
func (b *autoSupersedeStub) RecordFeedback(_ context.Context, _ string, _ []string) error {
	return nil
}
func (b *autoSupersedeStub) RecordFeedbackWithClass(_ context.Context, _ string, _ []string, _ string) error {
	return nil
}
func (b *autoSupersedeStub) AggregateMemories(_ context.Context, _, _, _ string, _ int) ([]types.AggregateRow, error) {
	return nil, nil
}
func (b *autoSupersedeStub) AggregateFailureClasses(_ context.Context, _ string, _ int) ([]types.AggregateRow, error) {
	return nil, nil
}
func (b *autoSupersedeStub) IncrementTimesRetrieved(_ context.Context, _ []string) error { return nil }
func (b *autoSupersedeStub) UpdateDynamicImportance(_ context.Context, _ string, _, _ float64) error {
	return nil
}
func (b *autoSupersedeStub) SetNextReviewAt(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (b *autoSupersedeStub) DecayStaleImportance(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) PruneStaleMemories(_ context.Context, _ string, _ float64, _ int) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) PruneColdDocuments(_ context.Context, _ string, _ float64, _ int) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) DeleteProject(_ context.Context, _ string) error { return nil }
func (b *autoSupersedeStub) SetProjectTTL(_ context.Context, _ string, _ time.Time, _ *time.Time) error {
	return nil
}
func (b *autoSupersedeStub) ListExpiredProjects(_ context.Context, _ string, _ time.Time, _ int) ([]string, error) {
	return nil, nil
}
func (b *autoSupersedeStub) FTSSearch(_ context.Context, _, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	return nil, nil
}
func (b *autoSupersedeStub) RebuildFTS(_ context.Context) error { return nil }
func (b *autoSupersedeStub) GetStats(_ context.Context, _ string) (*types.MemoryStats, error) {
	return nil, nil
}
func (b *autoSupersedeStub) ListAllProjects(_ context.Context) ([]string, error) { return nil, nil }
func (b *autoSupersedeStub) GetMemoryTypeMap(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil
}
func (b *autoSupersedeStub) GetMemoriesPendingSummary(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}
func (b *autoSupersedeStub) StoreSummary(_ context.Context, _, _ string) error { return nil }
func (b *autoSupersedeStub) GetPendingSummaryCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (b *autoSupersedeStub) ClearSummaries(_ context.Context, _ string) (int, error) { return 0, nil }
func (b *autoSupersedeStub) GetMemoriesMissingHash(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}
func (b *autoSupersedeStub) UpdateMemoryHash(_ context.Context, _, _ string) error { return nil }
func (b *autoSupersedeStub) ExistsWithContentHash(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (b *autoSupersedeStub) GetIntegrityStats(_ context.Context, _ string) (db.IntegrityStats, error) {
	return db.IntegrityStats{}, nil
}
func (b *autoSupersedeStub) StartEpisode(_ context.Context, _, _ string) (*types.Episode, error) {
	return nil, nil
}
func (b *autoSupersedeStub) EndEpisode(_ context.Context, _, _ string) error { return nil }
func (b *autoSupersedeStub) ListEpisodes(_ context.Context, _ string, _ int) ([]*types.Episode, error) {
	return nil, nil
}
func (b *autoSupersedeStub) RecallEpisode(_ context.Context, _ string) ([]*types.Memory, error) {
	return nil, nil
}
func (b *autoSupersedeStub) CloseStaleEpisodes(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
func (b *autoSupersedeStub) SearchChunksWithinMemory(_ context.Context, _ []float32, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (b *autoSupersedeStub) StoreDocument(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (b *autoSupersedeStub) DeleteDocument(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (b *autoSupersedeStub) DeleteDocumentTx(_ context.Context, _ db.Tx, _ string) (bool, error) {
	return false, nil
}
func (b *autoSupersedeStub) DeleteOrphanedDocumentTx(_ context.Context, _ db.Tx, _ string) (bool, error) {
	return false, nil
}
func (b *autoSupersedeStub) GetDocument(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (b *autoSupersedeStub) SetMemoryDocumentID(_ context.Context, _, _ string) error { return nil }
func (b *autoSupersedeStub) UpsertEntity(_ context.Context, _ *entity.Entity) (string, error) {
	return "", nil
}
func (b *autoSupersedeStub) GetEntitiesByProject(_ context.Context, _ string) ([]entity.Entity, error) {
	return nil, nil
}
func (b *autoSupersedeStub) EnqueueExtractionJob(_ context.Context, _, _ string) error { return nil }
func (b *autoSupersedeStub) ClaimExtractionJobs(_ context.Context, _ string, _ int) ([]db.ExtractionJob, error) {
	return nil, nil
}
func (b *autoSupersedeStub) CompleteExtractionJob(_ context.Context, _ string, _ error) error {
	return nil
}
func (b *autoSupersedeStub) StoreMemoryCluster(_ context.Context, _ *db.MemoryCluster) error {
	return nil
}
func (b *autoSupersedeStub) SetMemoryClusterID(_ context.Context, _, _ string) error { return nil }
func (b *autoSupersedeStub) FindNearestClusters(_ context.Context, _ string, _ []float32, _ int) ([]string, error) {
	return nil, nil
}
func (b *autoSupersedeStub) VectorSearchWithClusters(_ context.Context, _ string, _ []float32, _ int, _ []string, _, _ *time.Time) ([]db.VectorHit, error) {
	return nil, nil
}
func (b *autoSupersedeStub) TableExists(_ context.Context, _ string) (bool, error) { return false, nil }
func (b *autoSupersedeStub) ColumnExists(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

var _ db.Backend = (*autoSupersedeStub)(nil)

// ── test ──────────────────────────────────────────────────────────────────────

// fakeEmbedderUnit is a zero-alloc embedder for unit tests in this file.
type fakeEmbedderUnit struct{}

func (fakeEmbedderUnit) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, 4), nil
}
func (e fakeEmbedderUnit) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	v, err := e.Embed(ctx, text)
	return v, e.Name(), err
}
func (fakeEmbedderUnit) Name() string    { return "unit" }
func (fakeEmbedderUnit) Dimensions() int { return 4 }

var _ embed.Client = fakeEmbedderUnit{}

// TestAutoSupersede_SoftDeleteTxFailure verifies that when SoftDeleteMemoryTx
// returns an error the transaction is rolled back and AutoSupersede surfaces
// the error (superseded count remains 0).
func TestAutoSupersede_SoftDeleteTxFailure(t *testing.T) {
	ctx := context.Background()
	const project = "unit-tx-test"

	now := time.Now().UTC()
	newerID := "mem-newer"
	olderID := "mem-older"

	stub := &autoSupersedeStub{
		memories: map[string]*types.Memory{
			newerID: {
				ID:        newerID,
				Project:   project,
				Content:   "newer fact",
				CreatedAt: now,
				UpdatedAt: now,
			},
			olderID: {
				ID:        olderID,
				Project:   project,
				Content:   "older fact",
				CreatedAt: now.Add(-48 * time.Hour),
				UpdatedAt: now.Add(-48 * time.Hour),
			},
		},
		rels: map[string][]types.Relationship{
			newerID: {
				{
					ID:       "rel-1",
					SourceID: newerID,
					TargetID: olderID,
					RelType:  types.RelTypeContradicts,
					Strength: 1.0,
					Project:  project,
				},
			},
		},
		softDeleteTxErr: errors.New("injected SoftDeleteMemoryTx failure"),
	}

	runner := consolidate.NewRunner(stub, project, fakeEmbedderUnit{})

	count, err := runner.AutoSupersede(ctx)

	require.Error(t, err, "AutoSupersede must surface the SoftDeleteMemoryTx error")
	require.Equal(t, 0, count, "no memory must be counted as superseded on rollback")

	// The fake Tx must have been rolled back, not committed.
	require.NotNil(t, stub.tx, "Begin must have been called")
	require.True(t, stub.tx.rolled, "Rollback must have been called after SoftDeleteMemoryTx failure")
	require.False(t, stub.tx.committed, "Commit must NOT have been called")
}
