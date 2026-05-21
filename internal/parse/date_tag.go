// Package parse holds string-parsing utilities that operate on values
// embedded in higher-level data structures (e.g., memory tags). These are
// operational helpers, not wire-compatible type definitions — keeping them
// out of internal/types preserves that package's role as the source of
// truth for core data shapes.
package parse

import (
	"strings"
	"time"
)

// ParseDateTag scans tags for a "date:<value>" entry and returns the parsed
// UTC time (date portion only), or nil if no valid date tag is found.
// The first matching tag wins. Supported formats:
//
//   - "2006-01-02"                — ISO date
//   - "2006/01/02 (Mon) 15:04"   — LongMemEval haystack date format
//
// This function is shared by the MCP store handlers and the DB layer
// (e.g., UpdateMemory tag recalculation) without a circular import.
func ParseDateTag(tags []string) *time.Time {
	layouts := []string{"2006-01-02", "2006/01/02 (Mon) 15:04"}
	for _, tag := range tags {
		val, ok := strings.CutPrefix(tag, "date:")
		if !ok {
			continue
		}
		for _, layout := range layouts {
			t, err := time.Parse(layout, val)
			if err != nil {
				continue
			}
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			return &t
		}
	}
	return nil
}
