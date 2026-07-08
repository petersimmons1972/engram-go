package longmemeval

import (
	"encoding/json"
	"flag"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
)

// MCPDefaults reads the engram URL and Bearer token from ~/.claude/mcp_servers.json,
// which is kept current by the session-start hook. Falls back to localhost defaults.
func MCPDefaults() (url, token string) {
	url = "http://localhost:8788"
	home, err := os.UserHomeDir()
	if err != nil {
		return url, ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "mcp_servers.json"))
	if err != nil {
		return url, ""
	}
	var cfg struct {
		McpServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return url, ""
	}
	for name, srv := range cfg.McpServers {
		if name != "engram" {
			continue
		}
		// Strip /sse path component — the benchmark appends it in Connect().
		// Parse properly so query params don't break the suffix check.
		srvURL := srv.URL
		if u, err := neturl.Parse(srvURL); err == nil {
			u.Path = strings.TrimSuffix(u.Path, "/sse")
			u.RawQuery = ""
			srvURL = u.String()
		}
		if srvURL != "" {
			url = srvURL
		}
		if auth := srv.Headers["Authorization"]; len(auth) > 7 {
			token = auth[7:] // strip "Bearer "
		}
		return url, token
	}
	return url, token
}

// FlagWasProvided reports whether the named flag was set on the FlagSet
// (including an explicit empty value).
func FlagWasProvided(fs *flag.FlagSet, name string) bool {
	provided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
}

// EnvOr returns the value of the named environment variable, or fallback when
// the variable is unset or empty. It does not trim whitespace: a value of
// "   " is returned as-is. This matches historical cmd/longmemeval semantics
// (and is what DefaultAPIKey / DefaultServerURL use).
func EnvOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// EnvOrTrimmed returns the trimmed value of the named environment variable, or
// fallback when the variable is unset or blank after trimming. This matches
// historical cmd/wp05-retrofit-runner semantics for ENGRAM_API_KEY / ENGRAM_URL.
func EnvOrTrimmed(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

// DefaultAPIKey returns ENGRAM_API_KEY if set (non-empty, not trimmed), otherwise
// the Bearer token from MCPDefaults (empty when neither is available).
// Non-trimming matches historical cmd/longmemeval behavior.
func DefaultAPIKey() string {
	_, token := MCPDefaults()
	return EnvOr("ENGRAM_API_KEY", token)
}

// DefaultServerURL returns ENGRAM_URL if set (non-empty, not trimmed), otherwise
// the URL from MCPDefaults (localhost:8788 when neither is available).
// Non-trimming matches historical cmd/longmemeval behavior.
func DefaultServerURL() string {
	url, _ := MCPDefaults()
	return EnvOr("ENGRAM_URL", url)
}
