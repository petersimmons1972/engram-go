package search_test

// cross_encoder_reranker_test.go — TDD tests for the TEI cross-encoder reranker.
//
// Test strategy: all tests use a stub HTTP server — no live model required.
// The stub simulates the TEI /rerank response so unit tests are deterministic
// and run offline.
//
// Covered behaviours:
//   1. Flag OFF  -> NewCrossEncoderRerankerFromEnv returns nil (no-op in RecallOpts)
//   2. Flag ON, URL empty  -> returns nil (no-op; URL not configured yet)
//   3. Stub server: reranker promotes a jointly-relevant chunk that bi-encoder
//      ranked below cutoff -- verifies the rerank order changes
//   4. Stub server returns error -> RerankResults surfaces the error
//   5. Empty items slice -> nil, nil returned without HTTP call

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
)

// teiRerankItem mirrors the TEI /rerank response entry.
type teiRerankItem struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
}

// newTEIStub starts an httptest.Server that serves fixed scores for the items
// it receives. scores maps item-index to cross-encoder score. Items not in the
// map get score 0.0.
func newTEIStub(t *testing.T, scores map[int]float64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rerank" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var req struct {
			Query string   `json:"query"`
			Texts []string `json:"texts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		results := make([]teiRerankItem, len(req.Texts))
		for i := range req.Texts {
			s := 0.0
			if v, ok := scores[i]; ok {
				s = v
			}
			results[i] = teiRerankItem{Index: i, Score: s}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(results)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Test 1: flag OFF -> NewCrossEncoderRerankerFromEnv returns nil.
func TestCrossEncoderRerankerFromEnv_FlagOff(t *testing.T) {
	t.Setenv("ENGRAM_CROSS_ENCODER_RERANK", "false")
	t.Setenv("ENGRAM_CROSS_ENCODER_URL", "http://localhost:9999/rerank")
	r := search.NewCrossEncoderRerankerFromEnv()
	if r != nil {
		t.Errorf("expected nil reranker when flag is OFF, got %T", r)
	}
}

// Test 2: flag ON, URL empty -> NewCrossEncoderRerankerFromEnv returns nil.
func TestCrossEncoderRerankerFromEnv_FlagOnURLEmpty(t *testing.T) {
	t.Setenv("ENGRAM_CROSS_ENCODER_RERANK", "true")
	t.Setenv("ENGRAM_CROSS_ENCODER_URL", "")
	r := search.NewCrossEncoderRerankerFromEnv()
	if r != nil {
		t.Errorf("expected nil reranker when URL is empty, got %T", r)
	}
}

// Test 3: stub server reranks - jointly-relevant chunk promoted above bi-encoder rank.
//
// Setup: 3 items. Bi-encoder scored them [0.90, 0.80, 0.70] (item-0 top).
// The TEI stub says item-2 is jointly most relevant (score 0.95), item-0 second (0.60), item-1 last (0.40).
// After reranking, the order should be [item-2, item-0, item-1].
func TestCrossEncoderReranker_PromotesJointlyRelevantChunk(t *testing.T) {
	srv := newTEIStub(t, map[int]float64{
		0: 0.60,
		1: 0.40,
		2: 0.95,
	})

	r := search.NewCrossEncoderReranker(srv.URL + "/rerank")

	items := []search.RerankItem{
		{ID: "mem-0", Summary: "user prefers morning coffee", Score: 0.90},
		{ID: "mem-1", Summary: "user owns a blue car", Score: 0.80},
		{ID: "mem-2", Summary: "user drinks coffee every morning before work", Score: 0.70},
	}

	results, err := r.RerankResults(context.Background(), "what does the user drink in the morning?", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Sort by score desc to find ordering.
	sorted := make([]search.RerankResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })

	if sorted[0].ID != "mem-2" {
		t.Errorf("expected mem-2 ranked first (jointly relevant), got %s (score=%.3f)", sorted[0].ID, sorted[0].Score)
	}
	if sorted[1].ID != "mem-0" {
		t.Errorf("expected mem-0 ranked second, got %s", sorted[1].ID)
	}
	if sorted[2].ID != "mem-1" {
		t.Errorf("expected mem-1 ranked last, got %s", sorted[2].ID)
	}
}

// Test 4: stub server HTTP error -> RerankResults returns non-nil error.
func TestCrossEncoderReranker_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	r := search.NewCrossEncoderReranker(srv.URL + "/rerank")
	items := []search.RerankItem{
		{ID: "mem-0", Summary: "anything", Score: 0.5},
	}
	_, err := r.RerankResults(context.Background(), "query", items)
	if err == nil {
		t.Error("expected error from 500 response, got nil")
	}
}

// Test 5: empty items slice -> returns nil, nil (no HTTP call made).
func TestCrossEncoderReranker_EmptyItems(t *testing.T) {
	// Use a server that fails if called -- proves no HTTP call is made.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected HTTP call with empty items")
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	r := search.NewCrossEncoderReranker(srv.URL + "/rerank")
	results, err := r.RerankResults(context.Background(), "query", nil)
	if err != nil {
		t.Errorf("unexpected error with empty items: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty items, got %v", results)
	}
}

// Test 6: IsCrossEncoderRerankerEnabled reflects env var correctly.
func TestIsCrossEncoderRerankerEnabled(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"1", true},
		{"TRUE", true},
		{"false", false},
		{"0", false},
		{"", false},
	}
	for _, c := range cases {
		t.Run(c.val, func(t *testing.T) {
			t.Setenv("ENGRAM_CROSS_ENCODER_RERANK", c.val)
			got := search.IsCrossEncoderRerankerEnabled()
			if got != c.want {
				t.Errorf("IsCrossEncoderRerankerEnabled() = %v, want %v (env=%q)", got, c.want, c.val)
			}
		})
	}
}
