package claude_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

type exploreRT func(*http.Request) (*http.Response, error)

func (f exploreRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func exploreJSON(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func exploreClient(rt http.RoundTripper) *claude.Client {
	return claude.NewWithTransport("test-key", rt)
}

type mockFetcher struct {
	fetched []string
	upgrade map[string]*types.Memory
}

func (f *mockFetcher) FetchMemory(_ context.Context, _ string, id string) (*types.Memory, error) {
	f.fetched = append(f.fetched, id)
	if m, ok := f.upgrade[id]; ok {
		return m, nil
	}
	return nil, nil
}

type mockRecaller struct {
	calls [][]types.SearchResult
	idx   int
}

func (m *mockRecaller) Recall(_ context.Context, _ string, _ int, _ string) ([]types.SearchResult, error) {
	if m.idx >= len(m.calls) {
		return nil, nil
	}
	r := m.calls[m.idx]
	m.idx++
	return r, nil
}

type nilRelGetter struct{}

func (nilRelGetter) GetRelationships(_ context.Context, _, _ string) ([]types.Relationship, error) {
	return nil, nil
}

func makeResult(id, content string) types.SearchResult {
	return types.SearchResult{Memory: &types.Memory{ID: id, Content: content}, Score: 0.8}
}

func TestExplore_HappyTwoIterConvergence(t *testing.T) {
	var callCount int32
	c := exploreClient(exploreRT(func(*http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			payload, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": `{"confidence":0.4,"gaps":["more info"],"refined_query":"refined","stop":false}`}}, "usage": map[string]int{"input_tokens": 100, "output_tokens": 50}})
			return exploreJSON(http.StatusOK, string(payload)), nil
		case 2:
			payload, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": `{"confidence":0.9,"gaps":[],"refined_query":null,"stop":true}`}}, "usage": map[string]int{"input_tokens": 150, "output_tokens": 50}})
			return exploreJSON(http.StatusOK, string(payload)), nil
		default:
			payload, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": "The answer is based on the memories."}}, "usage": map[string]int{"input_tokens": 200, "output_tokens": 100}})
			return exploreJSON(http.StatusOK, string(payload)), nil
		}
	}))

	recaller := &mockRecaller{calls: [][]types.SearchResult{{makeResult("id1", "m1"), makeResult("id2", "m2"), makeResult("id3", "m3")}, {makeResult("id4", "m4"), makeResult("id5", "m5")}}}
	req := claude.ExploreRequest{Project: "test", Question: "what?", MaxIterations: 5, ConfidenceThreshold: 0.75, TokenBudget: 20000}
	result, err := claude.Explore(context.Background(), c, recaller, nil, nilRelGetter{}, req)
	require.NoError(t, err)
	require.Equal(t, 2, result.Iterations)
	require.NotEmpty(t, result.Answer)
	require.False(t, result.Truncated)
}

func TestExplore_BudgetExhaustion(t *testing.T) {
	c := exploreClient(exploreRT(func(*http.Request) (*http.Response, error) {
		payload, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": `{"confidence":0.3,"gaps":["more"],"refined_query":"q2","stop":false}`}}, "usage": map[string]int{"input_tokens": 10000, "output_tokens": 5000}})
		return exploreJSON(http.StatusOK, string(payload)), nil
	}))
	recaller := &mockRecaller{calls: [][]types.SearchResult{{makeResult("id1", "m1")}, {makeResult("id2", "m2")}, {makeResult("id3", "m3")}}}
	req := claude.ExploreRequest{Project: "test", Question: "q?", MaxIterations: 5, ConfidenceThreshold: 0.99, TokenBudget: 10}
	result, err := claude.Explore(context.Background(), c, recaller, nil, nilRelGetter{}, req)
	require.NoError(t, err)
	require.True(t, result.Truncated)
}

func TestExplore_NoProgress(t *testing.T) {
	var callCount int32
	c := exploreClient(exploreRT(func(*http.Request) (*http.Response, error) {
		atomic.AddInt32(&callCount, 1)
		payload, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": `{"confidence":0.4,"gaps":["more"],"refined_query":"same","stop":false}`}}, "usage": map[string]int{"input_tokens": 10, "output_tokens": 5}})
		return exploreJSON(http.StatusOK, string(payload)), nil
	}))
	same := []types.SearchResult{makeResult("id1", "m1")}
	recaller := &mockRecaller{calls: [][]types.SearchResult{same, same, same, same, same}}
	req := claude.ExploreRequest{Project: "test", Question: "q?", MaxIterations: 10, ConfidenceThreshold: 0.99, TokenBudget: 100000}
	result, err := claude.Explore(context.Background(), c, recaller, nil, nilRelGetter{}, req)
	require.NoError(t, err)
	require.LessOrEqual(t, result.Iterations, 3)
}
