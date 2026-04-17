package claude

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// DocumentQuery holds the parsed args for a memory_query_document request.
//
// Filtering strategy: if FilterRegex is set it wins; else if FilterSubs is
// non-empty each substring is matched via strings.Index; else the first
// WindowChars of the document are returned as a single span.
type DocumentQuery struct {
	Project     string
	MemoryID    string
	Question    string
	FilterRegex string   // optional — regex pattern
	FilterSubs  []string // optional — substrings (literal, case-sensitive)
	WindowChars int      // default 4000
	Semantic    bool     // default false
	TokenBudget int      // default 6000 (tokens, char-budget ≈ TokenBudget*4)
}

// DocumentQueryResult is the response from QueryDocument.
type DocumentQueryResult struct {
	Spans     []DocumentSpan `json:"spans"`
	Answer    string         `json:"answer"`
	Truncated bool           `json:"truncated,omitempty"`
}

// DocumentSpan is one extracted window from the document.
type DocumentSpan struct {
	Offset  int    `json:"offset"`
	Text    string `json:"text"`
	Matched string `json:"matched,omitempty"` // the regex/substring that triggered this window
}

const (
	defaultWindowChars = 4000
	defaultTokenBudget = 6000
	// charsPerToken is the rough chars→tokens conversion used for budgeting.
	charsPerToken = 4
)

const documentQuerySystem = "You are a precise document analyst. Answer questions by quoting exact spans " +
	"from the provided text. Do not speculate beyond what the text says."

// rawSpan is an internal helper: raw [start,end) offsets before merge.
type rawSpan struct {
	start, end int
	matched    string
}

// QueryDocument extracts relevant windows from content and asks Claude to
// answer q.Question against those windows. If no spans match a filter the
// function short-circuits and returns a canned "no matches" answer without
// calling the LLM.
func QueryDocument(ctx context.Context, c *Client, content string, q DocumentQuery) (*DocumentQueryResult, error) {
	if q.WindowChars <= 0 {
		q.WindowChars = defaultWindowChars
	}
	if q.TokenBudget <= 0 {
		q.TokenBudget = defaultTokenBudget
	}
	half := q.WindowChars / 2
	n := len(content)

	var raws []rawSpan
	switch {
	case q.FilterRegex != "":
		re, err := regexp.Compile(q.FilterRegex)
		if err != nil {
			return nil, fmt.Errorf("invalid filter.regex: %w", err)
		}
		for _, idx := range re.FindAllStringIndex(content, -1) {
			s, e := idx[0], idx[1]
			matched := content[s:e]
			start := s - half
			if start < 0 {
				start = 0
			}
			end := e + half
			if end > n {
				end = n
			}
			raws = append(raws, rawSpan{start: start, end: end, matched: matched})
		}
	case len(q.FilterSubs) > 0:
		for _, sub := range q.FilterSubs {
			if sub == "" {
				continue
			}
			// Find ALL occurrences of sub, not just the first.
			from := 0
			for from < n {
				i := strings.Index(content[from:], sub)
				if i < 0 {
					break
				}
				s := from + i
				e := s + len(sub)
				start := s - half
				if start < 0 {
					start = 0
				}
				end := e + half
				if end > n {
					end = n
				}
				raws = append(raws, rawSpan{start: start, end: end, matched: sub})
				from = e
			}
		}
	default:
		// No filter: return first WindowChars bytes as a single span.
		end := q.WindowChars
		if end > n {
			end = n
		}
		raws = append(raws, rawSpan{start: 0, end: end})
	}

	merged := mergeSpans(raws)

	// Empty-filter short-circuit: no LLM call.
	if (q.FilterRegex != "" || len(q.FilterSubs) > 0) && len(merged) == 0 {
		return &DocumentQueryResult{
			Spans:  []DocumentSpan{},
			Answer: "No matches found for the specified filter.",
		}, nil
	}

	// Apply token budget — stop accumulating once we cross the char cap,
	// but include the span that tipped us over so callers see a complete window.
	charCap := q.TokenBudget * charsPerToken
	spans := make([]DocumentSpan, 0, len(merged))
	total := 0
	truncated := false
	for _, m := range merged {
		text := content[m.start:m.end]
		spans = append(spans, DocumentSpan{
			Offset:  m.start,
			Text:    text,
			Matched: m.matched,
		})
		total += len(text)
		if total > charCap {
			truncated = true
			break
		}
	}

	// Build the user prompt by concatenating spans with dividers.
	var sb strings.Builder
	for i, s := range spans {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		sb.WriteString(s.Text)
	}
	sb.WriteString("\n\nQuestion: ")
	sb.WriteString(q.Question)

	answer, err := c.Complete(ctx, documentQuerySystem, sb.String(),
		"claude-sonnet-4-6", "claude-opus-4-6", 0, 1024)
	if err != nil {
		return nil, err
	}

	return &DocumentQueryResult{
		Spans:     spans,
		Answer:    answer,
		Truncated: truncated,
	}, nil
}

// mergeSpans merges overlapping or adjacent raw spans. Returns a copy sorted
// by start offset with no overlaps. The matched field on a merged span is set
// to the first contributing match.
func mergeSpans(raws []rawSpan) []rawSpan {
	if len(raws) == 0 {
		return nil
	}
	sort.Slice(raws, func(i, j int) bool { return raws[i].start < raws[j].start })
	out := []rawSpan{raws[0]}
	for _, s := range raws[1:] {
		last := &out[len(out)-1]
		if s.start <= last.end { // overlap or touching
			if s.end > last.end {
				last.end = s.end
			}
			// retain first matched
		} else {
			out = append(out, s)
		}
	}
	return out
}
