package chunk

import (
	"reflect"
	"testing"
)

func TestChunkText_OverlapCarriesForward(t *testing.T) {
	sentences := []string{
		"Alpha one.",
		"Bravo two.",
		"Charlie three.",
		"Delta four.",
	}

	chunks := ChunkText(joinSentences(sentences...), 7, 4)
	if len(chunks) != 3 {
		t.Fatalf("ChunkText() chunks = %d, want 3; chunks=%q", len(chunks), chunks)
	}
	if got, want := chunks[0], joinSentences(sentences[0], sentences[1]); got != want {
		t.Fatalf("chunk[0] = %q, want %q", got, want)
	}
	if got, want := chunks[1], joinSentences(sentences[1], sentences[2]); got != want {
		t.Fatalf("chunk[1] = %q, want %q", got, want)
	}
	if got, want := chunks[2], joinSentences(sentences[2], sentences[3]); got != want {
		t.Fatalf("chunk[2] = %q, want %q", got, want)
	}
}

func TestChunkText_ZeroOverlapUnchanged(t *testing.T) {
	sentences := []string{
		"Alpha one.",
		"Bravo two.",
		"Charlie three.",
		"Delta four.",
	}

	got := ChunkText(joinSentences(sentences...), 7, 0)
	want := []string{
		joinSentences(sentences[0], sentences[1]),
		joinSentences(sentences[2], sentences[3]),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ChunkText(..., overlap=0) = %#v, want %#v", got, want)
	}
}

func TestSplitSentences_ProtectsAbbreviations(t *testing.T) {
	text := "Dr. Smith arrived. Mr. Jones stayed in the U.S. for work. Final sentence."

	got := splitSentences(text)
	want := []string{
		"Dr. Smith arrived.",
		"Mr. Jones stayed in the U.S. for work.",
		"Final sentence.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitSentences() = %#v, want %#v", got, want)
	}
}

func joinSentences(sentences ...string) string {
	if len(sentences) == 0 {
		return ""
	}
	out := sentences[0]
	for _, sentence := range sentences[1:] {
		out += " " + sentence
	}
	return out
}
