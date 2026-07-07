package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
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

func atomRowCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, atomID, project string) int {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM atoms WHERE id = $1 AND project = $2`, atomID, project).Scan(&got)
	require.NoError(t, err)
	return got
}

func atomEmbeddingRowCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, atomID string) int {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM atom_embeddings WHERE atom_id = $1`, atomID).Scan(&got)
	require.NoError(t, err)
	return got
}

func TestAtomsStoreRejectsCallerSuppliedIDBeforeSideEffects(t *testing.T) {
	ctx := context.Background()
	dsn := testAtomsDSN(t)
	project := uniqueAtomsProject("atoms-id")

	pool := NewTestPoolWithDSN(t, ctx, dsn, project)
	s := &Server{pool: pool, cfg: testConfig()}

	queryPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(queryPool.Close)

	callerID := "caller-controlled-id-" + project
	at := testAtom("mallory", "coffee")
	at.ID = callerID
	resp := postAtomsStore(t, s, project, at, make([]float32, 1024))

	require.Equal(t, http.StatusBadRequest, resp.Code)
	require.Zero(t, atomRowCount(t, ctx, queryPool, callerID, project), "rejected caller ID must not insert an atom row")
	require.Zero(t, atomEmbeddingRowCount(t, ctx, queryPool, callerID), "rejected caller ID must not insert an embedding row")
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
	require.Zero(t, atomRowCount(t, ctx, queryPool, created.ID, projectB), "rejected cross-project request must not insert an atom row")
}
