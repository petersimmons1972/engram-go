package claude_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/stretchr/testify/require"
)

func rerankClient(rt http.RoundTripper) *claude.Client {
	return claude.NewWithTransport("test-key", rt)
}

func TestRerankResults_ReordersItems(t *testing.T) {
	responseText := `[{"id":"id1","score":0.3},{"id":"id2","score":0.9},{"id":"id3","score":0.5}]`
	c := rerankClient(reasonRT(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Tools []struct {
				Type string `json:"type"`
			} `json:"tools"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		payload, _ := json.Marshal(textResponse(responseText))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))
	items := []claude.RerankItem{{ID: "id1", Summary: "first item", Score: 0.9}, {ID: "id2", Summary: "second item", Score: 0.8}, {ID: "id3", Summary: "third item", Score: 0.7}}
	results, err := c.RerankResults(context.Background(), "test query", items)
	require.NoError(t, err)
	require.Len(t, results, 3)
	scoreByID := make(map[string]float64)
	for _, r := range results {
		scoreByID[r.ID] = r.Score
	}
	require.Equal(t, 0.9, scoreByID["id2"])
	require.Equal(t, 0.3, scoreByID["id1"])
	require.Equal(t, 0.5, scoreByID["id3"])
}

func TestRerankResults_ExtrasPassThrough(t *testing.T) {
	c := rerankClient(reasonRT(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		var reranked []map[string]interface{}
		for i := 0; i < 20; i++ {
			reranked = append(reranked, map[string]interface{}{"id": fmt.Sprintf("item-%d", i), "score": 0.5})
		}
		b, _ := json.Marshal(reranked)
		payload, _ := json.Marshal(textResponse(string(b)))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))
	items := make([]claude.RerankItem, 25)
	for i := range items {
		items[i] = claude.RerankItem{ID: fmt.Sprintf("item-%d", i), Summary: fmt.Sprintf("summary %d", i), Score: float64(i) * 0.04}
	}
	results, err := c.RerankResults(context.Background(), "test query", items)
	require.NoError(t, err)
	require.Len(t, results, 25)
	for i, r := range results[20:] {
		idx := 20 + i
		require.Equal(t, fmt.Sprintf("item-%d", idx), r.ID)
		require.InDelta(t, float64(idx)*0.04, r.Score, 1e-9)
	}
}

func TestRerankResults_ParseError(t *testing.T) {
	c := rerankClient(reasonRT(func(*http.Request) (*http.Response, error) {
		payload, _ := json.Marshal(textResponse("totally not json"))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))
	items := []claude.RerankItem{{ID: "x1", Summary: "something", Score: 0.5}}
	_, err := c.RerankResults(context.Background(), "query", items)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse rerank")
}

func TestRerankResults_EmptyItems(t *testing.T) {
	handlerCalled := false
	c := rerankClient(reasonRT(func(*http.Request) (*http.Response, error) {
		handlerCalled = true
		return reasonJSON(http.StatusInternalServerError, ""), nil
	}))
	results, err := c.RerankResults(context.Background(), "query", []claude.RerankItem{})
	require.NoError(t, err)
	require.Nil(t, results)
	require.False(t, handlerCalled)
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
	c := rerankClient(reasonRT(func(r *http.Request) (*http.Response, error) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		payload, _ := json.Marshal(textResponse(`[{"id":"a1","score":0.8}]`))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))
	items := []claude.RerankItem{{ID: "a1", Summary: "test item", Score: 0.7}}
	_, err := c.RerankResults(context.Background(), "query", items)
	require.NoError(t, err)
	require.Len(t, capturedBody.Tools, 1)
	require.Equal(t, 1, capturedBody.Tools[0].MaxUses)
}
