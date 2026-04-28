package db

import (
	"context"
	"fmt"
	"time"
)

// SessionRegistry is a narrow interface for MCP session persistence.
// PostgresBackend satisfies it; test stubs can implement only these four methods
// rather than the full Backend interface.
type SessionRegistry interface {
	RegisterSession(ctx context.Context, sessionID, apiKeyHash string) error
	UnregisterSession(ctx context.Context, sessionID string) error
	ListActiveSessions(ctx context.Context, since time.Duration) ([]string, error)
	TouchSession(ctx context.Context, sessionID string) error
}

// compile-time check that PostgresBackend satisfies SessionRegistry.
var _ SessionRegistry = (*PostgresBackend)(nil)

// RegisterSession persists a new MCP SSE session to the database so it can be
// replayed after a server restart. sessionID is the transport-layer session ID
// issued by mcp-go; apiKeyHash is SHA-256 hex of the API key (never plaintext).
func (b *PostgresBackend) RegisterSession(ctx context.Context, sessionID, apiKeyHash string) error {
	if b.pool == nil {
		return fmt.Errorf("backend has no pool")
	}
	if sessionID == "" {
		return fmt.Errorf("session_id must not be empty")
	}
	_, err := b.pool.Exec(ctx, `
		INSERT INTO mcp_sessions (session_id, api_key_hash)
		VALUES ($1, $2)
		ON CONFLICT (session_id) DO UPDATE
		    SET api_key_hash = EXCLUDED.api_key_hash,
		        last_seen_at = now()`,
		sessionID, apiKeyHash,
	)
	return err
}

// UnregisterSession removes a session from the registry when the client
// disconnects. Missing sessions are silently ignored (idempotent).
func (b *PostgresBackend) UnregisterSession(ctx context.Context, sessionID string) error {
	if b.pool == nil {
		return fmt.Errorf("backend has no pool")
	}
	if sessionID == "" {
		return fmt.Errorf("session_id must not be empty")
	}
	_, err := b.pool.Exec(ctx,
		"DELETE FROM mcp_sessions WHERE session_id = $1",
		sessionID,
	)
	return err
}

// ListActiveSessions returns session IDs whose last_seen_at is within the given
// duration from now. Used at startup to rehydrate sessions from before a restart.
// since must be positive.
func (b *PostgresBackend) ListActiveSessions(ctx context.Context, since time.Duration) ([]string, error) {
	if b.pool == nil {
		return nil, fmt.Errorf("backend has no pool")
	}
	if since <= 0 {
		return nil, fmt.Errorf("since must be a positive duration")
	}
	// Pass seconds as an integer and construct the interval server-side via
	// make_interval so the query does not depend on Go's Duration.String() format
	// (e.g. "2h0m0s" or "1µs"), which is not a documented PostgreSQL interval
	// literal syntax.
	rows, err := b.pool.Query(ctx, `
		SELECT session_id FROM mcp_sessions
		WHERE last_seen_at > now() - make_interval(secs => $1)
		ORDER BY last_seen_at DESC`,
		int64(since.Seconds()),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// TouchSession updates last_seen_at for an active session. Called on every
// POST /message so sessions that are in active use are not reaped by the
// stale-session cleanup even if the session remains open longer than the
// rehydration window.
func (b *PostgresBackend) TouchSession(ctx context.Context, sessionID string) error {
	if b.pool == nil {
		return fmt.Errorf("backend has no pool")
	}
	if sessionID == "" {
		return fmt.Errorf("session_id must not be empty")
	}
	_, err := b.pool.Exec(ctx,
		"UPDATE mcp_sessions SET last_seen_at = now() WHERE session_id = $1",
		sessionID,
	)
	return err
}
