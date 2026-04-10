package claude

import (
	"context"
	"encoding/json"
	"fmt"
)

const maxRerankItems = 20

// RerankItem is one result item to re-rank.
type RerankItem struct {
	ID      string  `json:"id"`
	Summary string  `json:"summary"` // truncated to 500 chars
	Score   float64 `json:"score"`
}

// RerankResult is Claude's re-scored result for one item.
type RerankResult struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"` // [0,1] assigned by Claude
}

const rerankSystem = "You are a relevance re-ranking engine. Given a query and a list of memory items with initial scores, assign each item a relevance score from 0 to 1 where 1 is maximally relevant to the query. Be precise and discriminating. Respond with a JSON array of re-ranked results."

// RerankResults asks Claude to re-score the provided items against query.
// Returns nil, nil when items is empty (no HTTP call is made).
// At most maxRerankItems (20) items are sent to Claude; any extras are appended
// at the end of the result with their original scores unchanged.
func (c *Client) RerankResults(ctx context.Context, query string, items []RerankItem) ([]RerankResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	// Stash extras beyond the cap.
	var extras []RerankItem
	if len(items) > maxRerankItems {
		extras = items[maxRerankItems:]
		items = items[:maxRerankItems]
	}

	prompt := buildRerankPrompt(query, items)

	raw, err := c.Complete(ctx, rerankSystem, prompt, "claude-sonnet-4-6", "claude-opus-4-6", 1, 1024)
	if err != nil {
		return nil, err
	}

	cleaned := extractJSON(raw)

	var results []RerankResult
	if err := json.Unmarshal([]byte(cleaned), &results); err != nil {
		return nil, fmt.Errorf("parse rerank: %w", err)
	}

	// Append extras with their original scores.
	for _, e := range extras {
		results = append(results, RerankResult{ID: e.ID, Score: e.Score})
	}

	return results, nil
}

// buildRerankPrompt constructs the prompt with query and items as JSON.
func buildRerankPrompt(query string, items []RerankItem) string {
	itemsJSON, _ := json.Marshal(items)
	return fmt.Sprintf("Query: %s\n\nItems:\n%s", query, string(itemsJSON))
}
