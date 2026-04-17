package claude_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

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
	return types.SearchResult{
		Memory: &types.Memory{ID: id, Content: content},
		Score:  0.8,
	}
}

func TestExplore_HappyTwoIterConvergence(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": `{"confidence":0.4,"gaps":["more info"],"refined_query":"refined","stop":false}`}},
				"usage":   map[string]int{"input_tokens": 100, "output_tokens": 50},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": `{"confidence":0.9,"gaps":[],"refined_query":null,"stop":true}`}},
				"usage":   map[string]int{"input_tokens": 150, "output_tokens": 50},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": "The answer is based on the memories."}},
				"usage":   map[string]int{"input_tokens": 200, "output_tokens": 100},
			})
		}
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	recaller := &mockRecaller{
		calls: [][]types.SearchResult{
			{makeResult("id1", "m1"), makeResult("id2", "m2"), makeResult("id3", "m3")},
			{makeResult("id4", "m4"), makeResult("id5", "m5")},
		},
	}

	req := claude.ExploreRequest{
		Project:             "test",
		Question:            "what?",
		MaxIterations:       5,
		ConfidenceThreshold: 0.75,
		TokenBudget:         20000,
	}
	result, err := claude.Explore(context.Background(), c, recaller, nilRelGetter{}, req)
	require.NoError(t, err)
	require.Equal(t, 2, result.Iterations)
	require.NotEmpty(t, result.Answer)
	require.False(t, result.Truncated)
}

func TestExplore_BudgetExhaustion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": `{"confidence":0.3,"gaps":["more"],"refined_query":"q2","stop":false}`}},
			"usage":   map[string]int{"input_tokens": 10000, "output_tokens": 5000},
		})
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	recaller := &mockRecaller{
		calls: [][]types.SearchResult{
			{makeResult("id1", "m1")},
			{makeResult("id2", "m2")},
			{makeResult("id3", "m3")},
		},
	}

	req := claude.ExploreRequest{
		Project:             "test",
		Question:            "q?",
		MaxIterations:       5,
		ConfidenceThreshold: 0.99,
		TokenBudget:         10,
	}
	result, err := claude.Explore(context.Background(), c, recaller, nilRelGetter{}, req)
	require.NoError(t, err)
	require.True(t, result.Truncated)
}

func TestExplore_NoProgress(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": `{"confidence":0.4,"gaps":["more"],"refined_query":"same","stop":false}`}},
			"usage":   map[string]int{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	// Recaller always returns the same memory.
	same := []types.SearchResult{makeResult("id1", "m1")}
	recaller := &mockRecaller{
		calls: [][]types.SearchResult{same, same, same, same, same},
	}

	req := claude.ExploreRequest{
		Project:             "test",
		Question:            "q?",
		MaxIterations:       10,
		ConfidenceThreshold: 0.99,
		TokenBudget:         100000,
	}
	result, err := claude.Explore(context.Background(), c, recaller, nilRelGetter{}, req)
	require.NoError(t, err)
	// After iter 0 (3 new), iter 1 (0 new), iter 2 (0 new) should stop.
	// Or refined_query == prev stops even sooner.
	require.LessOrEqual(t, result.Iterations, 3)
}

func TestExplore_UngroundedCitationStripped(t *testing.T) {
	ungrounded := "ffffffffffffffffffffffffffffffff"
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": `{"confidence":0.95,"gaps":[],"refined_query":null,"stop":true}`}},
				"usage":   map[string]int{"input_tokens": 10, "output_tokens": 5},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": "Answer cites " + ungrounded + " which is fake."}},
				"usage":   map[string]int{"input_tokens": 10, "output_tokens": 5},
			})
		}
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	goodID := strings.Repeat("a", 31) + "1"
	recaller := &mockRecaller{
		calls: [][]types.SearchResult{
			{makeResult(goodID, "m1")},
		},
	}

	req := claude.ExploreRequest{
		Project:             "test",
		Question:            "q?",
		MaxIterations:       3,
		ConfidenceThreshold: 0.75,
		TokenBudget:         20000,
	}
	result, err := claude.Explore(context.Background(), c, recaller, nilRelGetter{}, req)
	require.NoError(t, err)
	require.NotContains(t, result.Answer, ungrounded)
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "ungrounded_citation_stripped") {
			found = true
			break
		}
	}
	require.True(t, found, "expected ungrounded_citation_stripped warning: %v", result.Warnings)
}

func TestExplore_EmptyRecall(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": `{"confidence":0.9,"gaps":[],"refined_query":null,"stop":true}`}},
				"usage":   map[string]int{"input_tokens": 5, "output_tokens": 3},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": "No relevant memories found."}},
				"usage":   map[string]int{"input_tokens": 5, "output_tokens": 3},
			})
		}
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	recaller := &mockRecaller{calls: [][]types.SearchResult{{}}}

	req := claude.ExploreRequest{
		Project:             "test",
		Question:            "q?",
		MaxIterations:       3,
		ConfidenceThreshold: 0.75,
		TokenBudget:         20000,
	}
	result, err := claude.Explore(context.Background(), c, recaller, nilRelGetter{}, req)
	require.NoError(t, err)
	require.Equal(t, 1, result.Iterations)
	require.NotEmpty(t, result.Answer)
}
