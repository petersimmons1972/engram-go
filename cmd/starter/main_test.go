package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// patchDatabaseURLPassword
// ---------------------------------------------------------------------------

func TestPatchDatabaseURLPassword(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		password string
		want     string
	}{
		{
			name:     "standard postgres scheme with existing password",
			dsn:      "postgres://alice:oldpass@db.example.com:5432/mydb",
			password: "newpass",
			want:     "postgres://alice:newpass@db.example.com:5432/mydb",
		},
		{
			name:     "postgresql scheme",
			dsn:      "postgresql://bob:secret@localhost/testdb",
			password: "fresh",
			want:     "postgresql://bob:fresh@localhost/testdb",
		},
		{
			name:     "URL with no password field",
			dsn:      "postgres://carol@db.example.com/mydb",
			password: "injected",
			want:     "postgres://carol:injected@db.example.com/mydb",
		},
		{
			name:     "special characters in new password",
			dsn:      "postgres://user:old@host/db",
			password: "p@ss!w0rd#$%",
			// url.String() percent-encodes special chars in the password
			want:     "postgres://user:p%40ss%21w0rd%23$%25@host/db",
		},
		{
			name:     "empty DSN returns unchanged",
			dsn:      "",
			password: "anything",
			want:     "",
		},
		{
			name:     "unparseable DSN returns unchanged",
			dsn:      "not-a-url://\x00",
			password: "anything",
			want:     "not-a-url://\x00",
		},
		{
			name:     "DSN with no userinfo returns unchanged",
			dsn:      "postgres://host/db",
			password: "x",
			// url.Parse succeeds but u.User is nil -- function returns dsn unchanged
			want: "postgres://host/db",
		},
		{
			name:     "empty password clears existing password",
			dsn:      "postgres://user:old@host/db",
			password: "",
			want:     "postgres://user:@host/db",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := patchDatabaseURLPassword(tc.dsn, tc.password)
			if got != tc.want {
				t.Errorf("patchDatabaseURLPassword(%q, %q)\n  got  %q\n  want %q",
					tc.dsn, tc.password, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// filteredEnv
// ---------------------------------------------------------------------------

func TestFilteredEnv(t *testing.T) {
	// Set a known environment before calling filteredEnv so the test is
	// deterministic regardless of what the real process environment contains.
	keys := []string{
		"INFISICAL_CLIENT_ID",
		"INFISICAL_CLIENT_SECRET",
		"POSTGRES_PASSWORD",
		"SAFE_VAR_1",
		"SAFE_VAR_2",
	}
	values := map[string]string{
		"INFISICAL_CLIENT_ID":     "cid-value",
		"INFISICAL_CLIENT_SECRET": "csec-value",
		"POSTGRES_PASSWORD":       "pgpass-value",
		"SAFE_VAR_1":              "keep-me",
		"SAFE_VAR_2":              "keep-me-too",
	}
	// Set vars; restore on cleanup.
	for k, v := range values {
		t.Setenv(k, v)
	}

	result := filteredEnv("INFISICAL_CLIENT_ID", "INFISICAL_CLIENT_SECRET", "POSTGRES_PASSWORD")

	// Build a lookup map from the returned slice.
	got := make(map[string]string)
	for _, kv := range result {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			continue
		}
		got[kv[:idx]] = kv[idx+1:]
	}

	// Credential keys MUST be absent.
	for _, forbidden := range []string{"INFISICAL_CLIENT_ID", "INFISICAL_CLIENT_SECRET", "POSTGRES_PASSWORD"} {
		if _, present := got[forbidden]; present {
			t.Errorf("filteredEnv: %q was not stripped from the environment", forbidden)
		}
	}

	// Safe keys MUST be present with original values.
	for _, safe := range []string{"SAFE_VAR_1", "SAFE_VAR_2"} {
		v, present := got[safe]
		if !present {
			t.Errorf("filteredEnv: %q was incorrectly stripped", safe)
			continue
		}
		if v != values[safe] {
			t.Errorf("filteredEnv: %q value changed: got %q want %q", safe, v, values[safe])
		}
	}

	// Sanity-check: the caller sees something non-empty (at least PATH should survive).
	if len(result) == 0 {
		t.Error("filteredEnv returned an empty slice; expected at least some env vars to survive")
	}

	_ = keys // used implicitly via t.Setenv
}

func TestFilteredEnvNoKeysToRemove(t *testing.T) {
	t.Setenv("CANARY", "alive")
	result := filteredEnv() // no keys to drop
	got := make(map[string]string)
	for _, kv := range result {
		idx := strings.IndexByte(kv, '=')
		if idx >= 0 {
			got[kv[:idx]] = kv[idx+1:]
		}
	}
	if got["CANARY"] != "alive" {
		t.Errorf("filteredEnv with no remove keys should preserve all vars; CANARY lost")
	}
}

// ---------------------------------------------------------------------------
// isValidInfisicalDomain (covers both the FQDN regex and IP rejection)
// ---------------------------------------------------------------------------

func TestIsValidInfisicalDomain(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		// Valid Infisical domains
		{"default self-hosted", "https://infisical.petersimmons.com", true},
		{"official SaaS", "https://app.infisical.com", true},
		{"custom subdomain", "https://secrets.internal.company.io", true},
		{"hyphen in label", "https://my-infisical.example.com", true},
		{"deep subdomain", "https://a.b.c.infisical.com", true},

		// Invalid: single-label hostnames (no dot)
		{"single-label hostname", "https://infisical", false},
		{"localhost (single-label)", "https://localhost", false},

		// Invalid: raw IP addresses (have dots but are numeric)
		{"IPv4 private", "https://192.168.1.1", false},
		{"IPv4 loopback", "https://127.0.0.1", false},
		{"IPv4 public", "https://8.8.8.8", false},

		// Invalid: wrong scheme
		{"http not https", "http://app.infisical.com", false},
		{"no scheme", "app.infisical.com", false},
		{"ftp scheme", "ftp://app.infisical.com", false},

		// Invalid: paths/credentials/ports in URL
		{"with path", "https://app.infisical.com/api", false},
		{"with credentials in URL", "https://user:pass@infisical.com", false},
		{"with port", "https://infisical.com:8443", false},
		{"trailing slash", "https://infisical.com/", false},

		// Edge cases
		{"empty string", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidInfisicalDomain(tc.input)
			if got != tc.valid {
				t.Errorf("isValidInfisicalDomain(%q) = %v, want %v", tc.input, got, tc.valid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// getAccessToken (HTTP mock)
// ---------------------------------------------------------------------------

func TestGetAccessToken(t *testing.T) {
	t.Run("returns token on 200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/auth/universal-auth/login" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"accessToken": "tok-abc123"})
		}))
		defer srv.Close()

		token, err := getAccessToken(t.Context(), srv.URL, "cid", "csec")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token != "tok-abc123" {
			t.Errorf("got token %q, want %q", token, "tok-abc123")
		}
	})

	t.Run("returns error on non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}))
		defer srv.Close()

		_, err := getAccessToken(t.Context(), srv.URL, "bad", "creds")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "401") {
			t.Errorf("error should mention status 401, got: %v", err)
		}
	})

	t.Run("returns error on empty access token", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"accessToken": ""})
		}))
		defer srv.Close()

		_, err := getAccessToken(t.Context(), srv.URL, "cid", "csec")
		if err == nil {
			t.Fatal("expected error for empty access token, got nil")
		}
	})

	t.Run("returns error on malformed JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{not valid json`))
		}))
		defer srv.Close()

		_, err := getAccessToken(t.Context(), srv.URL, "cid", "csec")
		if err == nil {
			t.Fatal("expected error on malformed JSON, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// getSecret (HTTP mock)
// ---------------------------------------------------------------------------

func TestGetSecret(t *testing.T) {
	t.Run("returns secret value on 200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"secret": map[string]string{"secretValue": "supersecret"},
			})
		}))
		defer srv.Close()

		val, err := getSecret(t.Context(), srv.URL, "tok", "proj", "prod", "/engram", "MY_SECRET")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "supersecret" {
			t.Errorf("got %q, want %q", val, "supersecret")
		}
	})

	t.Run("returns error on 404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()

		_, err := getSecret(t.Context(), srv.URL, "tok", "proj", "prod", "/engram", "MISSING")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("error should mention 404, got: %v", err)
		}
	})

	t.Run("returns error on empty secret value", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"secret": map[string]string{"secretValue": ""},
			})
		}))
		defer srv.Close()

		_, err := getSecret(t.Context(), srv.URL, "tok", "proj", "prod", "/engram", "EMPTY_SECRET")
		if err == nil {
			t.Fatal("expected error for empty secret value, got nil")
		}
	})

	t.Run("secret name is path-escaped in URL", func(t *testing.T) {
		var capturedURI string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// r.URL.RequestURI() preserves the raw (percent-encoded) form as sent on the wire.
			capturedURI = r.URL.RequestURI()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"secret": map[string]string{"secretValue": "val"},
			})
		}))
		defer srv.Close()

		_, _ = getSecret(t.Context(), srv.URL, "tok", "proj", "prod", "/", "MY/SECRET")
		// url.PathEscape("MY/SECRET") => "MY%2FSECRET"; verify the slash was encoded.
		if !strings.Contains(capturedURI, "MY%2FSECRET") {
			t.Errorf("secret name not path-escaped in request URL; got URI %q", capturedURI)
		}
	})
}

// ---------------------------------------------------------------------------
// envOr helper
// ---------------------------------------------------------------------------

func TestEnvOr(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("TEST_ENVOR_KEY", "from-env")
		got := envOr("TEST_ENVOR_KEY", "default")
		if got != "from-env" {
			t.Errorf("got %q, want %q", got, "from-env")
		}
	})

	t.Run("returns default when env unset", func(t *testing.T) {
		os.Unsetenv("TEST_ENVOR_KEY_MISSING")
		got := envOr("TEST_ENVOR_KEY_MISSING", "my-default")
		if got != "my-default" {
			t.Errorf("got %q, want %q", got, "my-default")
		}
	})

	t.Run("returns default when env is empty string", func(t *testing.T) {
		t.Setenv("TEST_ENVOR_EMPTY", "")
		got := envOr("TEST_ENVOR_EMPTY", "fallback")
		if got != "fallback" {
			t.Errorf("got %q, want %q", got, "fallback")
		}
	})
}
