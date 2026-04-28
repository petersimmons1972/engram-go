package embed_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/embed"
)

// TestModelMaxTokensKnownModels verifies that the curated models have correct
// max-token values — mxbai-embed-large and nomic-embed-text cap at 512 tokens.
func TestModelMaxTokensKnownModels(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		{"mxbai-embed-large", 512},
		{"nomic-embed-text", 512},
		{"bge-m3", 512},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			got := embed.ModelMaxTokens(tc.model)
			if got != tc.want {
				t.Errorf("ModelMaxTokens(%q) = %d, want %d", tc.model, got, tc.want)
			}
		})
	}
}

// TestModelMaxTokensUnknownModelFallback verifies that an unknown model name
// returns the safe default (512) rather than panicking or returning zero.
func TestModelMaxTokensUnknownModelFallback(t *testing.T) {
	got := embed.ModelMaxTokens("some-future-model")
	if got <= 0 {
		t.Errorf("ModelMaxTokens(unknown) = %d, want > 0", got)
	}
}

// TestSuggestedModelsHaveMaxTokens verifies every entry in SuggestedModels
// has a non-zero MaxTokens field — guards against missing entries in new models.
func TestSuggestedModelsHaveMaxTokens(t *testing.T) {
	for _, m := range embed.SuggestedModels {
		if m.MaxTokens <= 0 {
			t.Errorf("ModelSpec %q has zero MaxTokens — embedding will silently truncate incorrectly", m.Name)
		}
	}
}

// TestTruncateToModelWindowNoopWhenShort verifies that text within the model
// window is returned unchanged.
func TestTruncateToModelWindowNoopWhenShort(t *testing.T) {
	short := "hello world"
	got := embed.TruncateToModelWindow(short, 512)
	if got != short {
		t.Errorf("TruncateToModelWindow(short text) mutated: got %q, want %q", got, short)
	}
}

// TestTruncateToModelWindowTruncatesAtSentenceBoundary verifies that text
// exceeding the window is cut at a sentence boundary (period), not mid-character.
func TestTruncateToModelWindowTruncatesAtSentenceBoundary(t *testing.T) {
	// Build text that exceeds 512*4=2048 chars. Insert a period at char 1800.
	prefix := string(make([]byte, 1799)) // 1799 'x' chars
	for i := range prefix {
		_ = i // can't range-assign; build differently
	}
	prefix = repeatByte('x', 1800) + ". " + repeatByte('y', 500)
	got := embed.TruncateToModelWindow(prefix, 512)
	// Result must end at the sentence boundary '.' not inside the 'y' run.
	if len(got) == len(prefix) {
		t.Error("expected truncation, got full string")
	}
	// Last non-whitespace char should be a period (sentence boundary).
	trimmed := got
	for len(trimmed) > 0 && (trimmed[len(trimmed)-1] == ' ' || trimmed[len(trimmed)-1] == '\n') {
		trimmed = trimmed[:len(trimmed)-1]
	}
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] != '.' {
		t.Errorf("expected truncation at sentence boundary '.', got last byte %q", string(trimmed[len(trimmed)-1]))
	}
}

// TestTruncateToModelWindowHardCutWhenNoBoundary verifies that text with no
// sentence boundary is still truncated (hard cut) rather than returned in full.
func TestTruncateToModelWindowHardCutWhenNoBoundary(t *testing.T) {
	long := repeatByte('a', 3000)
	got := embed.TruncateToModelWindow(long, 512)
	if len(got) >= len(long) {
		t.Errorf("expected truncation for %d-char text, got len %d", len(long), len(got))
	}
}

func repeatByte(b byte, n int) string {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = b
	}
	return string(buf)
}
