package mcp

// Tests for the auto-episode phase 1 implementation.
// Tests are written before implementation (TDD) per CLAUDE.md policy.
//
// Phase 1 scope:
//   - Context carrier (session.go)
//   - Session registration hook with opt-in ?auto_episode=1
//   - Unregister hook closes the episode with context.Background()
//   - memory_store reads episode ID from context when not in args

import (
	"context"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
)

// ---------------------------------------------------------------------------
// Context carrier tests — session.go
// ---------------------------------------------------------------------------

// TestEpisodeIDContext_RoundTrip verifies that withEpisodeID and
// episodeIDFromContext are inverses: a stored ID can be retrieved.
func TestEpisodeIDContext_RoundTrip(t *testing.T) {
	ctx := context.Background()
	id := "ep-abc-123"
	ctx = withEpisodeID(ctx, id)

	got, ok := episodeIDFromContext(ctx)
	if !ok {
		t.Fatal("episodeIDFromContext returned ok=false; expected to find the episode ID")
	}
	if got != id {
		t.Fatalf("episodeIDFromContext returned %q; want %q", got, id)
	}
}

// TestEpisodeIDContext_EmptyStringReturnsFalse verifies that storing an empty
// string is treated the same as absent: episodeIDFromContext returns ok=false.
func TestEpisodeIDContext_EmptyStringReturnsFalse(t *testing.T) {
	ctx := context.Background()
	ctx = withEpisodeID(ctx, "")

	_, ok := episodeIDFromContext(ctx)
	if ok {
		t.Fatal("episodeIDFromContext returned ok=true for an empty episode ID; should return false")
	}
}

// TestEpisodeIDContext_Absent verifies that a plain context returns ok=false.
func TestEpisodeIDContext_Absent(t *testing.T) {
	ctx := context.Background()
	_, ok := episodeIDFromContext(ctx)
	if ok {
		t.Fatal("episodeIDFromContext returned ok=true on a plain context; should return false")
	}
}

// TestEpisodeIDContext_ChildContextInherits verifies that a child context
// inherits the episode ID from its parent.
func TestEpisodeIDContext_ChildContextInherits(t *testing.T) {
	parent := withEpisodeID(context.Background(), "ep-parent")
	child, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()

	got, ok := episodeIDFromContext(child)
	if !ok {
		t.Fatal("episode ID should propagate through child contexts")
	}
	if got != "ep-parent" {
		t.Fatalf("child got %q; want %q", got, "ep-parent")
	}
}

// TestEpisodeIDContext_DoesNotLeakToParent verifies that setting an episode ID
// on a child context does not affect the parent.
func TestEpisodeIDContext_DoesNotLeakToParent(t *testing.T) {
	parent := context.Background()
	child := withEpisodeID(parent, "ep-child")
	_ = child

	_, ok := episodeIDFromContext(parent)
	if ok {
		t.Fatal("parent context should not be affected by child episode ID")
	}
}

// ---------------------------------------------------------------------------
// Session hooks — register/unregister with ?auto_episode=1
// ---------------------------------------------------------------------------

// episodeTrackingBackend embeds noopBackend and records StartEpisode /
// EndEpisode calls so tests can assert on them without a real database.
type episodeTrackingBackend struct {
	noopBackend
	started []string // descriptions passed to StartEpisode
	ended   []string // episode IDs passed to EndEpisode
}

func (b *episodeTrackingBackend) StartEpisode(_ context.Context, _, description string) (*types.Episode, error) {
	b.started = append(b.started, description)
	return &types.Episode{ID: "ep-auto-test-001"}, nil
}

func (b *episodeTrackingBackend) EndEpisode(_ context.Context, id, _ string) error {
	b.ended = append(b.ended, id)
	return nil
}

// newAutoEpisodeServer builds a *Server backed by episodeTrackingBackend.
func newAutoEpisodeServer(t *testing.T, backend *episodeTrackingBackend) *Server {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	pool := NewEnginePool(factory)
	s := NewServer(pool, Config{})
	return s
}

// TestSSEConnect_WithAutoEpisodeFlag_StartsEpisode verifies that connecting
// with ?auto_episode=1 triggers StartEpisode on the backend.
func TestSSEConnect_WithAutoEpisodeFlag_StartsEpisode(t *testing.T) {
	backend := &episodeTrackingBackend{}
	s := newAutoEpisodeServer(t, backend)
	s.registerSessionHooks("test-api-key")

	// Simulate an SSE connect with ?auto_episode=1 by injecting the flag into
	// the context (the same way applyMiddleware will inject it for the real
	// SSE handler path).
	ctx := withAutoEpisodeFlag(context.Background())
	sess := &fakeClientSession{id: "sess-auto-ep-001"}

	s.mcp.GetHooks().RegisterSession(ctx, sess)

	if len(backend.started) == 0 {
		t.Fatal("expected StartEpisode to be called when ?auto_episode=1; got 0 calls")
	}
}

// TestSSEConnect_WithoutAutoEpisodeFlag_DoesNotStartEpisode verifies that
// connecting WITHOUT ?auto_episode=1 does NOT trigger StartEpisode.
func TestSSEConnect_WithoutAutoEpisodeFlag_DoesNotStartEpisode(t *testing.T) {
	backend := &episodeTrackingBackend{}
	s := newAutoEpisodeServer(t, backend)
	s.registerSessionHooks("test-api-key")

	// Plain context — no auto_episode flag.
	ctx := context.Background()
	sess := &fakeClientSession{id: "sess-no-auto-ep"}

	s.mcp.GetHooks().RegisterSession(ctx, sess)

	if len(backend.started) > 0 {
		t.Fatalf("expected StartEpisode NOT called without ?auto_episode=1; got %d calls", len(backend.started))
	}
}

// TestSSEDisconnect_WithAutoEpisode_EndsEpisode verifies that unregistering a
// session that had an episode closes it.
func TestSSEDisconnect_WithAutoEpisode_EndsEpisode(t *testing.T) {
	backend := &episodeTrackingBackend{}
	s := newAutoEpisodeServer(t, backend)
	s.registerSessionHooks("test-api-key")

	// Register with the flag so an episode is started.
	ctx := withAutoEpisodeFlag(context.Background())
	sess := &fakeClientSession{id: "sess-end-ep-001"}

	s.mcp.GetHooks().RegisterSession(ctx, sess)

	if len(backend.started) == 0 {
		t.Fatal("prerequisite: StartEpisode must be called first")
	}

	// Now simulate disconnect (cancel the context like the SSE handler does when
	// the HTTP request ends, then fire the unregister hook).
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled — mirrors the session context state at disconnect

	s.mcp.GetHooks().UnregisterSession(cancelledCtx, sess)

	if len(backend.ended) == 0 {
		t.Fatal("expected EndEpisode to be called on session disconnect; got 0 calls")
	}
	if backend.ended[0] != "ep-auto-test-001" {
		t.Fatalf("EndEpisode called with wrong ID %q; want %q", backend.ended[0], "ep-auto-test-001")
	}
}

// TestSSEDisconnect_WithoutAutoEpisode_NoEndCalled verifies that disconnecting
// a session that never started an episode does not call EndEpisode.
func TestSSEDisconnect_WithoutAutoEpisode_NoEndCalled(t *testing.T) {
	backend := &episodeTrackingBackend{}
	s := newAutoEpisodeServer(t, backend)
	s.registerSessionHooks("test-api-key")

	ctx := context.Background()
	sess := &fakeClientSession{id: "sess-no-ep-disconnect"}

	// Register without flag.
	s.mcp.GetHooks().RegisterSession(ctx, sess)
	// Unregister — should be a no-op for episode.
	s.mcp.GetHooks().UnregisterSession(ctx, sess)

	if len(backend.ended) > 0 {
		t.Fatalf("EndEpisode should not be called when no episode was started; got %d calls", len(backend.ended))
	}
}

// TestSSEConnect_NilPoolDoesNotPanicWithoutFlag verifies that the existing
// test (DoesNotAutoStartEpisode) still passes: a nil pool + no flag = no panic.
func TestSSEConnect_NilPoolDoesNotPanicWithoutFlag(t *testing.T) {
	s := NewServer(nil, Config{}) // nil pool — any pool access panics
	s.registerSessionHooks("test-key")

	sess := &fakeClientSession{id: "sess-nil-pool-no-flag"}
	ctx := context.Background() // no flag

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterSession panicked with nil pool and no flag: %v", r)
		}
	}()

	s.mcp.GetHooks().RegisterSession(ctx, sess)
}

// ---------------------------------------------------------------------------
// episodeIDFromContextOrArgs shared helper
// ---------------------------------------------------------------------------

// TestEpisodeIDFromContextOrArgs_ContextWins verifies that when args has no
// episode_id, the context value is returned.
func TestEpisodeIDFromContextOrArgs_ContextWins(t *testing.T) {
	ctx := withEpisodeID(context.Background(), "ep-from-context")
	args := map[string]any{}
	got := episodeIDFromContextOrArgs(ctx, args)
	if got != "ep-from-context" {
		t.Fatalf("expected ep-from-context, got %q", got)
	}
}

// TestEpisodeIDFromContextOrArgs_ArgsWin verifies that an explicit episode_id
// arg takes priority over the context value.
func TestEpisodeIDFromContextOrArgs_ArgsWin(t *testing.T) {
	ctx := withEpisodeID(context.Background(), "ep-from-context")
	args := map[string]any{"episode_id": "ep-explicit"}
	got := episodeIDFromContextOrArgs(ctx, args)
	if got != "ep-explicit" {
		t.Fatalf("expected ep-explicit, got %q", got)
	}
}

// TestEpisodeIDFromContextOrArgs_NeitherSource verifies that an empty string
// is returned when neither args nor context carries an episode ID.
func TestEpisodeIDFromContextOrArgs_NeitherSource(t *testing.T) {
	got := episodeIDFromContextOrArgs(context.Background(), map[string]any{})
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// memory_store reads episode ID from context
// ---------------------------------------------------------------------------

// episodeStoreBackend embeds noopBackend and captures stored memories so tests
// can inspect the EpisodeID that handleMemoryStore attached.
type episodeStoreBackend struct {
	noopBackend
	stored []*types.Memory
}

func (b *episodeStoreBackend) StoreMemoryTx(_ context.Context, _ db.Tx, m *types.Memory) error {
	b.stored = append(b.stored, m)
	return nil
}

func (b *episodeStoreBackend) Begin(_ context.Context) (db.Tx, error) { return noopTx{}, nil }

// newEpisodeStoreServer builds a *Server backed by episodeStoreBackend.
func newEpisodeStoreServer(t *testing.T, backend *episodeStoreBackend) *Server {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	pool := NewEnginePool(factory)
	return &Server{pool: pool}
}

// TestMemoryStore_EpisodeIDFromContext_Attached verifies that when a handler
// context carries an episode ID (injected by the session hook), memory_store
// attaches that ID to the stored memory even when the caller does not pass
// episode_id in the args.
func TestMemoryStore_EpisodeIDFromContext_Attached(t *testing.T) {
	backend := &episodeStoreBackend{}
	s := newEpisodeStoreServer(t, backend)
	_ = s

	ctx := withEpisodeID(context.Background(), "ep-from-ctx-001")

	var req mcpgo.CallToolRequest
	req.Params.Arguments = map[string]any{
		"content": "test memory with auto-episode",
		"project": "global",
	}

	_, err := handleMemoryStore(ctx, s.pool, req)
	if err != nil {
		t.Fatalf("handleMemoryStore returned error: %v", err)
	}

	if len(backend.stored) == 0 {
		t.Fatal("expected a memory to be stored; got none")
	}
	m := backend.stored[0]
	if m.EpisodeID != "ep-from-ctx-001" {
		t.Fatalf("EpisodeID = %q; want %q", m.EpisodeID, "ep-from-ctx-001")
	}
}

// TestMemoryStore_ExplicitEpisodeIDWinsOverContext verifies that an explicit
// episode_id arg takes precedence over the context value.
func TestMemoryStore_ExplicitEpisodeIDWinsOverContext(t *testing.T) {
	backend := &episodeStoreBackend{}
	s := newEpisodeStoreServer(t, backend)

	ctx := withEpisodeID(context.Background(), "ep-from-ctx")

	var req mcpgo.CallToolRequest
	req.Params.Arguments = map[string]any{
		"content":    "explicit episode id test",
		"project":    "global",
		"episode_id": "ep-explicit-001",
	}

	_, err := handleMemoryStore(ctx, s.pool, req)
	if err != nil {
		t.Fatalf("handleMemoryStore returned error: %v", err)
	}

	if len(backend.stored) == 0 {
		t.Fatal("expected a memory to be stored; got none")
	}
	m := backend.stored[0]
	if m.EpisodeID != "ep-explicit-001" {
		t.Fatalf("EpisodeID = %q; want %q (explicit should win)", m.EpisodeID, "ep-explicit-001")
	}
}

// TestMemoryStore_NoEpisodeContext_EpisodeIDEmpty verifies that without a
// context episode ID and without an explicit arg, EpisodeID stays empty.
func TestMemoryStore_NoEpisodeContext_EpisodeIDEmpty(t *testing.T) {
	backend := &episodeStoreBackend{}
	s := newEpisodeStoreServer(t, backend)

	var req mcpgo.CallToolRequest
	req.Params.Arguments = map[string]any{
		"content": "no episode context",
		"project": "global",
	}

	_, err := handleMemoryStore(context.Background(), s.pool, req)
	if err != nil {
		t.Fatalf("handleMemoryStore returned error: %v", err)
	}

	if len(backend.stored) == 0 {
		t.Fatal("expected a memory to be stored; got none")
	}
	m := backend.stored[0]
	if m.EpisodeID != "" {
		t.Fatalf("EpisodeID = %q; want empty when no episode context", m.EpisodeID)
	}
}
