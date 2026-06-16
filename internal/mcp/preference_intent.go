package mcp

import "strings"

// preferenceSynthesisDirective is the instruction attached to recall responses
// for preference-intent queries. It is consumed CLIENT-SIDE: the calling agent's
// model reads it from the tool response and synthesises the answer. engram-go does
// not generate the answer itself (it has no API key and the subscription CLI is not
// reachable from the server). The wording is adapted from the eval-proven
// GenerationPromptPreferenceEnumerate (commit c448829), which lifted single-session
// preference strict-correct accuracy 16.7% -> 26.7% on LongMemEval.
//
// FM-76: the final sentence is a mandatory abstention clause — the directive must
// not manufacture a confident answer when the memories do not actually express a
// relevant preference.
const preferenceSynthesisDirective = "When answering this preference question, identify and list every specific named item, brand, model, attribute, feature, or location the user expressed a preference about in the returned memories. Name each one explicitly rather than summarising (e.g. the specific product or brand, not \"functional accessories\"). Begin your answer with \"The user prefers:\" followed by the enumerated list, and include what they would NOT prefer if the memories support it. Only include preferences actually present in the memories. If the memories do not clearly express a preference relevant to the question, say so rather than guessing."

// preferenceMarkers are strong, observable lexical cues that a query is asking about
// the user's preferences. The detector is intentionally PRECISION-biased (FM-77 +
// FM-76): a missed preference query is acceptable, a false fire on a non-preference
// query is not. Markers are matched against the lower-cased query as substrings, so
// each must be specific enough that it does not appear incidentally in factual,
// temporal, or recall-style questions. Bare "like" is deliberately excluded.
var preferenceMarkers = []string{
	"prefer",     // prefer, preference, preferred, do I prefer
	"favorite",   // favorite
	"favourite",  // favourite (en-GB)
	"like best",  // what ... do I like best
	"likes best", // what does the user like best
	"do i like",  // what ... do I like
	"do i enjoy", // what ... do I enjoy
	"fond of",    // what am I fond of
	"my taste",   // what suits my taste
	"my go-to",   // what is my go-to ...
	"i tend to like",
}

// preferenceIntent reports whether the query is asking about the user's preferences,
// using only observable lexical features of the query itself — never a question_type
// label (which does not exist for real production traffic; FM-77). It is conservative
// by design: it returns true only on explicit preference cues.
func preferenceIntent(query string) bool {
	q := strings.ToLower(query)
	for _, m := range preferenceMarkers {
		if strings.Contains(q, m) {
			return true
		}
	}
	return false
}

// attachSynthesisDirective adds the preference synthesis directive to a recall
// response map when (and only when) the query shows preference intent. Additive and
// optional — it never alters ranking, recall, or any existing response field, mirroring
// the existing fetch_hint / feedback_hint pattern.
func attachSynthesisDirective(out map[string]any, query string) {
	if preferenceIntent(query) {
		out["synthesis_directive"] = preferenceSynthesisDirective
	}
}
