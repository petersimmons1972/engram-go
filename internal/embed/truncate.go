package embed

// charsPerToken is the character-to-token approximation used throughout engram.
// All embedding models handled here average ~4 chars/token on typical English text.
const charsPerToken = 4

// TruncateToModelWindow truncates text to fit within maxTokens * charsPerToken
// characters. It tries to cut at the last sentence boundary (period or newline)
// within the window to preserve semantic coherence. If no boundary is found in
// the lower half of the window, it falls back to a hard character cut.
//
// This is a safety-net called in Embed() before sending text to Ollama. Callers
// should size chunks at store time via ModelMaxTokens to avoid truncation silently
// discarding content.
func TruncateToModelWindow(text string, maxTokens int) string {
	maxChars := maxTokens * charsPerToken
	if len(text) <= maxChars {
		return text
	}
	// Walk backwards from maxChars to find the last sentence boundary.
	for i := maxChars; i > maxChars/2; i-- {
		if text[i] == '.' || text[i] == '\n' {
			return text[:i+1]
		}
	}
	// No boundary found — hard cut.
	return text[:maxChars]
}
