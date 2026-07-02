package chunk_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/chunk"
)

// TestTurnBoundaryNeverSplitsMidTurn asserts that when a conversation is chunked
// with ChunkTextTurnBoundary, no chunk (after the first) starts in the middle of
// a role's content — every new chunk begins at a turn delimiter.
func TestTurnBoundaryNeverSplitsMidTurn(t *testing.T) {
	// 4 turns of ~125 chars each (~500 chars total).
	// maxTokens=50 → maxChars=200, so multiple chunks are required.
	var sb strings.Builder
	roles := []string{"user", "assistant"}
	for i := 0; i < 4; i++ {
		fmt.Fprintf(&sb, "%s: %s\n", roles[i%2], strings.Repeat("word ", 25))
	}
	text := strings.TrimRight(sb.String(), "\n")

	chunks := chunk.ChunkTextTurnBoundary(text, 50, 0)
	if len(chunks) < 2 {
		t.Fatalf("expected ≥2 chunks for %d-char text with maxTokens=50, got %d", len(text), len(chunks))
	}

	for i, c := range chunks {
		if i == 0 {
			continue // first chunk may start anywhere
		}
		if !strings.HasPrefix(c, "user: ") && !strings.HasPrefix(c, "assistant: ") {
			preview := c
			if len(preview) > 60 {
				preview = preview[:60]
			}
			t.Errorf("chunk %d starts mid-turn: %q", i, preview)
		}
	}
}

// TestTurnBoundaryChunkStartsAtTurnBeginning is a complementary assertion: every
// chunk beyond the first must begin with a recognised role prefix.
func TestTurnBoundaryChunkStartsAtTurnBeginning(t *testing.T) {
	// 6 turns, each ~240 chars; maxTokens=50 (200 chars) forces ≥3 chunks.
	var sb strings.Builder
	roles := []string{"user", "assistant"}
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&sb, "%s: %s\n", roles[i%2], strings.Repeat("content ", 30))
	}
	text := strings.TrimRight(sb.String(), "\n")

	chunks := chunk.ChunkTextTurnBoundary(text, 50, 0)
	if len(chunks) < 2 {
		t.Fatalf("expected ≥2 chunks, got %d", len(chunks))
	}

	for i, c := range chunks[1:] {
		if !strings.HasPrefix(c, "user: ") && !strings.HasPrefix(c, "assistant: ") {
			preview := c
			if len(preview) > 60 {
				preview = preview[:60]
			}
			t.Errorf("chunk %d does not start at turn boundary: %q", i+1, preview)
		}
	}
}

// TestTurnBoundaryFallsBackOnNoDelimiters verifies that text without role
// prefixes is handled gracefully (same chunk count as the regular ChunkText).
func TestTurnBoundaryFallsBackOnNoDelimiters(t *testing.T) {
	text := strings.Repeat("This is a sentence without any turn markers. ", 100)

	regular := chunk.ChunkText(text, 50, 0)
	tb := chunk.ChunkTextTurnBoundary(text, 50, 0)

	if len(tb) == 0 {
		t.Fatal("expected non-empty result for long input without delimiters")
	}
	// Chunk count should match the regular path since no boundaries were found.
	if len(tb) != len(regular) {
		t.Errorf("fallback: got %d chunks, regular ChunkText produced %d", len(tb), len(regular))
	}
}

// TestTurnBoundaryShortTextSingleChunk ensures short text (below maxChars) is
// returned as a single chunk without modification.
func TestTurnBoundaryShortTextSingleChunk(t *testing.T) {
	text := "user: hello\nassistant: hi there"
	chunks := chunk.ChunkTextTurnBoundary(text, 500, 0)
	if len(chunks) != 1 {
		t.Fatalf("short text should be a single chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("chunk content modified: got %q, want %q", chunks[0], text)
	}
}

// TestTurnBoundaryOverlapStartsAtTurnBoundary checks that when overlapTokens > 0,
// the overlap carried into the next chunk begins at a turn boundary (not mid-turn).
func TestTurnBoundaryOverlapStartsAtTurnBoundary(t *testing.T) {
	// 4 turns, each ~125 chars; overlap=25 tokens (100 chars).
	var sb strings.Builder
	roles := []string{"user", "assistant"}
	for i := 0; i < 4; i++ {
		fmt.Fprintf(&sb, "%s: %s\n", roles[i%2], strings.Repeat("overlap ", 15))
	}
	text := strings.TrimRight(sb.String(), "\n")

	chunks := chunk.ChunkTextTurnBoundary(text, 50, 25)
	if len(chunks) < 2 {
		t.Fatalf("expected ≥2 chunks with overlap, got %d", len(chunks))
	}

	for i, c := range chunks[1:] {
		if !strings.HasPrefix(c, "user: ") && !strings.HasPrefix(c, "assistant: ") {
			preview := c
			if len(preview) > 60 {
				preview = preview[:60]
			}
			t.Errorf("overlapping chunk %d does not start at turn boundary: %q", i+1, preview)
		}
	}
}
