package mcp

// Tests for the /quick-store REST endpoint (handleQuickStore).
// Uses a storeBackend (embeds noopBackend, overrides Begin) so Store succeeds
// without a real PostgreSQL instance.

import (
	"bytes"
	"context"
	"encoding/json"
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
	return &Server{pool: pool}
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

	req := httptest.NewRequest(http.MethodPost, "/quick-store", bytes.NewReader(body))
	req = req.WithContext(context.Background())
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

	req := httptest.NewRequest(http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestQuickStoreHandler_MissingContent verifies that a POST with no content key
// at all is rejected with 400.
func TestQuickStoreHandler_MissingContent(t *testing.T) {
	s := newQuickStoreServer(t)

	body, _ := json.Marshal(map[string]any{"project": "global"})

	req := httptest.NewRequest(http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestQuickStoreHandler_WrongMethod verifies that a GET to /quick-store
// is rejected with 405.
func TestQuickStoreHandler_WrongMethod(t *testing.T) {
	s := newQuickStoreServer(t)

	req := httptest.NewRequest(http.MethodGet, "/quick-store", nil)
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// TestQuickStoreHandler_InvalidJSON verifies that a POST with malformed JSON
// is rejected with 400.
func TestQuickStoreHandler_InvalidJSON(t *testing.T) {
	s := newQuickStoreServer(t)

	req := httptest.NewRequest(http.MethodPost, "/quick-store", bytes.NewReader([]byte("not json {")))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}
