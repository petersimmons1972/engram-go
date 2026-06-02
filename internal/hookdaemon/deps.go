package hookdaemon

import "context"

// EngramClient is the daemon's view of the Engram REST API. It is injected so
// the package can be unit-tested without a running Engram server. The token is
// passed per-call rather than held by the client because the daemon owns the
// token cache and may refresh it between calls.
type EngramClient interface {
	// Health probes GET /health. Returns nil when the server is reachable and
	// healthy. A non-nil error means unreachable/unhealthy — the caller decides
	// whether to attempt a restart.
	Health(ctx context.Context) error

	// CheckAuth probes POST /quick-recall with the given bearer token. It
	// reports whether the token is accepted (any status other than 401/000 is
	// treated as accepted, matching the shell scripts: a 500 means the recall
	// backend errored but the token was valid).
	CheckAuth(ctx context.Context, token string) (ok bool, err error)

	// Recall calls POST /quick-recall and returns the raw JSON body. Used by the
	// SessionStart recall injection.
	Recall(ctx context.Context, token, query, project string, limit int) ([]byte, error)

	// QuickStore calls POST /quick-store with the given JSON body. Used by the
	// Stop handler to record a session-end marker.
	QuickStore(ctx context.Context, token string, body []byte) error
}

// TokenStore reads and writes the Engram bearer token. The daemon caches the
// token in memory and only writes through this interface when the token
// actually changes (issue #396). The concrete implementation reads/writes
// ~/.claude/mcp_servers.json.
type TokenStore interface {
	// Load returns the current token, or "" if none is configured.
	Load() (token string, err error)
	// Store persists a new token. Implementations must write atomically.
	Store(token string) error
}

// MemoryWriter writes the session-recall section into MEMORY.md. It is injected
// so tests can capture the written content without touching the real file.
type MemoryWriter interface {
	// WriteRecallSection replaces the "## Engram Session Recall" section of the
	// memory file with the provided markdown section (which already includes the
	// heading). Implementations must write atomically.
	WriteRecallSection(section string) error
}

// FallbackStore persists fallback entries to disk. The daemon holds pending
// entries in memory and flushes through this interface from a single goroutine,
// so no flock is needed (issue #396).
type FallbackStore interface {
	// Append appends the given entries to the on-disk fallback file atomically.
	Append(entries []string) error
}

// Clock abstracts time for deterministic idle-timeout tests.
type Clock interface {
	Now() int64 // unix seconds
}
