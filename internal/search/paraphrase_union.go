package search

import (
	"strings"
)

// QueryParaphraser generates alternative phrasings of a recall query.
// Implementations are called server-side during RecallWithOpts when
// RecallOpts.ParaphraseUnion is true.  The paraphrases are used to issue
// additional recall passes whose candidate sets are merged (unioned) before
// final ranking, targeting the missing_recall failure class: gold sessions
// that contain the relevant fact only as an incidental off-topic mention,
// making BM25 miss them on the primary query phrasing.
//
// Implementations must be goroutine-safe; the same instance may be shared
// across concurrent recall calls.
type QueryParaphraser interface {
	// Paraphrase returns up to n alternative phrasings of query.
	// Returning fewer than n is acceptable (e.g. when the query is already
	// maximally atomic).  Returning zero paraphrases is safe — the caller
	// skips the extra passes.  The original query must NOT be included in
	// the returned slice (the engine always issues the primary pass first).
	Paraphrase(query string, n int) []string
}

// RuleBasedParaphraser generates paraphrases deterministically using
// linguistic transformation rules.  It requires no external LLM call,
// adds negligible latency (<1 µs), and is safe to use under high concurrency.
//
// Rules applied (in order, up to n results):
//  1. Strip wh-question prefix ("what is X" → "X", "who is X" → "X").
//  2. Negate with "not" for near-antonym coverage ("X" → "X not").
//  3. Expand known contractions ("what's" → "what is", "who's" → "who is", etc.).
//  4. Drop leading auxiliary verbs ("did X" → "X", "does X" → "X").
//  5. Paraphrase "tell me about X" → "X".
//  6. Add "information about" wrapper ("X" → "information about X").
//
// These rules are designed to surface memories that mention the subject
// incidentally (e.g. a session that mentions X's address while discussing
// something else) by widening the lexical surface covered by the BM25 pass.
type RuleBasedParaphraser struct{}

// whPrefixes are wh-question openers that can be stripped to expose the
// entity/topic the question is about.
var whPrefixes = []string{
	"what is ", "what are ", "what was ", "what were ",
	"what's ", "what're ",
	"who is ", "who are ", "who was ", "who were ",
	"who's ",
	"where is ", "where are ", "where was ", "where were ",
	"where's ",
	"when is ", "when was ",
	"which is ", "which are ",
	"how is ", "how was ",
	"how's ",
	"tell me about ",
	"tell me what ",
	"do you know ",
	"do you remember ",
	"can you tell me about ",
	"what do you know about ",
	"what do you remember about ",
}

// auxiliaryPrefixes are auxiliary-verb openers that can be stripped.
var auxiliaryPrefixes = []string{
	"did ", "does ", "do ", "has ", "have ", "had ",
	"is ", "are ", "was ", "were ",
	"can ", "could ", "would ", "should ",
}

// contractionExpansions maps common English contractions to their expansions.
// Applied before stripping prefixes so "what's" becomes "what is" and then
// the "what is " prefix rule fires.
var contractionExpansions = map[string]string{
	"what's":   "what is",
	"who's":    "who is",
	"where's":  "where is",
	"how's":    "how is",
	"when's":   "when is",
	"that's":   "that is",
	"there's":  "there is",
	"it's":     "it is",
	"he's":     "he is",
	"she's":    "she is",
	"they're":  "they are",
	"we're":    "we are",
	"you're":   "you are",
	"i'm":      "i am",
	"isn't":    "is not",
	"aren't":   "are not",
	"wasn't":   "was not",
	"weren't":  "were not",
	"don't":    "do not",
	"doesn't":  "does not",
	"didn't":   "did not",
	"won't":    "will not",
	"can't":    "cannot",
	"couldn't": "could not",
}

// Paraphrase implements QueryParaphraser using the rule set described on
// RuleBasedParaphraser.  It is deterministic and allocation-light.
func (RuleBasedParaphraser) Paraphrase(query string, n int) []string {
	if n <= 0 || strings.TrimSpace(query) == "" {
		return nil
	}

	seen := make(map[string]struct{}, n+4)
	original := strings.TrimSpace(strings.ToLower(query))
	seen[original] = struct{}{}

	add := func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		s = strings.ToLower(s)
		if _, dup := seen[s]; dup {
			return false
		}
		seen[s] = struct{}{}
		return true
	}

	var out []string

	// Rule 3: expand contractions on the original query.
	expanded := expandContractions(original)
	if add(expanded) {
		out = append(out, expanded)
		if len(out) >= n {
			return out
		}
	}

	// Rule 1: strip wh-question prefix.
	bare := stripWHPrefix(original)
	if add(bare) {
		out = append(out, bare)
		if len(out) >= n {
			return out
		}
	}

	// Also try stripping prefix from the contraction-expanded form.
	if expanded != original {
		bareExpanded := stripWHPrefix(expanded)
		if add(bareExpanded) {
			out = append(out, bareExpanded)
			if len(out) >= n {
				return out
			}
		}
	}

	// Rule 4: strip leading auxiliary verb (applied to bare form).
	stripped := stripAuxiliaryPrefix(bare)
	if add(stripped) {
		out = append(out, stripped)
		if len(out) >= n {
			return out
		}
	}

	// Rule 6: "information about X" wrapper (applied to bare / stripped form).
	subject := stripped
	if subject == original {
		subject = bare
	}
	infoAbout := "information about " + subject
	if add(infoAbout) {
		out = append(out, infoAbout)
		if len(out) >= n {
			return out
		}
	}

	// Rule 2: append "not" for near-antonym coverage — only when bare is short
	// enough that adding "not" doesn't create noise (max 60 chars).
	if len(bare) <= 60 {
		negated := bare + " not"
		if add(negated) {
			out = append(out, negated)
			if len(out) >= n {
				return out
			}
		}
	}

	return out
}

// expandContractions replaces all contraction tokens in s with their
// long-form equivalents.  Token boundaries are whitespace.
func expandContractions(s string) string {
	words := strings.Fields(s)
	changed := false
	for i, w := range words {
		if exp, ok := contractionExpansions[w]; ok {
			words[i] = exp
			changed = true
		}
	}
	if !changed {
		return s
	}
	return strings.Join(words, " ")
}

// stripWHPrefix removes the first matching wh-question or meta-query prefix
// from s and returns the remainder.  Returns s unchanged if no prefix matches.
func stripWHPrefix(s string) string {
	for _, p := range whPrefixes {
		if strings.HasPrefix(s, p) {
			remainder := strings.TrimSpace(s[len(p):])
			if remainder != "" {
				return remainder
			}
		}
	}
	return s
}

// stripAuxiliaryPrefix removes the first matching auxiliary verb from s
// and returns the remainder.  Returns s unchanged if no prefix matches.
func stripAuxiliaryPrefix(s string) string {
	for _, p := range auxiliaryPrefixes {
		if strings.HasPrefix(s, p) {
			remainder := strings.TrimSpace(s[len(p):])
			if remainder != "" {
				return remainder
			}
		}
	}
	return s
}

// unionCandidates merges two slices of candidate memory IDs, preserving
// insertion order from setA and appending IDs from setB that are not already
// in setA.  The result contains each ID at most once.
//
// This is used by RecallWithOpts (ParaphraseUnion path) to combine the primary
// candidate set with candidates surfaced by paraphrase-variant passes before
// the composite scoring step.
func unionCandidates(setA, setB []string) []string {
	if len(setB) == 0 {
		return setA
	}
	seen := make(map[string]struct{}, len(setA)+len(setB))
	for _, id := range setA {
		seen[id] = struct{}{}
	}
	out := make([]string, len(setA), len(setA)+len(setB))
	copy(out, setA)
	for _, id := range setB {
		if _, dup := seen[id]; !dup {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
