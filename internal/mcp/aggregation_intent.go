package mcp

import (
	"regexp"
	"strings"
)

// aggregationSynthesisDirective is the instruction attached to recall responses for
// counting/aggregation-intent queries. Like preferenceSynthesisDirective it is consumed
// CLIENT-SIDE — engram-go does not generate the answer. The wording is adapted from the
// eval-proven enumerate-first prompt plus the context-depth lesson from the 2026-06-21
// LongMemEval campaign: enumerate-first ALONE was net-neutral (34.1%); pairing it with a
// wider context window (context-topk 30) reached 42.2-43.0% (+7pp, replicated), because
// aggregation answers need MULTIPLE constituent sessions combined. The directive therefore
// folds the context-depth half in as a "review ALL returned memories" breadth instruction.
//
// FM-76: unlike the eval harness (which forces a guess to maximise LongMemEval's
// CORRECT-only score), the production directive carries a mandatory abstention clause —
// for real users, answering "insufficient" beats inventing a count.
const aggregationSynthesisDirective = "When answering this counting or aggregation question, do not answer from impression — build and operate over an explicit list from the returned memories. (1) Review ALL of the returned memories, not only the most relevant few: the items being counted may be spread across many separate memories and sessions. (2) List every candidate item, noting which memory it came from. (3) Drop any item that does not match the question's constraints (its time period, place, or category). (4) Merge duplicates so each distinct item is counted only once. (5) Total the remaining items explicitly and give that final count or sum as the answer. If the question needs a specific value (an amount, price, count, or date) that the returned memories do not state, say the information is insufficient rather than estimating or inventing it."

// aggregationCountRe matches explicit counting/aggregation phrasings. Precision-biased
// (FM-77): matched as a whole against the lower-cased query; a missed aggregation query is
// acceptable, a false fire on a factual/temporal/preference query is not.
var aggregationCountRe = regexp.MustCompile(
	`\b(how many(?: times)?|how often|total number of|sum of|count of|in total|altogether|number of (?:different|distinct))\b`)

// aggregationMonetaryRe catches "how much" questions that require summing/differencing
// values ("how much did I save/spend in total", "how much have I raised"). The verbs are
// kept narrow (FM-76 — a false fire is worse than a miss): bare "raise" is excluded
// (matches "how much can I raise my seat?"), and bare "total" is excluded (matches "how
// much does the total cost?"), so only the past-tense "raised" and the phrase "in total"
// qualify. A "how much does X cost" price lookup carries none of these and is NOT matched.
var aggregationMonetaryRe = regexp.MustCompile(
	`\bhow much\b.*\b(save|saved|spend|spent|raised|earn|earned|in total|altogether)\b`)

// aggregationTemporalExcludeRe excludes relative-time arithmetic ("how many days/weeks/
// hours ago/before/after"), which is temporal reasoning, not aggregation.
var aggregationTemporalExcludeRe = regexp.MustCompile(
	`\bhow many (?:days?|weeks?|months?|years?|hours?|minutes?) (?:ago|before|after)\b`)

// aggregationIntent reports whether the query is a counting/aggregation question, using
// only observable lexical features (never a question_type label; FM-77). Conservative by
// design: relative-time arithmetic is excluded so it does not steal temporal questions.
func aggregationIntent(query string) bool {
	q := strings.ToLower(query)
	if aggregationTemporalExcludeRe.MatchString(q) {
		return false
	}
	return aggregationCountRe.MatchString(q) || aggregationMonetaryRe.MatchString(q)
}
