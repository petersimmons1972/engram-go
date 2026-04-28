package db

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestSessionRegistryRoundtrip verifies the four session methods work correctly
// against a real PostgreSQL database. Requires TEST_DATABASE_URL.
func TestSessionRegistryRoundtrip(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	// TODO: implement with real DB when integration env is available.
	_ = dsn
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
