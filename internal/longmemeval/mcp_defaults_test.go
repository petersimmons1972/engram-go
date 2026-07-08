package longmemeval

import (
	"flag"
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

func TestEnvOr(t *testing.T) {
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
	t.Run("trims whitespace and falls back when blank", func(t *testing.T) {
		t.Setenv("__LONGMEMEVAL_ENVOR_BLANK_XYZ__", "   ")
		got := EnvOr("__LONGMEMEVAL_ENVOR_BLANK_XYZ__", "fallback")
		if got != "fallback" {
			t.Errorf("EnvOr() = %q, want fallback for whitespace-only value", got)
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
