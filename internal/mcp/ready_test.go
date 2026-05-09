//go:build ignore
// Remove the ignore tag when Pillar 2A (/ready endpoint + serverPhase) is implemented.

package mcp

// Tests for the /ready endpoint — Pillar 2A (phase tracking + readiness signal).
//
// The Server struct does NOT yet have:
//   - serverPhase atomic.Int32
//   - handleReady method
//   - phaseWarm / phaseStarting constants
//
// These tests will FAIL TO COMPILE until the implementation is added.
// That is the expected red-phase state.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReady_WarmPhase_Returns200_EmbedOK verifies that when the server is in
// the warm phase and embedding is healthy, /ready returns 200 with ready:true.
func TestReady_WarmPhase_Returns200_EmbedOK(t *testing.T) {
	s := &Server{}
	s.serverPhase.Store(int32(phaseWarm)) // phaseWarm constant doesn't exist yet
	s.embedDegraded = new(atomic.Bool)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	s.handleReady(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, true, body["ready"])
	require.Equal(t, "warm", body["phase"])
	require.Equal(t, "ok", body["embed"])
	require.Equal(t, "http", body["transport_hint"])
}

// TestReady_StartingPhase_Returns503 verifies that when the server is still
// starting (pool cold, not yet ready), /ready returns 503 with ready:false.
func TestReady_StartingPhase_Returns503(t *testing.T) {
	s := &Server{}
	s.serverPhase.Store(int32(phaseStarting)) // phaseStarting constant doesn't exist yet
	s.embedDegraded = new(atomic.Bool)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	s.handleReady(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, false, body["ready"])
}

// TestReady_WarmPhase_EmbedDegraded_Returns200 verifies that embed degradation
// does not make the server report unready — BM25 is still operational.
func TestReady_WarmPhase_EmbedDegraded_Returns200(t *testing.T) {
	s := &Server{}
	s.serverPhase.Store(int32(phaseWarm))
	s.embedDegraded = new(atomic.Bool)
	s.embedDegraded.Store(true)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	s.handleReady(w, req)

	// Still 200 — BM25 is operational even without semantic embedding.
	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, true, body["ready"])
	require.Equal(t, "degraded", body["embed"],
		"embed field must be 'degraded' when embedDegraded flag is set")
}

// TestReady_AlwaysIncludesTransportHint_HTTP verifies that every /ready response
// includes transport_hint: "http" so MCP clients know to use HTTP not SSE.
func TestReady_AlwaysIncludesTransportHint_HTTP(t *testing.T) {
	s := &Server{}
	s.serverPhase.Store(int32(phaseWarm))
	s.embedDegraded = new(atomic.Bool)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	s.handleReady(w, req)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "http", body["transport_hint"])
}

// TestReady_WarmingPhase_Returns503 verifies that when the server is warming
// (pool init in progress, not yet fully ready), /ready returns 503.
func TestReady_WarmingPhase_Returns503(t *testing.T) {
	s := &Server{}
	s.serverPhase.Store(int32(phaseWarming)) // phaseWarming constant doesn't exist yet
	s.embedDegraded = new(atomic.Bool)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	s.handleReady(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, false, body["ready"])
	require.Equal(t, "warming", body["phase"])
}

// TestReady_Returns503_TransportHintStillPresent verifies that even a 503
// response includes transport_hint so clients can log the correct URL.
func TestReady_Returns503_TransportHintStillPresent(t *testing.T) {
	s := &Server{}
	s.serverPhase.Store(int32(phaseStarting))
	s.embedDegraded = new(atomic.Bool)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	s.handleReady(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "http", body["transport_hint"],
		"transport_hint must be present even in 503 response")
}
