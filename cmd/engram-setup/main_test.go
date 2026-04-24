package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// healthCheck
// ---------------------------------------------------------------------------

func TestHealthCheck(t *testing.T) {
	t.Run("returns nil on 200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		if err := healthCheck(srv.URL); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("returns error on non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		err := healthCheck(srv.URL)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "503") {
			t.Errorf("error should mention status 503, got: %v", err)
		}
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		// Port 1 is reserved and not listening.
		err := healthCheck("http://127.0.0.1:1")
		if err == nil {
			t.Fatal("expected connection-refused error, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// fetchSetupToken
// ---------------------------------------------------------------------------

func TestFetchSetupToken(t *testing.T) {
	validToken := "abcdefghijkl" // exactly 12 chars — minimum required length

	t.Run("returns token and endpoint on 200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/setup-token" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(setupResponse{
				Token:    validToken,
				Endpoint: "http://127.0.0.1:8788/mcp",
				Name:     "engram",
			})
		}))
		defer srv.Close()

		resp, err := fetchSetupToken(srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Token != validToken {
			t.Errorf("Token: got %q, want %q", resp.Token, validToken)
		}
		if resp.Endpoint != "http://127.0.0.1:8788/mcp" {
			t.Errorf("Endpoint: got %q, want %q", resp.Endpoint, "http://127.0.0.1:8788/mcp")
		}
	})

	t.Run("returns error on 403 Forbidden (non-localhost call)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()

		_, err := fetchSetupToken(srv.URL)
		if err == nil {
			t.Fatal("expected error on 403, got nil")
		}
		if !strings.Contains(err.Error(), "localhost-only") {
			t.Errorf("error should mention localhost-only restriction, got: %v", err)
		}
	})

	t.Run("returns error on 404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()

		_, err := fetchSetupToken(srv.URL)
		if err == nil {
			t.Fatal("expected error on 404, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("error should mention status 404, got: %v", err)
		}
	})

	t.Run("returns error on empty token", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(setupResponse{Token: "", Endpoint: "http://x/mcp"})
		}))
		defer srv.Close()

		_, err := fetchSetupToken(srv.URL)
		if err == nil {
			t.Fatal("expected error for empty token, got nil")
		}
	})

	t.Run("returns error on token shorter than 12 chars", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(setupResponse{Token: "short", Endpoint: "http://x/mcp"})
		}))
		defer srv.Close()

		_, err := fetchSetupToken(srv.URL)
		if err == nil {
			t.Fatal("expected error for short token, got nil")
		}
	})

	t.Run("returns error on malformed JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{not valid json`))
		}))
		defer srv.Close()

		_, err := fetchSetupToken(srv.URL)
		if err == nil {
			t.Fatal("expected error on malformed JSON, got nil")
		}
	})

	t.Run("returns error when server unreachable (timeout path)", func(t *testing.T) {
		// Use a closed server so the connection is refused immediately.
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		srv.Close() // close before calling
		_, err := fetchSetupToken(srv.URL)
		if err == nil {
			t.Fatal("expected connection error, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// updateMCPConfig
// ---------------------------------------------------------------------------

func TestUpdateMCPConfig(t *testing.T) {
	entry := map[string]interface{}{
		"type": "sse",
		"url":  "http://127.0.0.1:8788/mcp",
		"headers": map[string]string{
			"Authorization": "Bearer test-token-123",
		},
	}

	t.Run("creates mcp_servers.json when absent", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, ".claude", "mcp_servers.json")

		action, err := updateMCPConfig(cfgPath, "engram", entry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if action != "added" {
			t.Errorf("action: got %q, want %q", action, "added")
		}

		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatalf("file not created: %v", err)
		}

		var cfg map[string]json.RawMessage
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("output is not valid JSON: %v", err)
		}
		if _, ok := cfg["mcpServers"]; !ok {
			t.Error("mcpServers key missing from written config")
		}

		var mcpServers map[string]json.RawMessage
		if err := json.Unmarshal(cfg["mcpServers"], &mcpServers); err != nil {
			t.Fatalf("mcpServers not valid JSON: %v", err)
		}
		if _, ok := mcpServers["engram"]; !ok {
			t.Error("engram entry missing from mcpServers")
		}
	})

	t.Run("updates existing entry in mcp_servers.json", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, ".claude", "mcp_servers.json")

		// Write an initial config with a stale token.
		initial := `{"mcpServers":{"engram":{"type":"sse","url":"http://127.0.0.1:8788/mcp","headers":{"Authorization":"Bearer old-token"}}}}`
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfgPath, []byte(initial), 0600); err != nil {
			t.Fatal(err)
		}

		action, err := updateMCPConfig(cfgPath, "engram", entry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if action != "updated" {
			t.Errorf("action: got %q, want %q", action, "updated")
		}

		raw, _ := os.ReadFile(cfgPath)
		if !strings.Contains(string(raw), "Bearer test-token-123") {
			t.Error("updated file does not contain the new token")
		}
	})

	t.Run("file permissions are 0600", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, ".claude", "mcp_servers.json")

		_, err := updateMCPConfig(cfgPath, "engram", entry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		info, err := os.Stat(cfgPath)
		if err != nil {
			t.Fatalf("stat failed: %v", err)
		}
		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Errorf("file permissions: got %o, want 0600", perm)
		}
	})

	t.Run("skips .claude.json when file absent", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, ".claude.json")
		// File does not exist.

		action, err := updateMCPConfig(cfgPath, "engram", entry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if action != "" {
			t.Errorf("expected empty action (skip), got %q", action)
		}
		if _, statErr := os.Stat(cfgPath); !os.IsNotExist(statErr) {
			t.Error(".claude.json should not have been created")
		}
	})

	t.Run("skips .claude.json when mcpServers key is absent", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, ".claude.json")

		// File exists but has no mcpServers key.
		if err := os.WriteFile(cfgPath, []byte(`{"theme":"dark"}`), 0600); err != nil {
			t.Fatal(err)
		}

		action, err := updateMCPConfig(cfgPath, "engram", entry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if action != "" {
			t.Errorf("expected empty action (skip), got %q", action)
		}

		// File content should be unchanged.
		raw, _ := os.ReadFile(cfgPath)
		if strings.Contains(string(raw), "mcpServers") {
			t.Error(".claude.json should not have gained an mcpServers key")
		}
	})

	t.Run("updates .claude.json when mcpServers key already present", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, ".claude.json")

		existing := `{"theme":"dark","mcpServers":{"other":{"type":"sse","url":"http://other/mcp"}}}`
		if err := os.WriteFile(cfgPath, []byte(existing), 0600); err != nil {
			t.Fatal(err)
		}

		action, err := updateMCPConfig(cfgPath, "engram", entry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if action != "added" {
			t.Errorf("action: got %q, want %q", action, "added")
		}

		raw, _ := os.ReadFile(cfgPath)
		var cfg map[string]json.RawMessage
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("output not valid JSON: %v", err)
		}
		// Both the new and the existing MCP entry should be present.
		var mcpServers map[string]json.RawMessage
		json.Unmarshal(cfg["mcpServers"], &mcpServers)
		if _, ok := mcpServers["engram"]; !ok {
			t.Error("engram entry missing from updated .claude.json")
		}
		if _, ok := mcpServers["other"]; !ok {
			t.Error("pre-existing 'other' MCP entry was lost")
		}
		// Original theme key must survive.
		if _, ok := cfg["theme"]; !ok {
			t.Error("theme key was lost from .claude.json")
		}
	})

	t.Run("returns error on invalid JSON in existing file", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, ".claude", "mcp_servers.json")
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfgPath, []byte(`{bad json`), 0600); err != nil {
			t.Fatal(err)
		}

		_, err := updateMCPConfig(cfgPath, "engram", entry)
		if err == nil {
			t.Fatal("expected error on malformed JSON, got nil")
		}
	})

	t.Run("written JSON is valid and ends with newline", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, ".claude", "mcp_servers.json")

		_, err := updateMCPConfig(cfgPath, "engram", entry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if len(raw) == 0 || raw[len(raw)-1] != '\n' {
			t.Error("written file should end with a newline")
		}
		var out interface{}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Errorf("written file is not valid JSON: %v", err)
		}
	})
}
