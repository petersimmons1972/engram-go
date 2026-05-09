package mcp

import (
	"github.com/mark3labs/mcp-go/server"
)

// buildStreamableHTTPServer wires the mcp-go Streamable HTTP server.
//
// Why Streamable HTTP over SSE: SSE requires a persistent long-lived TCP
// connection that is silently dropped by NAT/Docker on idle. Every MCP tool
// call that arrives while the connection is dead fails, and the client must
// run /mcp to reconnect. Streamable HTTP uses stateless POST requests —
// each tool call is an independent HTTP round-trip with no persistent
// connection to maintain, so there is nothing to drop or timeout (#612).
//
// The server mounts at /mcp (mcp-go default). Claude Code clients should
// use type:"http" and url:"http://127.0.0.1:8788/mcp" in mcp_servers.json.
//
// Auth: the returned handler is a plain http.Handler; wrap it with
// applyMiddleware before registering so it gets the same Bearer + rate-limit
// enforcement as all other authenticated routes.
func buildStreamableHTTPServer(mcp *server.MCPServer) *server.StreamableHTTPServer {
	return server.NewStreamableHTTPServer(mcp)
}
