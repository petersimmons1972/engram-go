package main

import (
	"encoding/json"
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

// resolveEngram returns the Engram base URL and bearer token to use.
// If baseFlag and tokenFlag are both non-empty they are used as-is.
// Otherwise the values are read from ~/.claude/mcp_servers.json.
// Ported verbatim from instinct-python/cmd/audit/main.go:262-284.
func resolveEngram(baseFlag, tokenFlag string) (string, string, error) {
	if baseFlag != "" && tokenFlag != "" {
		return baseFlag, tokenFlag, nil
	}
	cfgPath := filepath.Join(os.Getenv("HOME"), ".claude", "mcp_servers.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", "", err
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
	return sseURL, auth, nil
}
