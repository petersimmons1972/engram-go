package mcp

// session.go — per-session context values for auto-episode (#356).
//
// Episode IDs are injected into the MCP handler context when a session connects
// with ?auto_episode=1. The context key type is unexported and distinct from
// the contextKey type used in server.go for request correlation IDs.

import "context"

// sessionContextKey is a private integer type for session-scoped context values.
// Using a distinct named type prevents key collisions with server.go's
// contextKey (string) type and any values from third-party packages.
type sessionContextKey int

const (
	// episodeIDKey is the context key for the current session's episode ID.
	episodeIDKey sessionContextKey = iota
	// autoEpisodeFlagKey is the context key for the ?auto_episode=1 flag
	// injected by the HTTP middleware so the register hook can read it.
	autoEpisodeFlagKey
)

// withEpisodeID returns a context that carries the given episode ID.
// Subsequent calls to episodeIDFromContext on the returned context (or any
// context derived from it) will return this ID.
func withEpisodeID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, episodeIDKey, id)
}

// episodeIDFromContext returns the episode ID stored in ctx and ok=true.
// Returns ("", false) when no ID is present or when the stored ID is empty.
func episodeIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(episodeIDKey).(string)
	return id, ok && id != ""
}

// withAutoEpisodeFlag marks ctx as belonging to a session that connected with
// ?auto_episode=1. The register hook reads this flag to decide whether to
// start an episode automatically.
//
// Called by the HTTP middleware layer when it detects the query parameter, so
// that the context is enriched before the SSE register hook fires.
func withAutoEpisodeFlag(ctx context.Context) context.Context {
	return context.WithValue(ctx, autoEpisodeFlagKey, true)
}

// autoEpisodeFlagFromContext reports whether ctx carries the auto-episode flag.
func autoEpisodeFlagFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(autoEpisodeFlagKey).(bool)
	return v
}
