package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtomsRequestBodyLimit(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/atoms",
		bytes.NewReader([]byte(fmt.Sprintf(
			`{"action":"store","project":"global","padding":"%s"}`,
			string(bytes.Repeat([]byte("x"), atomsBodyLimitBytes)),
		))),
	)
	w := httptest.NewRecorder()

	s.handleAtoms(w, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestAtomsEmbeddingDimensionLimit(t *testing.T) {
	s := &Server{cfg: Config{EmbedDimensions: 3}}
	body, err := json.Marshal(map[string]any{
		"action":    "store",
		"project":   "global",
		"atom":      map[string]any{"type": "preference", "subject": "editor", "predicate": "prefers", "object": "vim"},
		"embedding": []float32{0.1, 0.2, 0.3, 0.4},
	})
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/atoms", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleAtoms(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "embedding dimension")
}
