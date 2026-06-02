// Package hookdaemon implements a long-running daemon that handles Claude Code
// hook events for Engram, replacing the per-event shell scripts.
//
// The daemon listens on a Unix domain socket. Hook shims (the `engram hook
// <EventName>` subcommand) forward the raw Claude Code hook stdin JSON to the
// daemon and relay the response back to Claude Code. Because the daemon is a
// single long-lived process, it owns all mutable state in memory — the auth
// token, the auth-OK cache, and the pending fallback buffer — eliminating the
// flock/temp-file races that the per-event scripts had to coordinate around
// (see issue #396).
package hookdaemon

import "encoding/json"

// Hook event names understood by the daemon. These mirror Claude Code's hook
// event types. Unknown events are handled as no-ops (the daemon never blocks a
// session).
const (
	HookSessionStart     = "SessionStart"
	HookUserPromptSubmit = "UserPromptSubmit"
	HookPostToolUse      = "PostToolUse"
	HookPreToolUse       = "PreToolUse"
	HookStop             = "Stop"
	HookPreCompact       = "PreCompact"
)

// Request is the JSON envelope a hook shim sends to the daemon over the socket.
//
//	{"hook":"SessionStart","payload":{...raw Claude Code hook stdin JSON...}}
type Request struct {
	Hook    string          `json:"hook"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Response is the JSON envelope the daemon returns to a hook shim. The shim
// prints Stdout verbatim to its own stdout and exits with ExitCode, which is
// how Claude Code receives hook output.
//
//	{"stdout":"...","systemMessage":"...","exit_code":0}
type Response struct {
	Stdout        string `json:"stdout,omitempty"`
	SystemMessage string `json:"systemMessage,omitempty"`
	ExitCode      int    `json:"exit_code"`
}

// hookOutput is the JSON object Claude Code itself reads from a hook's stdout.
// When a handler wants to surface a systemMessage, it marshals one of these
// into Response.Stdout so the shim can print it unchanged. Keeping this
// separate from Response lets the daemon also populate Response.SystemMessage
// for callers (e.g. the binary shim) that prefer the structured field.
type hookOutput struct {
	SystemMessage string `json:"systemMessage,omitempty"`
}

// marshalSystemMessage renders the Claude Code stdout JSON for a systemMessage.
func marshalSystemMessage(msg string) string {
	if msg == "" {
		return ""
	}
	b, err := json.Marshal(hookOutput{SystemMessage: msg})
	if err != nil {
		return ""
	}
	return string(b)
}
