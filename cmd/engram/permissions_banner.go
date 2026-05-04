package main

import (
	"encoding/json"
	"log/slog"
	"sort"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
)

// logRecommendedClientPermissions emits a one-time INFO log naming the read-only
// MCP tools that benefit from being on the Claude Code permissions allowlist.
// Operators can copy the JSON snippet directly out of the log into
// ~/.claude/settings.json.
//
// Why this exists: even with ReadOnlyHint=true on the tool annotation, an
// over-cautious deny rule or non-default permission policy can still block
// these calls. Showing the canonical allowlist at startup is the cheapest way
// to keep installers from hitting silent rejection loops.
func logRecommendedClientPermissions(srv *internalmcp.Server) {
	annotations := srv.RegisteredToolAnnotations()
	names := make([]string, 0, len(annotations))
	for name, ann := range annotations {
		if ann.ReadOnlyHint != nil && *ann.ReadOnlyHint {
			names = append(names, "mcp__engram__"+name)
		}
	}
	sort.Strings(names)

	snippet := map[string]any{
		"permissions": map[string]any{
			"allow": names,
		},
	}
	body, err := json.MarshalIndent(snippet, "", "  ")
	if err != nil {
		slog.Warn("could not render recommended permissions snippet", "err", err)
		return
	}

	slog.Info("recommended Claude Code permissions for engram read-only tools — paste into ~/.claude/settings.json under permissions.allow",
		"count", len(names),
		"snippet", string(body),
	)
}
