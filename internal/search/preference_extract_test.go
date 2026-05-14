package search

import (
	"context"
	"testing"
)

// TestPatternPreferenceExtractor_HappyPath verifies that a clear preference
// statement is detected and normalized.
func TestPatternPreferenceExtractor_HappyPath(t *testing.T) {
	ex := PatternPreferenceExtractor{}
	facts, err := ex.Extract(context.Background(), "I love jazz music.")
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(facts) == 0 {
		t.Error("Extract returned no facts for a clear preference statement, want at least 1")
	}
}

// TestPatternPreferenceExtractor_MultiPreference verifies that multiple preferences
// in a single content block are all extracted.
func TestPatternPreferenceExtractor_MultiPreference(t *testing.T) {
	ex := PatternPreferenceExtractor{}
	content := "I love jazz music. She hates spicy food. He is vegetarian."
	facts, err := ex.Extract(context.Background(), content)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(facts) < 2 {
		t.Errorf("Extract returned %d facts, want at least 2 for multi-preference content", len(facts))
	}
}

// TestPatternPreferenceExtractor_FalsePositive verifies that engineering prose
// containing preference keywords is NOT extracted as a preference.
func TestPatternPreferenceExtractor_FalsePositive(t *testing.T) {
	ex := PatternPreferenceExtractor{}
	content := "I love this PR, let's merge it."
	facts, err := ex.Extract(context.Background(), content)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("Extract returned %d facts for false-positive content, want 0; got: %v", len(facts), facts)
	}
}

// TestPatternPreferenceExtractor_EmptyContent verifies that empty content
// returns an empty slice and no error.
func TestPatternPreferenceExtractor_EmptyContent(t *testing.T) {
	ex := PatternPreferenceExtractor{}
	facts, err := ex.Extract(context.Background(), "")
	if err != nil {
		t.Fatalf("Extract returned error on empty content: %v", err)
	}
	if facts == nil {
		t.Error("Extract should return an empty slice (not nil) for empty content")
	}
	if len(facts) != 0 {
		t.Errorf("Extract returned %d facts for empty content, want 0", len(facts))
	}
}

// TestPatternPreferenceExtractor_NoPreference verifies that neutral content
// with no preference signals returns an empty slice.
func TestPatternPreferenceExtractor_NoPreference(t *testing.T) {
	ex := PatternPreferenceExtractor{}
	content := "The system deployed successfully at 14:00 UTC. All health checks passed."
	facts, err := ex.Extract(context.Background(), content)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("Extract returned %d facts for neutral content, want 0; got: %v", len(facts), facts)
	}
}
