package claude_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// makeClient builds a Client pointed at the given test server URL.
func makeClient(t *testing.T, baseURL string) *claude.Client {
	t.Helper()
	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = baseURL
	return c
}

// mergeServer builds an httptest.Server that returns the provided text as the
// first content block of a Anthropic messages response.
func mergeServer(text string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": text},
			},
		})
	}))
}

func TestReviewMergeCandidates_ParsesResponse(t *testing.T) {
	responseText := `[{"memory_a_id":"a1","memory_b_id":"b1","should_merge":true,"reason":"same fact","merged_content":"merged"}]`

	srv := mergeServer(responseText)
	defer srv.Close()

	c := makeClient(t, srv.URL)

	candidates := []claude.MergeCandidate{
		{
			MemoryA:    &types.Memory{ID: "a1", Content: "fact about X"},
			MemoryB:    &types.Memory{ID: "b1", Content: "fact about X repeated"},
			Similarity: 0.95,
		},
	}

	decisions, err := c.ReviewMergeCandidates(context.Background(), candidates)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	require.True(t, decisions[0].ShouldMerge)
	require.Equal(t, "a1", decisions[0].MemoryAID)
	require.Equal(t, "b1", decisions[0].MemoryBID)
	require.Equal(t, "merged", decisions[0].MergedContent)
}

func TestReviewMergeCandidates_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": "not valid json"},
			},
		})
	}))
	defer srv.Close()

	c := makeClient(t, srv.URL)

	candidates := []claude.MergeCandidate{
		{
			MemoryA:    &types.Memory{ID: "a1", Content: "some content"},
			MemoryB:    &types.Memory{ID: "b1", Content: "other content"},
			Similarity: 0.80,
		},
	}

	_, err := c.ReviewMergeCandidates(context.Background(), candidates)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse merge decisions")
}

func TestReviewMergeCandidates_EmptyCandidates(t *testing.T) {
	handlerCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := makeClient(t, srv.URL)

	decisions, err := c.ReviewMergeCandidates(context.Background(), []claude.MergeCandidate{})
	require.NoError(t, err)
	require.Nil(t, decisions)
	require.False(t, handlerCalled, "HTTP handler should never be called for empty candidate slice")
}

func TestReviewMergeCandidates_AdvisorToolDeclared(t *testing.T) {
	var capturedBody struct {
		Tools []struct {
			Type string `json:"type"`
		} `json:"tools"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": `[{"memory_a_id":"a1","memory_b_id":"b1","should_merge":false,"reason":"different"}]`},
			},
		})
	}))
	defer srv.Close()

	c := makeClient(t, srv.URL)

	candidates := []claude.MergeCandidate{
		{
			MemoryA:    &types.Memory{ID: "a1", Content: "content a"},
			MemoryB:    &types.Memory{ID: "b1", Content: "content b"},
			Similarity: 0.60,
		},
	}

	_, err := c.ReviewMergeCandidates(context.Background(), candidates)
	require.NoError(t, err)
	require.Len(t, capturedBody.Tools, 1)
	require.Equal(t, "advisor_20260301", capturedBody.Tools[0].Type)
}
