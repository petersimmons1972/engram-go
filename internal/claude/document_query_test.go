package claude_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/stretchr/testify/require"
)

// panicOnCallServer returns a test server that panics if any HTTP call is made.
// Used to assert that no LLM call is triggered on an empty-match short-circuit.
func panicOnCallServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatalf("claude API was called unexpectedly")
	}))
}

func newStubClaudeServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": body}},
		})
	}))
}

func TestQueryDocument_RegexMatch(t *testing.T) {
	srv := newStubClaudeServer("Two errors matched: A and B")
	defer srv.Close()
	c, _ := claude.New("test")
	c.BaseURL = srv.URL

	content := "preamble...\nerror: Alpha failed here\nmore text\nerror: Beta failed\ntail..."
	q := claude.DocumentQuery{
		Question:    "which errors?",
		FilterRegex: `error: \w+`,
		WindowChars: 20,
		TokenBudget: 4000,
	}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.NotEmpty(t, res.Spans)
	require.NotEmpty(t, res.Answer)
	// At least one span must quote one of the matched error strings.
	joined := ""
	for _, s := range res.Spans {
		joined += s.Text + "|"
	}
	require.True(t, strings.Contains(joined, "Alpha") || strings.Contains(joined, "Beta"))
}

func TestQueryDocument_SubstringMatch(t *testing.T) {
	srv := newStubClaudeServer("Found critical events")
	defer srv.Close()
	c, _ := claude.New("test")
	c.BaseURL = srv.URL

	content := strings.Repeat("x", 500) + " CRITICAL here " + strings.Repeat("y", 500)
	q := claude.DocumentQuery{
		Question:    "what happened?",
		FilterSubs:  []string{"CRITICAL"},
		WindowChars: 40,
		TokenBudget: 4000,
	}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Len(t, res.Spans, 1)
	require.Contains(t, res.Spans[0].Text, "CRITICAL")
	// Offset should be around index 500 (start of the substring minus half window).
	require.Greater(t, res.Spans[0].Offset, 470)
	require.Less(t, res.Spans[0].Offset, 520)
}

func TestQueryDocument_WindowExtraction(t *testing.T) {
	srv := newStubClaudeServer("ok")
	defer srv.Close()
	c, _ := claude.New("test")
	c.BaseURL = srv.URL

	content := "prefixprefixprefix TARGET suffixsuffixsuffix"
	q := claude.DocumentQuery{
		Question:    "target?",
		FilterSubs:  []string{"TARGET"},
		WindowChars: 20,
		TokenBudget: 4000,
	}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Len(t, res.Spans, 1)
	require.Contains(t, res.Spans[0].Text, "TARGET")
	// With half=10, we should see a few chars on each side.
	require.Contains(t, res.Spans[0].Text, "prefix")
	require.Contains(t, res.Spans[0].Text, "suffix")
}

func TestQueryDocument_OverlapMerge(t *testing.T) {
	srv := newStubClaudeServer("ok")
	defer srv.Close()
	c, _ := claude.New("test")
	c.BaseURL = srv.URL

	// Two hits close enough (separated by 5 chars) that windows of 40 merge.
	content := "aaaa HIT1 bbbb HIT2 cccc" + strings.Repeat("z", 200)
	q := claude.DocumentQuery{
		Question:    "hits?",
		FilterSubs:  []string{"HIT1", "HIT2"},
		WindowChars: 40,
		TokenBudget: 4000,
	}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Len(t, res.Spans, 1, "overlapping windows should merge")
	require.Contains(t, res.Spans[0].Text, "HIT1")
	require.Contains(t, res.Spans[0].Text, "HIT2")
}

func TestQueryDocument_EmptyMatch(t *testing.T) {
	srv := panicOnCallServer(t)
	defer srv.Close()
	c, _ := claude.New("test")
	c.BaseURL = srv.URL

	content := "no relevant content here at all"
	q := claude.DocumentQuery{
		Question:    "?",
		FilterSubs:  []string{"NOPE"},
		WindowChars: 40,
		TokenBudget: 4000,
	}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Empty(t, res.Spans)
	require.Contains(t, res.Answer, "No matches found")
}

func TestQueryDocument_NoFilter(t *testing.T) {
	srv := newStubClaudeServer("ok")
	defer srv.Close()
	c, _ := claude.New("test")
	c.BaseURL = srv.URL

	content := strings.Repeat("a", 10000)
	q := claude.DocumentQuery{
		Question:    "?",
		WindowChars: 1000,
		TokenBudget: 4000,
	}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Len(t, res.Spans, 1)
	require.Equal(t, 1000, len(res.Spans[0].Text))
	require.Equal(t, 0, res.Spans[0].Offset)
}

// --- Tests for fixes #184, #186, #188 ---

// Fix #184: FilterRegex longer than 1024 chars must be rejected before compile.
func TestQueryDocument_RegexTooLong(t *testing.T) {
	srv := panicOnCallServer(t)
	defer srv.Close()
	c, _ := claude.New("test")
	c.BaseURL = srv.URL

	q := claude.DocumentQuery{
		Question:    "?",
		FilterRegex: strings.Repeat("a", 1025), // one byte over the 1024-char limit
		WindowChars: 40,
		TokenBudget: 4000,
	}
	_, err := claude.QueryDocument(context.Background(), c, "some content", q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "filter.regex exceeds")
}

// Fix #195: charCap is a hard limit — when the last span would push total past
// the cap it is truncated to fit and Truncated=true is set.
func TestQueryDocument_LastSpanTruncatedToFitCap(t *testing.T) {
	srv := newStubClaudeServer("ok")
	defer srv.Close()
	c, _ := claude.New("test")
	c.BaseURL = srv.URL

	// Two ASCII spans; span1≈43 chars, span2≈43 chars, charCap=80.
	// Span2 would push total to ~86 > 80, so it gets truncated to ~37 chars.
	content := strings.Repeat("x", 500) + "HIT" + strings.Repeat("y", 500) + "HIT" + strings.Repeat("z", 100)
	q := claude.DocumentQuery{
		Question:    "?",
		FilterSubs:  []string{"HIT"},
		WindowChars: 40,
		TokenBudget: 20, // charCap = 80
	}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Len(t, res.Spans, 2, "both spans present (second truncated, not dropped)")
	require.True(t, res.Truncated, "Truncated must be true when a span was cut short")
	total := len(res.Spans[0].Text) + len(res.Spans[1].Text)
	require.LessOrEqual(t, total, 80, "total chars must not exceed charCap")
}

// Fix #188: Empty content must short-circuit with a canned answer, no LLM call.
func TestQueryDocument_EmptyContent(t *testing.T) {
	srv := panicOnCallServer(t)
	defer srv.Close()
	c, _ := claude.New("test")
	c.BaseURL = srv.URL

	q := claude.DocumentQuery{
		Question:    "anything?",
		WindowChars: 4000,
		TokenBudget: 6000,
	}
	res, err := claude.QueryDocument(context.Background(), c, "", q)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Empty(t, res.Spans)
	require.Equal(t, "No content available for this memory.", res.Answer)
}

func TestQueryDocument_TokenBudget(t *testing.T) {
	srv := newStubClaudeServer("ok")
	defer srv.Close()
	c, _ := claude.New("test")
	c.BaseURL = srv.URL

	// Many widely-separated hits, each producing a non-overlapping window.
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString(strings.Repeat("x", 500))
		b.WriteString("TAG")
	}
	content := b.String()
	budget := 200 // 200 tokens * 4 = 800 char cap
	q := claude.DocumentQuery{
		Question:    "?",
		FilterSubs:  []string{"TAG"},
		WindowChars: 40,
		TokenBudget: budget,
	}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.True(t, res.Truncated)
	total := 0
	for _, s := range res.Spans {
		total += len(s.Text)
	}
	require.LessOrEqual(t, total, budget*4+q.WindowChars)
}
