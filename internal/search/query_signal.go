package search

import "strings"

// preferenceSignals are words that indicate preference-related text.
// All lowercase; matched case-insensitively.
var preferenceSignals = []string{"prefer", "like", "favorite", "enjoy", "want", "love"}

func containsPreferenceSignal(text string) bool {
	t := strings.ToLower(text)
	for _, sig := range preferenceSignals {
		if strings.Contains(t, sig) {
			return true
		}
	}
	return false
}

// isPreferenceQuery returns true when the query contains preference-signal words,
// indicating the user is asking about stored preferences. Used to apply a
// scoring boost to preference-typed memories (#364).
func isPreferenceQuery(query string) bool {
	return containsPreferenceSignal(query)
}

// IsPreferenceContent returns true when content text expresses a preference
// (e.g., "I really prefer X", "My favorite is Y"). Used to auto-tag memories
// stored without an explicit memory_type (#364).
func IsPreferenceContent(content string) bool {
	return containsPreferenceSignal(content)
}
