package search

// cross_encoder_reranker.go — TEI cross-encoder reranker (LME LEVER-2).
//
// # Design
//
// After ANN/vector retrieval, this reranker re-scores the top-N dense
// candidates using a cross-encoder model (e.g. bge-reranker-v2-m3) that
// JOINTLY encodes (query, chunk) rather than using independent bi-encoder
// cosine similarity. The engine feeds the reranked dense signal into the
// hybrid scorer before BM25/recency fusion. This directly addresses the
// "compilation bottleneck":
// SmartSearch oracle analysis shows ~22.5% of gold evidence is lost to
// truncation, and cross-encoder reranking is the dominant SOTA lever
// (arXiv 2603.15599, +8–14pp).
//
// The implementation calls a TEI (Text Embeddings Inference) compatible
// /rerank endpoint:
//
//	POST /rerank
//	{"query": "...", "texts": ["chunk1", "chunk2", ...]}
//	→ [{"index": 0, "score": 0.92}, {"index": 1, "score": 0.31}, ...]
//
// This API is supported by:
//   - HuggingFace Text Embeddings Inference (TEI) — CPU build
//   - Infinity (SentenceTransformers serving)
//   - Any compatible endpoint
//
// # Hosting decision (ADV.1 resolved 2026-06-02)
//
// Option B was selected: configurable HTTP endpoint via ENGRAM_CROSS_ENCODER_URL.
// Rationale:
//   - Avoids re-saturating the MI50 GPU embed path (circuit breaker history #917/#1000)
//   - Decoupled from Olla's unknown /v1/rerank capability surface
//   - Operator defers standing up the sidecar until after interface is validated
//   - Default docker run: docker run -p 6006:80
//       ghcr.io/huggingface/text-embeddings-inference:cpu-1.2
//       --model-id BAAI/bge-reranker-v2-m3
//
// # Flag
//
// Feature-gated by ENGRAM_CROSS_ENCODER_RERANK=true|1 (default OFF).
// The endpoint URL is configured via ENGRAM_CROSS_ENCODER_URL.
// When the URL is empty, the reranker is disabled even if the flag is on.
//
// Set RecallOpts.DenseReranker = NewCrossEncoderRerankerFromEnv() at the call
// site; when the flag is off (or URL unset), the function returns nil and the
// hook is a no-op — baseline scores and ordering are unchanged.
//
// # Ablation bench command
//
//	ENGRAM_CROSS_ENCODER_RERANK=true \
//	ENGRAM_CROSS_ENCODER_URL=http://localhost:6006/rerank \
//	./longmemeval run \
//	  --data ~/path/to/longmemeval_m_cleaned.json \
//	  --url http://localhost:8788 \
//	  --llm-url "${LME_LLM_URL}" \
//	  --llm-model "${LME_LLM_MODEL}" \
//	  --workers 8 \
//	  --out ~/benchmarks/lme-lever2-cross-encoder \
//	  --recall-topk 100 \
//	  --context-topk 8 \
//	  --run-id lever2-$(date +%Y%m%dT%H%M%S)
//
// Use a fresh run_id. Never re-use run_id 7a87fd (golden baseline).
// Compare against golden-snapshot-20260602T1810Z/ (367/566 CORRECT).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// defaultCrossEncoderTimeout is the per-call HTTP timeout for the rerank endpoint.
	// 8 candidates × ~25ms each ≈ 200ms typical; 2s gives ample headroom for
	// a cold CPU sidecar without blocking the recall path excessively.
	defaultCrossEncoderTimeout = 2 * time.Second
)

// teiRerankRequest is the payload sent to the TEI /rerank endpoint.
type teiRerankRequest struct {
	Query string   `json:"query"`
	Texts []string `json:"texts"`
}

// teiRerankResponse is one scored entry from the TEI /rerank response.
type teiRerankResponse struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
}

// CrossEncoderReranker implements ResultReranker using a TEI-compatible
// /rerank HTTP endpoint. The reranker jointly encodes (query, chunk) pairs
// rather than using independent bi-encoder cosine similarity.
//
// Instantiate via NewCrossEncoderReranker or NewCrossEncoderRerankerFromEnv.
type CrossEncoderReranker struct {
	endpoint string
	timeout  time.Duration
	client   *http.Client
}

// NewCrossEncoderReranker returns a CrossEncoderReranker that calls the given
// TEI /rerank endpoint URL. endpoint must be the full path including /rerank,
// e.g. "http://localhost:6006/rerank".
func NewCrossEncoderReranker(endpoint string) *CrossEncoderReranker {
	return &CrossEncoderReranker{
		endpoint: endpoint,
		timeout:  defaultCrossEncoderTimeout,
		client:   &http.Client{Timeout: defaultCrossEncoderTimeout},
	}
}

// IsCrossEncoderRerankerEnabled returns true when ENGRAM_CROSS_ENCODER_RERANK
// is set to "true" or "1" (case-insensitive). Default is OFF.
//
// Evaluated at call time (not cached) so t.Setenv works in tests and the
// flag can be toggled at runtime without restart.
func IsCrossEncoderRerankerEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ENGRAM_CROSS_ENCODER_RERANK")))
	return v == "true" || v == "1"
}

// NewCrossEncoderRerankerFromEnv returns a *CrossEncoderReranker when:
//   - ENGRAM_CROSS_ENCODER_RERANK is "true" or "1", AND
//   - ENGRAM_CROSS_ENCODER_URL is non-empty.
//
// Returns nil (no-op) when either condition is not met. Assign directly to
// RecallOpts.DenseReranker:
//
//	opts := search.RecallOpts{
//	    DenseReranker: search.NewCrossEncoderRerankerFromEnv(),
//	}
//
// When nil, RecallWithOpts skips dense-leg reranking entirely — baseline unchanged.
func NewCrossEncoderRerankerFromEnv() ResultReranker {
	if !IsCrossEncoderRerankerEnabled() {
		return nil
	}
	url := strings.TrimSpace(os.Getenv("ENGRAM_CROSS_ENCODER_URL"))
	if url == "" {
		slog.Warn("cross-encoder rerank enabled but ENGRAM_CROSS_ENCODER_URL is empty — skipping")
		return nil
	}
	return NewCrossEncoderReranker(url)
}

// RerankResults calls the TEI /rerank endpoint with the query and item summaries,
// then maps the cross-encoder scores back to RerankResult by item ID.
//
// The TEI endpoint receives items in the same order they appear in items[];
// the response assigns scores by index. This mapping is stable regardless of
// what score order the endpoint returns results in.
//
// On HTTP or parse error, the error is returned and the caller falls through
// to the existing bi-encoder ordering for that dense candidate set.
func (r *CrossEncoderReranker) RerankResults(
	ctx context.Context,
	query string,
	items []RerankItem,
) ([]RerankResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	texts := make([]string, len(items))
	for i, it := range items {
		texts[i] = it.Summary
	}

	reqBody, err := json.Marshal(teiRerankRequest{Query: query, Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("cross-encoder: marshal request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, r.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("cross-encoder: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cross-encoder: HTTP request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cross-encoder: endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cross-encoder: read response: %w", err)
	}

	var teiResults []teiRerankResponse
	if err := json.Unmarshal(body, &teiResults); err != nil {
		return nil, fmt.Errorf("cross-encoder: parse response: %w", err)
	}

	// Map index → cross-encoder score.
	scoreByIndex := make(map[int]float64, len(teiResults))
	for _, tr := range teiResults {
		scoreByIndex[tr.Index] = tr.Score
	}

	results := make([]RerankResult, len(items))
	for i, it := range items {
		score, ok := scoreByIndex[i]
		if !ok {
			// Index missing from response: fall back to original score.
			slog.Warn("cross-encoder: missing index in response, using original score",
				"index", i, "id", it.ID)
			score = it.Score
		}
		results[i] = RerankResult{ID: it.ID, Score: score}
	}

	return results, nil
}

// Compile-time check: CrossEncoderReranker satisfies ResultReranker.
var _ ResultReranker = (*CrossEncoderReranker)(nil)
