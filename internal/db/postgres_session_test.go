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
	sessions, err := b.ListActiveSessions(ctx, 1*time.Hour)
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
	sessions, err = b.ListActiveSessions(ctx, 1*time.Hour)
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
	_, err := b.ListActiveSessions(context.Background(), 1*time.Hour)
	if err == nil {
		t.Error("ListActiveSessions with nil pool must return an error, not panic")
	}
}

// TestListActiveSessionsRejectsNegativeDuration verifies that a zero or negative
// since duration returns an error.
func TestListActiveSessionsRejectsNegativeDuration(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	_, err := b.ListActiveSessions(context.Background(), -1*time.Hour)
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
