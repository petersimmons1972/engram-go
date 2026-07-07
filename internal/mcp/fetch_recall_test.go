package mcp

// Unit tests for handle-mode handleMemoryRecall and the two execFetch boundary
// cases not covered by fetch_exec_test.go (GetMemory error propagation and the
// maxBytes=0 no-truncation guarantee).
//
// Uses newTestNoopPool from explore_handler_test.go for the recall tests.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/metrics"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// ── minimal fetcher stub ──────────────────────────────────────────────────────
// Named recallStubFetcher to avoid collision with fakeFetcher in fetch_exec_test.go.

type recallStubFetcher struct {
	mem    *types.Memory
	memErr error
	chunks []*types.Chunk
}

func (s *recallStubFetcher) GetMemoryByID(_ context.Context, _ string) (*types.Memory, error) {
	return s.mem, s.memErr
}

func (s *recallStubFetcher) GetMemoryByIDInProject(_ context.Context, _ string, project string) (*types.Memory, error) {
	if s.mem != nil && s.mem.Project != "" && s.mem.Project != project {
		return nil, nil
	}
	return s.mem, s.memErr
}

func (s *recallStubFetcher) GetChunksForMemory(_ context.Context, _ string) ([]*types.Chunk, error) {
	return s.chunks, nil
}

type recallTrackingBackend struct {
	noopBackend
	ftsResults   []db.FTSResult
	storedEvents atomic.Int64
	increments   atomic.Int64
	// lastEventID is the ID of the most recent event passed to
	// StoreRetrievalEvent. RecordFeedback validates against it so feedback
	// with an event_id that recall never issued fails, mirroring
	// PostgresBackend.RecordFeedback's "retrieval event %q not found" error
	// (internal/db/postgres_feedback.go). Without this, the #1259 round-trip
	// test would pass with any fabricated UUID.
	lastEventID atomic.Value // string
}

func (b *recallTrackingBackend) FTSSearch(_ context.Context, project, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	if b.ftsResults != nil {
		return b.ftsResults, nil
	}
	return []db.FTSResult{{
		Memory: &types.Memory{
			ID:        "mem-1",
			Project:   project,
			Content:   "read-only recall telemetry regression memory",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
		Score: 1,
	}}, nil
}

func (b *recallTrackingBackend) StoreRetrievalEvent(_ context.Context, event *types.RetrievalEvent) error {
	b.storedEvents.Add(1)
	b.lastEventID.Store(event.ID)
	return nil
}

// RecordFeedback fails for event IDs that were never stored via
// StoreRetrievalEvent, matching PostgresBackend.RecordFeedback's behavior
// when GetRetrievalEvent returns nil.
func (b *recallTrackingBackend) RecordFeedback(_ context.Context, eventID string, _ []string) error {
	if stored, _ := b.lastEventID.Load().(string); stored == "" || stored != eventID {
		return fmt.Errorf("retrieval event %q not found", eventID)
	}
	return nil
}

func (b *recallTrackingBackend) IncrementTimesRetrieved(_ context.Context, _ []string) error {
	b.increments.Add(1)
	return nil
}

func newRecallTrackingPool(t *testing.T, backend *recallTrackingBackend) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

type recallLimitBackend struct {
	noopBackend
}

func (b *recallLimitBackend) FTSSearch(_ context.Context, project, _ string, limit int, _, _ *time.Time) ([]db.FTSResult, error) {
	results := make([]db.FTSResult, 0, limit)
	now := time.Now().UTC()
	for i := 0; i < limit; i++ {
		results = append(results, db.FTSResult{
			Memory: &types.Memory{
				ID:        "mem-limit-" + string(rune('a'+i)),
				Project:   project,
				Content:   "limit alias regression memory",
				CreatedAt: now,
				UpdatedAt: now,
			},
			Score: float64(limit - i),
		})
	}
	return results, nil
}

type handleFastPathBackend struct {
	noopBackend
	relationshipBatchCalls atomic.Int64
	touchCalls             atomic.Int64
	updateChunkCalls       atomic.Int64
}

func (b *handleFastPathBackend) FTSSearch(_ context.Context, project, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	now := time.Now().UTC()
	return []db.FTSResult{{
		Memory: &types.Memory{
			ID:        "mem-handle-fast-path",
			Project:   project,
			Content:   "handle mode should stay lightweight",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Score: 1,
	}}, nil
}

func (b *handleFastPathBackend) GetRelationshipsBatch(_ context.Context, _ string, _ []string) (map[string][]types.Relationship, error) {
	b.relationshipBatchCalls.Add(1)
	return map[string][]types.Relationship{}, nil
}

func (b *handleFastPathBackend) TouchMemories(_ context.Context, _ []string) error {
	b.touchCalls.Add(1)
	return nil
}

func (b *handleFastPathBackend) UpdateChunkLastMatched(_ context.Context, _ string) error {
	b.updateChunkCalls.Add(1)
	return nil
}

func newHandleFastPathPool(t *testing.T, backend *handleFastPathBackend) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// ── execFetch boundary cases not in fetch_exec_test.go ───────────────────────

// TestExecFetch_GetMemoryError: GetMemoryByID returns a non-nil error → error is
// propagated unchanged (not swallowed or wrapped with different semantics).
func TestExecFetch_GetMemoryError(t *testing.T) {
	sentinel := errors.New("db exploded")
	f := &recallStubFetcher{memErr: sentinel}
	_, err := execFetch(context.Background(), f, "default", "any-id", "summary", 0, nil)
	require.ErrorIs(t, err, sentinel)
}

// TestExecFetch_FullDetail_ZeroMaxBytes: maxBytes=0 must disable truncation
// regardless of content length, and truncated must be false.
func TestExecFetch_FullDetail_ZeroMaxBytes(t *testing.T) {
	longContent := string(make([]byte, 200_000)) // 200 KB
	f := &recallStubFetcher{mem: &types.Memory{
		ID:      "big-mem",
		Project: "p",
		Content: longContent,
	}}
	out, err := execFetch(context.Background(), f, "p", "big-mem", "full", 0, nil)
	require.NoError(t, err)
	content, ok := out["content"].(string)
	require.True(t, ok)
	require.Equal(t, len(longContent), len(content), "content must not be truncated when maxBytes=0")
	truncated, _ := out["truncated"].(bool)
	require.False(t, truncated)
}

// ── handleMemoryRecall handle-mode tests ─────────────────────────────────────

// parseRecallResult decodes the first text content item of a non-error
// CallToolResult into a map[string]any.
func parseRecallResult(t *testing.T, res *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error tool result, got: %+v", res.Content)
	require.NotEmpty(t, res.Content, "result content must not be empty")
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "first content item must be TextContent, got %T", res.Content[0])
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &out))
	return out
}

// TestMemoryRecall_EmptyQuery_ReturnsValidationError: empty query must return a
// clean MCP tool error (IsError=true) and a nil Go error — not a WARN log.
// The handler must not reach the DB layer for caller input mistakes.
func TestMemoryRecall_EmptyQuery_ReturnsValidationError(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "",
	}
	cfg := Config{RecallDefaultMode: "handle"}

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err, "caller input error must NOT be a Go error (would produce WARN log)")
	require.NotNil(t, res)
	require.True(t, res.IsError, "empty query must return an MCP tool error result")
	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, text.Text, "query")
}

func TestHandleMemoryRecall_InvalidMode_ReturnsValidationError(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "some query",
		"mode":    "unsupported",
	}
	cfg := testConfig()

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.IsError, "invalid mode must return an MCP tool error")
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, text.Text, "mode must be one of")
}

// TestMemoryRecall_MissingQuery_ReturnsValidationError: missing query key (not
// supplied at all) must also return a clean MCP tool error, not a Go error.
func TestMemoryRecall_MissingQuery_ReturnsValidationError(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		// query key omitted entirely
	}
	cfg := Config{RecallDefaultMode: "handle"}

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err, "caller input error must NOT be a Go error (would produce WARN log)")
	require.NotNil(t, res)
	require.True(t, res.IsError, "missing query must return an MCP tool error result")
}

// TestHandleMemoryRecall_HandleMode_EmptyResults: valid query against noopBackend
// returns zero results; response must contain handles + count + fetch_hint and
// must NOT contain a results key.
func TestHandleMemoryRecall_HandleMode_EmptyResults(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "what is the meaning of life",
	}
	cfg := Config{RecallDefaultMode: "handle"}

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)

	out := parseRecallResult(t, res)

	// Required keys.
	_, hasHandles := out["handles"]
	require.True(t, hasHandles, "handle mode must return 'handles' key")
	count, hasCount := out["count"]
	require.True(t, hasCount, "handle mode must return 'count' key")
	require.Equal(t, float64(0), count, "empty backend → count must be 0")
	_, hasFetchHint := out["fetch_hint"]
	require.True(t, hasFetchHint, "handle mode must return 'fetch_hint' key")

	// Forbidden key.
	_, hasResults := out["results"]
	require.False(t, hasResults, "handle mode must NOT return 'results' key")
}

// TestHandleMemoryRecall_DefaultMode_ReturnsResultsKey: RecallDefaultMode="" (default
// full mode) must return results key and must not return handles key.
// Scope: single-project, non-rerank, non-federated, no conflicts enrichment.
func TestHandleMemoryRecall_DefaultMode_ReturnsResultsKey(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "what is the meaning of life",
	}
	cfg := testConfig() // RecallDefaultMode="" → full results path

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)

	out := parseRecallResult(t, res)

	_, hasResults := out["results"]
	require.True(t, hasResults, "default mode must return 'results' key")

	_, hasHandles := out["handles"]
	require.False(t, hasHandles, "default mode must NOT return 'handles' key")
}

func TestHandleMemoryRecall_DefaultDoesNotRecordRetrievalEvent(t *testing.T) {
	backend := &recallTrackingBackend{}
	pool := newRecallTrackingPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "read only recall telemetry",
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	out := parseRecallResult(t, res)

	require.Equal(t, float64(1), out["count"])
	require.NotContains(t, out, "event_id", "read-only recall must not return a feedback event by default")
	require.Zero(t, backend.storedEvents.Load(), "read-only recall must not store retrieval events by default")
	require.Zero(t, backend.increments.Load(), "read-only recall must not increment retrieval counters by default")
}

func TestHandleMemoryRecall_RecordEventOptInRecordsRetrievalEvent(t *testing.T) {
	backend := &recallTrackingBackend{}
	pool := newRecallTrackingPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":      "test",
		"query":        "record recall telemetry",
		"record_event": true,
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	out := parseRecallResult(t, res)

	require.Equal(t, float64(1), out["count"])
	require.NotEmpty(t, out["event_id"], "record_event=true should return a feedback event")
	require.Equal(t, int64(1), backend.storedEvents.Load())
	require.Equal(t, int64(1), backend.increments.Load())
}

// TestHandleMemoryRecall_EventIDRoundTripsToMemoryFeedback is the regression
// test for #1259: memory_feedback's event_id parameter is documented as "the
// id returned by memory_recall", but that id is only populated when the
// recall call opts in via record_event=true. This test drives the full
// round trip — recall with record_event=true, extract event_id from the
// response, and feed it back into memory_feedback — to prove the documented
// contract actually works end to end.
func TestHandleMemoryRecall_EventIDRoundTripsToMemoryFeedback(t *testing.T) {
	backend := &recallTrackingBackend{}
	pool := newRecallTrackingPool(t, backend)

	recallReq := mcpgo.CallToolRequest{}
	recallReq.Params.Arguments = map[string]any{
		"project":      "test",
		"query":        "round trip event id to feedback",
		"record_event": true,
	}
	recallRes, err := handleMemoryRecall(context.Background(), pool, recallReq, testConfig())
	require.NoError(t, err)
	recallOut := parseRecallResult(t, recallRes)

	eventID, ok := recallOut["event_id"].(string)
	require.True(t, ok, "event_id must be present and a string when record_event=true")
	require.NotEmpty(t, eventID)
	require.Equal(t, "Call memory_feedback with this event_id and the memory_ids you used", recallOut["feedback_hint"])

	feedbackReq := mcpgo.CallToolRequest{}
	feedbackReq.Params.Arguments = map[string]any{
		"project":    "test",
		"event_id":   eventID,
		"memory_ids": []any{"mem-1"},
	}
	feedbackRes, err := handleMemoryFeedback(context.Background(), pool, feedbackReq)
	require.NoError(t, err, "memory_feedback must accept the event_id returned by memory_recall")
	feedbackOut := parseRecallResult(t, feedbackRes)
	require.Equal(t, "recorded", feedbackOut["status"])
	require.Equal(t, float64(1), feedbackOut["count"])
}

// TestHandleMemoryFeedback_UnknownEventIDFails is the negative counterpart of
// the #1259 round-trip test: a syntactically valid UUID that memory_recall
// never issued must be rejected, mirroring PostgresBackend.RecordFeedback's
// "retrieval event %q not found" behavior. This guards the round-trip test
// itself — if the fake backend accepted any UUID, the round trip would prove
// nothing about the recall → feedback contract.
func TestHandleMemoryFeedback_UnknownEventIDFails(t *testing.T) {
	backend := &recallTrackingBackend{}
	pool := newRecallTrackingPool(t, backend)

	// Record a real event first so the backend has a known-good ID on file.
	recallReq := mcpgo.CallToolRequest{}
	recallReq.Params.Arguments = map[string]any{
		"project":      "test",
		"query":        "round trip event id to feedback",
		"record_event": true,
	}
	recallRes, err := handleMemoryRecall(context.Background(), pool, recallReq, testConfig())
	require.NoError(t, err)
	recallOut := parseRecallResult(t, recallRes)
	realEventID, _ := recallOut["event_id"].(string)
	require.NotEmpty(t, realEventID)

	// A valid UUID that recall never returned must fail.
	fabricated := "018f3c6e-1111-7222-8333-444455556666"
	require.NotEqual(t, realEventID, fabricated)
	feedbackReq := mcpgo.CallToolRequest{}
	feedbackReq.Params.Arguments = map[string]any{
		"project":    "test",
		"event_id":   fabricated,
		"memory_ids": []any{"mem-1"},
	}
	_, err = handleMemoryFeedback(context.Background(), pool, feedbackReq)
	require.Error(t, err, "memory_feedback must reject an event_id that recall never issued")
	require.Contains(t, err.Error(), "not found")
}

func TestHandleMemoryRecall_HandleModeRecordEventReturnsFeedbackEvent(t *testing.T) {
	backend := &recallTrackingBackend{}
	pool := newRecallTrackingPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":      "test",
		"query":        "record recall telemetry",
		"record_event": true,
	}

	cfg := Config{RecallDefaultMode: "handle"}
	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	out := parseRecallResult(t, res)

	require.Equal(t, float64(1), out["count"])
	require.NotEmpty(t, out["event_id"], "handle mode record_event=true should return a feedback event")
	require.Equal(t, int64(1), backend.storedEvents.Load())
	require.Equal(t, int64(1), backend.increments.Load())
}

func TestHandleMemoryRecall_EmptyResultsRecordEventReturnsFeedbackEvent(t *testing.T) {
	backend := &recallTrackingBackend{
		ftsResults: make([]db.FTSResult, 0),
	}
	pool := newRecallTrackingPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":      "test",
		"query":        "record recall telemetry miss",
		"record_event": true,
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	out := parseRecallResult(t, res)

	require.Equal(t, float64(0), out["count"])
	require.NotEmpty(t, out["event_id"], "record_event=true should return a feedback event on zero results")
	require.NotEmpty(t, out["feedback_hint"], "record_event=true should return a feedback_hint on zero results")
	require.Equal(t, int64(1), backend.storedEvents.Load())
}

func TestHandleMemoryRecall_NilMemoryResultsRecordEventReturnsFeedbackEvent(t *testing.T) {
	backend := &recallTrackingBackend{
		ftsResults: []db.FTSResult{{
			Memory: nil,
			Score:  0.5,
		}},
	}
	pool := newRecallTrackingPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":      "test",
		"query":        "record recall telemetry nil-mem",
		"record_event": true,
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	out := parseRecallResult(t, res)

	require.Equal(t, float64(1), out["count"])
	require.NotEmpty(t, out["event_id"], "record_event=true should return a feedback event when only nil memories are returned")
	require.NotEmpty(t, out["feedback_hint"], "record_event=true should return a feedback_hint when only nil memories are returned")
	require.Equal(t, int64(1), backend.storedEvents.Load())
}

func TestHandleMemoryRecall_LimitAliasMapsToTopK(t *testing.T) {
	backend := &recallLimitBackend{}
	limitPool := NewEnginePool(func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	})

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "limit alias",
		"limit":   3,
	}

	res, err := handleMemoryRecall(context.Background(), limitPool, req, testConfig())
	require.NoError(t, err)
	out := parseRecallResult(t, res)
	require.Equal(t, float64(3), out["count"], "memory_recall limit alias must map to top_k")
}

func TestHandleMemoryRecall_HandleModeSkipsEnrichmentAndWriteback(t *testing.T) {
	backend := &handleFastPathBackend{}
	pool := newHandleFastPathPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "lightweight handle mode",
	}
	cfg := Config{RecallDefaultMode: "handle"}

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	out := parseRecallResult(t, res)
	require.Equal(t, float64(1), out["count"])
	require.Zero(t, backend.relationshipBatchCalls.Load(), "handle mode must not fetch connected graph payloads")
	require.Equal(t, int64(1), backend.touchCalls.Load(), "handle mode must still refresh access timestamps for retention/prune safety")
	require.Zero(t, backend.updateChunkCalls.Load(), "FTS-only handle results must not write chunk match timestamps without a matched chunk id")
}

func TestHandleMemoryRecall_FederatedHandleModeSkipsEnrichmentAndWriteback(t *testing.T) {
	backend := &handleFastPathBackend{}
	pool := newHandleFastPathPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"projects": []any{"test-a", "test-b"},
		"query":    "lightweight handle mode",
	}
	cfg := testConfig()
	cfg.RecallDefaultMode = "handle"

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	out := parseRecallResult(t, res)
	require.Equal(t, float64(1), out["count"])
	require.Zero(t, backend.relationshipBatchCalls.Load(), "federated handle mode must not fetch connected graph payloads")
	require.Equal(t, int64(2), backend.touchCalls.Load(), "federated handle mode must still refresh access timestamps for retention/prune safety")
	require.Zero(t, backend.updateChunkCalls.Load(), "FTS-only federated handle results must not write chunk match timestamps without a matched chunk id")
}

func TestHandleMemoryRecall_FederatedDefaultDoesNotRecordRetrievalEvent(t *testing.T) {
	backend := &recallTrackingBackend{}
	pool := newRecallTrackingPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"projects": []any{"test-a", "test-b"},
		"query":    "federated read only recall telemetry",
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	out := parseRecallResult(t, res)

	require.Equal(t, float64(1), out["count"])
	require.Zero(t, backend.storedEvents.Load(), "federated read-only recall must not store retrieval events by default")
	require.Zero(t, backend.increments.Load(), "federated read-only recall must not increment retrieval counters by default")
}

func TestHandleMemoryRecall_FederatedRecordEventRejected(t *testing.T) {
	backend := &recallTrackingBackend{}
	pool := newRecallTrackingPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"projects":     []any{"test-a", "test-b"},
		"query":        "federated recall telemetry",
		"record_event": true,
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.IsError, "federated record_event must return an MCP tool error")
	require.Zero(t, backend.storedEvents.Load(), "rejected federated record_event must not store retrieval events")
	require.Zero(t, backend.increments.Load(), "rejected federated record_event must not increment retrieval counters")
}

func TestHandleMemoryRecall_FederatedInitFailuresReturnError(t *testing.T) {
	pool := NewEnginePool(func(ctx context.Context, project string) (*EngineHandle, error) {
		return nil, errors.New("boom")
	})
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"projects": []any{"test-a", "test-b"},
		"query":    "federated failure",
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.IsError, "all federated init failures should return an MCP tool error")
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, text.Text, "failed to initialize any requested federated project")
	require.Contains(t, text.Text, "test-a")
	require.Contains(t, text.Text, "test-b")
}

func TestHandleMemoryRecall_FederatedPartialInitFailureIncludesFailedProjects(t *testing.T) {
	pool := NewEnginePool(func(ctx context.Context, project string) (*EngineHandle, error) {
		if project == "project-fail-init" {
			return nil, errors.New("init failed for project-fail-init")
		}
		backend := &handleFastPathBackend{}
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	})
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"projects": []any{"project-ok", "project-fail-init"},
		"query":    "partial failures",
		"mode":     "handle",
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	out := parseRecallResult(t, res)
	require.Equal(t, float64(1), out["count"])
	failedRaw, ok := out["failed_projects"].([]any)
	require.True(t, ok, "partial failure metadata should be present as []any")
	require.Len(t, failedRaw, 1, "only one project should be reported as failed")
	failed, ok := failedRaw[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "project-fail-init", failed["project"])
	require.Equal(t, "init failed for project-fail-init", failed["error"])
}

// TestHandleMemoryRecall_EpisodeContextInjected verifies that
// episodeIDFromContext correctly extracts an episode ID placed by withEpisodeID,
// which is the mechanism handleMemoryRecall relies on for the Phase 3 episode
// boost. This is a unit-level test of the context plumbing; integration
// coverage lives in auto_episode_test.go.
func TestHandleMemoryRecall_EpisodeContextInjected(t *testing.T) {
	// Inject episode ID into context.
	ctx := withEpisodeID(context.Background(), "ep-recall-phase3")

	// Verify extraction succeeds and returns the correct ID.
	id, ok := episodeIDFromContext(ctx)
	if !ok || id != "ep-recall-phase3" {
		t.Fatalf("episodeIDFromContext failed: ok=%v id=%q", ok, id)
	}

	// Verify that a context without an episode ID returns ok=false.
	_, okEmpty := episodeIDFromContext(context.Background())
	if okEmpty {
		t.Fatal("episodeIDFromContext must return ok=false on a plain context")
	}

	// Verify that handleMemoryRecall runs without error when episode context is
	// present — the noopBackend returns empty results, so the episode path
	// exits cleanly without event recording.
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "episode boost smoke test",
	}
	cfg := testConfig() // full results mode
	res, err := handleMemoryRecall(ctx, pool, req, cfg)
	if err != nil {
		t.Fatalf("handleMemoryRecall returned unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("handleMemoryRecall returned nil result")
	}
}

// ── Fix A: degraded.embed boolean/reason consistency (#634 fix#4) ────────────

// TestDegradedMap_WhenNotDegraded_OmitsReasonKey asserts that when embedDegraded
// is false the "reason" key is absent from the degraded map. This prevents
// callers from seeing the inconsistent {"embed":false,"reason":"embed_timeout"}
// combination that was produced before the fix.
func TestDegradedMap_WhenNotDegraded_OmitsReasonKey(t *testing.T) {
	m := degradedMap(false, "embed_timeout")
	require.Equal(t, false, m["embed"])
	_, hasReason := m["reason"]
	require.False(t, hasReason, "reason key must be absent when embed is not degraded")
}

// TestDegradedMap_WhenDegraded_IncludesReasonKey asserts that when embedDegraded
// is true the "reason" key is present and matches the supplied string.
func TestDegradedMap_WhenDegraded_IncludesReasonKey(t *testing.T) {
	m := degradedMap(true, "embed_timeout")
	require.Equal(t, true, m["embed"])
	reason, hasReason := m["reason"]
	require.True(t, hasReason, "reason key must be present when embed is degraded")
	require.Equal(t, "embed_timeout", reason)
}

// TestHandleMemoryRecall_FullMode_NoDegradation_OmitsReason exercises the full
// results path of handleMemoryRecall with a healthy embedder (the noop pool
// always succeeds). It asserts that degraded.reason is absent from the response.
func TestHandleMemoryRecall_FullMode_NoDegradation_OmitsReason(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "something",
	}
	cfg := testConfig() // EmbedderHealth returns ok=true with no reason

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	out := parseRecallResult(t, res)

	degradedRaw, hasDegraded := out["degraded"]
	require.True(t, hasDegraded, "response must include a 'degraded' key")
	degraded, ok := degradedRaw.(map[string]any)
	require.True(t, ok, "degraded must be a map")
	require.Equal(t, false, degraded["embed"])
	_, hasReason := degraded["reason"]
	require.False(t, hasReason, "reason must be absent when embedder is healthy (Fix A: #634 fix#4)")
	_, hasWarnings := out["warnings"]
	require.False(t, hasWarnings, "warnings must be absent when recall is not degraded")
}

// TestHandleMemoryRecall_FullMode_Degraded_AddsWarnings verifies that when the
// EmbedderHealth probe reports unhealthy, the MCP response includes a degraded
// warning in the warnings field and the degraded map signals embed=true.
//
// NOTE: RecallDegradedTotal is NOT asserted here because it is owned by the
// engine layer (RecallWithOpts / RecallWithinMemory). The health-probe path
// (EmbedderHealth.Snapshot returning !ok) does not constitute a per-call embed
// failure — no embed call errored — so the engine never increments the counter.
// The MCP layer correctly adds warnings and response shape without touching the
// metric. (#973/#917 blocker fix — removed duplicate .Inc())
func TestHandleMemoryRecall_FullMode_Degraded_AddsWarnings(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "something",
	}
	cfg := testConfig()
	cfg.RouterURL = "http://litellm:4000"
	cfg.EmbedderHealth = NewEmbedderHealth(func(context.Context) (bool, string) {
		return false, "litellm_unreachable"
	}, 5*time.Second)

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	out := parseRecallResult(t, res)

	warningsRaw, hasWarnings := out["warnings"]
	require.True(t, hasWarnings, "warnings must be present when recall is degraded")
	warnings, ok := warningsRaw.([]any)
	require.True(t, ok)
	require.Contains(t, warnings, recallEmbedDegradedWarning)
}

// ── Blocker 2: embed-timeout degradation path counter coverage ───────────────

// errorEmbedder always returns an error from Embed, simulating an embed backend
// that is unavailable or timing out. This drives the embedDegraded=true path in
// RecallWithOpts (search/engine.go) without relying on the EmbedderHealth probe.
type errorEmbedder struct{}

func (errorEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("embed backend unavailable")
}
func (errorEmbedder) EmbedWithModel(_ context.Context, _ string) ([]float32, string, error) {
	return nil, "", errors.New("embed backend unavailable")
}
func (errorEmbedder) Name() string    { return "error-embedder" }
func (errorEmbedder) Dimensions() int { return 384 }

// newErrorEmbedPool builds an EnginePool backed by a noopBackend + errorEmbedder.
// RecallWithOpts on this pool always sets embedDegraded=true.
func newErrorEmbedPool(t *testing.T) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, noopBackend{}, errorEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// TestHandleMemoryRecall_EmbedError_IncrementsDegradedCounter verifies that
// when RecallWithOpts sets embedDegraded=true (embed backend hard-error path),
// RecallDegradedTotal with label "embed_error" is incremented exactly once by
// the engine layer and warnings are added to the response.
//
// Uses errorEmbedder (synchronous hard error, ctx.Err()==nil) so the engine
// classifies the reason as "embed_error" (not "embed_timeout"). The previous
// test checked "embed_timeout" because the MCP layer hardcoded that label —
// that mislabeling is now fixed: the engine is the sole counter owner and
// uses the actual failure reason. (#973/#917 blocker fix)
func TestHandleMemoryRecall_EmbedError_IncrementsDegradedCounter(t *testing.T) {
	pool := newErrorEmbedPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "embed error degraded path",
	}
	// EmbedderHealth returns healthy — only the embedDegraded=true path fires,
	// not the !ok path. This isolates the counter increment to the embed-error branch.
	cfg := testConfig()
	cfg.RouterURL = "http://litellm:4000"

	before := testutil.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_error"))
	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	out := parseRecallResult(t, res)

	// Verify warnings are present.
	warningsRaw, hasWarnings := out["warnings"]
	require.True(t, hasWarnings, "warnings must be present when embed backend errors")
	warnings, ok := warningsRaw.([]any)
	require.True(t, ok)
	require.Contains(t, warnings, recallEmbedDegradedWarning)

	// Verify the degraded field signals embed=true.
	degradedRaw, hasDegraded := out["degraded"]
	require.True(t, hasDegraded, "degraded key must be present")
	degradedMap, ok := degradedRaw.(map[string]any)
	require.True(t, ok, "degraded must be a map")
	require.Equal(t, true, degradedMap["embed"], "degraded.embed must be true on embed error path")

	// Counter must increment exactly once with label "embed_error" (engine is sole owner).
	after := testutil.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_error"))
	require.Equal(t, before+1, after, "RecallDegradedTotal[embed_error] must increment once on hard embed error (engine-owned)")
}

// TestHandleMemoryRecall_NilMemoryResultSurfacesDroppedHitsInDegradedField verifies that
// a nil-Memory FTS hit (dropped at engine layer) contributes to "count" and appears as
// dropped_hits inside the "degraded" map, while the results payload remains empty.
func TestHandleMemoryRecall_NilMemoryResultSurfacesDroppedHitsInDegradedField(t *testing.T) {
	backend := &recallTrackingBackend{
		ftsResults: []db.FTSResult{{
			Memory: nil,
			Score:  0.5,
		}},
	}
	pool := newRecallTrackingPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "record recall telemetry nil-mem degraded field",
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	out := parseRecallResult(t, res)

	require.Equal(t, float64(1), out["count"], "count must include the dropped nil-Memory hit")
	degraded, ok := out["degraded"].(map[string]any)
	require.True(t, ok, "degraded field must be a map")
	require.Equal(t, float64(1), degraded["dropped_hits"], "degraded.dropped_hits must reflect the one dropped hit")
	warnings, ok := out["warnings"].([]any)
	require.True(t, ok, "warnings field must be a list")
	require.Contains(t, warnings, "recall degraded: dropped backend hits with missing memory records")
	require.NotContains(t, warnings, recallEmbedDegradedWarning, "nil-memory dropped hits must not imply an embed fallback")
	results, ok := out["results"].([]any)
	require.True(t, ok, "results field must be a list")
	require.Empty(t, results, "the memories payload must never include a nil-Memory entry")
}
