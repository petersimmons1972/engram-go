package claude_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func reviewClient(rt http.RoundTripper) *claude.Client {
	return claude.NewWithTransport("test-key", rt)
}

func reviewJSON(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestReviewMergeCandidates_ParsesResponse(t *testing.T) {
	responseText := `[{"memory_a_id":"a1","memory_b_id":"b1","should_merge":true,"reason":"same fact","merged_content":"merged"}]`
	c := reviewClient(rtFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/messages", r.URL.Path)
		payload, _ := json.Marshal(map[string]any{
			"content": []map[string]string{{"type": "text", "text": responseText}},
		})
		return reviewJSON(http.StatusOK, string(payload)), nil
	}))

	candidates := []claude.MergeCandidate{{MemoryA: &types.Memory{ID: "a1", Content: "fact about X"}, MemoryB: &types.Memory{ID: "b1", Content: "fact about X repeated"}, Similarity: 0.95}}
	decisions, err := c.ReviewMergeCandidates(context.Background(), candidates)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	require.True(t, decisions[0].ShouldMerge)
	require.Equal(t, "a1", decisions[0].MemoryAID)
	require.Equal(t, "b1", decisions[0].MemoryBID)
	require.Equal(t, "merged", decisions[0].MergedContent)
}

func TestReviewMergeCandidates_MalformedJSON(t *testing.T) {
	c := reviewClient(rtFunc(func(*http.Request) (*http.Response, error) {
		payload, _ := json.Marshal(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "not valid json"}},
		})
		return reviewJSON(http.StatusOK, string(payload)), nil
	}))

	candidates := []claude.MergeCandidate{{MemoryA: &types.Memory{ID: "a1", Content: "some content"}, MemoryB: &types.Memory{ID: "b1", Content: "other content"}, Similarity: 0.80}}
	_, err := c.ReviewMergeCandidates(context.Background(), candidates)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse merge decisions")
}

func TestReviewMergeCandidates_EmptyCandidates(t *testing.T) {
	handlerCalled := false
	c := reviewClient(rtFunc(func(*http.Request) (*http.Response, error) {
		handlerCalled = true
		return reviewJSON(http.StatusInternalServerError, ""), nil
	}))

	decisions, err := c.ReviewMergeCandidates(context.Background(), []claude.MergeCandidate{})
	require.NoError(t, err)
	require.Nil(t, decisions)
	require.False(t, handlerCalled)
}

func TestReviewMergeCandidates_AdvisorToolDeclared(t *testing.T) {
	var capturedBody struct {
		Tools []struct {
			Type string `json:"type"`
		} `json:"tools"`
	}
	c := reviewClient(rtFunc(func(r *http.Request) (*http.Response, error) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		payload, _ := json.Marshal(map[string]any{
			"content": []map[string]string{{"type": "text", "text": `[{"memory_a_id":"a1","memory_b_id":"b1","should_merge":false,"reason":"different"}]`}},
		})
		return reviewJSON(http.StatusOK, string(payload)), nil
	}))

	candidates := []claude.MergeCandidate{{MemoryA: &types.Memory{ID: "a1", Content: "content a"}, MemoryB: &types.Memory{ID: "b1", Content: "content b"}, Similarity: 0.60}}
	_, err := c.ReviewMergeCandidates(context.Background(), candidates)
	require.NoError(t, err)
	require.Len(t, capturedBody.Tools, 1)
	require.Equal(t, "advisor_20260301", capturedBody.Tools[0].Type)
}
