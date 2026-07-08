package mcp

// Tests for the /quick-store REST endpoint (handleQuickStore).
// Uses a storeBackend (embeds noopBackend, overrides Begin) so Store succeeds
// without a real PostgreSQL instance.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

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

func TestQuickStoreRequestBodyLimit(t *testing.T) {
	s := newQuickStoreServer(t)

	body, err := json.Marshal(map[string]any{
		"content": "short body",
		"project": "global",
		"padding": string(bytes.Repeat([]byte("x"), 2*1024*1024)),
	})
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
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

// TestQuickStoreHandler_InvalidImportance verifies that importance outside [0,4] is rejected with 400.
// The range was previously documented as 0–100 but the handler (handleMemoryStore) always enforced
// 0–4. This test was updated as part of fix #768 to match the narrowed validator.
func TestQuickStoreHandler_InvalidImportance(t *testing.T) {
	tests := []int{-1, 5}
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

// TestValidateQuickStoreInput_RejectsImportanceAbove4 is the regression test for #768.
// Values in the formerly-accepted range (5–100) must now be rejected by the validator
// before they reach handleMemoryStore, giving callers an early, descriptive error.
func TestValidateQuickStoreInput_RejectsImportanceAbove4(t *testing.T) {
	for _, imp := range []int{5, 10, 50, 100} {
		t.Run(fmt.Sprintf("importance=%d", imp), func(t *testing.T) {
			err := validateQuickStoreInput("some content", "global", nil, imp)
			require.Error(t, err, "importance=%d should be rejected — was accepted by the buggy 0-100 validator", imp)
			require.Contains(t, err.Error(), "importance must be", "error message should describe the valid range")
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

// ttlCaptureBackend embeds storeBackend and records SetProjectTTL calls.
type ttlCaptureBackend struct {
	storeBackend
	mu              sync.Mutex
	capturedProject string
	capturedExpires *time.Time
	beginCalls      int
	returnErr       error
}

var _ db.Backend = (*ttlCaptureBackend)(nil)

func (b *ttlCaptureBackend) Begin(_ context.Context) (db.Tx, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.beginCalls++
	return noopTx{}, nil
}

func (b *ttlCaptureBackend) SetProjectTTL(_ context.Context, project string, _ time.Time, expiresAt *time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.capturedProject = project
	if expiresAt != nil {
		t := *expiresAt
		b.capturedExpires = &t
	}
	return b.returnErr
}

// newQuickStoreServerWithBackend builds a *Server that uses the given backend,
// letting tests observe SetProjectTTL calls.
func newQuickStoreServerWithBackend(t *testing.T, backend db.Backend) *Server {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	pool := NewEnginePool(factory)
	cfg := testConfig()
	return &Server{pool: pool, cfg: cfg, embedderHealth: cfg.EmbedderHealth}
}

// TestQuickStoreHandler_ExpiresAt_FutureTimestamp verifies that a POST with a
// future expires_at stores the memory without mutating project-level TTL. The
// field describes a memory write, not permission to make the whole project
// prune-eligible.
func TestQuickStoreHandler_ExpiresAt_FutureTimestamp(t *testing.T) {
	backend := &ttlCaptureBackend{}
	s := newQuickStoreServerWithBackend(t, backend)

	future := time.Now().UTC().Add(48 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"content":    "lme session content",
		"project":    "lme-run1-q001",
		"tags":       []string{"lme"},
		"expires_at": future.Format(time.RFC3339),
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["ok"])

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Empty(t, backend.capturedProject, "expires_at alone must not call SetProjectTTL")
	require.Nil(t, backend.capturedExpires, "expires_at alone must not stamp project_ttl")
}

// TestQuickStoreHandler_ExpiresAt_PastTimestamp verifies that a past expires_at
// is rejected with 400 before the store is written.
func TestQuickStoreHandler_ExpiresAt_PastTimestamp(t *testing.T) {
	backend := &ttlCaptureBackend{}
	s := newQuickStoreServerWithBackend(t, backend)

	past := time.Now().UTC().Add(-1 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"content":    "lme session content",
		"project":    "lme-run1-q001",
		"expires_at": past.Format(time.RFC3339),
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Empty(t, backend.capturedProject, "SetProjectTTL must not be called when expires_at is in the past")
}

// TestQuickStoreHandler_ProjectTTL_ExplicitIntent verifies that project-level
// TTL is only stamped when callers use the explicit project TTL fields.
func TestQuickStoreHandler_ProjectTTL_ExplicitIntent(t *testing.T) {
	backend := &ttlCaptureBackend{}
	s := newQuickStoreServerWithBackend(t, backend)

	future := time.Now().UTC().Add(48 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"content":            "lme session content",
		"project":            "lme-run1-q001",
		"tags":               []string{"lme"},
		"set_project_ttl":    true,
		"project_expires_at": future.Format(time.RFC3339),
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, "lme-run1-q001", backend.capturedProject, "SetProjectTTL should be called with the correct project")
	require.NotNil(t, backend.capturedExpires, "SetProjectTTL should receive a non-nil project_expires_at")
	delta := backend.capturedExpires.Sub(future)
	if delta < 0 {
		delta = -delta
	}
	require.Less(t, delta, 2*time.Second, "captured expiresAt should be within 2s of the requested value")
}

// TestQuickStoreHandler_ProjectTTL_RequiresExpiresAt verifies that setting the
// project TTL flag without a timestamp is rejected before the memory is stored.
func TestQuickStoreHandler_ProjectTTL_RequiresExpiresAt(t *testing.T) {
	backend := &ttlCaptureBackend{}
	s := newQuickStoreServerWithBackend(t, backend)

	body, _ := json.Marshal(map[string]any{
		"content":         "lme session content",
		"project":         "lme-run1-q001",
		"set_project_ttl": true,
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Empty(t, backend.capturedProject, "SetProjectTTL must not be called when project_expires_at is absent")
	require.Zero(t, backend.beginCalls, "memory must not be stored when project_expires_at is absent")
}

// TestQuickStoreHandler_ProjectTTL_RequiresExplicitFlag verifies that a project
// expiry timestamp without the explicit TTL flag is rejected before storing.
func TestQuickStoreHandler_ProjectTTL_RequiresExplicitFlag(t *testing.T) {
	backend := &ttlCaptureBackend{}
	s := newQuickStoreServerWithBackend(t, backend)

	future := time.Now().UTC().Add(48 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"content":            "lme session content",
		"project":            "lme-run1-q001",
		"project_expires_at": future.Format(time.RFC3339),
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Empty(t, backend.capturedProject, "SetProjectTTL must not be called without set_project_ttl")
	require.Zero(t, backend.beginCalls, "memory must not be stored without set_project_ttl")
}

// TestQuickStoreHandler_ProjectTTL_RequiresFutureTimestamp verifies that a past
// project_expires_at is rejected before storing.
func TestQuickStoreHandler_ProjectTTL_RequiresFutureTimestamp(t *testing.T) {
	backend := &ttlCaptureBackend{}
	s := newQuickStoreServerWithBackend(t, backend)

	past := time.Now().UTC().Add(-1 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"content":            "lme session content",
		"project":            "lme-run1-q001",
		"set_project_ttl":    true,
		"project_expires_at": past.Format(time.RFC3339),
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Empty(t, backend.capturedProject, "SetProjectTTL must not be called for past project_expires_at")
	require.Zero(t, backend.beginCalls, "memory must not be stored for past project_expires_at")
}

// TestQuickStoreHandler_ExpiresAt_Absent verifies that omitting expires_at stores
// the memory successfully without calling SetProjectTTL.
func TestQuickStoreHandler_ExpiresAt_Absent(t *testing.T) {
	backend := &ttlCaptureBackend{}
	s := newQuickStoreServerWithBackend(t, backend)

	body, _ := json.Marshal(map[string]any{
		"content": "ordinary memory",
		"project": "global",
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Empty(t, backend.capturedProject, "SetProjectTTL must not be called when expires_at is absent")
}

// TestQuickStoreHandler_ProjectTTL_TTLError verifies that a SetProjectTTL error
// does not fail the store when project TTL was explicitly requested.
func TestQuickStoreHandler_ProjectTTL_TTLError(t *testing.T) {
	backend := &ttlCaptureBackend{returnErr: errors.New("simulated TTL write failure")}
	s := newQuickStoreServerWithBackend(t, backend)

	future := time.Now().UTC().Add(24 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"content":            "lme session content",
		"project":            "lme-run1-q002",
		"set_project_ttl":    true,
		"project_expires_at": future.Format(time.RFC3339),
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	// Store must succeed even when TTL stamping fails.
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["ok"])

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, "lme-run1-q002", backend.capturedProject, "SetProjectTTL must be called even when it returns an error")
}
