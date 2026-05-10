package search

import (
	"testing"
)

// TestIsPreferenceQueryDetectsSignals verifies that queries containing known
// preference-signal words are detected correctly. Only high-signal words that
// rarely appear in engineering prose are retained (#369).
func TestIsPreferenceQueryDetectsSignals(t *testing.T) {
	hits := []string{
		"what does the user prefer for coffee?",
		"what is the user's favorite music?",
		"what kind of books does the user enjoy?",
		"WHAT DOES THE USER PREFER?", // case-insensitive
	}
	for _, q := range hits {
		if !isPreferenceQuery(q) {
			t.Errorf("isPreferenceQuery(%q) = false, want true", q)
		}
	}
}

// TestIsPreferenceQueryIgnoresNeutralQueries verifies that neutral recall
// queries are not classified as preference queries.
func TestIsPreferenceQueryIgnoresNeutralQueries(t *testing.T) {
	misses := []string{
		"what is the project deadline?",
		"when was the last meeting?",
		"summarize the architecture decisions",
		"find all error patterns",
	}
	for _, q := range misses {
		if isPreferenceQuery(q) {
			t.Errorf("isPreferenceQuery(%q) = true, want false", q)
		}
	}
}

// TestIsPreferenceQueryExpandedSignals verifies that expanded personal-preference
// words ("like", "love", "hate", "dislike", "interested") fire for query boosting.
// These are distinct from content auto-tagging signals (#369 concern was about
// engineering prose in stored memories; queries for personal preferences are safe).
func TestIsPreferenceQueryExpandedSignals(t *testing.T) {
	hits := []string{
		"what does the user like to eat?",
		"what music does the user love?",
		"what food does the user hate?",
		"what does the user dislike about commuting?",
		"what is the user interested in for hobbies?",
	}
	for _, q := range hits {
		if !isPreferenceQuery(q) {
			t.Errorf("isPreferenceQuery(%q) = false, want true", q)
		}
	}
}

// TestIsPreferenceContentStillNarrow verifies that IsPreferenceContent (used for
// auto-tagging stored memories) does NOT fire on "like/love/want" to avoid
// false positives on engineering prose (#369).
func TestIsPreferenceContentStillNarrow(t *testing.T) {
	misses := []string{
		"I'd like to fix this bug",
		"we want to improve throughput",
		"would love feedback on this PR",
	}
	for _, c := range misses {
		if IsPreferenceContent(c) {
			t.Errorf("IsPreferenceContent(%q) = true, want false (should stay narrow)", c)
		}
	}
}

// TestIsTemporalQueryDetectsSignals verifies that time-anchored recall queries
// are classified as temporal so the engine can apply recency-boosted weights.
func TestIsTemporalQueryDetectsSignals(t *testing.T) {
	hits := []string{
		"when did the user start the new job?",
		"how many days ago did we discuss this?",
		"what happened before the meeting?",
		"what occurred after the incident?",
		"which event happened first?",
		"what did the user do recently?",
		"how long ago was the trip?",
		"what was the sequence of events?",
	}
	for _, q := range hits {
		if !isTemporalQuery(q) {
			t.Errorf("isTemporalQuery(%q) = false, want true", q)
		}
	}
}

func TestIsTemporalQueryIgnoresNeutralQueries(t *testing.T) {
	misses := []string{
		"what is the user's favorite food?",
		"summarize the project decisions",
		"what is the user's name?",
		"find memories tagged auth",
	}
	for _, q := range misses {
		if isTemporalQuery(q) {
			t.Errorf("isTemporalQuery(%q) = true, want false", q)
		}
	}
}

func TestTemporalWeightsBoostRecency(t *testing.T) {
	def := DefaultWeights()
	tmp := TemporalWeights()
	if tmp.Recency <= def.Recency {
		t.Errorf("TemporalWeights recency %.3f must exceed default %.3f", tmp.Recency, def.Recency)
	}
	// Weights must still sum to 1.0.
	sum := tmp.Vector + tmp.BM25 + tmp.Recency + tmp.Precision
	if sum < 0.999 || sum > 1.001 {
		t.Errorf("TemporalWeights do not sum to 1.0: got %.4f", sum)
	}
}

// TestPreferenceBoostApplied verifies that a preference-typed memory scores
// higher than an identical context memory when the query is preference-shaped.
func TestPreferenceBoostApplied(t *testing.T) {
	base := ScoreInput{
		Cosine:     0.8,
		BM25:       0.5,
		HoursSince: 1,
		Importance: 2,
	}
	w := DefaultWeights()

	withoutBoost := ScoreInput{MemoryType: "context", IsPreferenceQuery: true}
	withoutBoost.Cosine = base.Cosine
	withoutBoost.BM25 = base.BM25
	withoutBoost.HoursSince = base.HoursSince
	withoutBoost.Importance = base.Importance

	withBoost := ScoreInput{MemoryType: "preference", IsPreferenceQuery: true}
	withBoost.Cosine = base.Cosine
	withBoost.BM25 = base.BM25
	withBoost.HoursSince = base.HoursSince
	withBoost.Importance = base.Importance

	scoreWithout := CompositeScoreWithWeights(withoutBoost, w)
	scoreWith := CompositeScoreWithWeights(withBoost, w)

	if scoreWith <= scoreWithout {
		t.Errorf("preference boost not applied: preference score %.4f ≤ context score %.4f", scoreWith, scoreWithout)
	}
}

// TestPreferenceBoostNotAppliedForNeutralQuery verifies that preference-typed
// memories do NOT receive a boost when the query is not preference-shaped.
func TestPreferenceBoostNotAppliedForNeutralQuery(t *testing.T) {
	input := ScoreInput{
		Cosine:            0.8,
		BM25:              0.5,
		HoursSince:        1,
		Importance:        2,
		MemoryType:        "preference",
		IsPreferenceQuery: false, // neutral query
	}
	w := DefaultWeights()

	scorePreference := CompositeScoreWithWeights(input, w)

	input.MemoryType = "context"
	scoreContext := CompositeScoreWithWeights(input, w)

	if scorePreference != scoreContext {
		t.Errorf("preference boost should not apply for neutral queries: preference=%.4f context=%.4f",
			scorePreference, scoreContext)
	}
}

// TestPreferenceBoostMagnitude verifies the boost multiplier is exactly 1.35×.
func TestPreferenceBoostMagnitude(t *testing.T) {
	base := ScoreInput{
		Cosine: 0.8, BM25: 0.5, HoursSince: 1, Importance: 2,
		IsPreferenceQuery: true,
	}
	w := DefaultWeights()

	base.MemoryType = "context"
	scoreContext := CompositeScoreWithWeights(base, w)

	base.MemoryType = "preference"
	scorePreference := CompositeScoreWithWeights(base, w)

	if scoreContext == 0 {
		t.Skip("base score is zero — cannot verify multiplier")
	}
	ratio := scorePreference / scoreContext
	const want = 1.35
	const eps = 0.001
	if ratio < want-eps || ratio > want+eps {
		t.Errorf("preference boost ratio = %.4f, want %.4f ± %.4f", ratio, want, eps)
	}
}
