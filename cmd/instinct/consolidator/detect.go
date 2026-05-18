// Package consolidator owns the domain logic for pattern detection: prompt
// construction, LLM call dispatch, and response parsing/validation.
//
// The LLM client (transport layer) is injected via the llm.LLMClient interface
// so backends (Anthropic, Olla) can be swapped without touching this package.
package consolidator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/petersimmons1972/engram/cmd/instinct/llm"
)

// Event is one tool-use record from the PostToolUse hook buffer.
// Field names match the JSONL schema written by the hook.
type Event struct {
	Timestamp         string `json:"timestamp"`
	SessionID         string `json:"session_id"`
	ProjectID         string `json:"project_id"`
	ToolName          string `json:"tool_name"`
	ToolInputHash     string `json:"tool_input_hash"`
	ToolOutputSummary string `json:"tool_output_summary"`
	ExitStatus        int    `json:"exit_status"`
	SchemaVersion     int    `json:"schema_version"`
}

// Pattern is the structured output produced by the LLM for a batch of events.
// Field names and valid types must match what the LLM is instructed to produce
// and what Engram stores.
type Pattern struct {
	Type         string `json:"type"`
	Description  string `json:"description"`
	Domain       string `json:"domain"`
	Evidence     string `json:"evidence"`
	TagSignature string `json:"tag_signature"`
}

// validTypes lists the pattern types the LLM is allowed to produce.
// Any other value is filtered out (wrong-type model hallucination).
var validTypes = map[string]struct{}{
	"correction":       {},
	"error_resolution": {},
	"workflow":         {},
}

// systemPrompt is the production-tuned prompt for pattern detection.
// Ported verbatim from ~/projects/instinct/consolidator/instinct/haiku_client.py:7-24.
// Do not edit without updating the Python source and re-running the benchmark.
const systemPrompt = `You are a pattern detection system analyzing Claude Code tool call sequences.

Analyze the tool call events and identify recurring patterns of these types:

1. CORRECTION: Evidence the user corrected the AI — re-do after rollback, "don't X" instruction, same action reversed within 3 steps.
2. ERROR_RESOLUTION: The same error (matching exit_status=1 + similar output_summary) followed by the same fix tool sequence, 2+ times.
3. WORKFLOW: A sequence of 3+ tool calls that recurs within the same session or across sessions in this batch.

Return a JSON array. Each pattern object must have these exact fields:
{
  "type": "correction" | "error_resolution" | "workflow",
  "description": "<human-readable pattern, one sentence, present tense>",
  "domain": "<one word: testing | git | editing | bash | agent | general>",
  "evidence": "<brief explanation of what you observed, max 100 chars>",
  "tag_signature": "<stable slug for deduplication, e.g. 'sig-edit-test-fail-edit'>"
}

If no patterns are found, return []. Return ONLY the JSON array — no prose, no markdown fences.`

// buildUserMessage formats events into the user-turn content sent to the LLM.
// Format matches _build_user_message in haiku_client.py:70-75.
func buildUserMessage(events []Event) string {
	var sb strings.Builder
	sb.WriteString("Tool call events:\n")
	for _, e := range events {
		fmt.Fprintf(&sb, "[%s] %s | %s | exit=%d\n",
			e.Timestamp, e.ToolName, e.ToolOutputSummary, e.ExitStatus)
	}
	return sb.String()
}

// validPattern returns true when p has all required fields and a valid type.
// Matches _valid_pattern in haiku_client.py:78-80, plus explicit type check.
func validPattern(p Pattern) bool {
	if p.Type == "" || p.Description == "" || p.Domain == "" || p.Evidence == "" || p.TagSignature == "" {
		return false
	}
	_, ok := validTypes[p.Type]
	return ok
}

// stripMarkdownFences removes accidental triple-backtick wrappers from text.
// Belt-and-suspenders: Olla strips at client level; Anthropic backend also
// strips; this layer catches any remaining cases.
func stripMarkdownFences(text string) string {
	if strings.HasPrefix(text, "```") {
		parts := strings.SplitN(text, "\n", 2)
		if len(parts) == 2 {
			text = parts[1]
		}
		if idx := strings.LastIndex(text, "```"); idx >= 0 {
			text = text[:idx]
		}
	}
	return strings.TrimSpace(text)
}

// Detect calls client with the production system prompt and the formatted
// events, then parses and validates the LLM's JSON response.
//
// Returns:
//   - ([]Pattern, nil) on success — may be empty if the LLM found nothing.
//   - (nil, err) when the LLM client returns an error — caller decides whether
//     to log-and-skip or propagate.
//   - ([]Pattern, nil) with empty slice on JSON parse failure or bad LLM output
//     — graceful degradation so the consolidator never crashes on bad LLM output.
//
// Zero events short-circuits before calling the LLM.
func Detect(ctx context.Context, client llm.LLMClient, events []Event) ([]Pattern, error) {
	if len(events) == 0 {
		return nil, nil
	}

	userMsg := buildUserMessage(events)
	raw, err := client.Complete(ctx, systemPrompt, userMsg)
	if err != nil {
		return nil, err
	}

	// Strip markdown fences defensively before JSON parsing.
	raw = stripMarkdownFences(raw)

	var rawPatterns []Pattern
	if err := json.Unmarshal([]byte(raw), &rawPatterns); err != nil {
		slog.Warn("consolidator: LLM JSON parse failed", "err", err)
		return nil, nil //nolint:nilerr // intentional: bad LLM output is non-fatal
	}

	var patterns []Pattern
	for _, p := range rawPatterns {
		if !validPattern(p) {
			slog.Warn("consolidator: skipping pattern with missing/invalid fields", "tag", p.TagSignature, "type", p.Type)
			continue
		}
		patterns = append(patterns, p)
	}
	return patterns, nil
}
