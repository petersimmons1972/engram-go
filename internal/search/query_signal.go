package search

import (
	"strings"
	"unicode"
)

// preferenceContentFalsePositivePhrases are phrases that contain preference
// signal words but are NOT personal preferences — they are engineering prose,
// social niceties, or meta-commentary. When any of these phrases appears in the
// content (case-insensitive), IsPreferenceContent returns false regardless of
// which keyword matched. Guards against false positives on "I love this PR",
// "I'd like to fix", "I hate to say it" (#369 follow-up).
var preferenceContentFalsePositivePhrases = []string{
	"i'd like to",
	"i would like",
	"i love this approach",
	"i love this pr",
	"i love this idea",
	"i love this solution",
	"would love to",
	"would love feedback",
	"i hate to say",
	"i hate to admit",
}

// preferenceContentSignals are high-confidence words for auto-tagging stored
// memories as memory_type="preference". Expanded from the original narrow set
// to cover common natural-language preference expressions while preserving the
// false-positive guard for engineering prose (#369).
var preferenceContentSignals = []string{
	"prefer", "prefers", "preferred",
	"favorite", "favourite",
	"enjoy", "enjoys", "enjoyed",
	// Expanded set — strong personal preference expressions
	"like", "love", "loves", "loved",
	"hate", "hates", "hated",
	"dislike", "dislikes", "disliked",
	"adore", "adores", "adored",
	"detest", "detests", "detested",
	"allergic",
	"vegetarian",
	"vegan",
	"avoid", "avoids",
}

// preferenceContentPhraseSignals are multi-word signals that require phrase
// matching rather than word-boundary tokenization. Checked separately.
var preferenceContentPhraseSignals = []string{
	"can't stand",
	"cannot stand",
	"can't eat",
	"cannot eat",
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

// IsPreferenceContent returns true when content text expresses a personal preference
// (e.g., "I love jazz", "She is vegetarian", "I can't stand opera").
// Uses the expanded content signal set with a false-positive guard that rejects
// engineering prose and social niceties ("I love this PR", "I'd like to", etc.) (#364, #369).
func IsPreferenceContent(content string) bool {
	lower := strings.ToLower(content)

	// False-positive guard: if the content matches any engineering-prose pattern,
	// reject immediately regardless of which keyword triggered.
	for _, phrase := range preferenceContentFalsePositivePhrases {
		if strings.Contains(lower, phrase) {
			return false
		}
	}

	// Keyword match via word-boundary tokenization.
	if containsSignalFrom(content, preferenceContentSet) {
		return true
	}

	// Phrase match for multi-word signals.
	for _, phrase := range preferenceContentPhraseSignals {
		if strings.Contains(lower, phrase) {
			return true
		}
	}

	return false
}

// temporalQuerySignals are words that indicate the recall query is anchored to
// time — asking when something happened, how long ago, or in what order.
// Detecting these lets the engine apply recency-boosted weights so that
// chronologically ordered memories surface ahead of semantically similar ones.
var temporalQuerySignals = map[string]struct{}{
	"when": {}, "ago": {}, "before": {}, "after": {},
	"first": {}, "last": {}, "recent": {}, "recently": {},
	"long": {}, "sequence": {}, "order": {}, "earliest": {},
	"latest": {}, "previous": {}, "prior": {}, "since": {},
}

// isTemporalQuery returns true when the query asks about timing, order, or
// recency. Used by the engine to select recency-boosted scoring weights.
func isTemporalQuery(query string) bool {
	return containsSignalFrom(query, temporalQuerySignals)
}

// IsTemporalQuery is the exported form of isTemporalQuery.
func IsTemporalQuery(query string) bool { return isTemporalQuery(query) }

// knowledgeUpdateQuerySignals detect queries asking about the current or changed
// state of a mutable fact ("where does X live currently?", "does X still work there?").
// Conservative set — only words that rarely appear outside KU-shaped questions.
var knowledgeUpdateQuerySignals = map[string]struct{}{
	"currently": {}, "anymore": {}, "still": {}, "current": {},
}

// isKnowledgeUpdateQuery returns true when the query asks about present or
// changed state, suggesting memories may contain superseded facts that should
// rank lower than the most recent version.
func isKnowledgeUpdateQuery(query string) bool {
	return containsSignalFrom(query, knowledgeUpdateQuerySignals)
}

// IsKnowledgeUpdateQuery is the exported form of isKnowledgeUpdateQuery.
func IsKnowledgeUpdateQuery(query string) bool { return isKnowledgeUpdateQuery(query) }
