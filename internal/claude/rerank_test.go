package claude_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/stretchr/testify/require"
)

func TestRerankResults_ReordersItems(t *testing.T) {
	// Server returns re-scored values: id1→0.3, id2→0.9, id3→0.5
	// The method returns them as-is (sorting happens in engine.go).
	responseText := `[{"id":"id1","score":0.3},{"id":"id2","score":0.9},{"id":"id3","score":0.5}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": responseText},
			},
		})
	}))
	defer srv.Close()

	c := makeClient(t, srv.URL)

	items := []claude.RerankItem{
		{ID: "id1", Summary: "first item", Score: 0.9},
		{ID: "id2", Summary: "second item", Score: 0.8},
		{ID: "id3", Summary: "third item", Score: 0.7},
	}

	results, err := c.RerankResults(context.Background(), "test query", items)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// id2 got the highest score (0.9) from Claude — verify it's present in results
	// with the correct score. Results are returned in Claude's order.
	scoreByID := make(map[string]float64)
	for _, r := range results {
		scoreByID[r.ID] = r.Score
	}
	require.Equal(t, 0.9, scoreByID["id2"], "id2 should have score 0.9 from Claude")
	require.Equal(t, 0.3, scoreByID["id1"], "id1 should have score 0.3 from Claude")
	require.Equal(t, 0.5, scoreByID["id3"], "id3 should have score 0.5 from Claude")
}

func TestRerankResults_ExtrasPassThrough(t *testing.T) {
	// Server responds with 20 re-ranked results (the cap).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decode request to verify only 20 items were sent.
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		// Build 20 RerankResult entries for the response.
		var reranked []map[string]interface{}
		for i := 0; i < 20; i++ {
			reranked = append(reranked, map[string]interface{}{
				"id":    fmt.Sprintf("item-%d", i),
				"score": 0.5,
			})
		}
		responseBytes, _ := json.Marshal(reranked)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": string(responseBytes)},
			},
		})
	}))
	defer srv.Close()

	c := makeClient(t, srv.URL)

	// Build 25 items.
	items := make([]claude.RerankItem, 25)
	for i := range items {
		items[i] = claude.RerankItem{
			ID:      fmt.Sprintf("item-%d", i),
			Summary: fmt.Sprintf("summary %d", i),
			Score:   float64(i) * 0.04,
		}
	}

	results, err := c.RerankResults(context.Background(), "test query", items)
	require.NoError(t, err)
	require.Len(t, results, 25, "first 20 re-ranked + 5 extras = 25 total")

	// The last 5 results should be the extras with their original scores.
	for i, r := range results[20:] {
		idx := 20 + i
		require.Equal(t, fmt.Sprintf("item-%d", idx), r.ID)
		require.InDelta(t, float64(idx)*0.04, r.Score, 1e-9, "extra item should retain original score")
	}
}

func TestRerankResults_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": "totally not json"},
			},
		})
	}))
	defer srv.Close()

	c := makeClient(t, srv.URL)

	items := []claude.RerankItem{
		{ID: "x1", Summary: "something", Score: 0.5},
	}

	_, err := c.RerankResults(context.Background(), "query", items)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse rerank")
}

func TestRerankResults_EmptyItems(t *testing.T) {
	handlerCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := makeClient(t, srv.URL)

	results, err := c.RerankResults(context.Background(), "query", []claude.RerankItem{})
	require.NoError(t, err)
	require.Nil(t, results)
	require.False(t, handlerCalled, "HTTP handler should never be called for empty items slice")
}

func TestRerankResults_AdvisorMaxUsesIsOne(t *testing.T) {
	var capturedBody struct {
		Tools []struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Model   string `json:"model"`
			MaxUses int    `json:"max_uses"`
		} `json:"tools"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": `[{"id":"a1","score":0.8}]`},
			},
		})
	}))
	defer srv.Close()

	c := makeClient(t, srv.URL)

	items := []claude.RerankItem{
		{ID: "a1", Summary: "test item", Score: 0.7},
	}

	_, err := c.RerankResults(context.Background(), "query", items)
	require.NoError(t, err)
	require.Len(t, capturedBody.Tools, 1)
	require.Equal(t, 1, capturedBody.Tools[0].MaxUses, "rerank uses advisorMaxUses=1 (lighter task)")
}
