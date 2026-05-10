package search

import (
	"strings"
	"unicode"
)

// preferenceContentSignals are high-confidence words for auto-tagging stored
// memories as memory_type="preference". Kept narrow to avoid false positives
// in engineering prose ("I'd like to", "we want to") — see issue #369.
var preferenceContentSignals = []string{
	"prefer", "prefers", "preferred",
	"favorite", "favourite",
	"enjoy", "enjoys", "enjoyed",
}

// preferenceQuerySignals extends the content set with common personal-preference
// expressions that appear in recall queries about a person's likes and interests.
// These are safe to use for query-time boosting because the cost of a false
// positive (boosting preference memories slightly) is much lower than the cost
// of a false negative (failing to surface the correct preference memory).
var preferenceQuerySignals = append(
	append([]string{}, preferenceContentSignals...),
	"like", "likes", "liked",
	"love", "loves", "loved",
	"hate", "hates", "hated",
	"dislike", "dislikes", "disliked",
	"interested",
)

// preferenceContentSet and preferenceQuerySet provide O(1) lookup after tokenisation.
var preferenceContentSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(preferenceContentSignals))
	for _, s := range preferenceContentSignals {
		m[s] = struct{}{}
	}
	return m
}()

var preferenceQuerySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(preferenceQuerySignals))
	for _, s := range preferenceQuerySignals {
		m[s] = struct{}{}
	}
	return m
}()

// wordBoundary returns true for runes that delimit word tokens.
func wordBoundary(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func containsSignalFrom(text string, set map[string]struct{}) bool {
	for _, word := range strings.FieldsFunc(strings.ToLower(text), wordBoundary) {
		if _, ok := set[word]; ok {
			return true
		}
	}
	return false
}

// isPreferenceQuery returns true when the query contains preference-signal words,
// indicating the user is asking about stored preferences. Uses the expanded
// query signal set which includes "like/love/hate" — safe for query-time boosting
// where false positives have low cost (#364, expanded from #369 narrow set).
func isPreferenceQuery(query string) bool {
	return containsSignalFrom(query, preferenceQuerySet)
}

// IsPreferenceContent returns true when content text expresses a preference
// (e.g., "I really prefer X", "My favorite is Y"). Uses the narrow content
// signal set to avoid false positives when auto-tagging stored memories (#364, #369).
func IsPreferenceContent(content string) bool {
	return containsSignalFrom(content, preferenceContentSet)
}
