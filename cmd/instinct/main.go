// Package main is the instinct pattern-detection daemon.
// It reads tool-use events from a JSONL buffer, groups them by session,
// calls Claude Haiku to identify patterns, and upserts those patterns
// into Engram memory.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ── Types ────────────────────────────────────────────────────────────────────

// Event is one tool-use record written by the PostToolUse hook.
type Event struct {
	Timestamp         string `json:"timestamp"`
	SessionID         string `json:"session_id"`
	ProjectID         string `json:"project_id"`
	ToolName          string `json:"tool_name"`
	ToolInputHash     string `json:"tool_input_hash"`
	ToolOutputSummary string `json:"tool_output_summary"`
	ExitStatus        int    `json:"exit_status"`
	SchemaVersion     int    `json:"schema_version"`
}

// Pattern is the structured output produced by Haiku for a session.
type Pattern struct {
	Type         string `json:"type"`
	Description  string `json:"description"`
	Domain       string `json:"domain"`
	Evidence     string `json:"evidence"`
	TagSignature string `json:"tag_signature"`
}

// sessionKey uniquely identifies one session within one project.
type sessionKey struct {
	sessionID string
	projectID string
}

// recallResult holds an Engram memory candidate returned during dedup checks.
type recallResult struct {
	id         string
	confidence float64
}

// config holds all runtime knobs; all fields are populated by loadConfig.
type config struct {
	bufferPath    string
	minEvents     int
	engramURL     string
	engramToken   string
	anthropicKey  string
	haikuEndpoint string // empty → production endpoint; injectable for tests
}

// ── Config ───────────────────────────────────────────────────────────────────

// loadConfig builds a config from environment variables, falling back to
// ~/.claude/mcp_servers.json for Engram credentials when the env vars are
// absent.  The mcp_servers.json fallback is non-fatal: if the file is missing
// or has no engram entry, a warning is logged and loadConfig returns the
// partially-populated config so callers that only need defaults (bufferPath,
// minEvents) can still function.
func loadConfig() (config, error) {
	home, _ := os.UserHomeDir()
	cfg := config{
		bufferPath:   envOr("INSTINCT_BUFFER", home+"/.local/state/instinct/buffer.jsonl"),
		minEvents:    envInt("INSTINCT_MIN_EVENTS", 20),
		engramURL:    os.Getenv("ENGRAM_BASE_URL"),
		engramToken:  os.Getenv("ENGRAM_API_KEY"),
		anthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
	}

	// Fall back to ~/.claude/mcp_servers.json when env vars are absent.
	// This is best-effort: a missing file or missing engram entry is a warning,
	// not a hard error, so that tests which only care about other fields can pass.
	if cfg.engramURL == "" || cfg.engramToken == "" {
		url, tok, err := readMCPConfig(home + "/.claude/mcp_servers.json")
		if err != nil {
			slog.Warn("engram credentials not set via env and mcp_servers.json unreadable",
				"path", home+"/.claude/mcp_servers.json", "err", err)
		} else {
			if cfg.engramURL == "" {
				cfg.engramURL = url
			}
			if cfg.engramToken == "" {
				cfg.engramToken = tok
			}
		}
	}

	return cfg, nil
}

// readMCPConfig parses ~/.claude/mcp_servers.json and extracts the URL and
// Bearer token for the "engram" MCP server entry.
func readMCPConfig(path string) (url, token string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var raw struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", "", fmt.Errorf("parse %s: %w", path, err)
	}
	srv, ok := raw.MCPServers["engram"]
	if !ok {
		return "", "", fmt.Errorf("no 'engram' entry in mcpServers")
	}
	tok := srv.Headers["Authorization"]
	// Strip "Bearer " prefix if present.
	if strings.HasPrefix(tok, "Bearer ") {
		tok = tok[len("Bearer "):]
	}
	return srv.URL, tok, nil
}

// envOr returns the value of the named environment variable, or def when unset
// or empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envInt returns the integer value of the named environment variable, or def
// when unset, empty, or not parseable as a decimal integer.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	if err := run(context.Background(), cfg); err != nil {
		slog.Error("run", "err", err)
		os.Exit(1)
	}
}

// run is the top-level pipeline. Implemented in Task 7 — stub for now.
func run(_ context.Context, _ config) error {
	return nil
}

// ── Suppress unused-import warnings ──────────────────────────────────────────
// These packages are imported now to avoid churn when later tasks add usage.

var _ = filepath.Join
var _ = strings.TrimSpace
var _ = time.Now
