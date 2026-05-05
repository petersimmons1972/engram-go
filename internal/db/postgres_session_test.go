package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/testutil"
)

// TestSessionRegistryRoundtrip verifies the four session methods work correctly
// against a real PostgreSQL database. Requires TEST_DATABASE_URL.
func TestSessionRegistryRoundtrip(t *testing.T) {
	dsn := testutil.DSN(t)
	ctx := context.Background()

	pool, err := NewSharedPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewSharedPool: %v", err)
	}
	defer pool.Close()

	b, err := NewPostgresBackendWithPool(ctx, testutil.UniqueProject("sess"), pool)
	if err != nil {
		t.Fatalf("NewPostgresBackendWithPool: %v", err)
	}

	sessionID := fmt.Sprintf("test-session-%d", time.Now().UnixNano())
	apiKeyHash := "deadbeef"

	// Register
	if err := b.RegisterSession(ctx, sessionID, apiKeyHash); err != nil {
		t.Fatalf("RegisterSession: %v", err)
	}

	// List — should appear
	sessions, err := b.ListActiveSessions(ctx, 1*time.Hour, apiKeyHash)
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	found := false
	for _, s := range sessions {
		if s == sessionID {
			found = true
		}
	}
	if !found {
		t.Errorf("registered session %q not found in ListActiveSessions", sessionID)
	}

	// Touch — should not error
	if err := b.TouchSession(ctx, sessionID); err != nil {
		t.Errorf("TouchSession: %v", err)
	}

	// Unregister
	if err := b.UnregisterSession(ctx, sessionID); err != nil {
		t.Fatalf("UnregisterSession: %v", err)
	}

	// List again — should be gone
	sessions, err = b.ListActiveSessions(ctx, 1*time.Hour, apiKeyHash)
	if err != nil {
		t.Fatalf("ListActiveSessions after unregister: %v", err)
	}
	for _, s := range sessions {
		if s == sessionID {
			t.Errorf("unregistered session %q still appears in ListActiveSessions", sessionID)
		}
	}
}

// TestRegisterSessionRejectsNilPool verifies that calling RegisterSession on a
// backend with no pool returns an error instead of panicking.
func TestRegisterSessionRejectsNilPool(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	err := b.RegisterSession(context.Background(), "valid-session-id", "hashvalue")
	if err == nil {
		t.Error("RegisterSession with nil pool must return an error, not panic")
	}
}

// TestRegisterSessionRejectsEmptyID verifies that registering an empty session
// ID returns an error without touching the database.
func TestRegisterSessionRejectsEmptyID(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	err := b.RegisterSession(context.Background(), "", "hashvalue")
	if err == nil {
		t.Error("RegisterSession with empty session_id must return an error")
	}
}

// TestUnregisterSessionRejectsNilPool verifies nil-pool guard.
func TestUnregisterSessionRejectsNilPool(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	err := b.UnregisterSession(context.Background(), "valid-session-id")
	if err == nil {
		t.Error("UnregisterSession with nil pool must return an error, not panic")
	}
}

// TestUnregisterSessionRejectsEmptyID verifies that unregistering an empty
// session ID returns an error.
func TestUnregisterSessionRejectsEmptyID(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	err := b.UnregisterSession(context.Background(), "")
	if err == nil {
		t.Error("UnregisterSession with empty session_id must return an error")
	}
}

// TestListActiveSessionsRejectsNilPool verifies nil-pool guard.
func TestListActiveSessionsRejectsNilPool(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	_, err := b.ListActiveSessions(context.Background(), 1*time.Hour, "apikeyHash")
	if err == nil {
		t.Error("ListActiveSessions with nil pool must return an error, not panic")
	}
}

// TestListActiveSessionsRejectsNegativeDuration verifies that a zero or negative
// since duration returns an error.
func TestListActiveSessionsRejectsNegativeDuration(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	_, err := b.ListActiveSessions(context.Background(), -1*time.Hour, "apiKeyHash")
	if err == nil {
		t.Error("ListActiveSessions with negative duration must return an error")
	}
}

// TestTouchSessionRejectsNilPool verifies nil-pool guard.
func TestTouchSessionRejectsNilPool(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	err := b.TouchSession(context.Background(), "valid-session-id")
	if err == nil {
		t.Error("TouchSession with nil pool must return an error, not panic")
	}
}

// TestTouchSessionRejectsEmptyID verifies that touching an empty session ID
// returns an error.
func TestTouchSessionRejectsEmptyID(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	err := b.TouchSession(context.Background(), "")
	if err == nil {
		t.Error("TouchSession with empty session_id must return an error")
	}
}

// TestListActiveSessionsFiltersByAPIKeyHash verifies that ListActiveSessions
// only returns sessions associated with the given api_key_hash (#548).
//
// Scenario: API key rotates. Old sessions bound to old key_hash should NOT be
// returned when calling ListActiveSessions with new key_hash. This prevents
// cross-key session access when the key changes.
func TestListActiveSessionsFiltersByAPIKeyHash(t *testing.T) {
	dsn := testutil.DSN(t)
	ctx := context.Background()

	pool, err := NewSharedPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewSharedPool: %v", err)
	}
	defer pool.Close()

	b, err := NewPostgresBackendWithPool(ctx, testutil.UniqueProject("sess-filter"), pool)
	if err != nil {
		t.Fatalf("NewPostgresBackendWithPool: %v", err)
	}

	oldKeyHash := fmt.Sprintf("old-key-hash-%d", time.Now().UnixNano())
	newKeyHash := fmt.Sprintf("new-key-hash-%d", time.Now().UnixNano())

	// Register two sessions with the old key hash
	oldSession1 := fmt.Sprintf("session-old-1-%d", time.Now().UnixNano())
	oldSession2 := fmt.Sprintf("session-old-2-%d", time.Now().UnixNano())
	if err := b.RegisterSession(ctx, oldSession1, oldKeyHash); err != nil {
		t.Fatalf("RegisterSession old-1: %v", err)
	}
	if err := b.RegisterSession(ctx, oldSession2, oldKeyHash); err != nil {
		t.Fatalf("RegisterSession old-2: %v", err)
	}

	// Register two sessions with the new key hash
	newSession1 := fmt.Sprintf("session-new-1-%d", time.Now().UnixNano())
	newSession2 := fmt.Sprintf("session-new-2-%d", time.Now().UnixNano())
	if err := b.RegisterSession(ctx, newSession1, newKeyHash); err != nil {
		t.Fatalf("RegisterSession new-1: %v", err)
	}
	if err := b.RegisterSession(ctx, newSession2, newKeyHash); err != nil {
		t.Fatalf("RegisterSession new-2: %v", err)
	}

	// List sessions for the old key hash — should return only old sessions
	oldSessions, err := b.ListActiveSessions(ctx, 1*time.Hour, oldKeyHash)
	if err != nil {
		t.Fatalf("ListActiveSessions(oldKeyHash): %v", err)
	}

	if len(oldSessions) < 2 {
		t.Errorf("expected at least 2 old sessions, got %d: %v", len(oldSessions), oldSessions)
	}
	oldSessionsMap := make(map[string]bool)
	for _, id := range oldSessions {
		oldSessionsMap[id] = true
	}
	if !oldSessionsMap[oldSession1] || !oldSessionsMap[oldSession2] {
		t.Errorf("old sessions not found. Got %v, expected to contain %q and %q",
			oldSessions, oldSession1, oldSession2)
	}

	// List sessions for the new key hash — should return only new sessions
	newSessions, err := b.ListActiveSessions(ctx, 1*time.Hour, newKeyHash)
	if err != nil {
		t.Fatalf("ListActiveSessions(newKeyHash): %v", err)
	}

	if len(newSessions) < 2 {
		t.Errorf("expected at least 2 new sessions, got %d: %v", len(newSessions), newSessions)
	}
	newSessionsMap := make(map[string]bool)
	for _, id := range newSessions {
		newSessionsMap[id] = true
	}
	if !newSessionsMap[newSession1] || !newSessionsMap[newSession2] {
		t.Errorf("new sessions not found. Got %v, expected to contain %q and %q",
			newSessions, newSession1, newSession2)
	}

	// Verify no cross-contamination
	for _, id := range newSessions {
		if oldSessionsMap[id] {
			t.Errorf("new session %q appears in old sessions list (cross-contamination)", id)
		}
	}
}
