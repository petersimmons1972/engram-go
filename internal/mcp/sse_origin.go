package mcp

import (
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// sseKeepaliveInterval is the MCP ping interval for SSE connections.
// Overridden in tests to avoid waiting the full production interval.
// 15s keeps the TCP session alive through typical NAT/Docker idle timeouts (#612).
var sseKeepaliveInterval = 15 * time.Second

// buildSSEServer wires the mcp-go SSE server with engram's transport policy.
//
// Why WithUseFullURLForMessageEndpoint(false): the mcp-go default is to emit
// the `endpoint` SSE event as a fully-qualified URL built from baseURL. The
// SSE client then enforces endpoint.Host == c.baseURL.Host. When the server
// is configured for one loopback alias (e.g. 127.0.0.1) and the client
// connects via another (e.g. localhost), the hosts differ verbatim even
// though both resolve to the loopback interface, so the client silently
// drops every endpoint event with "Endpoint origin does not match
// connection origin" (#406). Emitting a path-only endpoint sidesteps the
// check entirely — the client resolves it against its own connection URL,
// so loopback aliases are interchangeable.
//
// Why WithKeepAliveInterval: without keepalives the SSE TCP session is
// silently dropped by NAT/Docker after ~60s of idle, causing the next MCP
// tool call to hang indefinitely. mcp-go sends a proper MCP ping message on
// the ticker so the client processes it via its normal message loop (#612).
func buildSSEServer(mcp *server.MCPServer, baseURL string) *server.SSEServer {
	return server.NewSSEServer(
		mcp,
		server.WithBaseURL(baseURL),
		server.WithUseFullURLForMessageEndpoint(false),
		server.WithKeepAliveInterval(sseKeepaliveInterval),
	)
}
