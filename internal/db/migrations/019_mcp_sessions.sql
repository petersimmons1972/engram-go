-- 019_mcp_sessions.sql
-- Persists MCP SSE session registrations so they survive server restarts (#362).
-- Sessions are keyed by session_id (issued by the mcp-go transport layer).
-- api_key_hash stores SHA-256 of the API key — never the plaintext key.
-- last_seen_at is updated on every POST /message to detect stale sessions.

CREATE TABLE IF NOT EXISTS mcp_sessions (
    session_id   TEXT        NOT NULL PRIMARY KEY,
    api_key_hash TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mcp_sessions_last_seen
    ON mcp_sessions (last_seen_at DESC);
