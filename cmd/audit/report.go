package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// buildReport aggregates individual audit results into a top-level report.
// Ported verbatim from instinct-python/cmd/audit/main.go:222-241.
//
// ERROR verdicts count as REJECT for weekly-cron metrics.
// FalsePositiveRate = (records where false_positive=="yes") / total.
func buildReport(results []auditResult) report {
	r := report{Total: len(results), Patterns: results}
	fps := 0
	for _, res := range results {
		switch res.Verdict {
		case "KEEP":
			r.Keep++
		case "TUNE":
			r.Tune++
		case "REJECT", "ERROR":
			r.Reject++
		}
		if strings.ToLower(res.FalsePositive) == "yes" {
			fps++
		}
	}
	if r.Total > 0 {
		r.FalsePositiveRate = float64(fps) / float64(r.Total)
	}
	return r
}

// extractTags extracts the pattern type, domain, and sig-* tag from the tags
// slice. Ported verbatim from instinct-python/cmd/audit/main.go:244-260.
func extractTags(tags []string) (ptype, domain, sig string) {
	for _, t := range tags {
		switch t {
		case "correction", "error_resolution", "workflow":
			ptype = t
		case "instinct":
			// skip — not a domain or type
		default:
			if strings.HasPrefix(t, "sig-") {
				sig = t
			} else {
				domain = t
			}
		}
	}
	return
}

// truncate returns s truncated to n runes with a trailing "…" when truncated.
// Ported verbatim from instinct-python/cmd/audit/main.go:286-291.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// envOr returns the value of environment variable key, or fallback when the
// variable is absent or empty. Ported from instinct-python/cmd/audit/main.go:293-298.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// errNoEngram is returned when no Engram credentials can be resolved from
// flags, environment variables, or the developer config file.
var errNoEngram = errors.New(
	"engram credentials not found: set ENGRAM_URL and ENGRAM_TOKEN env vars, " +
		"or pass -engram and -token flags",
)
// resolveEngram returns the Engram base URL and bearer token to use.
//
// Resolution order (first fully-populated pair wins):
//  1. -engram / -token CLI flags (baseFlag, tokenFlag)
//  2. ENGRAM_URL / ENGRAM_TOKEN environment variables
//  3. ~/.claude/mcp_servers.json — developer-only fallback, attempted only
//     when the file exists and no env vars are set
//
// If none of the above provide both values, errNoEngram is returned.
func resolveEngram(baseFlag, tokenFlag string) (string, string, error) {
	// 1. Flags take priority.
	if baseFlag != "" && tokenFlag != "" {
		return baseFlag, tokenFlag, nil
	}

	// 2. Environment variables.
	envURL := os.Getenv("ENGRAM_URL")
	envToken := os.Getenv("ENGRAM_TOKEN")
	if envURL != "" && envToken != "" {
		return envURL, envToken, nil
	}

	// 3. Developer config file — silent fallback when the file exists.
	cfgPath := filepath.Join(os.Getenv("HOME"), ".claude", "mcp_servers.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		// File absent or unreadable — not a developer machine.
		return "", "", errNoEngram
	}
	var cfg struct {
		McpServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", "", err
	}
	e := cfg.McpServers["engram"]
	sseURL := strings.TrimSuffix(strings.TrimRight(e.URL, "/"), "/sse")
	auth := strings.TrimPrefix(e.Headers["Authorization"], "Bearer ")
	if sseURL == "" || auth == "" {
		return "", "", errNoEngram
	}
	return sseURL, auth, nil
}
