package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestHTTPClient(fn func(*http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{Transport: roundTripFunc(fn)}
}

// ---------------------------------------------------------------------------
// healthCheck
// ---------------------------------------------------------------------------

func TestHealthCheck(t *testing.T) {
	t.Run("returns nil on 200", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/health" {
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		if err := healthCheckWithClient("http://example", client); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("returns error on non-200", func(t *testing.T) {
		client := newTestHTTPClient(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		err := healthCheckWithClient("http://example", client)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "503") {
			t.Errorf("error should mention status 503, got: %v", err)
		}
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		client := newTestHTTPClient(func(*http.Request) (*http.Response, error) {
			return nil, http.ErrServerClosed
		})
		err := healthCheckWithClient("http://example", client)
		if err == nil {
			t.Fatal("expected connection error, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// fetchSetupToken
// ---------------------------------------------------------------------------

func TestFetchSetupToken(t *testing.T) {
	validToken := "abcdefghijkl" // exactly 12 chars — minimum required length

	t.Run("returns token and endpoint on 200", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/setup-token" {
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			var buf strings.Builder
			json.NewEncoder(&buf).Encode(setupResponse{
				Token:    validToken,
				Endpoint: "http://127.0.0.1:8788/mcp",
				Name:     "engram",
			})
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(buf.String()))}, nil
		})
		resp, err := fetchSetupTokenWithClient("http://example", client)
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
		client := newTestHTTPClient(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		_, err := fetchSetupTokenWithClient("http://example", client)
		if err == nil {
			t.Fatal("expected error on 403, got nil")
		}
		if !strings.Contains(err.Error(), "localhost-only") {
			t.Errorf("error should mention localhost-only restriction, got: %v", err)
		}
	})

	t.Run("returns error on 404", func(t *testing.T) {
		client := newTestHTTPClient(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		_, err := fetchSetupTokenWithClient("http://example", client)
		if err == nil {
			t.Fatal("expected error on 404, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("error should mention status 404, got: %v", err)
		}
	})

	t.Run("returns error on empty token", func(t *testing.T) {
		client := newTestHTTPClient(func(*http.Request) (*http.Response, error) {
			var buf strings.Builder
			json.NewEncoder(&buf).Encode(setupResponse{Token: "", Endpoint: "http://x/mcp"})
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(buf.String()))}, nil
		})
		_, err := fetchSetupTokenWithClient("http://example", client)
		if err == nil {
			t.Fatal("expected error for empty token, got nil")
		}
	})

	t.Run("returns error on token shorter than 12 chars", func(t *testing.T) {
		client := newTestHTTPClient(func(*http.Request) (*http.Response, error) {
			var buf strings.Builder
			json.NewEncoder(&buf).Encode(setupResponse{Token: "short", Endpoint: "http://x/mcp"})
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(buf.String()))}, nil
		})
		_, err := fetchSetupTokenWithClient("http://example", client)
		if err == nil {
			t.Fatal("expected error for short token, got nil")
		}
	})

	t.Run("returns error on malformed JSON", func(t *testing.T) {
		client := newTestHTTPClient(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{not valid json`))}, nil
		})
		_, err := fetchSetupTokenWithClient("http://example", client)
		if err == nil {
			t.Fatal("expected error on malformed JSON, got nil")
		}
	})

	t.Run("returns error when server unreachable (timeout path)", func(t *testing.T) {
		client := newTestHTTPClient(func(*http.Request) (*http.Response, error) {
			return nil, http.ErrHandlerTimeout
		})
		_, err := fetchSetupTokenWithClient("http://example", client)
		if err == nil {
			t.Fatal("expected connection error, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// tryKeyFromDisk (#614, #616)
// ---------------------------------------------------------------------------

// TestTryKeyFromDisk verifies that tryKeyFromDisk reads a key from a file,
// probes /quick-recall to validate it, and returns the key only when auth
// succeeds. Covers absent files, short keys, and wrong keys (#614, #616).
func TestTryKeyFromDisk(t *testing.T) {
	const validKey = "valid-fallback-key-0123456789abcd" // 32 chars

	// Test server: /quick-recall returns 200 only for the valid key.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quick-recall" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") == "Bearer "+validKey {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	t.Run("valid key file returns key", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "api_key")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(validKey); err != nil {
			t.Fatal(err)
		}
		f.Close()

		got, err := tryKeyFromDisk(f.Name(), srv.URL, srv.Client())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != validKey {
			t.Errorf("got %q, want %q", got, validKey)
		}
	})

	t.Run("valid key with trailing newline is trimmed", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "api_key")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(validKey + "\n"); err != nil {
			t.Fatal(err)
		}
		f.Close()

		got, err := tryKeyFromDisk(f.Name(), srv.URL, srv.Client())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != validKey {
			t.Errorf("got %q, want %q (trailing newline not trimmed)", got, validKey)
		}
	})

	t.Run("absent file returns empty key without error", func(t *testing.T) {
		got, err := tryKeyFromDisk(filepath.Join(t.TempDir(), "absent_api_key"), srv.URL, srv.Client())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("expected empty key for absent file, got %q", got)
		}
	})

	t.Run("wrong key returns empty without error", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "api_key")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("wrong-key-0123456789abcdef012345"); err != nil {
			t.Fatal(err)
		}
		f.Close()

		got, err := tryKeyFromDisk(f.Name(), srv.URL, srv.Client())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("expected empty for wrong key, got %q", got)
		}
	})

	t.Run("key shorter than 12 chars returns empty without probing server", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "api_key")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("short"); err != nil {
			t.Fatal(err)
		}
		f.Close()

		got, err := tryKeyFromDisk(f.Name(), srv.URL, srv.Client())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("expected empty for short key, got %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// readKeyFromEnvFile (#616)
// ---------------------------------------------------------------------------

// TestReadKeyFromEnvFile verifies that readKeyFromEnvFile extracts the
// ENGRAM_API_KEY value from a .env-format file (#616).
func TestReadKeyFromEnvFile(t *testing.T) {
	write := func(t *testing.T, content string) string {
		t.Helper()
		f, err := os.CreateTemp(t.TempDir(), ".env")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(content); err != nil {
			t.Fatal(err)
		}
		f.Close()
		return f.Name()
	}

	t.Run("returns key from valid .env file", func(t *testing.T) {
		path := write(t, "POSTGRES_PASSWORD=secret\nENGRAM_API_KEY=mykey12345678901234\nOTHER=x\n")
		got := readKeyFromEnvFile(path)
		if got != "mykey12345678901234" {
			t.Errorf("got %q, want %q", got, "mykey12345678901234")
		}
	})

	t.Run("last ENGRAM_API_KEY wins when duplicated", func(t *testing.T) {
		path := write(t, "ENGRAM_API_KEY=first\nENGRAM_API_KEY=second\n")
		got := readKeyFromEnvFile(path)
		if got != "second" {
			t.Errorf("got %q, want last value %q", got, "second")
		}
	})

	t.Run("absent file returns empty string", func(t *testing.T) {
		got := readKeyFromEnvFile(filepath.Join(t.TempDir(), "nonexistent.env"))
		if got != "" {
			t.Errorf("expected empty for absent file, got %q", got)
		}
	})

	t.Run("file without ENGRAM_API_KEY returns empty string", func(t *testing.T) {
		path := write(t, "POSTGRES_PASSWORD=secret\nOTHER=x\n")
		got := readKeyFromEnvFile(path)
		if got != "" {
			t.Errorf("expected empty when key absent, got %q", got)
		}
	})

	t.Run("ENGRAM_API_KEY with empty value returns empty string", func(t *testing.T) {
		path := write(t, "ENGRAM_API_KEY=\n")
		got := readKeyFromEnvFile(path)
		if got != "" {
			t.Errorf("expected empty for empty value, got %q", got)
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
