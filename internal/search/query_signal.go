package search

import (
	"strings"
	"unicode"
)

// preferenceSignals are words that indicate preference-related text.
// All lowercase; matched as whole words to avoid false positives from
// substrings (e.g. "like" in "likely", "want" in "unwanted").
var preferenceSignals = []string{"prefer", "prefers", "preferred", "like", "likes", "liked",
	"favorite", "favourite", "enjoy", "enjoys", "enjoyed", "want", "wants", "love", "loves"}

// preferenceSignalSet is the same list in a map for O(1) lookup after tokenisation.
var preferenceSignalSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(preferenceSignals))
	for _, s := range preferenceSignals {
		m[s] = struct{}{}
	}
	return m
}()

// wordBoundary returns true for runes that delimit word tokens.
func wordBoundary(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func containsPreferenceSignal(text string) bool {
	for _, word := range strings.FieldsFunc(strings.ToLower(text), wordBoundary) {
		if _, ok := preferenceSignalSet[word]; ok {
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
