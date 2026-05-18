package db_test

// Tests for the pattern_confidence column added in migration 021.
//
// These are integration tests that require a live PostgreSQL database.
// They follow the same conventions as postgres_memory_test.go:
//   - uniqueProject / newTestBackend from postgres_relationship_test.go helpers
//   - storeMemory helper for boilerplate
//
// Coverage targets (per Track E1 brief):
//   - TestStorePatternConfidence
//   - TestStoreNilPatternConfidence
//   - TestCorrectUpdatesPatternConfidence
//   - TestCorrectNilLeavesPatternConfidence
//   - TestStoreThenCorrectThenRead
//   - TestPatternConfidenceBoundary
//   - TestBackwardCompatExistingRows

import (
	"context"
	"math"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/testutil"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// newRawBackend returns a *db.PostgresBackend (concrete type) so tests can
// call Pool() for raw SQL injection in TestBackwardCompatExistingRows.
func newRawBackend(t *testing.T, project string) *db.PostgresBackend {
	t.Helper()
	ctx := context.Background()
	b, err := db.NewPostgresBackend(ctx, project, testutil.DSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { b.Close() })
	return b
}

// storeMemoryWithConfidence is a test helper that stores a memory with the
// given pattern_confidence value (nil = not set).
func storeMemoryWithConfidence(t *testing.T, b db.Backend, proj string, content string, pc *float64) *types.Memory {
	t.Helper()
	m := &types.Memory{
		ID:                types.NewMemoryID(),
		Content:           content,
		Project:           proj,
		MemoryType:        types.MemoryTypePattern,
		Importance:        2,
		PatternConfidence: pc,
	}
	require.NoError(t, b.StoreMemory(context.Background(), m))
	return m
}

// floatPtr is a convenience helper for inline *float64 literals.
func floatPtr(f float64) *float64 { return &f }

// withinEpsilon asserts a and b are within epsilon of each other.
func withinEpsilon(t *testing.T, label string, a, b, eps float64) {
	t.Helper()
	if math.Abs(a-b) > eps {
		t.Errorf("%s: got %v, want ~%v (epsilon %v)", label, a, b, eps)
	}
}

// TestStorePatternConfidence stores a memory with pattern_confidence=0.7 and
// reads it back, asserting the value is preserved within float epsilon.
func TestStorePatternConfidence(t *testing.T) {
	proj := uniqueProject("pc-store")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	mem := storeMemoryWithConfidence(t, b, proj, "store confidence test", floatPtr(0.7))

	got, err := b.GetMemory(ctx, mem.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.PatternConfidence, "PatternConfidence should not be nil")
	withinEpsilon(t, "PatternConfidence", *got.PatternConfidence, 0.7, 1e-9)
}

// TestStoreNilPatternConfidence stores a memory without pattern_confidence
// and verifies the field comes back nil (no silent default to 0.0).
func TestStoreNilPatternConfidence(t *testing.T) {
	proj := uniqueProject("pc-store-nil")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	mem := storeMemoryWithConfidence(t, b, proj, "nil confidence test", nil)

	got, err := b.GetMemory(ctx, mem.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Nil(t, got.PatternConfidence, "PatternConfidence must be nil when not set — must not default to 0.0")
}

// TestCorrectUpdatesPatternConfidence stores a memory with nil confidence,
// then calls UpdateMemory with 0.8, and verifies the field is updated.
func TestCorrectUpdatesPatternConfidence(t *testing.T) {
	proj := uniqueProject("pc-correct-update")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	mem := storeMemoryWithConfidence(t, b, proj, "initially no confidence", nil)

	updated, err := b.UpdateMemory(ctx, mem.ID, nil, nil, nil, floatPtr(0.8))
	require.NoError(t, err)
	require.NotNil(t, updated, "UpdateMemory must return the updated memory")
	require.NotNil(t, updated.PatternConfidence)
	withinEpsilon(t, "PatternConfidence after update", *updated.PatternConfidence, 0.8, 1e-9)

	// Verify by independent read.
	got, err := b.GetMemory(ctx, mem.ID)
	require.NoError(t, err)
	require.NotNil(t, got.PatternConfidence)
	withinEpsilon(t, "PatternConfidence on re-read", *got.PatternConfidence, 0.8, 1e-9)
}

// TestCorrectNilLeavesPatternConfidence stores a memory with 0.5, then calls
// UpdateMemory with nil patternConfidence, and asserts the field is unchanged.
func TestCorrectNilLeavesPatternConfidence(t *testing.T) {
	proj := uniqueProject("pc-correct-nil")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	mem := storeMemoryWithConfidence(t, b, proj, "has confidence initially", floatPtr(0.5))

	// Update importance only; patternConfidence=nil → do not touch.
	newImportance := 1
	updated, err := b.UpdateMemory(ctx, mem.ID, nil, nil, &newImportance, nil)
	require.NoError(t, err)
	require.NotNil(t, updated)

	got, err := b.GetMemory(ctx, mem.ID)
	require.NoError(t, err)
	require.NotNil(t, got.PatternConfidence, "PatternConfidence must survive a nil-patternConfidence update")
	withinEpsilon(t, "PatternConfidence unchanged", *got.PatternConfidence, 0.5, 1e-9)
}

// TestStoreThenCorrectThenRead does a full round-trip: store with 0.3,
// update to 0.9, read back and assert 0.9.
func TestStoreThenCorrectThenRead(t *testing.T) {
	proj := uniqueProject("pc-roundtrip")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	mem := storeMemoryWithConfidence(t, b, proj, "round-trip content", floatPtr(0.3))

	_, err := b.UpdateMemory(ctx, mem.ID, nil, nil, nil, floatPtr(0.9))
	require.NoError(t, err)

	got, err := b.GetMemory(ctx, mem.ID)
	require.NoError(t, err)
	require.NotNil(t, got.PatternConfidence)
	withinEpsilon(t, "PatternConfidence after full round-trip", *got.PatternConfidence, 0.9, 1e-9)
}

// TestPatternConfidenceBoundary stores 0.0 and 1.0 exactly and verifies
// they read back without loss (exact boundaries must be preserved).
func TestPatternConfidenceBoundary(t *testing.T) {
	proj := uniqueProject("pc-boundary")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	m0 := storeMemoryWithConfidence(t, b, proj, "zero confidence boundary", floatPtr(0.0))
	m1 := storeMemoryWithConfidence(t, b, proj, "one confidence boundary", floatPtr(1.0))

	g0, err := b.GetMemory(ctx, m0.ID)
	require.NoError(t, err)
	require.NotNil(t, g0.PatternConfidence)
	withinEpsilon(t, "PatternConfidence at 0.0 boundary", *g0.PatternConfidence, 0.0, 1e-9)

	g1, err := b.GetMemory(ctx, m1.ID)
	require.NoError(t, err)
	require.NotNil(t, g1.PatternConfidence)
	withinEpsilon(t, "PatternConfidence at 1.0 boundary", *g1.PatternConfidence, 1.0, 1e-9)
}

// TestBackwardCompatExistingRows directly INSERTs a row with pattern_confidence=NULL
// (simulating pre-migration data), reads it back via the standard path, and asserts
// no crash and a nil PatternConfidence field.
func TestBackwardCompatExistingRows(t *testing.T) {
	proj := uniqueProject("pc-backward-compat")
	b := newRawBackend(t, proj)
	ctx := context.Background()

	memID := types.NewMemoryID()
	// Direct INSERT omitting pattern_confidence — it will be NULL.
	_, err := b.Pool().Exec(ctx, `
		INSERT INTO memories
		  (id, content, memory_type, project, tags,
		   importance, access_count, last_accessed, created_at, updated_at,
		   immutable, storage_mode)
		VALUES ($1, $2, 'context', $3, '[]'::jsonb,
		        2, 0, NOW(), NOW(), NOW(),
		        false, 'focused')`,
		memID, "pre-migration row without pattern_confidence", proj,
	)
	require.NoError(t, err, "direct INSERT must succeed — simulates pre-migration row")

	got, err := b.GetMemory(ctx, memID)
	require.NoError(t, err, "GetMemory must not crash on a row with NULL pattern_confidence")
	require.NotNil(t, got)
	require.Nil(t, got.PatternConfidence, "pre-migration row must surface PatternConfidence=nil, not a zero value")
}
