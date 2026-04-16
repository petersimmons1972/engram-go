package mcp

import "regexp"

// operationalConfigPatterns are signals that a memory contains live infrastructure
// config rather than learned knowledge. Content matching 2+ patterns is skipped
// during CLAUDE.md import and ingest — it belongs in config files, not Engram.
var operationalConfigPatterns = []*regexp.Regexp{
	// Connection strings: postgresql://, redis://, http://host, https://host
	regexp.MustCompile(`(?i)(postgresql|postgres|redis|mongodb|mysql|https?)://\S+`),
	// DNS arrow patterns: hostname → IP or hostname -> IP
	regexp.MustCompile(`\S+\s*(→|->)\s*\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`),
	// Bare IP:port pairs
	regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}:\d{2,5}`),
	// Environment variable assignments with URLs
	regexp.MustCompile(`[A-Z][A-Z0-9_]+=https?://\S+`),
}

// isOperationalConfig returns true if content looks like infrastructure/operational
// config that belongs in CLAUDE.md files rather than Engram memories.
//
// A memory is classified as operational config when it matches 2+ patterns,
// reducing false positives from content that mentions a URL in passing.
func isOperationalConfig(content string) bool {
	if content == "" {
		return false
	}
	matches := 0
	for _, p := range operationalConfigPatterns {
		if p.MatchString(content) {
			matches++
			if matches >= 2 {
				return true
			}
		}
	}
	return false
}
