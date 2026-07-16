package search

// HyDE (Hypothetical Document Embeddings) — LME Experiment #8.
//
// Write-time: for each stored memory, a local LLM generates a hypothetical
// question that the memory would answer. That question is embedded and stored
// in the hyde_embeddings side table.
//
// Query-time: the real query is embedded and matched against HyDE embeddings
// rather than (or in addition to) raw chunk text. The HyDE scores are merged
// with raw chunk cosine scores via Reciprocal Rank Fusion (RRF).
//
// Target failure class: vocabulary_mismatch — queries that use different
// vocabulary than the memory text but share vocabulary with the hypothetical
// question generated at ingest time.
//
// Flag gate: ENGRAM_HYDE_ENABLED=false (default). When OFF, all HyDE paths
// are no-ops. When ON, the backend must implement the hydeBackend interface;
// if it does not (e.g. test stubs, pre-migration), HyDE is silently skipped.
//
// Atom composability: the hyde_embeddings table has memory_id FK. Once the
// atom layer (PR #986) adds atom rows, hyde indexing can be extended to
// atoms via a separate call to IndexHydeForMemory with atom content.
//
// DO NOT change the default from false without running a full LME benchmark.

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/types"
)

// HydeEnabled returns true when ENGRAM_HYDE_ENABLED=true.
// Default is false. All HyDE logic gates on this value.
func HydeEnabled() bool {
	return os.Getenv("ENGRAM_HYDE_ENABLED") == "true"
}

// hydeBackend is the optional-capability interface that a db.Backend must
// implement to participate in HyDE indexing and search. Using a separate
// interface (rather than extending db.Backend) avoids breaking all existing
// test stubs: at runtime we check via type assertion; if absent, HyDE is
// silently skipped. PostgresBackend (internal/db/postgres_hyde.go) implements
// this interface.
type hydeBackend interface {
	UpsertHydeEmbedding(ctx context.Context, memoryID, project, question string, embedding []float32) error
	HydeVectorSearch(ctx context.Context, project string, queryVec []float32, limit int) ([]db.HydeVectorHit, error)
}

// HydeQuestionGenerator generates a hypothetical question from memory content.
// The default implementation calls a local LLM; tests inject a stub.
type HydeQuestionGenerator interface {
	GenerateHydeQuestion(ctx context.Context, content string) (string, error)
}

// HydeScore is one result from a HyDE vector search.
type HydeScore struct {
	MemoryID string
	Score    float64 // cosine similarity (1 - distance)
	Question string  // the hypothetical question that matched
}

// --- HydeIndexer (write-time) ---

// HydeIndexer generates and stores HyDE embeddings for memories at write-time.
type HydeIndexer struct {
	backend   db.Backend
	embedder  embed.Client
	generator HydeQuestionGenerator
}

// NewHydeIndexer constructs a HydeIndexer. All three arguments are required.
func NewHydeIndexer(backend db.Backend, embedder embed.Client, gen HydeQuestionGenerator) *HydeIndexer {
	return &HydeIndexer{backend: backend, embedder: embedder, generator: gen}
}

// IndexHydeForMemory generates a hypothetical question for mem.Content,
// embeds it, and upserts the result into hyde_embeddings. It is a no-op when:
//   - mem.Content is empty
//   - the backend does not implement hydeBackend
//
// Called from SearchEngine.StoreWithRawBody after the memory + chunks are
// committed when HydeEnabled() is true.
func (h *HydeIndexer) IndexHydeForMemory(ctx context.Context, mem *types.Memory) error {
	if mem.Content == "" {
		return nil
	}
	hb, ok := h.backend.(hydeBackend)
	if !ok {
		return nil // backend doesn't support HyDE (e.g. test stubs, pre-migration)
	}

	question, err := h.generator.GenerateHydeQuestion(ctx, mem.Content)
	if err != nil {
		return err
	}
	if question == "" {
		return nil // LLM returned empty; skip rather than embed a zero-entropy vector
	}

	vec, err := h.embedder.Embed(ctx, question)
	if err != nil {
		return err
	}

	return hb.UpsertHydeEmbedding(ctx, mem.ID, mem.Project, question, vec)
}

// --- HydeScorer (query-time) ---

// HydeScorer performs ANN search on the hyde_embeddings index.
type HydeScorer struct {
	backend  db.Backend
	embedder embed.Client
}

// NewHydeScorer constructs a HydeScorer.
func NewHydeScorer(backend db.Backend, embedder embed.Client) *HydeScorer {
	return &HydeScorer{backend: backend, embedder: embedder}
}

// Score embeds query and searches the hyde_embeddings HNSW index.
// Returns HydeScore slice sorted descending by score.
// Returns an empty slice (no error) when the backend does not implement hydeBackend.
func (hs *HydeScorer) Score(ctx context.Context, query, project string, limit int) ([]HydeScore, error) {
	hb, ok := hs.backend.(hydeBackend)
	if !ok {
		return nil, nil
	}

	vec, err := hs.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	hits, err := hb.HydeVectorSearch(ctx, project, vec, limit)
	if err != nil {
		return nil, err
	}

	out := make([]HydeScore, 0, len(hits))
	for _, h := range hits {
		out = append(out, HydeScore{
			MemoryID: h.MemoryID,
			Score:    1.0 - h.Distance, // cosine similarity
			Question: h.Question,
		})
	}
	// Sort descending by score.
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// --- Reciprocal Rank Fusion ---

// mergeRRF combines raw chunk cosine scores with HyDE scores using
// Reciprocal Rank Fusion (RRF).
//
//	merged[id] = 1/(k + rawRank(id)) + 1/(k + hydeRank(id))
//
// k=60 is the standard RRF constant. IDs present in only one list receive
// the worst possible rank (len+1) in the missing list, not a zero contribution.
// This ensures a strong HyDE-only hit is not penalised by absence from the
// raw score list.
//
// Returns a map of memoryID → merged RRF score. The caller is responsible for
// sorting and truncating.
func mergeRRF(rawScores map[string]float64, hydeScores []HydeScore, k int) map[string]float64 {
	if k <= 0 {
		k = 60
	}

	// Build sorted raw rank.
	type kv struct {
		id    string
		score float64
	}
	rawSorted := make([]kv, 0, len(rawScores))
	for id, s := range rawScores {
		rawSorted = append(rawSorted, kv{id, s})
	}
	sort.Slice(rawSorted, func(i, j int) bool { return rawSorted[i].score > rawSorted[j].score })
	rawRank := make(map[string]int, len(rawSorted))
	for i, kv := range rawSorted {
		rawRank[kv.id] = i + 1
	}

	// Build HyDE rank.
	hydeRank := make(map[string]int, len(hydeScores))
	for i, h := range hydeScores {
		hydeRank[h.MemoryID] = i + 1
	}

	// Union of all IDs.
	allIDs := make(map[string]struct{}, len(rawScores)+len(hydeScores))
	for id := range rawScores {
		allIDs[id] = struct{}{}
	}
	for _, h := range hydeScores {
		allIDs[h.MemoryID] = struct{}{}
	}

	worstRaw := len(rawSorted) + 1
	worstHyde := len(hydeScores) + 1

	merged := make(map[string]float64, len(allIDs))
	for id := range allIDs {
		rr := rawRank[id]
		if rr == 0 {
			rr = worstRaw
		}
		hr := hydeRank[id]
		if hr == 0 {
			hr = worstHyde
		}
		merged[id] = 1.0/float64(k+rr) + 1.0/float64(k+hr)
	}
	return merged
}

// --- LLM-backed question generator ---

// LLMQuestionGenerator generates hypothetical questions using an LLMClient.
// The prompt instructs the model to produce one concise question that the
// memory content would answer. Long-form responses are truncated to 500 chars.
type LLMQuestionGenerator struct {
	client interface {
		Complete(ctx context.Context, system, user string) (string, error)
	}
}

const hydeSystemPrompt = `You are an indexing assistant. Given a memory fragment, output ONE concise question (≤25 words) that this memory would answer. Output only the question, no explanation.`

// NewLLMQuestionGenerator constructs a generator backed by any client that
// implements Complete(ctx, system, user) (string, error).
func NewLLMQuestionGenerator(client interface {
	Complete(ctx context.Context, system, user string) (string, error)
}) *LLMQuestionGenerator {
	return &LLMQuestionGenerator{client: client}
}

// GenerateHydeQuestion calls the LLM and returns the generated question.
// Trims whitespace and truncates to 500 characters.
func (g *LLMQuestionGenerator) GenerateHydeQuestion(ctx context.Context, content string) (string, error) {
	q, err := g.client.Complete(ctx, hydeSystemPrompt, content)
	if err != nil {
		return "", err
	}
	q = trimString(q, 500)
	return q, nil
}

func trimString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
