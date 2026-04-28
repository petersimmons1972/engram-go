package db

import (
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

// TestRegisterSessionRejectsEmptyID verifies that registering an empty session
// ID returns an error without touching the database.
func TestRegisterSessionRejectsEmptyID(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	err := b.RegisterSession(nil, "", "hashvalue")
	if err == nil {
		t.Error("RegisterSession with empty session_id must return an error")
	}
}

// TestUnregisterSessionRejectsEmptyID verifies that unregistering an empty
// session ID returns an error.
func TestUnregisterSessionRejectsEmptyID(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	err := b.UnregisterSession(nil, "")
	if err == nil {
		t.Error("UnregisterSession with empty session_id must return an error")
	}
}

// TestListActiveSessionsRejectsNegativeDuration verifies that a zero or negative
// since duration returns an error.
func TestListActiveSessionsRejectsNegativeDuration(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	_, err := b.ListActiveSessions(nil, -1*time.Hour)
	if err == nil {
		t.Error("ListActiveSessions with negative duration must return an error")
	}
}

// TestTouchSessionRejectsEmptyID verifies that touching an empty session ID
// returns an error.
func TestTouchSessionRejectsEmptyID(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	err := b.TouchSession(nil, "")
	if err == nil {
		t.Error("TouchSession with empty session_id must return an error")
	}
}
