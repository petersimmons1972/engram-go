package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// storeMemoryWithTags stores a memory with the given tags, reusing the shared
// storeMemory helper but enriching with tags after construction.
func storeMemoryWithTags(t *testing.T, b interface {
	StoreMemory(ctx context.Context, m *types.Memory) error
}, proj, content string, tags []string) *types.Memory {
	t.Helper()
	m := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    content,
		Project:    proj,
		MemoryType: types.MemoryTypeContext,
		Importance: 1,
		Tags:       tags,
	}
	require.NoError(t, b.StoreMemory(context.Background(), m))
	return m
}

// storeMemoryWithType stores a memory with the given memory_type (no tags).
func storeMemoryWithType(t *testing.T, b interface {
	StoreMemory(ctx context.Context, m *types.Memory) error
}, proj, content, memType string) *types.Memory {
	t.Helper()
	m := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    content,
		Project:    proj,
		MemoryType: memType,
		Importance: 1,
	}
	require.NoError(t, b.StoreMemory(context.Background(), m))
	return m
}

// findRowByLabel is a test helper that returns the AggregateRow whose Label
// matches label, or nil if not found.
func findRowByLabel(rows []types.AggregateRow, label string) *types.AggregateRow {
	for i := range rows {
		if rows[i].Label == label {
			return &rows[i]
		}
	}
	return nil
}

// ── AggregateMemories ──────────────────────────────────────────────────────

// TestAggregateMemories_ByTag stores 2 memories tagged "auth" and 1 tagged
// "billing", then asserts the tag-grouped aggregate returns correct counts.
func TestAggregateMemories_ByTag(t *testing.T) {
	proj := uniqueProject("agg-bytag")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	storeMemoryWithTags(t, b, proj, "auth memory one", []string{"auth"})
	storeMemoryWithTags(t, b, proj, "auth memory two", []string{"auth"})
	storeMemoryWithTags(t, b, proj, "billing memory", []string{"billing"})

	rows, err := b.AggregateMemories(ctx, proj, "tag", "", 20)
	require.NoError(t, err)

	authRow := findRowByLabel(rows, "auth")
	require.NotNil(t, authRow, "expected a row with label=auth")
	require.Equal(t, 2, authRow.Count)

	billingRow := findRowByLabel(rows, "billing")
	require.NotNil(t, billingRow, "expected a row with label=billing")
	require.Equal(t, 1, billingRow.Count)
}

// TestAggregateMemories_ByTagFilter stores 2 memories tagged "auth-token" and
// 1 tagged "billing", then asserts that filter="auth" returns only rows whose
// label matches ILIKE "%auth%".
func TestAggregateMemories_ByTagFilter(t *testing.T) {
	proj := uniqueProject("agg-tagfilter")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	storeMemoryWithTags(t, b, proj, "auth token mem 1", []string{"auth-token"})
	storeMemoryWithTags(t, b, proj, "auth token mem 2", []string{"auth-token"})
	storeMemoryWithTags(t, b, proj, "billing mem", []string{"billing"})

	rows, err := b.AggregateMemories(ctx, proj, "tag", "auth", 20)
	require.NoError(t, err)

	// "auth-token" matches ILIKE "%auth%" — must be present.
	authTokenRow := findRowByLabel(rows, "auth-token")
	require.NotNil(t, authTokenRow, "expected row with label=auth-token after filter")
	require.Equal(t, 2, authTokenRow.Count)

	// "billing" does not match "%auth%" — must be absent.
	billingRow := findRowByLabel(rows, "billing")
	require.Nil(t, billingRow, "billing row must be filtered out")
}

// TestAggregateMemories_ByType stores 1 context memory and 1 error memory,
// then asserts the type-grouped aggregate returns one row per type with count=1.
func TestAggregateMemories_ByType(t *testing.T) {
	proj := uniqueProject("agg-bytype")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	storeMemoryWithType(t, b, proj, "a context memory", types.MemoryTypeContext)
	storeMemoryWithType(t, b, proj, "an error memory", types.MemoryTypeError)

	rows, err := b.AggregateMemories(ctx, proj, "type", "", 20)
	require.NoError(t, err)

	ctxRow := findRowByLabel(rows, types.MemoryTypeContext)
	require.NotNil(t, ctxRow, "expected row for context type")
	require.Equal(t, 1, ctxRow.Count)

	errRow := findRowByLabel(rows, types.MemoryTypeError)
	require.NotNil(t, errRow, "expected row for error type")
	require.Equal(t, 1, errRow.Count)
}

// TestAggregateMemories_EmptyProject calls AggregateMemories on a project
// that has no memories and asserts a non-nil empty slice is returned.
func TestAggregateMemories_EmptyProject(t *testing.T) {
	proj := uniqueProject("agg-empty")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	rows, err := b.AggregateMemories(ctx, proj, "tag", "", 20)
	require.NoError(t, err)
	require.NotNil(t, rows, "result must be non-nil even for an empty project")
	require.Empty(t, rows)
}

// TestAggregateMemories_InvalidBy passes an unrecognised "by" value and
// asserts an error is returned.
func TestAggregateMemories_InvalidBy(t *testing.T) {
	proj := uniqueProject("agg-invalidby")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	_, err := b.AggregateMemories(ctx, proj, "invalid_value", "", 20)
	require.Error(t, err, "invalid 'by' value must return an error")
}

// ── AggregateFailureClasses ────────────────────────────────────────────────

// TestAggregateFailureClasses stores a retrieval event, records feedback with
// failure_class="aggregation_failure", then asserts AggregateFailureClasses
// returns a row for that class with count=1, and no rows for NULL classes.
func TestAggregateFailureClasses(t *testing.T) {
	proj := uniqueProject("agg-failclass")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	// Store a retrieval event.
	evt := &types.RetrievalEvent{
		ID:        types.NewMemoryID(),
		Project:   proj,
		Query:     "some query",
		ResultIDs: []string{},
		CreatedAt: time.Now(),
	}
	require.NoError(t, b.StoreRetrievalEvent(ctx, evt))

	// Record feedback with a failure class.
	require.NoError(t, b.RecordFeedbackWithClass(ctx, evt.ID, []string{}, types.FailureClassAggregationFailure))

	rows, err := b.AggregateFailureClasses(ctx, proj, 20)
	require.NoError(t, err)

	row := findRowByLabel(rows, types.FailureClassAggregationFailure)
	require.NotNil(t, row, "expected row for aggregation_failure class")
	require.Equal(t, 1, row.Count)

	// NULL failure_class rows must not appear.
	nullRow := findRowByLabel(rows, "")
	require.Nil(t, nullRow, "rows with NULL failure_class must be excluded")
}

// ── RecordFeedbackWithClass ────────────────────────────────────────────────

// TestRecordFeedbackWithClass_SetsClass stores a retrieval event, records
// feedback with failure_class="stale_ranking", and asserts the persisted event
// has FailureClass == "stale_ranking".
func TestRecordFeedbackWithClass_SetsClass(t *testing.T) {
	proj := uniqueProject("rfwc-sets")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	// Need at least one memory so we can reference it.
	m := storeMemory(t, b, proj, "some stored memory")

	evt := &types.RetrievalEvent{
		ID:        types.NewMemoryID(),
		Project:   proj,
		Query:     "test query",
		ResultIDs: []string{m.ID},
		CreatedAt: time.Now(),
	}
	require.NoError(t, b.StoreRetrievalEvent(ctx, evt))

	require.NoError(t, b.RecordFeedbackWithClass(ctx, evt.ID, []string{m.ID}, types.FailureClassStaleRanking))

	fetched, err := b.GetRetrievalEvent(ctx, evt.ID)
	require.NoError(t, err)
	require.Equal(t, types.FailureClassStaleRanking, fetched.FailureClass)
}

// TestRecordFeedbackWithClass_EmptyClass records feedback with an empty
// failure_class and asserts the stored value is empty (NULL stored as "").
func TestRecordFeedbackWithClass_EmptyClass(t *testing.T) {
	proj := uniqueProject("rfwc-empty")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	evt := &types.RetrievalEvent{
		ID:        types.NewMemoryID(),
		Project:   proj,
		Query:     "empty class query",
		ResultIDs: []string{},
		CreatedAt: time.Now(),
	}
	require.NoError(t, b.StoreRetrievalEvent(ctx, evt))

	require.NoError(t, b.RecordFeedbackWithClass(ctx, evt.ID, []string{}, ""))

	fetched, err := b.GetRetrievalEvent(ctx, evt.ID)
	require.NoError(t, err)
	require.Equal(t, "", fetched.FailureClass, "empty failure_class must round-trip as empty string")
}

// TestRecordFeedbackWithClass_UnknownEvent calls RecordFeedbackWithClass with
// a non-existent event ID and asserts an error is returned.
func TestRecordFeedbackWithClass_UnknownEvent(t *testing.T) {
	proj := uniqueProject("rfwc-unknown")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	err := b.RecordFeedbackWithClass(ctx, types.NewMemoryID(), []string{}, types.FailureClassMissingContent)
	require.Error(t, err, "non-existent event ID must return an error")
}
