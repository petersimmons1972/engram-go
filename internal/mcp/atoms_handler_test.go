package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/stretchr/testify/require"
)

func testAtomsDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	return dsn
}

func uniqueAtomsProject(base string) string {
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

func testAtom(subject, value string) *atom.Atom {
	return &atom.Atom{
		Type:       atom.TypePreference,
		Subject:    subject,
		Predicate:  "prefers",
		Value:      value,
		Statement:  fmt.Sprintf("%s prefers %s.", subject, value),
		Scope:      atom.ScopeGlobal,
		Confidence: 0.9,
	}
}

func postAtomsStore(t *testing.T, s *Server, project string, at *atom.Atom, embedding []float32) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"action":    "store",
		"project":   project,
		"atom":      at,
		"embedding": embedding,
	})
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/atoms", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleAtoms(w, req)
	return w
}

func atomEmbeddingText(t *testing.T, ctx context.Context, pool *pgxpool.Pool, atomID string) string {
	t.Helper()
	var got string
	err := pool.QueryRow(ctx, `SELECT embedding::text FROM atom_embeddings WHERE atom_id = $1`, atomID).Scan(&got)
	require.NoError(t, err)
	return got
}

func TestAtomsStoreRejectsCrossProjectCallerSuppliedID(t *testing.T) {
	ctx := context.Background()
	dsn := testAtomsDSN(t)
	projectA := uniqueAtomsProject("atoms-a")
	projectB := uniqueAtomsProject("atoms-b")

	pool := NewTestPoolWithDSN(t, ctx, dsn, projectA)
	t.Cleanup(func() {
		if h, err := pool.Get(ctx, projectB); err == nil && h != nil && h.Engine != nil {
			h.Engine.Close()
		}
	})
	s := &Server{pool: pool, cfg: testConfig()}

	initial := make([]float32, 1024)
	initial[0] = 0.25
	createResp := postAtomsStore(t, s, projectA, testAtom("alice", "tea"), initial)
	require.Equal(t, http.StatusCreated, createResp.Code)

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID)

	queryPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(queryPool.Close)
	before := atomEmbeddingText(t, ctx, queryPool, created.ID)

	attackerAtom := testAtom("mallory", "coffee")
	attackerAtom.ID = created.ID
	replacement := make([]float32, 1024)
	replacement[0] = 0.99
	attackResp := postAtomsStore(t, s, projectB, attackerAtom, replacement)

	require.Equal(t, http.StatusBadRequest, attackResp.Code)
	after := atomEmbeddingText(t, ctx, queryPool, created.ID)
	require.Equal(t, before, after, "cross-project atom ID reuse must not mutate the existing embedding")
}
