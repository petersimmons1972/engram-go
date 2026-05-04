package mcp_test

// Feature: Noise-Aware Recall (Step 2)
// Tests are written BEFORE implementation (TDD).
// They must fail until conflicts.go and the updated handleMemoryRecall are in place.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Stub backend ─────────────────────────────────────────────────────────────

// conflictStubBackend is a minimal in-memory backend for unit testing
// enrichWithConflicts without a live PostgreSQL connection.
// Only GetRelationships and GetMemory are implemented; all other methods panic.
type conflictStubBackend struct {
	memories      map[string]*types.Memory
	relationships map[string][]types.Relationship // keyed by memory ID
}

func newConflictStub() *conflictStubBackend {
	return &conflictStubBackend{
		memories:      make(map[string]*types.Memory),
		relationships: make(map[string][]types.Relationship),
	}
}

func (s *conflictStubBackend) addMemory(m *types.Memory) {
	s.memories[m.ID] = m
}

// addRelationship records the relationship and indexes it by both endpoints.
func (s *conflictStubBackend) addRelationship(rel types.Relationship) {
	s.relationships[rel.SourceID] = append(s.relationships[rel.SourceID], rel)
	s.relationships[rel.TargetID] = append(s.relationships[rel.TargetID], rel)
}

// GetRelationships returns all edges where memoryID is source or target.
func (s *conflictStubBackend) GetRelationships(_ context.Context, _ string, memoryID string) ([]types.Relationship, error) {
	return s.relationships[memoryID], nil
}

// GetRelationshipsBatch returns edges for multiple memory IDs in one call.
func (s *conflictStubBackend) GetRelationshipsBatch(_ context.Context, _ string, ids []string) (map[string][]types.Relationship, error) {
	result := make(map[string][]types.Relationship, len(ids))
	for _, id := range ids {
		result[id] = s.relationships[id]
	}
	return result, nil
}

// GetMemory returns the memory by ID (ignores project scope for test simplicity).
func (s *conflictStubBackend) GetMemory(_ context.Context, id string) (*types.Memory, error) {
	m, ok := s.memories[id]
	if !ok {
		return nil, nil
	}
	return m, nil
}

// conflictStubBackend satisfies internalmcp.ConflictReader via Go structural
// typing — no explicit declaration needed.

// ── Unit tests for enrichWithConflicts ───────────────────────────────────────

func makeMemory(id, content string) *types.Memory {
	return &types.Memory{
		ID:         id,
		Content:    content,
		MemoryType: types.MemoryTypeContext,
		CreatedAt:  time.Now(),
	}
}

func makeResult(m *types.Memory) types.SearchResult {
	return types.SearchResult{Memory: m, Score: 0.9}
}

// TestEnrichWithConflicts_ReturnsContradictions verifies that when a recall
// result has a "contradicts" edge to another memory, that memory appears in
// the returned ConflictingResult slice with correct metadata.
func TestEnrichWithConflicts_ReturnsContradictions(t *testing.T) {
	ctx := context.Background()
	stub := newConflictStub()

	memA := makeMemory("id-a", "Use Redis for session storage.")
	memB := makeMemory("id-b", "Never use Redis; use database-backed sessions only.")
	stub.addMemory(memA)
	stub.addMemory(memB)

	rel := types.Relationship{
		ID:       "rel-1",
		SourceID: "id-a",
		TargetID: "id-b",
		RelType:  types.RelTypeContradicts,
		Strength: 0.85,
		Project:  "test",
	}
	stub.addRelationship(rel)

	results := []types.SearchResult{makeResult(memA)}
	conflicts := internalmcp.EnrichWithConflicts(ctx, stub, "test", results)

	require.Len(t, conflicts, 1)
	assert.Equal(t, "id-b", conflicts[0].Memory.ID)
	assert.Equal(t, "id-a", conflicts[0].ContradictsID)
	assert.InDelta(t, 0.85, conflicts[0].Strength, 0.001)
	assert.NotEmpty(t, conflicts[0].MatchedChunk)
}

// TestEnrichWithConflicts_EmptyWhenNoContradictions verifies that when recall
// results have relationships of other types (e.g. "relates_to") but no
// "contradicts" edges, the return value is empty (not nil, but zero length).
func TestEnrichWithConflicts_EmptyWhenNoContradictions(t *testing.T) {
	ctx := context.Background()
	stub := newConflictStub()

	memA := makeMemory("id-a", "Always use connection pooling.")
	memC := makeMemory("id-c", "Related: monitor pool exhaustion.")
	stub.addMemory(memA)
	stub.addMemory(memC)

	rel := types.Relationship{
		ID:       "rel-2",
		SourceID: "id-a",
		TargetID: "id-c",
		RelType:  types.RelTypeRelatesTo,
		Strength: 0.7,
		Project:  "test",
	}
	stub.addRelationship(rel)

	results := []types.SearchResult{makeResult(memA)}
	conflicts := internalmcp.EnrichWithConflicts(ctx, stub, "test", results)

	assert.Empty(t, conflicts, "non-contradicts edges must not produce conflicting results")
}

// TestEnrichWithConflicts_NilMemorySkipped verifies that SearchResults with a
// nil Memory pointer are skipped without panicking.
func TestEnrichWithConflicts_NilMemorySkipped(t *testing.T) {
	ctx := context.Background()
	stub := newConflictStub()

	results := []types.SearchResult{{Memory: nil, Score: 0.5}}
	conflicts := internalmcp.EnrichWithConflicts(ctx, stub, "test", results)

	assert.Empty(t, conflicts)
}

// TestEnrichWithConflicts_Deduplicates verifies that when two recall results
// both have "contradicts" edges to the same memory, that memory only appears
// once in the conflict list.
func TestEnrichWithConflicts_Deduplicates(t *testing.T) {
	ctx := context.Background()
	stub := newConflictStub()

	memA := makeMemory("id-a", "Use Go modules.")
	memB := makeMemory("id-b", "Use vendoring instead of modules.")
	memD := makeMemory("id-d", "Go modules are the way.")
	stub.addMemory(memA)
	stub.addMemory(memB)
	stub.addMemory(memD)

	// Both id-a and id-d contradict id-b.
	rel1 := types.Relationship{
		ID: "rel-3", SourceID: "id-a", TargetID: "id-b",
		RelType: types.RelTypeContradicts, Strength: 0.9, Project: "test",
	}
	rel2 := types.Relationship{
		ID: "rel-4", SourceID: "id-d", TargetID: "id-b",
		RelType: types.RelTypeContradicts, Strength: 0.8, Project: "test",
	}
	stub.addRelationship(rel1)
	stub.addRelationship(rel2)

	results := []types.SearchResult{makeResult(memA), makeResult(memD)}
	conflicts := internalmcp.EnrichWithConflicts(ctx, stub, "test", results)

	// id-b should appear exactly once even though two results point to it.
	count := 0
	for _, c := range conflicts {
		if c.Memory.ID == "id-b" {
			count++
		}
	}
	assert.Equal(t, 1, count, "duplicate contradicting memory must appear only once")
}

// TestEnrichWithConflicts_ExcludesPrimaryResults verifies that a memory
// appearing in the primary results is NOT also returned in conflicting_results.
func TestEnrichWithConflicts_ExcludesPrimaryResults(t *testing.T) {
	stub := newConflictStub()
	mA := makeMemory("id-a", "original claim")
	mB := makeMemory("id-b", "conflicting claim")
	stub.addMemory(mB)
	stub.addRelationship(types.Relationship{
		SourceID: "id-a", TargetID: "id-b",
		RelType: types.RelTypeContradicts, Strength: 0.9,
	})
	// id-b is BOTH a contradiction target AND a primary result.
	results := []types.SearchResult{
		{Memory: mA, Score: 0.9},
		{Memory: mB, Score: 0.8},
	}
	conflicts := internalmcp.EnrichWithConflicts(context.Background(), stub, "proj", results)
	assert.Empty(t, conflicts, "memory appearing in primary results must not appear in conflicting_results")
}

// TestEnrichWithConflicts_BackendError verifies best-effort behavior when
// the backend returns an error from GetRelationships.
func TestEnrichWithConflicts_BackendError(t *testing.T) {
	errBackend := &errorStubBackend{}
	results := []types.SearchResult{
		{Memory: makeMemory("id-a", "some content"), Score: 0.9},
	}
	conflicts := internalmcp.EnrichWithConflicts(context.Background(), errBackend, "proj", results)
	assert.Empty(t, conflicts, "backend errors must not propagate — return empty conflicts")
}

// TestEnrichWithConflicts_TruncatesLongContent verifies that MatchedChunk
// is capped at 500 bytes for large memory content.
func TestEnrichWithConflicts_TruncatesLongContent(t *testing.T) {
	longContent := strings.Repeat("x", 1000)
	stub := newConflictStub()
	mB := makeMemory("id-b", longContent)
	stub.addMemory(mB)
	stub.addRelationship(types.Relationship{
		SourceID: "id-a", TargetID: "id-b",
		RelType: types.RelTypeContradicts, Strength: 0.8,
	})
	results := []types.SearchResult{
		{Memory: makeMemory("id-a", "short"), Score: 0.9},
	}
	conflicts := internalmcp.EnrichWithConflicts(context.Background(), stub, "proj", results)
	require.Len(t, conflicts, 1)
	assert.Len(t, conflicts[0].MatchedChunk, 500, "MatchedChunk must be truncated to 500 bytes")
}

// errorStubBackend always returns errors.
type errorStubBackend struct{}

func (e *errorStubBackend) GetRelationships(_ context.Context, _, _ string) ([]types.Relationship, error) {
	return nil, fmt.Errorf("simulated backend failure")
}

func (e *errorStubBackend) GetRelationshipsBatch(_ context.Context, _ string, _ []string) (map[string][]types.Relationship, error) {
	return nil, fmt.Errorf("simulated backend failure")
}

func (e *errorStubBackend) GetMemory(_ context.Context, _ string) (*types.Memory, error) {
	return nil, fmt.Errorf("simulated backend failure")
}

// ── Integration test (skips without TEST_DATABASE_URL) ───────────────────────

func testRecallDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

// TestHandleMemoryRecall_IncludeConflicts_Integration stores two memories,
// creates a "contradicts" edge between them, recalls the source, and verifies
// that include_conflicts=true returns the contradicting memory.
func TestHandleMemoryRecall_IncludeConflicts_Integration(t *testing.T) {
	dsn := testRecallDSN(t)
	ctx := context.Background()
	proj := fmt.Sprintf("test-conflicts-%d", time.Now().UnixNano())

	pool := internalmcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	// Store two contradicting memories.
	m1 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Deploy on Friday afternoons for faster iteration.",
		MemoryType:  types.MemoryTypeDecision,
		Importance:  2,
		StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Never deploy on Friday — too risky before the weekend.",
		MemoryType:  types.MemoryTypeDecision,
		Importance:  2,
		StorageMode: "focused",
	}

	h, err := pool.Get(ctx, proj)
	require.NoError(t, err)
	require.NoError(t, h.Engine.Store(ctx, m1))
	require.NoError(t, h.Engine.Store(ctx, m2))

	// Create the contradicts edge directly on the backend.
	rel := &types.Relationship{
		ID:       types.NewMemoryID(),
		SourceID: m1.ID,
		TargetID: m2.ID,
		RelType:  types.RelTypeContradicts,
		Strength: 0.9,
		Project:  proj,
	}
	require.NoError(t, h.Engine.Backend().StoreRelationship(ctx, rel))

	// Recall with include_conflicts=true.
	out := internalmcp.CallHandleMemoryRecall(ctx, t, pool, proj, "deploy Friday", true)

	conflicts, ok := out["conflicting_results"]
	require.True(t, ok, "conflicting_results key must be present when include_conflicts=true")

	conflictSlice, ok := conflicts.([]types.ConflictingResult)
	require.True(t, ok, "conflicting_results must be []types.ConflictingResult")
	require.NotEmpty(t, conflictSlice, "expected at least one conflicting result")

	// m2 should appear as the contradicting memory.
	found := false
	for _, c := range conflictSlice {
		if c.Memory != nil && c.Memory.ID == m2.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "contradicting memory m2 must appear in conflicting_results")
}

// TestHandleMemoryRecall_IncludesFeedbackHint verifies that when
// handleMemoryRecall returns an event_id (single-project, non-rerank path),
// the response also contains a feedback_hint key so callers know how to
// submit outcome feedback.
func TestHandleMemoryRecall_IncludesFeedbackHint(t *testing.T) {
	dsn := testRecallDSN(t)
	ctx := context.Background()
	proj := fmt.Sprintf("test-feedback-hint-%d", time.Now().UnixNano())

	pool := internalmcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	h, err := pool.Get(ctx, proj)
	require.NoError(t, err)

	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Feedback hint: recall responses must advertise the feedback API.",
		MemoryType:  types.MemoryTypePattern,
		Importance:  1,
		StorageMode: "focused",
	}
	require.NoError(t, h.Engine.Store(ctx, m))

	out := internalmcp.CallHandleMemoryRecallFull(ctx, t, pool, proj, "feedback hint recall API", nil)

	eventID, hasEventID := out["event_id"]
	require.True(t, hasEventID, "recall response must contain event_id on the single-project path")
	require.NotEmpty(t, eventID, "event_id must be non-empty")

	hint, hasHint := out["feedback_hint"]
	require.True(t, hasHint, "recall response must contain feedback_hint when event_id is present")
	hintStr, ok := hint.(string)
	require.True(t, ok, "feedback_hint must be a string")
	require.NotEmpty(t, hintStr, "feedback_hint string must not be empty")
}

// TestHandleMemoryRecall_IncludeConflicts_FalseByDefault verifies that the
// conflicting_results key is absent when include_conflicts is not set.
func TestHandleMemoryRecall_IncludeConflicts_FalseByDefault(t *testing.T) {
	dsn := testRecallDSN(t)
	ctx := context.Background()
	proj := fmt.Sprintf("test-no-conflicts-%d", time.Now().UnixNano())

	pool := internalmcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	h, err := pool.Get(ctx, proj)
	require.NoError(t, err)

	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Always write tests before implementation.",
		MemoryType:  types.MemoryTypePattern,
		Importance:  1,
		StorageMode: "focused",
	}
	require.NoError(t, h.Engine.Store(ctx, m))

	out := internalmcp.CallHandleMemoryRecall(ctx, t, pool, proj, "tests implementation", false)

	_, present := out["conflicting_results"]
	assert.False(t, present, "conflicting_results must not be present when include_conflicts=false")
}
