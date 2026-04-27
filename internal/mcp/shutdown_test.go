package mcp

// shutdown_test.go — verifies that active episodes are closed when the server
// context is cancelled (simulating SIGTERM / graceful shutdown) (#356).
//
// The concern: cmd/engram/main.go uses signal.NotifyContext() so that SIGTERM
// cancels the main context.  The graceful shutdown window (10 s) closes SSE
// connections, which fires AddOnUnregisterSession for every active session.
// The unregister hook must still reach EndEpisode even though the session
// context is cancelled by the time it fires.
//
// Key invariant under test:
//   The hook uses context.Background() + 5 s timeout, NOT the (already-
//   cancelled) session or server context.  EndEpisode must therefore succeed
//   even when both outer contexts are cancelled.

import (
	"context"
	"fmt"
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
)

// TestGracefulShutdown_ClosesOpenEpisodes verifies that cancelling the server
// context (i.e. SIGTERM) does not prevent the unregister hook from calling
// EndEpisode on any session that had an auto-episode running.
//
// Test sequence:
//  1. Build a server backed by episodeTrackingBackend.
//  2. Simulate a session connecting with ?auto_episode=1 → episode starts.
//  3. Cancel the server context (SIGTERM simulation).
//  4. Simulate SSE disconnect → unregister hook fires with a cancelled context.
//  5. Assert EndEpisode was called with the correct episode ID.
func TestGracefulShutdown_ClosesOpenEpisodes(t *testing.T) {
	backend := &episodeTrackingBackend{}
	s := newAutoEpisodeServer(t, backend)
	s.registerSessionHooks("test-api-key")

	// Step 2: session connects with ?auto_episode=1.
	connectCtx := withAutoEpisodeFlag(context.Background())
	sess := &fakeClientSession{id: "sess-shutdown-001"}

	s.mcp.GetHooks().RegisterSession(connectCtx, sess)

	if len(backend.started) == 0 {
		t.Fatal("prerequisite: StartEpisode must fire on connect with auto_episode=1")
	}

	// Step 3: cancel the server context — this is what SIGTERM does via
	// signal.NotifyContext.  After this the main context is done; SSE
	// connections are being torn down inside the 10 s shutdown window.
	serverCtx, cancelServer := context.WithCancel(context.Background())
	cancelServer() // simulate SIGTERM — immediately cancelled

	// Step 4: SSE disconnect fires the unregister hook.  The context the
	// library passes to the hook is the (already-cancelled) session context.
	// In production this is the same context that was cancelled when the HTTP
	// request ended; here we use the cancelled serverCtx as a stand-in because
	// both represent the same invariant: the context is done.
	s.mcp.GetHooks().UnregisterSession(serverCtx, sess)

	// Step 5: EndEpisode must have been called despite both the server context
	// and the hook-argument context being cancelled.  The hook must use
	// context.Background()+5 s, not the passed context.
	if len(backend.ended) == 0 {
		t.Fatal("expected EndEpisode to be called during graceful shutdown; got 0 calls — " +
			"the unregister hook may be using the cancelled session context instead of context.Background()")
	}
	if backend.ended[0] != "ep-auto-test-001" {
		t.Fatalf("EndEpisode called with wrong ID %q; want %q", backend.ended[0], "ep-auto-test-001")
	}
}

// TestGracefulShutdown_MultipleSessions_AllEpisodesClose verifies that when
// multiple sessions are active at shutdown time, every open episode is closed.
//
// This guards against a race where only the first session's episode is closed
// because the hook short-circuits after the first EndEpisode error.
func TestGracefulShutdown_MultipleSessions_AllEpisodesClose(t *testing.T) {
	backend := &multiEpisodeTrackingBackend{}
	s := newMultiEpisodeServer(t, backend)
	s.registerSessionHooks("test-api-key")

	// Connect three sessions with auto_episode=1.
	sessions := []*fakeClientSession{
		{id: "sess-shutdown-multi-001"},
		{id: "sess-shutdown-multi-002"},
		{id: "sess-shutdown-multi-003"},
	}
	connectCtx := withAutoEpisodeFlag(context.Background())
	for _, sess := range sessions {
		s.mcp.GetHooks().RegisterSession(connectCtx, sess)
	}

	if len(backend.started) != 3 {
		t.Fatalf("expected 3 episodes started; got %d", len(backend.started))
	}

	// Cancel server context — SIGTERM.
	serverCtx, cancelServer := context.WithCancel(context.Background())
	cancelServer()

	// Disconnect all sessions under the cancelled context.
	for _, sess := range sessions {
		s.mcp.GetHooks().UnregisterSession(serverCtx, sess)
	}

	if len(backend.ended) != 3 {
		t.Fatalf("expected 3 EndEpisode calls during shutdown; got %d — "+
			"not all episodes were closed before the server would have exited", len(backend.ended))
	}
}

// TestGracefulShutdown_NoAutoEpisode_NoEndCalled verifies that sessions which
// connected WITHOUT ?auto_episode=1 do not trigger EndEpisode during shutdown.
// This ensures the shutdown path does not attempt to close episodes that were
// never started.
func TestGracefulShutdown_NoAutoEpisode_NoEndCalled(t *testing.T) {
	backend := &episodeTrackingBackend{}
	s := newAutoEpisodeServer(t, backend)
	s.registerSessionHooks("test-api-key")

	// Connect without the auto_episode flag.
	sess := &fakeClientSession{id: "sess-shutdown-no-ep-001"}
	s.mcp.GetHooks().RegisterSession(context.Background(), sess)

	// Cancel server context and fire unregister.
	serverCtx, cancelServer := context.WithCancel(context.Background())
	cancelServer()
	s.mcp.GetHooks().UnregisterSession(serverCtx, sess)

	if len(backend.ended) > 0 {
		t.Fatalf("EndEpisode should not be called when no episode was started; got %d calls", len(backend.ended))
	}
}

// ---------------------------------------------------------------------------
// Multi-episode backend: each StartEpisode returns a unique ID so the test
// can verify that all distinct episode IDs were closed.
// ---------------------------------------------------------------------------

type multiEpisodeTrackingBackend struct {
	noopBackend
	started []string
	ended   []string
	counter int
}

func (b *multiEpisodeTrackingBackend) StartEpisode(_ context.Context, _, description string) (*types.Episode, error) {
	b.counter++
	id := fmt.Sprintf("ep-multi-%03d", b.counter)
	b.started = append(b.started, id)
	return &types.Episode{ID: id}, nil
}

func (b *multiEpisodeTrackingBackend) EndEpisode(_ context.Context, id, _ string) error {
	b.ended = append(b.ended, id)
	return nil
}

// newMultiEpisodeServer is like newAutoEpisodeServer but backed by
// multiEpisodeTrackingBackend.
func newMultiEpisodeServer(t *testing.T, backend *multiEpisodeTrackingBackend) *Server {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	pool := NewEnginePool(factory)
	return NewServer(pool, Config{})
}
