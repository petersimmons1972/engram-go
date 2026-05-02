package claude_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/stretchr/testify/require"
)

type dqRT func(*http.Request) (*http.Response, error)

func (f dqRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func dqJSON(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func stubClaude(body string) *claude.Client {
	return claude.NewWithTransport("test", dqRT(func(*http.Request) (*http.Response, error) {
		payload, _ := json.Marshal(map[string]any{
			"content": []map[string]string{{"type": "text", "text": body}},
		})
		return dqJSON(http.StatusOK, string(payload)), nil
	}))
}

func panicOnCallClient() *claude.Client {
	return claude.NewWithTransport("test", dqRT(func(*http.Request) (*http.Response, error) {
		panic("claude API was called unexpectedly")
	}))
}

func TestQueryDocument_RegexMatch(t *testing.T) {
	c := stubClaude("Two errors matched: A and B")
	content := "preamble...\nerror: Alpha failed here\nmore text\nerror: Beta failed\ntail..."
	q := claude.DocumentQuery{Question: "which errors?", FilterRegex: `error: \w+`, WindowChars: 20, TokenBudget: 4000}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.NotEmpty(t, res.Spans)
	require.NotEmpty(t, res.Answer)
	joined := ""
	for _, s := range res.Spans {
		joined += s.Text + "|"
	}
	require.True(t, strings.Contains(joined, "Alpha") || strings.Contains(joined, "Beta"))
}

func TestQueryDocument_SubstringMatch(t *testing.T) {
	c := stubClaude("Found critical events")
	content := strings.Repeat("x", 500) + " CRITICAL here " + strings.Repeat("y", 500)
	q := claude.DocumentQuery{Question: "what happened?", FilterSubs: []string{"CRITICAL"}, WindowChars: 40, TokenBudget: 4000}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Len(t, res.Spans, 1)
	require.Contains(t, res.Spans[0].Text, "CRITICAL")
	require.Greater(t, res.Spans[0].Offset, 470)
	require.Less(t, res.Spans[0].Offset, 520)
}

func TestQueryDocument_WindowExtraction(t *testing.T) {
	c := stubClaude("ok")
	content := "prefixprefixprefix TARGET suffixsuffixsuffix"
	q := claude.DocumentQuery{Question: "target?", FilterSubs: []string{"TARGET"}, WindowChars: 20, TokenBudget: 4000}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Len(t, res.Spans, 1)
	require.Contains(t, res.Spans[0].Text, "TARGET")
	require.Contains(t, res.Spans[0].Text, "prefix")
	require.Contains(t, res.Spans[0].Text, "suffix")
}

func TestQueryDocument_OverlapMerge(t *testing.T) {
	c := stubClaude("ok")
	content := "aaaa HIT1 bbbb HIT2 cccc" + strings.Repeat("z", 200)
	q := claude.DocumentQuery{Question: "hits?", FilterSubs: []string{"HIT1", "HIT2"}, WindowChars: 40, TokenBudget: 4000}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Len(t, res.Spans, 1)
	require.Contains(t, res.Spans[0].Text, "HIT1")
	require.Contains(t, res.Spans[0].Text, "HIT2")
}

func TestQueryDocument_EmptyMatch(t *testing.T) {
	c := panicOnCallClient()
	content := "no relevant content here at all"
	q := claude.DocumentQuery{Question: "?", FilterSubs: []string{"NOPE"}, WindowChars: 40, TokenBudget: 4000}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Empty(t, res.Spans)
	require.Contains(t, res.Answer, "No matches found")
}

func TestQueryDocument_NoFilter(t *testing.T) {
	c := stubClaude("ok")
	content := strings.Repeat("a", 10000)
	q := claude.DocumentQuery{Question: "?", WindowChars: 1000, TokenBudget: 4000}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Len(t, res.Spans, 1)
	require.Equal(t, 1000, len(res.Spans[0].Text))
	require.Equal(t, 0, res.Spans[0].Offset)
}

func TestQueryDocument_RegexTooLong(t *testing.T) {
	c := panicOnCallClient()
	q := claude.DocumentQuery{Question: "?", FilterRegex: strings.Repeat("a", 1025), WindowChars: 40, TokenBudget: 4000}
	_, err := claude.QueryDocument(context.Background(), c, "some content", q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "filter.regex exceeds")
}

func TestQueryDocument_LastSpanTruncatedToFitCap(t *testing.T) {
	c := stubClaude("ok")
	content := strings.Repeat("x", 500) + "HIT" + strings.Repeat("y", 500) + "HIT" + strings.Repeat("z", 100)
	q := claude.DocumentQuery{Question: "?", FilterSubs: []string{"HIT"}, WindowChars: 40, TokenBudget: 20}
	res, err := claude.QueryDocument(context.Background(), c, content, q)
	require.NoError(t, err)
	require.Len(t, res.Spans, 2)
	require.True(t, res.Truncated)
	total := len(res.Spans[0].Text) + len(res.Spans[1].Text)
	require.LessOrEqual(t, total, 80)
}

func TestQueryDocument_EmptyContent(t *testing.T) {
	c := panicOnCallClient()
	q := claude.DocumentQuery{Question: "anything?", WindowChars: 4000, TokenBudget: 6000}
	res, err := claude.QueryDocument(context.Background(), c, "", q)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Empty(t, res.Spans)
	require.Contains(t, res.Answer, "No content available")
}
