package runner

import "strings"

// thinkingMarkers lists token sequences that indicate thinking-mode content
// has leaked into the JSON output field. Keyed by description; each value
// is a string to search for (case-sensitive).
var thinkingMarkers = []string{
	"<think>",
	"</think>",
	"<thinking>",
	"</thinking>",
	" Thought:",            // DeepSeek R1 style
	"<|channel|>analysis", // GPT-OSS Harmony format
}

// DetectThinkingLeak returns true if content contains known thinking-mode tokens.
// Called on Ollama's message.content field. Also check message.thinking separately.
func DetectThinkingLeak(content string) bool {
	for _, marker := range thinkingMarkers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}
