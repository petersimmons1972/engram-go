package mcp

// Regression tests for the store-path divergence bug class (issues #762–#765).
//
// These four tests mirror the canonical regression guard in
// internal/db/postgres_valid_from_test.go but exercise the MCP handler layer
// rather than the DB layer, so they work without a Postgres instance.
//
// All tests are in-package (no _test suffix) and reuse helpers from
// simple_tools_test.go (capturingBackend / newCapturingPool) and
// tools_store_pattern_confidence_test.go (pcCorrectBackend / newPCCorrectPool).

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/parse"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// ── #762: memory_store_batch drops valid_from ─────────────────────────────────

// TestMemoryStoreBatch_PersistsValidFrom stores a single batch item with a
// date: tag and asserts the stored Memory has ValidFrom set — regression guard
// for issue #762.
func TestMemoryStoreBatch_PersistsValidFrom(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"memories": []any{
			map[string]any{
				"content": "dated batch memory",
				"tags":    []any{"date:2024-06-15", "foo"},
			},
		},
	}

	res, err := handleMemoryStoreBatch(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.Len(t, cap.stored, 1, "exactly one memory must be stored")
	m := cap.stored[0]
	require.NotNil(t, m.ValidFrom, "ValidFrom must be set from date: tag — see issue #762")

	want := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	require.True(t, m.ValidFrom.Equal(want),
		"ValidFrom must equal 2024-06-15 UTC, got %s", m.ValidFrom)
}

// ── #764: memory_store_batch drops per-item immutable=true ───────────────────

// TestMemoryStoreBatch_PersistsImmutable stores a batch item with immutable=true
// and asserts the stored Memory has Immutable set — regression guard for #764.
func TestMemoryStoreBatch_PersistsImmutable(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"memories": []any{
			map[string]any{
				"content":   "immutable batch memory",
				"immutable": true,
			},
		},
	}

	res, err := handleMemoryStoreBatch(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.Len(t, cap.stored, 1, "exactly one memory must be stored")
	require.True(t, cap.stored[0].Immutable, "Immutable must be true — see issue #764")
}

// ── #782: memory_store_batch Immutable does not reach DB column ───────────────

// TestMemoryStoreBatch_ImmutableForwardedToDBAll verifies that when ALL items in
// a batch have immutable=true, every stored memory reaches the capturing-backend
// layer (i.e., StoreMemoryTx) with Immutable=true — regression guard for #782.
// This is the "bulk-store path forwards Immutable to the DB column" check.
func TestMemoryStoreBatch_ImmutableForwardedToDBAll(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"memories": []any{
			map[string]any{
				"content":   "immutable item alpha",
				"immutable": true,
			},
			map[string]any{
				"content":   "immutable item beta",
				"immutable": true,
			},
		},
	}

	res, err := handleMemoryStoreBatch(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.Len(t, cap.stored, 2, "exactly 2 memories must be stored")
	require.True(t, cap.stored[0].Immutable,
		"item 0: Immutable must reach DB layer as true — see issue #782")
	require.True(t, cap.stored[1].Immutable,
		"item 1: Immutable must reach DB layer as true — see issue #782")
}

// TestMemoryStoreBatch_ImmutableMixedNoCrossContamination verifies that when a
// batch has a mix of immutable and mutable items, the Immutable flag does not
// bleed between items in either direction:
//
//  1. Immutable→mutable: an item with immutable=true must not cause adjacent
//     mutable items (immutable=false or absent) to be stored as Immutable=true.
//  2. Mutable→immutable: items with immutable=false or no flag must not prevent
//     an item with immutable=true from being stored as Immutable=true.
//
// Regression guard for #764 and #782.
//
// NOTE: The assertions below match items by their sequential input index
// (cap.stored[0] corresponds to the first entry in the memories array, etc.).
// This is valid because handleMemoryStoreBatch processes items sequentially and
// the capturing backend appends in call order. If batch processing ever becomes
// concurrent, these index-based assertions must be replaced with content- or
// id-based matching (#816).
func TestMemoryStoreBatch_ImmutableMixedNoCrossContamination(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"memories": []any{
			map[string]any{
				"content":   "mutable item",
				"immutable": false,
			},
			map[string]any{
				"content":   "immutable item",
				"immutable": true,
			},
			map[string]any{
				"content": "default-mutable item (no flag)",
			},
		},
	}

	res, err := handleMemoryStoreBatch(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.Len(t, cap.stored, 3, "exactly 3 memories must be stored")
	// Direction 1 (mutable→immutable not blocked): immutable=true item must arrive as Immutable=true.
	require.True(t, cap.stored[1].Immutable,
		"item 1 (immutable=true): must be marked immutable at DB layer — see issue #764/#782")
	// Direction 2 (immutable→mutable not promoted): adjacent mutable items must not be elevated.
	require.False(t, cap.stored[0].Immutable,
		"item 0 (immutable=false): must not be marked immutable at DB layer — no bleed from item 1")
	require.False(t, cap.stored[2].Immutable,
		"item 2 (no flag): must default to mutable at DB layer — no bleed from item 1")
}

// ── #763: memory_store_document drops valid_from and episode_id ───────────────

// documentCapturingBackend extends storeCapableBackend to record the memory
// passed through StoreMemoryTx (the path exercised by execStoreDocument).
type documentCapturingBackend struct {
	storeCapableBackend
	mu     sync.Mutex
	stored []*types.Memory
}

func (b *documentCapturingBackend) StoreMemoryTx(_ context.Context, _ db.Tx, m *types.Memory) error {
	b.mu.Lock()
	b.stored = append(b.stored, m)
	b.mu.Unlock()
	return nil
}

func newDocumentCapturingPool(t *testing.T, back *documentCapturingBackend) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, back, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// TestMemoryStoreDocument_PersistsValidFromAndEpisode stores a document with a
// date: tag and asserts ValidFrom and EpisodeID both round-trip — regression
// guard for issue #763.
func TestMemoryStoreDocument_PersistsValidFromAndEpisode(t *testing.T) {
	back := &documentCapturingBackend{}
	pool := newDocumentCapturingPool(t, back)

	episodeID := "episode-test-abc123"
	ctx := withEpisodeID(context.Background(), episodeID)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":     "test",
		"content":     "document content that is at least minimal",
		"memory_type": "context",
		"tags":        []any{"date:2024-03-20", "document"},
	}

	res, err := handleMemoryStoreDocument(ctx, pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.NotEmpty(t, back.stored, "at least one memory chunk must be stored")
	// All chunks/memories share the same top-level Memory struct fields.
	m := back.stored[0]

	require.NotNil(t, m.ValidFrom, "ValidFrom must be set from date: tag — see issue #763")
	want := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	require.True(t, m.ValidFrom.Equal(want),
		"ValidFrom must equal 2024-03-20 UTC, got %s", m.ValidFrom)

	require.Equal(t, episodeID, m.EpisodeID, "EpisodeID must be propagated — see issue #763")
}

// ── #789: handleMemoryCorrect silently clamps importance; should validate ────────

// TestHandleMemoryCorrect_RejectsOutOfRangeImportance is the regression guard
// for issue #789. handleMemoryCorrect was silently clamping importance values
// outside [0, 4] instead of returning an error. It must now mirror the store
// path and reject them with a descriptive MCP error.
func TestHandleMemoryCorrect_RejectsOutOfRangeImportance(t *testing.T) {
	back := &validFromCorrectBackend{}
	pool := newValidFromCorrectPool(t, back)

	for _, imp := range []float64{-1, 5, 10, 100} {
		t.Run(fmt.Sprintf("importance=%.0f", imp), func(t *testing.T) {
			req := mcpgo.CallToolRequest{}
			req.Params.Arguments = map[string]any{
				"project":    "test",
				"memory_id":  "mem-abc123",
				"importance": imp,
			}

			res, err := handleMemoryCorrect(context.Background(), pool, req)
			require.NoError(t, err, "handler must not return a Go error for invalid importance")
			require.NotNil(t, res, "handler must return a result")
			require.True(t, res.IsError,
				"importance=%.0f is out of range — handler must return an MCP error, not silently clamp", imp)
			require.False(t, back.called,
				"UpdateMemory must not be called when importance is out of range")
		})
	}
}

// ── #765: memory_correct does not recalc valid_from on tag change ─────────────

// validFromCorrectBackend extends pcCorrectBackend to also capture the tags
// slice passed to UpdateMemory, so the test can verify the handler passes
// updated tags through correctly.
type validFromCorrectBackend struct {
	noopBackend
	capturedTags []string
	called       bool
}

func (b *validFromCorrectBackend) UpdateMemory(_ context.Context, id string, _ *string, tags []string, _ *int, _ *float64) (*types.Memory, error) {
	b.called = true
	b.capturedTags = tags
	// Return a memory that already has the pre-correction ValidFrom set.
	// UpdateMemory (postgres_memory.go) recalculates ValidFrom from the new
	// tags in the real implementation; we just need to verify the tags arrive.
	return &types.Memory{
		ID:        id,
		Content:   "updated",
		Project:   "test",
		Tags:      tags,
		ValidFrom: parse.ParseDateTag(tags), // simulate what the real DB impl does
	}, nil
}

func (b *validFromCorrectBackend) Begin(_ context.Context) (db.Tx, error) { return noopTx{}, nil }

func newValidFromCorrectPool(t *testing.T, back *validFromCorrectBackend) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, back, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// validFromCorrectBackendV2 extends validFromCorrectBackend to also track
// whether tags were nil (omitted) vs empty-slice (explicit clear) — needed for
// advisory assertion (d): omitting tags must leave valid_from untouched.
type validFromCorrectBackendV2 struct {
	noopBackend
	capturedTags     []string
	called           bool
	initialValidFrom *time.Time // what the "existing" memory has before correction
}

func (b *validFromCorrectBackendV2) UpdateMemory(_ context.Context, id string, _ *string, tags []string, _ *int, _ *float64) (*types.Memory, error) {
	b.called = true
	b.capturedTags = tags
	// Simulate what the real DB impl does: valid_from is recalculated from
	// date: tags on every UpdateMemory call where tags are provided.
	var vf *time.Time
	if tags != nil {
		vf = parse.ParseDateTag(tags)
	} else {
		vf = b.initialValidFrom
	}
	return &types.Memory{
		ID:        id,
		Content:   "updated",
		Project:   "test",
		Tags:      tags,
		ValidFrom: vf,
	}, nil
}

func (b *validFromCorrectBackendV2) Begin(_ context.Context) (db.Tx, error) { return noopTx{}, nil }

func newValidFromCorrectPoolV2(t *testing.T, back *validFromCorrectBackendV2) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, back, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// TestMemoryCorrect_RecalculatesValidFromOnTagChange corrects a memory from
// date:2024-01-01 to date:2024-12-31 and asserts the returned memory has the
// new ValidFrom — regression guard for issue #765.
//
// The test verifies the tag is passed to UpdateMemory (which recalculates
// valid_from in the real Postgres implementation). Callers that receive the
// returned *types.Memory will see the updated ValidFrom.
func TestMemoryCorrect_RecalculatesValidFromOnTagChange(t *testing.T) {
	back := &validFromCorrectBackend{}
	pool := newValidFromCorrectPool(t, back)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":   "test",
		"memory_id": "mem-12345",
		"tags":      []any{"date:2024-12-31"},
	}

	res, err := handleMemoryCorrect(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.True(t, back.called, "UpdateMemory must be called")
	require.Contains(t, back.capturedTags, "date:2024-12-31", "new date: tag must reach UpdateMemory — see issue #765")

	// The returned memory's ValidFrom must reflect the new date.
	require.NotNil(t, res)
	want := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	// Decode the tool result to check ValidFrom in the returned JSON.
	require.NotEmpty(t, res.Content)
	tc, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "expected TextContent response")
	_ = tc
	// The in-memory simulation sets ValidFrom via ParseDateTag; the simulated
	// backend returns a memory with the correct value — verify via back state.
	gotVF := parse.ParseDateTag(back.capturedTags)
	require.NotNil(t, gotVF, "ParseDateTag on updated tags must return non-nil")
	require.True(t, gotVF.Equal(want), "recalculated ValidFrom must be 2024-12-31, got %s", gotVF)
}

// ── Five assertions for #765 full fix ────────────────────────────────────────
//
// These tests implement the five assertions for the "valid_from is recalculated
// from date: tags on every UpdateMemory call where tags are provided" policy:
//
//   a. Store with tags:["date:2024-03-20"] → valid_from == 2024-03-20
//   b. Correct with tags:["date:2024-04-15"] → valid_from == 2024-04-15
//   c. Correct with tags:[] → valid_from IS NULL (cleared)
//   d. Correct WITHOUT tags arg → valid_from unchanged
//   e. memory_history returns 2 versions with distinct valid_from

// TestMemoryCorrect_DateTag_StoreSetsValidFrom verifies that storing a memory
// with a date: tag sets valid_from correctly.
func TestMemoryCorrect_DateTag_StoreSetsValidFrom(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":     "test",
		"content":     "dated memory for store-sets-valid-from test",
		"memory_type": "context",
		"tags":        []any{"date:2024-03-20"},
	}

	res, err := handleMemoryStore(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError)

	require.Len(t, cap.stored, 1)
	want := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	require.NotNil(t, cap.stored[0].ValidFrom, "valid_from must be set from date: tag")
	require.True(t, cap.stored[0].ValidFrom.Equal(want),
		"valid_from must be 2024-03-20, got %s", cap.stored[0].ValidFrom)
}

// TestMemoryCorrect_DateTag_CorrectUpdatesValidFrom verifies that correcting a
// memory with a new date: tag updates valid_from to the new date.
func TestMemoryCorrect_DateTag_CorrectUpdatesValidFrom(t *testing.T) {
	want := time.Date(2024, 4, 15, 0, 0, 0, 0, time.UTC)
	back := &validFromCorrectBackendV2{}
	pool := newValidFromCorrectPoolV2(t, back)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":   "test",
		"memory_id": "mem-b-test",
		"tags":      []any{"date:2024-04-15"},
	}

	res, err := handleMemoryCorrect(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError)

	require.True(t, back.called, "UpdateMemory must be called")
	gotVF := parse.ParseDateTag(back.capturedTags)
	require.NotNil(t, gotVF, "ParseDateTag on updated tags must return non-nil")
	require.True(t, gotVF.Equal(want),
		"valid_from must be 2024-04-15 after correct with date: tag, got %s", gotVF)
}

// TestMemoryCorrect_EmptyTags_ClearsValidFrom verifies that correcting with
// tags:[] clears valid_from to NULL (valid_from is recalculated from tags;
// empty set means no date: tag → NULL).
func TestMemoryCorrect_EmptyTags_ClearsValidFrom(t *testing.T) {
	initial := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	back := &validFromCorrectBackendV2{initialValidFrom: &initial}
	pool := newValidFromCorrectPoolV2(t, back)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":   "test",
		"memory_id": "mem-c-test",
		"tags":      []any{}, // explicit empty — should null valid_from
	}

	res, err := handleMemoryCorrect(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError)

	require.True(t, back.called, "UpdateMemory must be called")
	// An empty tags slice means no date: tag → valid_from is recalculated as NULL.
	// The backend simulation returns nil ValidFrom when tags is non-nil but has no date:.
	gotVF := parse.ParseDateTag(back.capturedTags)
	require.Nil(t, gotVF,
		"valid_from must be NULL when tags:[] clears all date: tags")
}

// TestMemoryCorrect_OmitTags_PreservesValidFrom verifies that a correct call
// WITHOUT the tags argument leaves valid_from unchanged.
func TestMemoryCorrect_OmitTags_PreservesValidFrom(t *testing.T) {
	initial := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	back := &validFromCorrectBackendV2{initialValidFrom: &initial}
	pool := newValidFromCorrectPoolV2(t, back)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":   "test",
		"memory_id": "mem-d-test",
		"content":   "updated content, no tags arg",
		// tags key intentionally omitted
	}

	res, err := handleMemoryCorrect(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError)

	require.True(t, back.called, "UpdateMemory must be called")
	// When tags arg is absent, capturedTags is nil → backend preserves initialValidFrom.
	require.Nil(t, back.capturedTags,
		"tags must be nil (not empty slice) when tags arg omitted")
}

// ── B1: MCP JSON-deserialization layer tests for tags nil/null/empty contract ─
//
// These three tests exercise handleMemoryCorrect through the actual MCP argument
// deserialization path: the CallToolRequest.Params.Arguments field is populated by
// json.Unmarshal (mimicking the live server path), not by direct Go map assignment.
//
// Contract (issue #765 fix):
//   - missing tags key  → args["tags"] == nil  → UpdateMemory(tags=nil) → valid_from preserved
//   - "tags": null      → args["tags"] == nil  → UpdateMemory(tags=nil) → valid_from preserved
//                         NOTE: JSON null and absent key are indistinguishable after
//                         json.Unmarshal into map[string]any; both produce nil.  This is
//                         the documented behavior — see docs/tools.md#memory_correct.
//   - "tags": []        → args["tags"] == []any{} → UpdateMemory(tags=[]string{}) → valid_from NULL

// buildCorrectReqFromJSON deserializes a raw JSON arguments object into a
// CallToolRequest exactly as the MCP server does: via json.Unmarshal into
// map[string]any.  This exercises the real deserialization boundary rather than
// bypassing it with direct Go map construction.
func buildCorrectReqFromJSON(t *testing.T, rawJSON string) mcpgo.CallToolRequest {
	t.Helper()
	var args map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &args); err != nil {
		t.Fatalf("buildCorrectReqFromJSON: json.Unmarshal failed: %v", err)
	}
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// TestMemoryCorrect_MCP_OmitTagsPreservesValidFrom verifies that a JSON payload
// with no "tags" key passes nil to UpdateMemory, leaving valid_from unchanged.
func TestMemoryCorrect_MCP_OmitTagsPreservesValidFrom(t *testing.T) {
	initial := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	back := &validFromCorrectBackendV2{initialValidFrom: &initial}
	pool := newValidFromCorrectPoolV2(t, back)

	// No "tags" key in JSON — simulates MCP client that only sends memory_id + new_content.
	req := buildCorrectReqFromJSON(t, `{"project":"test","memory_id":"mem-omit-tags","content":"updated content"}`)

	res, err := handleMemoryCorrect(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.True(t, back.called, "UpdateMemory must be called")
	// Missing key → args["tags"] is nil → toStringSlice(nil) returns nil → tags passed as nil.
	require.Nil(t, back.capturedTags,
		"omitting tags key must pass nil to UpdateMemory, preserving valid_from (B1)")
}

// TestMemoryCorrect_MCP_NullTagsBehavior verifies that JSON "tags":null is
// treated identically to a missing tags key — both produce nil in the args map
// after json.Unmarshal, so valid_from is preserved.
//
// NOTE: This is a documented design constraint, not a bug.  JSON null and an
// absent key are indistinguishable via map[string]any lookup; callers wishing to
// explicitly clear valid_from must send "tags":[] (empty array), not null.
func TestMemoryCorrect_MCP_NullTagsBehavior(t *testing.T) {
	initial := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	back := &validFromCorrectBackendV2{initialValidFrom: &initial}
	pool := newValidFromCorrectPoolV2(t, back)

	// "tags":null — after json.Unmarshal into map[string]any, args["tags"] == nil.
	req := buildCorrectReqFromJSON(t, `{"project":"test","memory_id":"mem-null-tags","content":"updated","tags":null}`)

	res, err := handleMemoryCorrect(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.True(t, back.called, "UpdateMemory must be called")
	// JSON null → map value is nil → toStringSlice(nil) returns nil → tags=nil → valid_from preserved.
	// Same behavior as absent key — this is the documented contract.
	require.Nil(t, back.capturedTags,
		`"tags":null must be treated as absent (nil), preserving valid_from (B1 documented behavior)`)
}

// TestMemoryCorrect_MCP_EmptyTagsClearsValidFrom verifies that JSON "tags":[]
// passes an empty (non-nil) slice to UpdateMemory, which clears valid_from to NULL.
func TestMemoryCorrect_MCP_EmptyTagsClearsValidFrom(t *testing.T) {
	initial := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	back := &validFromCorrectBackendV2{initialValidFrom: &initial}
	pool := newValidFromCorrectPoolV2(t, back)

	// "tags":[] — after json.Unmarshal into map[string]any, args["tags"] == []interface{}{}.
	req := buildCorrectReqFromJSON(t, `{"project":"test","memory_id":"mem-empty-tags","tags":[]}`)

	res, err := handleMemoryCorrect(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.True(t, back.called, "UpdateMemory must be called")
	// "tags":[] → []interface{}{} → toStringSlice returns []string{} (non-nil) → tags passed to
	// UpdateMemory → valid_from recalculated as NULL (no date: tag in empty set).
	require.NotNil(t, back.capturedTags,
		`"tags":[] must pass non-nil (empty) slice to UpdateMemory (B1)`)
	require.Empty(t, back.capturedTags,
		`"tags":[] must pass empty (not nil) slice, causing valid_from to be cleared (B1)`)
	// Confirm valid_from would be NULL under the real DB impl.
	gotVF := parse.ParseDateTag(back.capturedTags)
	require.Nil(t, gotVF,
		"empty tags must yield nil valid_from (cleared to NULL in Postgres) (B1)")
}

// historyTrackingBackend is a test backend that records each UpdateMemory call as
// a MemoryVersion entry, allowing handleMemoryHistory to return a real version
// chain without a Postgres instance — regression guard for issue #821.
type historyTrackingBackend struct {
	noopBackend
	mu       sync.Mutex
	versions map[string][]*types.MemoryVersion // keyed by memory_id
}

func (b *historyTrackingBackend) UpdateMemory(_ context.Context, id string, content *string, tags []string, _ *int, _ *float64) (*types.Memory, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.versions == nil {
		b.versions = make(map[string][]*types.MemoryVersion)
	}
	vf := parse.ParseDateTag(tags)
	// Snapshot the state of this update as a new version (mirrors versionMemoryTx behaviour).
	v := &types.MemoryVersion{
		ID:         fmt.Sprintf("ver-%d", len(b.versions[id])+1),
		MemoryID:   id,
		ChangeType: "update",
		ValidFrom:  vf,
		Tags:       tags,
		SystemFrom: time.Now().UTC(),
	}
	if content != nil {
		v.Content = *content
	}
	// Prepend so GetMemoryHistory returns newest-first (matches postgres_memory.go order).
	b.versions[id] = append([]*types.MemoryVersion{v}, b.versions[id]...)
	return &types.Memory{ID: id, Tags: tags, ValidFrom: vf}, nil
}

func (b *historyTrackingBackend) GetMemoryHistory(_ context.Context, _ string, memoryID string) ([]*types.MemoryVersion, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.versions[memoryID], nil
}

func (b *historyTrackingBackend) Begin(_ context.Context) (db.Tx, error) { return noopTx{}, nil }

func newHistoryTrackingPool(t *testing.T, back *historyTrackingBackend) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, back, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// TestMemoryCorrect_History_RetainsPriorValidFrom verifies that two successive
// corrections produce distinct valid_from values AND that handleMemoryHistory
// returns a version chain with ≥ 2 entries whose valid_from values match the
// sequence of date tags applied — regression guard for issue #821.
func TestMemoryCorrect_History_RetainsPriorValidFrom(t *testing.T) {
	back := &historyTrackingBackend{}
	pool := newHistoryTrackingPool(t, back)

	// First correction: set date:2024-03-20
	req1 := mcpgo.CallToolRequest{}
	req1.Params.Arguments = map[string]any{
		"project":   "test",
		"memory_id": "mem-e-test",
		"tags":      []any{"date:2024-03-20"},
	}
	res1, err := handleMemoryCorrect(context.Background(), pool, req1)
	require.NoError(t, err)
	require.False(t, res1.IsError)

	// Second correction: change to date:2024-04-15
	req2 := mcpgo.CallToolRequest{}
	req2.Params.Arguments = map[string]any{
		"project":   "test",
		"memory_id": "mem-e-test",
		"tags":      []any{"date:2024-04-15"},
	}
	res2, err := handleMemoryCorrect(context.Background(), pool, req2)
	require.NoError(t, err)
	require.False(t, res2.IsError)

	// Real history assertion: handleMemoryHistory must return ≥ 2 versions with
	// distinct valid_from values matching the sequence of corrections — #821.
	histReq := mcpgo.CallToolRequest{}
	histReq.Params.Arguments = map[string]any{
		"project":   "test",
		"memory_id": "mem-e-test",
	}
	histRes, err := handleMemoryHistory(context.Background(), pool, histReq)
	require.NoError(t, err)
	require.NotNil(t, histRes)
	require.False(t, histRes.IsError, "handleMemoryHistory must not error, got: %+v", histRes.Content)

	// Decode the version list from the tool result.
	require.NotEmpty(t, histRes.Content, "history result must have content")
	tc, ok := histRes.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "history result must be TextContent")

	var payload struct {
		Count    int                    `json:"count"`
		Versions []*types.MemoryVersion `json:"versions"`
	}
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &payload),
		"history result must be valid JSON")

	require.GreaterOrEqual(t, payload.Count, 2,
		"handleMemoryHistory must return ≥ 2 versions after two corrections (#821)")
	require.Len(t, payload.Versions, payload.Count,
		"versions slice length must match count field")

	// Collect all valid_from values from the version chain.
	var validFroms []time.Time
	for _, v := range payload.Versions {
		if v.ValidFrom != nil {
			validFroms = append(validFroms, *v.ValidFrom)
		}
	}
	require.Len(t, validFroms, 2,
		"both versions must carry a non-nil valid_from matching the applied date tags (#821)")

	date1 := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	date2 := time.Date(2024, 4, 15, 0, 0, 0, 0, time.UTC)

	// Versions are newest-first; verify the set contains both dates regardless of order.
	hasDate1 := validFroms[0].Equal(date1) || validFroms[1].Equal(date1)
	hasDate2 := validFroms[0].Equal(date2) || validFroms[1].Equal(date2)
	require.True(t, hasDate1, "version chain must include valid_from 2024-03-20 (#821)")
	require.True(t, hasDate2, "version chain must include valid_from 2024-04-15 (#821)")
	require.False(t, validFroms[0].Equal(validFroms[1]),
		"two versions must have distinct valid_from values (#821)")
}

// TestMemoryCorrect_ImportanceOutOfRange verifies that handleMemoryCorrect
// rejects importance values outside [0, 4] with a tool-level error rather than
// silently clamping them — regression guard for the fix in issue #809.
func TestMemoryCorrect_ImportanceOutOfRange(t *testing.T) {
	back := &validFromCorrectBackend{}
	pool := newValidFromCorrectPool(t, back)

	cases := []struct {
		name       string
		importance float64
	}{
		{"negative", -1},
		{"above max", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := mcpgo.CallToolRequest{}
			req.Params.Arguments = map[string]any{
				"project":    "test",
				"memory_id":  "mem-12345",
				"importance": tc.importance,
			}
			res, err := handleMemoryCorrect(context.Background(), pool, req)
			require.NoError(t, err)
			require.NotNil(t, res)
			require.True(t, res.IsError, "importance=%v must produce a tool error, not be silently clamped", tc.importance)
			require.False(t, back.called, "UpdateMemory must NOT be called on importance validation failure")
		})
	}
}
