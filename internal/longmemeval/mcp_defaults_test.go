package longmemeval

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPDefaults_FallbackWhenNoConfig(t *testing.T) {
	// Isolate from the developer's real ~/.claude/mcp_servers.json.
	t.Setenv("HOME", t.TempDir())

	url, token := MCPDefaults()
	if url != "http://localhost:8788" {
		t.Errorf("MCPDefaults() url = %q, want http://localhost:8788", url)
	}
	if token != "" {
		t.Errorf("MCPDefaults() token = %q, want empty", token)
	}
}

func TestMCPDefaults_HappyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfg := `{
  "mcpServers": {
    "engram": {
      "url": "http://engram.example:8788",
      "headers": {
        "Authorization": "Bearer sometoken"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(claudeDir, "mcp_servers.json"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	url, token := MCPDefaults()
	if url != "http://engram.example:8788" {
		t.Errorf("MCPDefaults() url = %q, want http://engram.example:8788", url)
	}
	if token != "sometoken" {
		t.Errorf("MCPDefaults() token = %q, want sometoken", token)
	}
}

func TestMCPDefaults_StripSSEPathAndQuery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfg := `{
  "mcpServers": {
    "engram": {
      "url": "http://host:8788/sse?foo=bar",
      "headers": {
        "Authorization": "Bearer tok"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(claudeDir, "mcp_servers.json"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	url, token := MCPDefaults()
	if url != "http://host:8788" {
		t.Errorf("MCPDefaults() url = %q, want http://host:8788 (/sse stripped, query dropped)", url)
	}
	if token != "tok" {
		t.Errorf("MCPDefaults() token = %q, want tok", token)
	}
}

func TestEnvOr(t *testing.T) {
	// Non-trimming: historical cmd/longmemeval semantics.
	t.Run("returns fallback when unset", func(t *testing.T) {
		got := EnvOr("__LONGMEMEVAL_ENVOR_UNSET_XYZ__", "fallback")
		if got != "fallback" {
			t.Errorf("EnvOr() = %q, want fallback", got)
		}
	})
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("__LONGMEMEVAL_ENVOR_SET_XYZ__", "fromenv")
		got := EnvOr("__LONGMEMEVAL_ENVOR_SET_XYZ__", "fallback")
		if got != "fromenv" {
			t.Errorf("EnvOr() = %q, want fromenv", got)
		}
	})
	t.Run("returns empty string when env is empty", func(t *testing.T) {
		t.Setenv("__LONGMEMEVAL_ENVOR_EMPTY_XYZ__", "")
		got := EnvOr("__LONGMEMEVAL_ENVOR_EMPTY_XYZ__", "fallback")
		if got != "fallback" {
			t.Errorf("EnvOr() = %q, want fallback for empty value", got)
		}
	})
	t.Run("preserves whitespace-only value without trimming", func(t *testing.T) {
		// Regression for Finding 1: longmemeval historically did not trim.
		t.Setenv("__LONGMEMEVAL_ENVOR_BLANK_XYZ__", "   ")
		got := EnvOr("__LONGMEMEVAL_ENVOR_BLANK_XYZ__", "fallback")
		if got != "   " {
			t.Errorf("EnvOr() = %q, want %q (non-trimming longmemeval semantics)", got, "   ")
		}
	})
}

func TestEnvOrTrimmed(t *testing.T) {
	// Trimming: historical cmd/wp05-retrofit-runner semantics.
	t.Run("returns fallback when unset", func(t *testing.T) {
		got := EnvOrTrimmed("__LONGMEMEVAL_ENVORTRIM_UNSET_XYZ__", "fallback")
		if got != "fallback" {
			t.Errorf("EnvOrTrimmed() = %q, want fallback", got)
		}
	})
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("__LONGMEMEVAL_ENVORTRIM_SET_XYZ__", "fromenv")
		got := EnvOrTrimmed("__LONGMEMEVAL_ENVORTRIM_SET_XYZ__", "fallback")
		if got != "fromenv" {
			t.Errorf("EnvOrTrimmed() = %q, want fromenv", got)
		}
	})
	t.Run("trims surrounding whitespace", func(t *testing.T) {
		t.Setenv("__LONGMEMEVAL_ENVORTRIM_PAD_XYZ__", "  padded  ")
		got := EnvOrTrimmed("__LONGMEMEVAL_ENVORTRIM_PAD_XYZ__", "fallback")
		if got != "padded" {
			t.Errorf("EnvOrTrimmed() = %q, want padded", got)
		}
	})
	t.Run("falls back when whitespace-only", func(t *testing.T) {
		// Regression for Finding 1: wp05 historically trimmed then treated blank as unset.
		t.Setenv("__LONGMEMEVAL_ENVORTRIM_BLANK_XYZ__", "   ")
		got := EnvOrTrimmed("__LONGMEMEVAL_ENVORTRIM_BLANK_XYZ__", "fallback")
		if got != "fallback" {
			t.Errorf("EnvOrTrimmed() = %q, want fallback for whitespace-only value", got)
		}
	})
}

func TestDefaultAPIKey(t *testing.T) {
	// Isolate MCP config so token fallback is deterministic.
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfg := `{
  "mcpServers": {
    "engram": {
      "url": "http://mcp-host:8788",
      "headers": {
        "Authorization": "Bearer mcp-token"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(claudeDir, "mcp_servers.json"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Run("returns MCP token when env unset", func(t *testing.T) {
		t.Setenv("ENGRAM_API_KEY", "")
		// Unset is cleaner than empty for "not set" — but empty also falls through EnvOr.
		// Clear any ambient value: Setenv("") still sets the var empty, which EnvOr treats as unset.
		got := DefaultAPIKey()
		if got != "mcp-token" {
			t.Errorf("DefaultAPIKey() = %q, want mcp-token", got)
		}
	})
	t.Run("returns ENGRAM_API_KEY when set", func(t *testing.T) {
		t.Setenv("ENGRAM_API_KEY", "env-key")
		got := DefaultAPIKey()
		if got != "env-key" {
			t.Errorf("DefaultAPIKey() = %q, want env-key", got)
		}
	})
	t.Run("whitespace-only env is preserved not fallen back", func(t *testing.T) {
		// longmemeval non-trimming path: "   " is non-empty so it wins over MCP token.
		t.Setenv("ENGRAM_API_KEY", "   ")
		got := DefaultAPIKey()
		if got != "   " {
			t.Errorf("DefaultAPIKey() = %q, want %q (non-trimming)", got, "   ")
		}
	})
}

func TestDefaultServerURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfg := `{
  "mcpServers": {
    "engram": {
      "url": "http://mcp-host:8788",
      "headers": {
        "Authorization": "Bearer mcp-token"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(claudeDir, "mcp_servers.json"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Run("returns MCP URL when env unset", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "")
		got := DefaultServerURL()
		if got != "http://mcp-host:8788" {
			t.Errorf("DefaultServerURL() = %q, want http://mcp-host:8788", got)
		}
	})
	t.Run("returns ENGRAM_URL when set", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "http://from-env:9999")
		got := DefaultServerURL()
		if got != "http://from-env:9999" {
			t.Errorf("DefaultServerURL() = %q, want http://from-env:9999", got)
		}
	})
	t.Run("whitespace-only env is preserved not fallen back", func(t *testing.T) {
		t.Setenv("ENGRAM_URL", "   ")
		got := DefaultServerURL()
		if got != "   " {
			t.Errorf("DefaultServerURL() = %q, want %q (non-trimming)", got, "   ")
		}
	})
}

func TestFlagWasProvided(t *testing.T) {
	t.Run("false when flag omitted", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		var v string
		fs.StringVar(&v, "api-key", "", "key")
		if err := fs.Parse(nil); err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if FlagWasProvided(fs, "api-key") {
			t.Error("FlagWasProvided(api-key) = true, want false when omitted")
		}
	})
	t.Run("true when flag provided", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		var v string
		fs.StringVar(&v, "api-key", "", "key")
		if err := fs.Parse([]string{"--api-key=secret"}); err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if !FlagWasProvided(fs, "api-key") {
			t.Error("FlagWasProvided(api-key) = false, want true when provided")
		}
	})
}
