package mcp

// Tests for the /quick-recall REST endpoint (handleQuickRecall).
// Uses a noopBackend-backed pool so recall succeeds (returning zero results)
// without a real PostgreSQL instance.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// newQuickRecallServer builds a minimal *Server with a recall-capable pool
// for /quick-recall tests. noopBackend returns empty results, which is valid.
func newQuickRecallServer(t *testing.T) *Server {
	t.Helper()
	pool := newTestNoopPool(t)
	cfg := testConfig()
	return &Server{pool: pool, cfg: cfg, embedderHealth: cfg.EmbedderHealth}
}

// TestHandleQuickRecall_HappyPath verifies that a POST with a valid project
// and query returns 200 and a results array (empty from noopBackend).
func TestHandleQuickRecall_HappyPath(t *testing.T) {
	s := newQuickRecallServer(t)

	body, _ := json.Marshal(map[string]any{
		"query":   "tco section for edr vendor comparison",
		"project": "clearwatch",
		"tags":    []string{"section:tco", "canonical"},
		"limit":   3,
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-recall", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickRecall(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	results, ok := resp["results"]
	require.True(t, ok, "response must contain 'results' key")
	// noopBackend returns no memories; results must be an array (possibly empty).
	arr, ok := results.([]any)
	require.True(t, ok, "'results' must be a JSON array")
	require.Empty(t, arr, "noopBackend returns no memories")
}

// TestHandleQuickRecall_MissingProject verifies that a POST without a project
// field is rejected with 400.
func TestHandleQuickRecall_MissingProject(t *testing.T) {
	s := newQuickRecallServer(t)

	body, _ := json.Marshal(map[string]any{"query": "something"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-recall", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickRecall(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestHandleQuickRecall_MissingQuery verifies that a POST without a query
// field is rejected with 400.
func TestHandleQuickRecall_MissingQuery(t *testing.T) {
	s := newQuickRecallServer(t)

	body, _ := json.Marshal(map[string]any{"project": "clearwatch"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-recall", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickRecall(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestHandleQuickRecall_WrongMethod verifies that a GET to /quick-recall
// is rejected with 405.
func TestHandleQuickRecall_WrongMethod(t *testing.T) {
	s := newQuickRecallServer(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/quick-recall", nil)
	w := httptest.NewRecorder()

	s.handleQuickRecall(w, req)

	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// TestHandleQuickRecall_InvalidJSON verifies that a POST with malformed JSON
// is rejected with 400.
func TestHandleQuickRecall_InvalidJSON(t *testing.T) {
	s := newQuickRecallServer(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-recall", bytes.NewReader([]byte("not json {")))
	w := httptest.NewRecorder()

	s.handleQuickRecall(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestHandleQuickRecall_LimitClamped verifies that a limit > 20 is clamped to 20
// and the request still succeeds.
func TestHandleQuickRecall_LimitClamped(t *testing.T) {
	s := newQuickRecallServer(t)

	body, _ := json.Marshal(map[string]any{
		"query":   "anything",
		"project": "global",
		"limit":   999,
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-recall", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickRecall(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

// TestHandleQuickRecall_DefaultLimit verifies that a request with no limit
// still succeeds (uses the default of 5).
func TestHandleQuickRecall_DefaultLimit(t *testing.T) {
	s := newQuickRecallServer(t)

	body, _ := json.Marshal(map[string]any{
		"query":   "anything",
		"project": "global",
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-recall", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickRecall(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	_, ok := resp["results"]
	require.True(t, ok)
}

// TestHandleQuickRecall_InvalidProjectName verifies that project names with spaces,
// special chars, or too many chars are rejected with 400.
func TestHandleQuickRecall_InvalidProjectName(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
	}{
		{"spaces", "foo bar"},
		{"uppercase", "Foo"},
		{"parent_dir", "../etc"},
		{"special_chars", "foo@bar"},
		{"too_long", string(bytes.Repeat([]byte("x"), 65))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newQuickRecallServer(t)

			body, _ := json.Marshal(map[string]any{
				"query":   "test",
				"project": tc.projectID,
			})

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-recall", bytes.NewReader(body))
			w := httptest.NewRecorder()

			s.handleQuickRecall(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestNormalizeRecallMode_AllValidModes verifies that all documented modes
// are accepted and that an unknown mode is rejected.
func TestNormalizeRecallMode_AllValidModes(t *testing.T) {
	valid := []string{"", "handle", "full", "summary", "id_only",
		"SUMMARY", "ID_ONLY", "  full  "}
	for _, raw := range valid {
		got, err := normalizeRecallMode(raw)
		require.NoErrorf(t, err, "mode %q should be valid", raw)
		_ = got
	}

	_, err := normalizeRecallMode("unknown_mode")
	require.Error(t, err, "unknown mode should be rejected")
	require.Contains(t, err.Error(), "id_only", "error should list id_only as a valid mode")
}

// TestHandleQuickRecall_SummaryMode verifies that mode "summary" is accepted
// and returns 200 with a results array (fixes bug: normalizeRecallMode rejected "summary").
func TestHandleQuickRecall_SummaryMode(t *testing.T) {
	s := newQuickRecallServer(t)

	body, _ := json.Marshal(map[string]any{
		"query":   "test query",
		"project": "clearwatch",
		"mode":    "summary",
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-recall", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickRecall(w, req)

	require.Equal(t, http.StatusOK, w.Code, "mode=summary should return 200, not 400")
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	_, ok := resp["results"]
	require.True(t, ok, "response must contain 'results' key")
}
