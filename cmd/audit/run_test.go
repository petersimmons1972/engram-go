package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// writeJSONResponse encodes v as JSON and writes it to w.
func writeJSONResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// TestRunAuditWithMockAll exercises runAudit end-to-end with mock Engram and
// mock LLM client. Covers the main orchestration path in runAudit().
func TestRunAuditWithMockAll(t *testing.T) {
	pattern := engramMemory{
		ID:         "run-id",
		Content:    "pattern for run test",
		Tags:       []string{"instinct", "workflow", "git", "sig-run"},
		Importance: 0.75,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/quick-recall" {
			writeJSONResponse(w, map[string]any{
				"results": []engramMemory{pattern},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := &mockLLMClient{
		response: "IS_VALID: yes\nIS_ACTIONABLE: yes\nIS_SPECIFIC: yes\nFALSE_POSITIVE: no\nVERDICT: TUNE\nREASON: Needs slight rewording.",
	}

	var buf bytes.Buffer
	err := runAudit(srv.URL, "tok", client, 5*time.Second, &buf)
	if err != nil {
		t.Fatalf("runAudit: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected JSON output, got empty buffer")
	}
}

// TestRunAuditDefaultTimeout verifies that passing timeout=0 applies the default.
func TestRunAuditDefaultTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, map[string]any{"results": []engramMemory{}})
	}))
	defer srv.Close()

	client := &mockLLMClient{}
	var buf bytes.Buffer
	// timeout=0 should use defaultTimeout internally — must not panic.
	err := runAudit(srv.URL, "tok", client, 0, &buf)
	if err != nil {
		t.Fatalf("runAudit with timeout=0: %v", err)
	}
}

// TestRunReturnsOneOnBadConfig verifies that run() returns non-zero when
// HOME has no mcp_servers.json and no -engram/-token flags are set.
func TestRunReturnsOneOnBadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// run() calls flag.Parse() which is global — reset os.Args to avoid
	// parsing test flags that would confuse the flag package.
	origArgs := os.Args
	os.Args = []string{"instinct-audit-go"}
	defer func() { os.Args = origArgs }()

	code := run()
	if code == 0 {
		t.Error("expected non-zero exit when config is missing, got 0")
	}
}
