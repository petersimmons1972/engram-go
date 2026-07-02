package chunk

import "strings"

// turnRolePrefixes lists the lowercase role prefixes produced by
// longmemeval.SessionContent — see internal/longmemeval/engram.go.
var turnRolePrefixes = []string{"user: ", "assistant: "}

// isTurnBoundary reports whether line begins a new conversation turn.
func isTurnBoundary(line string) bool {
	for _, p := range turnRolePrefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// parseTurns splits session text (produced by SessionContent) into individual
// turns. Each turn is a contiguous block starting with a role prefix; lines
// without a recognised prefix are treated as continuations of the current turn.
// Pre-role content (e.g. "Session date: …") is returned as its own segment.
// Empty lines are dropped.
func parseTurns(text string) []string {
	lines := strings.Split(text, "\n")
	var turns []string
	var cur strings.Builder

	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			turns = append(turns, cur.String())
		}
		cur.Reset()
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if isTurnBoundary(line) {
			flush()
			cur.WriteString(line)
		} else {
			// Continuation of current turn (or pre-role segment).
			if cur.Len() > 0 {
				cur.WriteByte('\n')
			}
			cur.WriteString(line)
		}
	}
	flush()
	return turns
}

// ChunkTextTurnBoundary is a turn-aware replacement for ChunkText when the
// input text contains role-prefixed conversation turns (SessionContent format).
//
// Guarantee: every chunk after the first begins at a turn boundary (a line
// starting with "user: " or "assistant: "). A single oversized turn is
// sub-chunked using the regular ChunkText so we never exceed maxChars.
//
// If no turn delimiters are found the function falls back to ChunkText,
// preserving existing behaviour for plain-text content.
func ChunkTextTurnBoundary(text string, maxTokens, overlapTokens int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	const charsPerToken = 4
	maxChars := maxTokens * charsPerToken
	overlapChars := overlapTokens * charsPerToken

	// Short enough to fit in one chunk — return as-is.
	if len(text) <= maxChars {
		return []string{text}
	}

	turns := parseTurns(text)
	// Fallback: no recognised turn delimiters.
	if len(turns) == 0 || !isTurnBoundary(turns[0]) && len(turns) == 1 {
		return ChunkText(text, maxTokens, overlapTokens)
	}
	// If parseTurns found only pre-role content (no role prefixes at all),
	// none of the turns will be turn-boundaries — treat as fallback.
	hasBoundary := false
	for _, t := range turns {
		if isTurnBoundary(t) {
			hasBoundary = true
			break
		}
	}
	if !hasBoundary {
		return ChunkText(text, maxTokens, overlapTokens)
	}

	var chunks []string
	var current []string // turns packed into the current chunk
	currentLen := 0

	emitChunk := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, strings.Join(current, "\n"))

		// Build overlap from the tail of the emitted chunk.
		// Overlap is retained whole turns only, starting from the last turn.
		if overlapChars > 0 {
			var overlap []string
			overlapLen := 0
			for i := len(current) - 1; i >= 0; i-- {
				t := current[i]
				sep := 1
				if len(overlap) == 0 {
					sep = 0
				}
				if overlapLen+len(t)+sep > overlapChars {
					break
				}
				overlap = append([]string{t}, overlap...)
				overlapLen += len(t) + sep
			}
			current = overlap
			currentLen = overlapLen
		} else {
			current = nil
			currentLen = 0
		}
	}

	for _, turn := range turns {
		sep := 0
		if len(current) > 0 {
			sep = 1 // "\n" separator
		}

		// Turn is larger than the entire window: sub-chunk it.
		if len(turn) > maxChars {
			emitChunk()
			subChunks := ChunkText(turn, maxTokens, overlapTokens)
			chunks = append(chunks, subChunks...)
			// Reset; no overlap carried from sub-chunked turns to keep
			// the boundary guarantee intact.
			current = nil
			currentLen = 0
			continue
		}

		if currentLen+len(turn)+sep > maxChars && len(current) > 0 {
			emitChunk()
			// Recompute sep after overlap reuse.
			sep = 0
			if len(current) > 0 {
				sep = 1
			}
		}

		current = append(current, turn)
		currentLen += len(turn) + sep
	}
	emitChunk()

	return chunks
}
