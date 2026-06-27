package longmemeval_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// contains reports whether sub is a substring of s.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestIsStaleSessionError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("invalid params: Invalid session ID"), true},
		{fmt.Errorf("Invalid session ID"), true},
		{fmt.Errorf("invalid session id"), true},
		{fmt.Errorf("some other error"), false},
		{fmt.Errorf("connection refused"), false},
		{nil, false},
	}
	for _, c := range cases {
		got := longmemeval.IsStaleSessionError(c.err)
		if got != c.want {
			t.Errorf("IsStaleSessionError(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

// TestRestClient_QuickStore_HappyPath verifies the REST path succeeds.
func TestRestClient_QuickStore_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quick-store" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "mem-abc"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	ctx := context.Background()
	id, err := rc.QuickStore(ctx, "proj-1", "content here", []string{"tag1"}, nil)
	if err != nil {
		t.Fatalf("QuickStore: %v", err)
	}
	if id != "mem-abc" {
		t.Errorf("id = %q, want mem-abc", id)
	}
}

// TestRestClient_QuickStore_RateLimitRetry verifies that 429 triggers retry
// and that a subsequent success is returned.
func TestRestClient_QuickStore_RateLimitRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "rate limited"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "mem-retry"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	// Use a short-timeout context so we don't wait real exponential backoff.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := rc.QuickStore(ctx, "proj-x", "hello", nil, nil)
	if err != nil {
		t.Fatalf("QuickStore after retry: %v", err)
	}
	if id != "mem-retry" {
		t.Errorf("id = %q, want mem-retry", id)
	}
	if calls < 3 {
		t.Errorf("expected at least 3 calls (2 rate-limited + 1 success), got %d", calls)
	}
}

// TestRestClient_QuickStore_ServerError verifies 5xx triggers retry.
func TestRestClient_QuickStore_ServerError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "internal error"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "mem-ok"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := rc.QuickStore(ctx, "p", "c", nil, nil)
	if err != nil {
		t.Fatalf("QuickStore after 500+retry: %v", err)
	}
	if id != "mem-ok" {
		t.Errorf("id = %q, want mem-ok", id)
	}
}

// TestRestClient_QuickStore_NonObjectResponseError verifies that decode
// failures surface the called URL, HTTP status, body prefix, and the root-vs-
// /mcp hint instead of the raw Go struct unmarshal error (#1192).
func TestRestClient_QuickStore_NonObjectResponseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp/quick-store" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("42"))
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL+"/mcp", "")
	_, err := rc.QuickStore(context.Background(), "proj-1", "content here", []string{"tag1"}, nil)
	if err == nil {
		t.Fatal("QuickStore error = nil, want non-object response error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unexpected non-object response from POST") {
		t.Fatalf("error = %q, want non-object response framing", msg)
	}
	if !strings.Contains(msg, "/mcp/quick-store") {
		t.Fatalf("error = %q, want called URL", msg)
	}
	if !strings.Contains(msg, "status 200") {
		t.Fatalf("error = %q, want status code", msg)
	}
	if !strings.Contains(msg, "\"42\"") {
		t.Fatalf("error = %q, want body prefix", msg)
	}
	if !strings.Contains(msg, "server root rather than /mcp") {
		t.Fatalf("error = %q, want base URL hint", msg)
	}
}

// TestRestClient_QuickRecall_HappyPath verifies QuickRecall returns IDs.
func TestRestClient_QuickRecall_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quick-recall" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"ids": []string{"id-1", "id-2"}})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	ids, err := rc.QuickRecall(context.Background(), "proj", "query", 5)
	if err != nil {
		t.Fatalf("QuickRecall: %v", err)
	}
	if len(ids) != 2 || ids[0] != "id-1" {
		t.Errorf("ids = %v, want [id-1, id-2]", ids)
	}
}

// TestRestClient_QuickRecall_NonObjectResponseError verifies QuickRecall uses
// the same wrapped decode error shape as QuickStore when the server returns a
// scalar or other non-object body (#1192).
func TestRestClient_QuickRecall_NonObjectResponseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp/quick-recall" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("7"))
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL+"/mcp", "")
	_, err := rc.QuickRecall(context.Background(), "proj", "query", 5)
	if err == nil {
		t.Fatal("QuickRecall error = nil, want non-object response error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unexpected non-object response from POST") {
		t.Fatalf("error = %q, want non-object response framing", msg)
	}
	if !strings.Contains(msg, "/mcp/quick-recall") {
		t.Fatalf("error = %q, want called URL", msg)
	}
	if !strings.Contains(msg, "status 404") {
		t.Fatalf("error = %q, want status code", msg)
	}
	if !strings.Contains(msg, "\"7\"") {
		t.Fatalf("error = %q, want body prefix", msg)
	}
	if !strings.Contains(msg, "server root rather than /mcp") {
		t.Fatalf("error = %q, want base URL hint", msg)
	}
}

// TestRestClient_QuickRecall_ObjectErrorResponse verifies that a structured
// JSON error response does not decode as a false success with empty IDs (#1192).
func TestRestClient_QuickRecall_ObjectErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp/quick-recall" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "not found",
			"hint":  "REST endpoints (/quick-store,/atoms,/quick-recall) are at the server root, not under /mcp",
		})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL+"/mcp", "")
	_, err := rc.QuickRecall(context.Background(), "proj", "query", 5)
	if err == nil {
		t.Fatal("QuickRecall error = nil, want structured 404 error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "quick-recall failed: not found (status 404)") {
		t.Fatalf("error = %q, want status error framing", msg)
	}
	if !strings.Contains(msg, "REST endpoints (/quick-store,/atoms,/quick-recall) are at the server root, not under /mcp") {
		t.Fatalf("error = %q, want server hint", msg)
	}
}

// TestRestClient_QuickStore_ExpiresAt_Set verifies that a non-nil expiresAt is
// serialized as "expires_at" (RFC3339) in the POST body.
func TestRestClient_QuickStore_ExpiresAt_Set(t *testing.T) {
	future := time.Now().UTC().Add(72 * time.Hour)
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "mem-ttl"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	id, err := rc.QuickStore(context.Background(), "proj-ttl", "content", nil, &future)
	if err != nil {
		t.Fatalf("QuickStore: %v", err)
	}
	if id != "mem-ttl" {
		t.Errorf("id = %q, want mem-ttl", id)
	}
	raw, ok := gotBody["expires_at"]
	if !ok {
		t.Fatal("expires_at field missing from request body")
	}
	parsed, err := time.Parse(time.RFC3339, raw.(string))
	if err != nil {
		t.Fatalf("expires_at is not RFC3339: %v", err)
	}
	delta := parsed.Sub(future)
	if delta < 0 {
		delta = -delta
	}
	if delta > 2*time.Second {
		t.Errorf("expires_at delta = %v, want < 2s", delta)
	}
}

// TestRestClient_QuickStore_ExpiresAt_Nil verifies that a nil expiresAt does NOT
// include an "expires_at" key in the POST body (not null — fully absent).
func TestRestClient_QuickStore_ExpiresAt_Nil(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "mem-no-ttl"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	id, err := rc.QuickStore(context.Background(), "proj-no-ttl", "content", nil, nil)
	if err != nil {
		t.Fatalf("QuickStore: %v", err)
	}
	if id != "mem-no-ttl" {
		t.Errorf("id = %q, want mem-no-ttl", id)
	}
	if _, ok := gotBody["expires_at"]; ok {
		t.Error("expires_at must be absent from request body when expiresAt is nil")
	}
}

// TestSessionContent_Basic covers basic role labelling.
func TestSessionContent_Basic(t *testing.T) {
	turns := []longmemeval.Turn{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	got := longmemeval.SessionContent(turns)
	if !strings.Contains(got, "user: hello") {
		t.Errorf("missing user turn: %q", got)
	}
	if !strings.Contains(got, "assistant: world") {
		t.Errorf("missing assistant turn: %q", got)
	}
}

// TestSessionContent_EmptyContentSkipped verifies empty turns are omitted.
func TestSessionContent_EmptyContentSkipped(t *testing.T) {
	turns := []longmemeval.Turn{
		{Role: "user", Content: ""},
		{Role: "assistant", Content: "non-empty"},
	}
	got := longmemeval.SessionContent(turns)
	if strings.Contains(got, "user:") {
		t.Errorf("empty user turn should be skipped: %q", got)
	}
	if !strings.Contains(got, "non-empty") {
		t.Errorf("non-empty assistant turn missing: %q", got)
	}
}

// TestSessionContent_SanitizesControlChars verifies C0/C1 removal.
func TestSessionContent_SanitizesControlChars(t *testing.T) {
	turns := []longmemeval.Turn{
		{Role: "user", Content: "hello\x0Bworld\x00end"},
	}
	got := longmemeval.SessionContent(turns)
	for _, bad := range []string{"\x0B", "\x00"} {
		if strings.Contains(got, bad) {
			t.Errorf("control char %q not stripped from %q", bad, got)
		}
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("content stripped too aggressively: %q", got)
	}
}
