package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
)

const (
	rerankBatchSize = 50
	rerankTimeout   = 120 * time.Second
)

// Reranker implements search.ResultReranker using an Ollama generation model.
// It sends items in batches, asks the model to score each (query, passage) pair
// 0.0–1.0, and returns the re-scored results. Falls back to original scores on
// any batch error so a partial failure never silently drops results.
type Reranker struct {
	client *Client
	model  string
}

// NewReranker returns a Reranker backed by the given Ollama client and model.
func NewReranker(client *Client, model string) *Reranker {
	return &Reranker{client: client, model: model}
}

// RerankResults implements search.ResultReranker.
func (r *Reranker) RerankResults(ctx context.Context, query string, items []search.RerankItem) ([]search.RerankResult, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]search.RerankResult, 0, len(items))
	for i := 0; i < len(items); i += rerankBatchSize {
		end := i + rerankBatchSize
		if end > len(items) {
			end = len(items)
		}
		batch := items[i:end]
		results, err := r.rerankBatch(ctx, query, batch)
		if err != nil {
			slog.Warn("ollama rerank batch failed, keeping original scores",
				"model", r.model, "batch_start", i, "err", err)
			for _, it := range batch {
				out = append(out, search.RerankResult{ID: it.ID, Score: it.Score})
			}
			continue
		}
		out = append(out, results...)
	}
	return out, nil
}

type rerankPassage struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type rerankScore struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

func (r *Reranker) rerankBatch(ctx context.Context, query string, items []search.RerankItem) ([]search.RerankResult, error) {
	passages := make([]rerankPassage, len(items))
	for i, it := range items {
		text := it.Summary
		if len(text) > 400 {
			text = text[:400]
		}
		passages[i] = rerankPassage{ID: it.ID, Text: text}
	}
	passagesJSON, _ := json.Marshal(passages)

	userMsg := fmt.Sprintf(
		"Query: %s\n\nPassages:\n%s\n\nReturn a JSON array with one object per passage: [{\"id\":\"<id>\",\"score\":<0.0-1.0>},...]. Score 1.0 = maximally relevant to query, 0.0 = irrelevant. Include every passage id.",
		query, string(passagesJSON),
	)

	// Use Background context so the reranker isn't cancelled by the MCP request
	// deadline — qwen3:8b can take 30-60s per batch, which exceeds typical write
	// timeouts. Same pattern as embed.Client.Embed in engine.go.
	batchCtx, cancel := context.WithTimeout(context.Background(), rerankTimeout)
	defer cancel()

	resp, err := r.client.Chat(batchCtx, ChatRequest{
		Model: r.model,
		Messages: []Message{
			{Role: "system", Content: "You are a relevance scorer. Given a query and passages, score each passage's relevance to the query from 0.0 to 1.0. Respond with only a JSON array."},
			{Role: "user", Content: userMsg},
		},
		Stream: false,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama chat: %w", err)
	}

	raw := strings.TrimSpace(resp.Message.Content)
	// Strip optional markdown code fences.
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var scored []rerankScore
	if err := json.Unmarshal([]byte(raw), &scored); err != nil {
		return nil, fmt.Errorf("parse rerank response: %w (raw: %.200s)", err, raw)
	}

	// Build lookup and return results in original order.
	scoreMap := make(map[string]float64, len(scored))
	for _, s := range scored {
		scoreMap[s.ID] = s.Score
	}
	out := make([]search.RerankResult, len(items))
	for i, it := range items {
		score, ok := scoreMap[it.ID]
		if !ok {
			score = it.Score // model omitted this id — keep original
		}
		out[i] = search.RerankResult{ID: it.ID, Score: score}
	}
	return out, nil
}
