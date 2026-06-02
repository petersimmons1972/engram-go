package search

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Unit tests for extractTopicAnchorTokens (LME experiment #3 — H-TAB)
// ---------------------------------------------------------------------------

// TestExtractTopicAnchorTokens_DomainTokensPresent verifies that domain-specific
// noun tokens survive after stop-word stripping.
func TestExtractTopicAnchorTokens_DomainTokensPresent(t *testing.T) {
	cases := []struct {
		query      string
		wantTokens []string
	}{
		{
			// PreferenceSubjectAnchorQuery output for "recommend a conference about AI in healthcare"
			query:      "user preference conference AI healthcare like dislike use avoid",
			wantTokens: []string{"conference", "AI", "healthcare"},
		},
		{
			query:      "user preference books machine learning beginners like dislike",
			wantTokens: []string{"books", "machine", "learning"},
		},
		{
			query:      "user preference restaurants like dislike use avoid",
			wantTokens: []string{"restaurants"},
		},
		{
			query:      "user preference coffee like dislike use avoid",
			wantTokens: []string{"coffee"},
		},
	}
	for _, c := range cases {
		tokens := extractTopicAnchorTokens(c.query)
		for _, want := range c.wantTokens {
			found := false
			for _, tok := range tokens {
				if strings.EqualFold(tok, want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("extractTopicAnchorTokens(%q) = %v, missing token %q", c.query, tokens, want)
			}
		}
	}
}

// TestExtractTopicAnchorTokens_NonPreferenceQueryNoPanic verifies no panic on
// arbitrary or degenerate inputs.
func TestExtractTopicAnchorTokens_NonPreferenceQueryNoPanic(t *testing.T) {
	queries := []string{
		"",
		"recent software architecture decisions",
		"user preference like dislike use avoid",
	}
	for _, q := range queries {
		tokens := extractTopicAnchorTokens(q) // must not panic
		_ = tokens
	}
}

// ---------------------------------------------------------------------------
// Unit tests for contentContainsTopicAnchor
// ---------------------------------------------------------------------------

// TestContentContainsTopicAnchor_HitOnTopicContent verifies that content
// mentioning a domain topic is detected.
func TestContentContainsTopicAnchor_HitOnTopicContent(t *testing.T) {
	tokens := []string{"coffee", "espresso"}
	content := "user: I really love coffee in the morning. The espresso at that new place was excellent."
	if !contentContainsTopicAnchor(content, tokens) {
		t.Errorf("contentContainsTopicAnchor(%q, %v) = false, want true", content, tokens)
	}
}

// TestContentContainsTopicAnchor_MissOnOffTopicContent verifies off-topic content
// does not match.
func TestContentContainsTopicAnchor_MissOnOffTopicContent(t *testing.T) {
	tokens := []string{"coffee", "espresso"}
	content := "user: I prefer hiking on weekends. Mountains are great."
	if contentContainsTopicAnchor(content, tokens) {
		t.Errorf("contentContainsTopicAnchor(%q, %v) = true, want false (off-topic)", content, tokens)
	}
}

// TestContentContainsTopicAnchor_CaseInsensitive verifies case-insensitive matching.
func TestContentContainsTopicAnchor_CaseInsensitive(t *testing.T) {
	tokens := []string{"coffee"}
	if !contentContainsTopicAnchor("I Love Coffee every morning", tokens) {
		t.Error("contentContainsTopicAnchor case-insensitive: got false, want true")
	}
}

// TestContentContainsTopicAnchor_EmptyTokensReturnsFalse verifies that empty
// token list returns false.
func TestContentContainsTopicAnchor_EmptyTokensReturnsFalse(t *testing.T) {
	if contentContainsTopicAnchor("some content about coffee", nil) {
		t.Error("nil tokens: got true, want false")
	}
	if contentContainsTopicAnchor("some content about coffee", []string{}) {
		t.Error("empty tokens: got true, want false")
	}
}

// TestContentContainsTopicAnchor_EmptyContentReturnsFalse verifies that empty
// content always returns false.
func TestContentContainsTopicAnchor_EmptyContentReturnsFalse(t *testing.T) {
	if contentContainsTopicAnchor("", []string{"coffee"}) {
		t.Error("empty content: got true, want false")
	}
}

// ---------------------------------------------------------------------------
// Unit tests for TopicAnchorMatch scoring integration
// ---------------------------------------------------------------------------

// TestTopicAnchorBoostApplied verifies that a preference memory with
// TopicAnchorMatch=true scores strictly higher than the same memory without.
func TestTopicAnchorBoostApplied(t *testing.T) {
	w := DefaultWeights()
	base := ScoreInput{
		Cosine:            0.7,
		BM25:              0.5,
		HoursSince:        10,
		Importance:        2,
		MemoryType:        "preference",
		IsPreferenceQuery: true,
	}

	onTopic := base
	onTopic.TopicAnchorMatch = true

	offTopic := base
	offTopic.TopicAnchorMatch = false

	scoreOn := CompositeScoreWithWeights(onTopic, w)
	scoreOff := CompositeScoreWithWeights(offTopic, w)

	if scoreOn <= scoreOff {
		t.Errorf("TopicAnchorBoost: on-topic %.4f should exceed off-topic %.4f", scoreOn, scoreOff)
	}
}

// TestTopicAnchorBoostFlagOff verifies that TopicAnchorMatch=false is identical
// to zero value (flag-off = baseline unchanged).
func TestTopicAnchorBoostFlagOff(t *testing.T) {
	w := DefaultWeights()
	in1 := ScoreInput{
		Cosine: 0.7, BM25: 0.5, HoursSince: 10, Importance: 2,
		MemoryType: "preference", IsPreferenceQuery: true,
		TopicAnchorMatch: false,
	}
	in2 := ScoreInput{
		Cosine: 0.7, BM25: 0.5, HoursSince: 10, Importance: 2,
		MemoryType: "preference", IsPreferenceQuery: true,
		// TopicAnchorMatch zero value = false
	}
	if CompositeScoreWithWeights(in1, w) != CompositeScoreWithWeights(in2, w) {
		t.Error("TopicAnchorMatch=false must equal zero-value baseline")
	}
}

// TestTopicAnchorBoostOnlyForPreferenceMemories verifies the boost only fires
// on preference-typed memories, not context.
func TestTopicAnchorBoostOnlyForPreferenceMemories(t *testing.T) {
	w := DefaultWeights()
	base := ScoreInput{
		Cosine: 0.7, BM25: 0.5, HoursSince: 10, Importance: 2,
		IsPreferenceQuery: true, TopicAnchorMatch: true,
	}

	base.MemoryType = "context"
	scoreContext := CompositeScoreWithWeights(base, w)

	base.MemoryType = "preference"
	scorePref := CompositeScoreWithWeights(base, w)

	if scorePref <= scoreContext {
		t.Errorf("preference+anchor %.4f should exceed context %.4f", scorePref, scoreContext)
	}
}

// TestTopicAnchorBoostNonPreferenceQueryNoEffect verifies TopicAnchorMatch
// has no effect when IsPreferenceQuery=false.
func TestTopicAnchorBoostNonPreferenceQueryNoEffect(t *testing.T) {
	w := DefaultWeights()
	withMatch := ScoreInput{
		Cosine: 0.7, BM25: 0.5, HoursSince: 10, Importance: 2,
		MemoryType: "preference", IsPreferenceQuery: false,
		TopicAnchorMatch: true,
	}
	withoutMatch := withMatch
	withoutMatch.TopicAnchorMatch = false

	if CompositeScoreWithWeights(withMatch, w) != CompositeScoreWithWeights(withoutMatch, w) {
		t.Error("TopicAnchorMatch must have no effect when IsPreferenceQuery=false")
	}
}

// TestTopicAnchorBoostMagnitude verifies the boost is meaningful (≥1.1×) but
// not absurd (≤2.5×).
func TestTopicAnchorBoostMagnitude(t *testing.T) {
	w := DefaultWeights()
	base := ScoreInput{
		Cosine: 0.7, BM25: 0.5, HoursSince: 10, Importance: 2,
		MemoryType: "preference", IsPreferenceQuery: true,
	}

	base.TopicAnchorMatch = false
	scoreOff := CompositeScoreWithWeights(base, w)

	base.TopicAnchorMatch = true
	scoreOn := CompositeScoreWithWeights(base, w)

	if scoreOff == 0 {
		t.Skip("base score is zero — cannot verify multiplier magnitude")
	}
	ratio := scoreOn / scoreOff
	if ratio < 1.1 {
		t.Errorf("TopicAnchorBoost too small: ratio=%.3f, want ≥1.1 for meaningful separation", ratio)
	}
	if ratio > 2.5 {
		t.Errorf("TopicAnchorBoost too large: ratio=%.3f, want ≤2.5 to avoid distortion", ratio)
	}
}

// TestTopicAnchorBoostComposableWithDualPreference verifies that an on-topic
// candidate with lower cosine beats a high-cosine off-topic candidate.
// This is the core multi-preference-session distraction scenario.
func TestTopicAnchorBoostComposableWithDualPreference(t *testing.T) {
	w := DefaultWeights()
	// On-topic: lower cosine but mentions the right topic domain.
	onTopic := ScoreInput{
		Cosine: 0.6, BM25: 0.55, HoursSince: 5, Importance: 2,
		MemoryType: "preference", IsPreferenceQuery: true,
		TopicAnchorMatch: true,
	}
	// Off-topic: higher cosine (generic preference language) but wrong domain.
	offTopic := ScoreInput{
		Cosine: 0.8, BM25: 0.4, HoursSince: 5, Importance: 2,
		MemoryType: "preference", IsPreferenceQuery: true,
		TopicAnchorMatch: false,
	}

	scoreOn := CompositeScoreWithWeights(onTopic, w)
	scoreOff := CompositeScoreWithWeights(offTopic, w)

	if scoreOn <= scoreOff {
		t.Errorf("on-topic+anchor (%.4f) should beat off-topic-high-cosine (%.4f)", scoreOn, scoreOff)
	}
}

// TestRecallOptsTopicAnchorBoostDefaultOff verifies the zero value of
// RecallOpts has TopicAnchorBoost=false (default-off contract).
func TestRecallOptsTopicAnchorBoostDefaultOff(t *testing.T) {
	var opts RecallOpts
	if opts.TopicAnchorBoost {
		t.Error("RecallOpts.TopicAnchorBoost default should be false (off)")
	}
}
