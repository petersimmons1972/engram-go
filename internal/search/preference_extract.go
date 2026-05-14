package search

import (
	"context"
	"fmt"
	"strings"
)

// PreferenceExtractor extracts preference statements from raw text.
// The default PatternPreferenceExtractor uses keyword matching.
// Swap in an OpenAI-compatible implementation (e.g. pointing at the Olla routing layer
// in front of Ollama/vLLM) without changing callers — the same OllamaPreferenceExtractor
// works whether Olla routes to qwen2.5:32b, llama3.3:70b, or any future model.
type PreferenceExtractor interface {
	Extract(ctx context.Context, content string) ([]string, error)
}

// PatternPreferenceExtractor is the default, zero-cost implementation.
// It scans sentences in content for preference signals and returns short
// normalized facts: "User loves jazz music", "User is vegetarian".
// Returns an empty slice (not error) when no preferences are found.
type PatternPreferenceExtractor struct{}

// Extract scans content sentence by sentence, identifies sentences that express
// a personal preference, and returns them as short normalized facts.
// Multi-sentence content may yield multiple facts (one per preference sentence).
func (p PatternPreferenceExtractor) Extract(ctx context.Context, content string) ([]string, error) {
	if strings.TrimSpace(content) == "" {
		return []string{}, nil
	}

	// Split content into sentences on common delimiters.
	sentences := splitSentences(content)

	var facts []string
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}
		// Use IsPreferenceContent to check — it handles the false-positive guard
		// and all keyword/phrase signals in one call.
		if IsPreferenceContent(sentence) {
			fact := normalizeFact(sentence)
			facts = append(facts, fact)
		}
	}

	if facts == nil {
		return []string{}, nil
	}
	return facts, nil
}

// splitSentences splits text on sentence-ending punctuation. Simple heuristic:
// split on ". ", "! ", "? ", and newlines. Does not handle abbreviations.
func splitSentences(text string) []string {
	// Normalize newlines to spaces so multi-line content is handled uniformly.
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", ". ")

	// Insert a sentinel after sentence-terminal punctuation.
	// Use a rare Unicode character as a split marker.
	const sentinel = "\x00"
	text = strings.ReplaceAll(text, ". ", "."+sentinel)
	text = strings.ReplaceAll(text, "! ", "!"+sentinel)
	text = strings.ReplaceAll(text, "? ", "?"+sentinel)
	// Also split on lone "." at the end of the string.
	if strings.HasSuffix(text, ".") || strings.HasSuffix(text, "!") || strings.HasSuffix(text, "?") {
		text = text + sentinel
	}

	parts := strings.Split(text, sentinel)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// normalizeFact converts a raw preference sentence into a short normalized fact.
// Replaces common first-person subjects with "User" for consistency in storage.
// Example: "I love jazz music." → "User loves jazz music.".
func normalizeFact(sentence string) string {
	// Strip trailing punctuation.
	sentence = strings.TrimRight(sentence, ".!?")
	sentence = strings.TrimSpace(sentence)
	if sentence == "" {
		return sentence
	}

	lower := strings.ToLower(sentence)

	// Common first-person patterns → normalize to "User ..."
	replacements := []struct {
		from string
		to   string
	}{
		{"i am ", "User is "},
		{"i'm ", "User is "},
		{"i love ", "User loves "},
		{"i hate ", "User hates "},
		{"i dislike ", "User dislikes "},
		{"i like ", "User likes "},
		{"i adore ", "User adores "},
		{"i detest ", "User detests "},
		{"i prefer ", "User prefers "},
		{"i enjoy ", "User enjoys "},
		{"i avoid ", "User avoids "},
		{"i can't stand ", "User cannot stand "},
		{"i cannot stand ", "User cannot stand "},
		{"i can't eat ", "User cannot eat "},
		{"i cannot eat ", "User cannot eat "},
		{"i'm allergic ", "User is allergic "},
		{"i am allergic ", "User is allergic "},
	}
	for _, r := range replacements {
		if strings.HasPrefix(lower, r.from) {
			return fmt.Sprintf("User%s%s", r.to[4:], sentence[len(r.from):])
		}
	}

	// No match — return as-is with "User: " prefix so callers know the subject.
	return "User: " + sentence
}
