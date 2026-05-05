package mcp

// Tests for the /quick-store REST endpoint (handleQuickStore).
// Uses a storeBackend (embeds noopBackend, overrides Begin) so Store succeeds
// without a real PostgreSQL instance.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

// storeBackend embeds noopBackend but returns a real (no-op) Tx from Begin,
// so the Store path can commit without panicking.
type storeBackend struct{ noopBackend }

func (storeBackend) Begin(_ context.Context) (db.Tx, error) { return noopTx{}, nil }

var _ db.Backend = storeBackend{}

// newQuickStoreServer builds a minimal *Server with a store-capable pool for /quick-store tests.
func newQuickStoreServer(t *testing.T) *Server {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, storeBackend{}, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	pool := NewEnginePool(factory)
	cfg := testConfig()
	return &Server{pool: pool, cfg: cfg, embedderHealth: cfg.EmbedderHealth}
}

// TestQuickStoreHandler_HappyPath verifies that a POST with valid content
// returns 200 and {"ok":true}.
func TestQuickStoreHandler_HappyPath(t *testing.T) {
	s := newQuickStoreServer(t)

	body, _ := json.Marshal(map[string]any{
		"content":    "pre-compact session snapshot",
		"project":    "global",
		"tags":       []string{"pre-compact", "test"},
		"importance": 1,
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["ok"])
}

// TestQuickStoreHandler_EmptyContent verifies that a POST with an empty content
// field is rejected with 400.
func TestQuickStoreHandler_EmptyContent(t *testing.T) {
	s := newQuickStoreServer(t)

	body, _ := json.Marshal(map[string]any{"content": "", "project": "global"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestQuickStoreHandler_MissingContent verifies that a POST with no content key
// at all is rejected with 400.
func TestQuickStoreHandler_MissingContent(t *testing.T) {
	s := newQuickStoreServer(t)

	body, _ := json.Marshal(map[string]any{"project": "global"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestQuickStoreHandler_WrongMethod verifies that a GET to /quick-store
// is rejected with 405.
func TestQuickStoreHandler_WrongMethod(t *testing.T) {
	s := newQuickStoreServer(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/quick-store", nil)
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// TestQuickStoreHandler_InvalidJSON verifies that a POST with malformed JSON
// is rejected with 400.
func TestQuickStoreHandler_InvalidJSON(t *testing.T) {
	s := newQuickStoreServer(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader([]byte("not json {")))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestQuickStoreHandler_OversizedContent verifies that content > 1 MiB is rejected with 400.
func TestQuickStoreHandler_OversizedContent(t *testing.T) {
	s := newQuickStoreServer(t)

	oversized := bytes.Repeat([]byte("x"), 1024*1024+1)
	body, _ := json.Marshal(map[string]any{
		"content": string(oversized),
		"project": "global",
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestQuickStoreHandler_TooManyTags verifies that > 64 tags are rejected with 400.
func TestQuickStoreHandler_TooManyTags(t *testing.T) {
	s := newQuickStoreServer(t)

	tags := make([]string, 65)
	for i := range tags {
		tags[i] = "tag"
	}

	body, _ := json.Marshal(map[string]any{
		"content":    "test",
		"project":    "global",
		"tags":       tags,
		"importance": 1,
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestQuickStoreHandler_TagTooLong verifies that tags > 256 chars are rejected with 400.
func TestQuickStoreHandler_TagTooLong(t *testing.T) {
	s := newQuickStoreServer(t)

	longTag := bytes.Repeat([]byte("x"), 257)
	body, _ := json.Marshal(map[string]any{
		"content":    "test",
		"project":    "global",
		"tags":       []string{string(longTag)},
		"importance": 1,
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestQuickStoreHandler_InvalidImportance verifies that importance outside [0,100] is rejected with 400.
func TestQuickStoreHandler_InvalidImportance(t *testing.T) {
	tests := []int{-1, 101}
	for _, imp := range tests {
		t.Run(fmt.Sprintf("importance=%d", imp), func(t *testing.T) {
			s := newQuickStoreServer(t)

			body, _ := json.Marshal(map[string]any{
				"content":    "test",
				"project":    "global",
				"importance": imp,
			})

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
			w := httptest.NewRecorder()

			s.handleQuickStore(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestQuickStoreHandler_InvalidProjectName verifies that project names with spaces,
// special chars, or too many chars are rejected with 400.
func TestQuickStoreHandler_InvalidProjectName(t *testing.T) {
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
			s := newQuickStoreServer(t)

			body, _ := json.Marshal(map[string]any{
				"content": "test",
				"project": tc.projectID,
			})

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
			w := httptest.NewRecorder()

			s.handleQuickStore(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}
