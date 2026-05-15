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
	id, err := rc.QuickStore(ctx, "proj-1", "content here", []string{"tag1"})
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
	id, err := rc.QuickStore(ctx, "proj-x", "hello", nil)
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
	id, err := rc.QuickStore(ctx, "p", "c", nil)
	if err != nil {
		t.Fatalf("QuickStore after 500+retry: %v", err)
	}
	if id != "mem-ok" {
		t.Errorf("id = %q, want mem-ok", id)
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
