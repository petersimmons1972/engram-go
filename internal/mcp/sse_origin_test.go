package mcp

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
)

// TestBuildSSEServer_EndpointEventIsRelative is the regression guard for
// #406. The SSE client (mark3labs/mcp-go transport/sse.go) rejects any
// `endpoint` event whose Host differs from the URL the client connected
// with. When the server's configured baseURL uses one loopback alias
// (e.g. 127.0.0.1) and the client uses another (e.g. localhost), an
// absolute endpoint URL fails that check despite both being loopback.
//
// buildSSEServer must emit a path-only endpoint URL so the client resolves
// it against whatever Host header it used to connect.
func TestBuildSSEServer_EndpointEventIsRelative(t *testing.T) {
	mcp := server.NewMCPServer("test", "0.0.0", server.WithToolCapabilities(false))
	// Reproduce #406's mismatch: server configured with 127.0.0.1, client
	// will connect via the httptest URL (also 127.0.0.1, but the assertion
	// is on the *shape* of the emitted endpoint, not the host strings).
	sse := buildSSEServer(mcp, "http://127.0.0.1:8788")

	ts := httptest.NewServer(sse.SSEHandler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	endpoint := readEndpointEvent(t, resp.Body)

	require.Falsef(t, strings.HasPrefix(endpoint, "http://"),
		"endpoint event must be relative (#406); got absolute %q", endpoint)
	require.Falsef(t, strings.HasPrefix(endpoint, "https://"),
		"endpoint event must be relative (#406); got absolute %q", endpoint)
	require.Truef(t, strings.HasPrefix(endpoint, "/"),
		"endpoint event must be a path-only URL; got %q", endpoint)
}

// TestBuildSSEServer_EmitsKeepalive verifies that the SSE stream sends a
// keepalive ping within the configured interval. Regression guard for #612.
func TestBuildSSEServer_EmitsKeepalive(t *testing.T) {
	// Use a short interval so the test doesn't wait the full 15s production value.
	sseKeepaliveInterval = 200 * time.Millisecond
	t.Cleanup(func() { sseKeepaliveInterval = 15 * time.Second })

	mcp := server.NewMCPServer("test", "0.0.0", server.WithToolCapabilities(false))
	sse := buildSSEServer(mcp, "http://127.0.0.1:8788")

	ts := httptest.NewServer(sse.SSEHandler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	_ = readEndpointEvent(t, resp.Body)

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if strings.Contains(sc.Text(), `"ping"`) {
			return // keepalive received within timeout
		}
	}
	t.Fatal("no keepalive ping received within 3s — WithKeepAliveInterval not enabled (#612)")
}

// readEndpointEvent reads SSE frames until it captures the `endpoint`
// event's data line, then returns it.
func readEndpointEvent(t *testing.T, body interface{ Read(p []byte) (int, error) }) string {
	t.Helper()
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var sawEndpointEvent bool
	for sc.Scan() {
		line := sc.Text()
		if line == "event: endpoint" {
			sawEndpointEvent = true
			continue
		}
		if sawEndpointEvent {
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				return strings.TrimSpace(data)
			}
		}
	}
	t.Fatalf("never saw endpoint event; scanner err: %v", sc.Err())
	return ""
}
