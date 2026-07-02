package ledgerprobe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// LLMExtractor implements EventExtractor using a local OAI-compatible endpoint
// (olla, model "inference" by default). It is precision-biased: the prompt
// instructs the model to extract ONLY genuine, distinct countable events the user
// actually performed or owns — not hypotheticals, questions, web-search results,
// or generic prose. This directly addresses the over-extraction that inflates
// shallow-regex totals.
//
// Name() == "llm-olla".
// Override base URL: LEDGERPROBE_LLM_URL
// Override model:   LEDGERPROBE_LLM_MODEL
type LLMExtractor struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewLLMExtractor creates an LLMExtractor. Zero-value construction is safe;
// use this to get the environment-variable-aware defaults.
func NewLLMExtractor() *LLMExtractor {
	base := os.Getenv("LEDGERPROBE_LLM_URL")
	if base == "" {
		base = "http://192.168.0.138:30411/olla/openai/v1"
	}
	model := os.Getenv("LEDGERPROBE_LLM_MODEL")
	if model == "" {
		model = "inference"
	}
	return &LLMExtractor{
		baseURL: strings.TrimRight(base, "/"),
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (e *LLMExtractor) Name() string { return "llm-olla" }

// llmEventResponse is the per-event JSON object the model returns.
type llmEventResponse struct {
	EventType  string  `json:"event_type"`
	Object     string  `json:"object"`
	Quantity   float64 `json:"quantity"`
	Unit       string  `json:"unit"`
	Money      float64 `json:"money"`
	Polarity   int     `json:"polarity"`
	ObservedAt string  `json:"observed_at"`
	SourceSpan string  `json:"source_span"`
	Confidence float64 `json:"confidence"`
}

const llmSystemPrompt = `You are a precise countable-event extractor. Given a passage of conversational text from a user's memory/journal, extract ONLY genuine, distinct countable events the user actually performed, experienced, or owns.

EXTRACT:
- Things the user bought, purchased, acquired ("I bought 3 model kits")
- Money the user spent, saved, earned, donated ("spent $50", "raised $200")
- Activities the user completed ("ran 5km", "read 100 pages", "worked 3 hours")
- Concrete counts of objects the user has or obtained

DO NOT EXTRACT:
- Hypotheticals, questions, plans, or intentions ("I might buy...", "should I get?")
- Information from web search results, news, or third-party sources
- Numbers that are prices of things not yet purchased
- Generic statements without a real user action ("there are 5 colors available")
- Repeated mentions of the same event — output ONE row per real distinct occurrence

For each genuine event, output a JSON object with these fields:
- event_type: one of "purchase", "expense", "activity", "count", "generic"
- object: the counted noun phrase (e.g. "model kit", "book")
- quantity: numeric count for this event (default 1 if not specified)
- unit: "item", "km", "hour", "dollar", or "" as appropriate
- money: monetary amount as a number (0 if not a money event)
- polarity: 1 for normal, -1 for refund/return/cancel
- observed_at: ISO date like "2024-03-01" if mentioned, else ""
- source_span: a short verbatim quote from the text (≤120 chars) that contains this event
- confidence: 0.0–1.0 (how certain you are this is a real, distinct, countable event)

Return ONLY a JSON array of these objects. No prose, no markdown fences. If nothing qualifies, return [].`

// Extract calls the local olla endpoint and parses the JSON result into EventAtoms.
// On parse failure or HTTP error the session yields no events (and a warning is logged).
// This never panics.
func (e *LLMExtractor) Extract(sessionID, text string) []EventAtom {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	// Truncate very long sessions — 8000 chars covers most sessions without
	// overflowing a small local model's context window.
	const maxChars = 8000
	if len(text) > maxChars {
		text = text[:maxChars]
	}

	prompt := "Extract countable events from the following session text:\n\n" + text

	raw, err := e.callOlla(prompt)
	if err != nil {
		log.Printf("[llm_extractor] session=%s: olla call failed: %v", sessionID, err)
		return nil
	}

	events, err := parseLLMEvents(raw)
	if err != nil {
		log.Printf("[llm_extractor] session=%s: JSON parse failed: %v (raw=%q)", sessionID, err, truncate(raw, 200))
		return nil
	}

	out := make([]EventAtom, 0, len(events))
	for _, ev := range events {
		if ev.Confidence <= 0 {
			ev.Confidence = 0.7
		}
		pol := ev.Polarity
		if pol == 0 {
			pol = 1
		}
		// Normalise event_type to the set used by AggregationFrame / filterByObject.
		evType := normaliseEventType(ev.EventType)
		out = append(out, EventAtom{
			SessionID:  sessionID,
			EventType:  evType,
			Object:     strings.TrimSpace(ev.Object),
			Quantity:   ev.Quantity,
			Unit:       ev.Unit,
			Money:      ev.Money,
			Polarity:   pol,
			ObservedAt: ev.ObservedAt,
			SourceSpan: trimSpan(ev.SourceSpan),
			Confidence: ev.Confidence,
		})
	}
	return out
}

// normaliseEventType maps the model's output values to the set the aggregate layer
// recognises. ShallowExtractor uses: "expense","saving","income","donation","refund",
// "activity","count","generic". The prompt asks for "purchase|expense|activity|count|generic";
// we harmonise any "purchase" → "expense" so money filtering works correctly.
func normaliseEventType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "purchase":
		return "expense"
	case "expense", "saving", "income", "donation", "refund", "activity", "count", "generic":
		return t
	}
	if t == "" {
		return "generic"
	}
	return "generic"
}

// oaiRequest is a minimal OpenAI chat-completions request body.
type oaiRequest struct {
	Model    string       `json:"model"`
	Messages []oaiMessage `json:"messages"`
	// Low temperature for precision — we want deterministic extraction.
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaiResponse struct {
	Choices []struct {
		Message struct {
			// Content is a pointer because reasoning models return null content
			// when the token budget is exhausted on reasoning (finish_reason="length").
			// A nil pointer means empty output; we log a warning and skip the session.
			Content *string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// callOlla sends a single user message to the OAI-compatible completions endpoint
// and returns the assistant's reply text.
func (e *LLMExtractor) callOlla(userMsg string) (string, error) {
	req := oaiRequest{
		Model: e.model,
		Messages: []oaiMessage{
			{Role: "system", Content: llmSystemPrompt},
			{Role: "user", Content: userMsg},
		},
		Temperature: 0.0,
		// 4096 tokens gives the reasoning model enough budget to complete both
		// its internal chain-of-thought and the JSON output. 1024 was insufficient
		// for the local inference model which uses reasoning tokens.
		MaxTokens: 4096,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	resp, err := e.client.Post(e.baseURL+"/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("olla returned HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var oai oaiResponse
	if err := json.Unmarshal(raw, &oai); err != nil {
		return "", fmt.Errorf("unmarshal oai response: %w", err)
	}
	if oai.Error != nil {
		return "", fmt.Errorf("olla error: %s", oai.Error.Message)
	}
	if len(oai.Choices) == 0 {
		return "", fmt.Errorf("olla returned no choices")
	}
	content := oai.Choices[0].Message.Content
	if content == nil {
		return "", fmt.Errorf("olla returned null content (reasoning model exhausted token budget)")
	}
	return *content, nil
}

// parseLLMEvents robustly decodes the model's JSON output. It tolerates:
//   - markdown fences (```json ... ```)
//   - leading/trailing prose
//   - <think>...</think> blocks (reasoning models)
//
// On any decode failure it returns an error and the caller logs + returns nil events.
func parseLLMEvents(raw string) ([]llmEventResponse, error) {
	raw = strings.TrimSpace(raw)
	// strip <think>...</think>
	raw = stripThinkTagsLLM(raw)
	// strip markdown fences
	raw = stripMDFencesLLM(raw)
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return nil, fmt.Errorf("empty response after stripping")
	}

	// Try direct parse first.
	if json.Valid([]byte(raw)) {
		var out []llmEventResponse
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, err
		}
		return out, nil
	}

	// Try to extract the first balanced JSON array from the response.
	if arr := firstJSONArray(raw); arr != "" {
		var out []llmEventResponse
		if err := json.Unmarshal([]byte(arr), &out); err != nil {
			return nil, fmt.Errorf("array parse after scan: %w", err)
		}
		return out, nil
	}

	return nil, fmt.Errorf("no JSON array found in response")
}

// stripThinkTagsLLM removes <think>...</think> blocks (case-insensitive).
func stripThinkTagsLLM(raw string) string {
	lower := strings.ToLower(raw)
	for {
		start := strings.Index(lower, "<think>")
		if start == -1 {
			break
		}
		end := strings.Index(lower[start:], "</think>")
		if end == -1 {
			break
		}
		end += start + len("</think>")
		raw = raw[:start] + raw[end:]
		lower = strings.ToLower(raw)
	}
	return raw
}

// stripMDFencesLLM removes lines that are pure markdown code fences.
func stripMDFencesLLM(raw string) string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// firstJSONArray scans raw for the first '[' and returns the balanced array string.
func firstJSONArray(raw string) string {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '[' {
			continue
		}
		depth := 0
		inStr := false
		esc := false
		for j := i; j < len(raw); j++ {
			c := raw[j]
			if inStr {
				if esc {
					esc = false
					continue
				}
				if c == '\\' {
					esc = true
				} else if c == '"' {
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case '[', '{':
				depth++
			case ']', '}':
				depth--
				if depth == 0 {
					candidate := raw[i : j+1]
					if json.Valid([]byte(candidate)) {
						return candidate
					}
					// Not valid — move on
					goto nextStart
				}
			}
		}
	nextStart:
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
