package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

var errStub = errors.New("stub error")

// TestHashAPIKey verifies the HMAC-SHA-256 hex output is deterministic and non-empty.
func TestHashAPIKey(t *testing.T) {
	key := "test-api-key"
	got := hashAPIKey(key)
	if got == "" {
		t.Fatal("hashAPIKey returned empty string")
	}
	// HMAC-SHA-256 produces 32 bytes → 64 hex chars.
	if len(got) != 64 {
		t.Errorf("hashAPIKey len = %d, want 64", len(got))
	}
	// Verify determinism.
	if hashAPIKey(key) != got {
		t.Error("hashAPIKey is not deterministic")
	}
	// Verify correctness against stdlib HMAC with the same pepper.
	mac := hmac.New(sha256.New, []byte(sessionFingerprintPepper))
	mac.Write([]byte(key))
	want := hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Errorf("hashAPIKey = %q, want %q", got, want)
	}
}

// TestHashAPIKeyDifferentKeys verifies that different keys produce different hashes.
func TestHashAPIKeyDifferentKeys(t *testing.T) {
	if hashAPIKey("key-a") == hashAPIKey("key-b") {
		t.Error("different keys produced the same hash")
	}
}

// TestRehydratedSessionImplementsClientSession verifies the rehydratedSession
// struct correctly satisfies all ClientSession methods.
func TestRehydratedSessionImplementsClientSession(t *testing.T) {
	sess := newRehydratedSession("test-session-id")

	if sess.SessionID() != "test-session-id" {
		t.Errorf("SessionID = %q, want %q", sess.SessionID(), "test-session-id")
	}
	// Initialized() must return true (session is pre-warmed).
	if !sess.Initialized() {
		t.Error("Initialized() must return true for rehydrated sessions")
	}
	// Initialize() must be callable without panic.
	sess.Initialize()
	// NotificationChannel must return a non-nil writeable channel.
	ch := sess.NotificationChannel()
	if ch == nil {
		t.Error("NotificationChannel() returned nil")
	}
}

// TestRehydrateSessionsNilDBIsNoop verifies that RehydrateSessions returns nil
// when SessionDB is not configured — no panic, no error.
func TestRehydrateSessionsNilDBIsNoop(t *testing.T) {
	pool := newTestNoopPool(t)
	cfg := Config{} // SessionDB is nil
	srv := NewServer(pool, cfg)

	err := srv.RehydrateSessions(context.Background(), "api-key")
	if err != nil {
		t.Errorf("RehydrateSessions with nil SessionDB: got error %v, want nil", err)
	}
}

// mockSessionRegistry is a test double that returns a fixed list of session IDs.
type mockSessionRegistry struct {
	sessions []string
	err      error
}

func (m *mockSessionRegistry) RegisterSession(_ context.Context, _, _ string) error { return nil }
func (m *mockSessionRegistry) UnregisterSession(_ context.Context, _ string) error  { return nil }
func (m *mockSessionRegistry) ListActiveSessions(_ context.Context, _ time.Duration, _ string) ([]string, error) {
	return m.sessions, m.err
}
func (m *mockSessionRegistry) TouchSession(_ context.Context, _ string) error { return nil }

// TestRehydrateSessionsRegistersInTransport verifies that rehydrated sessions
// are registered in the mcp-go transport so POST /message recognises them.
func TestRehydrateSessionsRegistersInTransport(t *testing.T) {
	pool := newTestNoopPool(t)
	sessionDB := &mockSessionRegistry{sessions: []string{"session-abc", "session-def"}}
	cfg := Config{SessionDB: sessionDB}
	srv := NewServer(pool, cfg)

	err := srv.RehydrateSessions(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("RehydrateSessions: %v", err)
	}

	// After rehydration, each session ID must have a fingerprint in the sync.Map.
	for _, id := range sessionDB.sessions {
		if _, ok := srv.sessionFingerprints.Load(id); !ok {
			t.Errorf("session %q: fingerprint not restored after rehydration", id)
		}
	}
}

// TestRehydrateSessionsDBError verifies that a DB error is propagated to the caller.
func TestRehydrateSessionsDBError(t *testing.T) {
	pool := newTestNoopPool(t)
	sessionDB := &mockSessionRegistry{err: errStub}
	cfg := Config{SessionDB: sessionDB}
	srv := NewServer(pool, cfg)

	err := srv.RehydrateSessions(context.Background(), "test-key")
	if err == nil {
		t.Error("expected error from DB, got nil")
	}
}
