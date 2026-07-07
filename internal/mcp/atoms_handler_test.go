package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/stretchr/testify/require"
)

func newAtomsTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := testConfig()
	return &Server{pool: newTestNoopPool(t), cfg: cfg}
}

func TestAtomsRequestBodyLimit(t *testing.T) {
	s := newAtomsTestServer(t)

	body, err := json.Marshal(map[string]any{
		"project": "global",
		"padding": string(bytes.Repeat([]byte("x"), 2*1024*1024)),
	})
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/atoms", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleAtoms(w, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestAtomsEmbeddingDimensionLimit(t *testing.T) {
	s := newAtomsTestServer(t)
	s.cfg.EmbedDimensions = 3

	body, err := json.Marshal(map[string]any{
		"action":  "store",
		"project": "global",
		"atom": atom.Atom{
			Type:       atom.TypePreference,
			Subject:    "user",
			Predicate:  "likes",
			Value:      "tea",
			Statement:  "user likes tea",
			Scope:      atom.ScopeGlobal,
			Confidence: 1,
		},
		"embedding": []float32{0.1, 0.2, 0.3, 0.4},
	})
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/atoms", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleAtoms(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp["error"], "EmbedDimensions")
}
