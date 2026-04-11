package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/petersimmons1972/engram/internal/chunk"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/minhash"
	"github.com/petersimmons1972/engram/internal/reembed"
	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/petersimmons1972/engram/internal/types"
)

// MergeReviewer reviews candidate near-duplicate pairs and returns merge decisions.
// Implemented by *claude.Client via an adapter in internal/mcp; declared here to avoid import cycle.
type MergeReviewer interface {
	ReviewMergeCandidates(ctx context.Context, candidates []MergeCandidate) ([]MergeDecision, error)
}

// ResultReranker re-ranks recall results using an external model.
// Implemented by an adapter in the mcp package; declared here to avoid import cycle.
type ResultReranker interface {
	RerankResults(ctx context.Context, query string, items []RerankItem) ([]RerankResult, error)
}

// RerankItem is a memory result passed to the reranker.
type RerankItem struct {
	ID      string  `json:"id"`
	Summary string  `json:"summary"`
	Score   float64 `json:"score"`
}

// RerankResult is the re-ranked score for one item.
type RerankResult struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// RecallOpts controls optional post-processing for Recall.
type RecallOpts struct {
	Reranker ResultReranker // nil = skip re-ranking
}

// MergeCandidate is a pair of memories with their similarity score.
type MergeCandidate struct {
	MemoryA    *types.Memory
	MemoryB    *types.Memory
	Similarity float64
}

// MergeDecision is the reviewer's verdict on whether to merge a candidate pair.
type MergeDecision struct {
	MemoryAID     string `json:"memory_a_id"`
	MemoryBID     string `json:"memory_b_id"`
	ShouldMerge   bool   `json:"should_merge"`
	Reason        string `json:"reason"`
	MergedContent string `json:"merged_content,omitempty"`
}

// SearchEngine is the core retrieval engine: it stores memories (chunked + embedded)
// and recalls them via composite vector+FTS scoring.
type SearchEngine struct {
	backend    db.Backend
	embedder   embed.Client
	project    string
	ollamaURL  string
	summarizer *summarize.Worker
	reembedder *reembed.Worker
}

// New constructs a SearchEngine and starts background summarize and reembed workers.
// claudeClient may be nil, in which case summarization falls back to Ollama.
func New(ctx context.Context, backend db.Backend, embedder embed.Client, project string,
	ollamaURL, summarizeModel string, summarizeEnabled bool,
	claudeClient summarize.ClaudeCompleter) *SearchEngine {

	sum := summarize.NewWorkerWithClaude(backend, project, ollamaURL, summarizeModel, summarizeEnabled, claudeClient)
	sum.Start()

	reb := reembed.NewWorkerFromMeta(ctx, backend, embedder, project)
	reb.Start()

	return &SearchEngine{
		backend:    backend,
		embedder:   embedder,
		project:    project,
		ollamaURL:  ollamaURL,
		summarizer: sum,
		reembedder: reb,
	}
}

// Close shuts down the engine and stops all background workers.
func (e *SearchEngine) Close() {
	e.summarizer.Stop()
	e.reembedder.Stop()
}

// Backend exposes the underlying db.Backend for callers that need direct access
// (e.g. EnginePool, MCP tool handlers).
func (e *SearchEngine) Backend() db.Backend { return e.backend }

// storeChunksForMemory chunks content, embeds each chunk, and returns the new
// chunk records ready for storage. It is used by both Store (new memories) and
// Correct (re-chunking after a content change). Embedding is done outside any
// transaction because it is slow; callers are responsible for writing the
// returned chunks inside a transaction.
func (e *SearchEngine) storeChunksForMemory(ctx context.Context, m *types.Memory) ([]*types.Chunk, error) {
	// Produce chunk candidates. ChunkDocument returns []ChunkCandidate (with heading
	// metadata). ChunkText returns plain []string which we wrap into candidates.
	var candidates []chunk.ChunkCandidate
	if m.StorageMode == "document" {
		candidates = chunk.ChunkDocument(m.Content, 0 /* use package default */)
	} else {
		// ChunkText(text, maxTokens, overlapTokens). Use same defaults as Python:
		// 512 max tokens, 50 overlap.
		for _, text := range chunk.ChunkText(m.Content, 512, 50) {
			candidates = append(candidates, chunk.ChunkCandidate{
				Text:      text,
				ChunkType: "sentence_window",
			})
		}
	}

	// If ChunkText produced nothing (empty content edge case), store content as one chunk.
	if len(candidates) == 0 {
		candidates = []chunk.ChunkCandidate{{Text: m.Content, ChunkType: "sentence_window"}}
	}

	var chunks []*types.Chunk
	for i, c := range candidates {
		hash := chunk.ChunkHash(c.Text)

		exists, err := e.backend.ChunkHashExists(ctx, hash, m.ID)
		if err != nil {
			return nil, fmt.Errorf("check chunk hash: %w", err)
		}
		if exists {
			continue
		}

		embedding, err := e.embedder.Embed(ctx, c.Text)
		if err != nil {
			return nil, fmt.Errorf("embed chunk %d: %w", i, err)
		}

		ch := &types.Chunk{
			ID:         types.NewMemoryID(),
			MemoryID:   m.ID,
			ChunkText:  c.Text,
			ChunkIndex: i,
			ChunkHash:  hash,
			ChunkType:  c.ChunkType,
			Project:    e.project,
			Embedding:  embedding,
		}
		if c.HasHeading {
			heading := c.SectionHeading
			ch.SectionHeading = &heading
		}
		chunks = append(chunks, ch)
	}
	return chunks, nil
}

// Store persists a memory: sets defaults, chunks content, deduplicates by hash,
// embeds new chunks, and writes everything inside a single transaction.
func (e *SearchEngine) Store(ctx context.Context, m *types.Memory) error {
	if m.ID == "" {
		m.ID = types.NewMemoryID()
	}
	m.Project = e.project

	if m.StorageMode == "" {
		if len(m.Content) > 10_000 {
			m.StorageMode = "document"
		} else {
			m.StorageMode = "focused"
		}
	}

	if err := e.checkEmbedderMeta(ctx); err != nil {
		return err
	}

	chunks, err := e.storeChunksForMemory(ctx, m)
	if err != nil {
		return err
	}

	tx, err := e.backend.Begin(ctx)
	if err != nil {
		return err
	}
	if err := e.backend.StoreMemoryTx(ctx, tx, m); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if len(chunks) > 0 {
		if err := e.backend.StoreChunksTx(ctx, tx, chunks); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
	}
	return tx.Commit(ctx)
}

// Recall retrieves the topK most relevant memories for query.
func (e *SearchEngine) Recall(ctx context.Context, query string, topK int, detail string) ([]types.SearchResult, error) {
	return e.RecallWithOpts(ctx, query, topK, detail, RecallOpts{})
}

// RecallWithOpts retrieves the topK most relevant memories for query, using composite
// vector+FTS scoring via the pgvector HNSW index. detail controls content truncation:
// "id_only", "summary", or "full" (default). opts allows optional post-processing
// such as re-ranking.
func (e *SearchEngine) RecallWithOpts(ctx context.Context, query string, topK int, detail string, opts RecallOpts) ([]types.SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	queryVec, err := e.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// ANN vector search via pgvector HNSW index.
	vecHits, err := e.backend.VectorSearch(ctx, e.project, queryVec, topK*3)
	if err != nil {
		return nil, err
	}

	// Fan-out FTS search concurrently.
	type ftsResult struct {
		results []db.FTSResult
		err     error
	}
	ftsCh := make(chan ftsResult, 1)
	go func() {
		res, err := e.backend.FTSSearch(ctx, e.project, query, topK*3, nil, nil)
		ftsCh <- ftsResult{res, err}
	}()

	// Build per-memory best cosine from vector hits.
	// pgvector returns cosine distance (0-2); convert to similarity (1-0).
	bestCosine := make(map[string]float64)
	bestChunkText := make(map[string]string)
	bestChunkIndex := make(map[string]int)
	bestChunkSection := make(map[string]*string)
	bestChunkID := make(map[string]string)
	uniqueIDs := make([]string, 0, len(vecHits))
	seen := make(map[string]bool, len(vecHits))

	for _, h := range vecHits {
		cosine := 1.0 - h.Distance
		if cosine > bestCosine[h.MemoryID] {
			bestCosine[h.MemoryID] = cosine
			bestChunkText[h.MemoryID] = h.ChunkText
			bestChunkIndex[h.MemoryID] = h.ChunkIndex
			bestChunkSection[h.MemoryID] = h.SectionHeading
			bestChunkID[h.MemoryID] = h.ChunkID
		}
		if !seen[h.MemoryID] {
			seen[h.MemoryID] = true
			uniqueIDs = append(uniqueIDs, h.MemoryID)
		}
	}

	// Batch-fetch memory records for vector hits.
	batchMems, err := e.backend.GetMemoriesByIDs(ctx, e.project, uniqueIDs)
	if err != nil {
		return nil, err
	}
	memories := make(map[string]*types.Memory, len(batchMems))
	for _, m := range batchMems {
		memories[m.ID] = m
	}

	// Merge FTS results.
	ftsRes := <-ftsCh
	if ftsRes.err != nil {
		return nil, ftsRes.err
	}
	ftsScores := make(map[string]float64)
	maxBM25 := 0.0
	for _, r := range ftsRes.results {
		ftsScores[r.Memory.ID] = r.Score
		if r.Score > maxBM25 {
			maxBM25 = r.Score
		}
		memories[r.Memory.ID] = r.Memory
	}

	// Composite scoring per memory.
	var results []types.SearchResult
	for id, m := range memories {
		bm25 := 0.0
		if maxBM25 > 0 {
			bm25 = ftsScores[id] / maxBM25
		}
		input := ScoreInput{
			Cosine:     bestCosine[id],
			BM25:       bm25,
			HoursSince: hoursSince(m.LastAccessed),
			Importance: m.Importance,
		}
		score := CompositeScore(input)

		result := types.SearchResult{
			Memory:     m,
			Score:      score,
			ChunkScore: bestCosine[id],
			ScoreBreakdown: map[string]float64{
				"cosine":  bestCosine[id],
				"bm25":    bm25,
				"recency": RecencyDecay(input.HoursSince),
			},
			MatchedChunk:        bestChunkText[id],
			MatchedChunkIndex:   bestChunkIndex[id],
			MatchedChunkSection: bestChunkSection[id],
		}
		switch detail {
		case "id_only":
			result.Memory = &types.Memory{ID: m.ID}
		case "summary":
			if m.Summary != nil {
				result.Memory.Content = *m.Summary
			} else {
				content := m.Content
				if len(content) > 500 {
					content = content[:500] + "…"
				}
				result.Memory.Content = content
			}
		}
		results = append(results, result)
	}

	sortResults(results)

	// Optional re-ranking (unchanged).
	if opts.Reranker != nil && len(results) > 0 {
		items := make([]RerankItem, len(results))
		for i, r := range results {
			summary := ""
			if r.Memory.Summary != nil {
				summary = *r.Memory.Summary
			} else {
				summary = r.Memory.Content
				if len(summary) > 500 {
					summary = summary[:500]
				}
			}
			items[i] = RerankItem{
				ID:      r.Memory.ID,
				Summary: summary,
				Score:   r.Score,
			}
		}
		reranked, err := opts.Reranker.RerankResults(ctx, query, items)
		if err == nil && len(reranked) > 0 {
			scoreMap := make(map[string]float64, len(reranked))
			for _, rr := range reranked {
				scoreMap[rr.ID] = rr.Score
			}
			for i := range results {
				if newScore, ok := scoreMap[results[i].Memory.ID]; ok {
					results[i].Score = newScore
				}
			}
			sortResults(results)
		}
	}

	if len(results) > topK {
		results = results[:topK]
	}

	// Fetch relationships only for the final topK results, not for every
	// candidate. This reduces DB round-trips from up to topK*6 to exactly topK.
	for i := range results {
		if rels, err := e.backend.GetRelationships(ctx, e.project, results[i].Memory.ID); err == nil {
			results[i].Connected = toConnectedMemories(rels, results[i].Memory.ID)
		}
	}

	// Update access timestamps.
	for _, r := range results {
		if err := e.backend.TouchMemory(ctx, r.Memory.ID); err != nil {
			slog.Warn("touch memory failed", "id", r.Memory.ID, "err", err)
		}
		if chunkID, ok := bestChunkID[r.Memory.ID]; ok {
			_ = e.backend.UpdateChunkLastMatched(ctx, chunkID)
		}
	}

	return results, nil
}

// checkEmbedderMeta ensures the stored embedder name matches the current client,
// or registers it if this is the first store for the project.
func (e *SearchEngine) checkEmbedderMeta(ctx context.Context) error {
	storedName, ok, err := e.backend.GetMeta(ctx, e.project, "embedder_name")
	if err != nil {
		return err
	}
	if !ok {
		if err := e.backend.SetMeta(ctx, e.project, "embedder_name", e.embedder.Name()); err != nil {
			return err
		}
		return e.backend.SetMeta(ctx, e.project, "embedder_dimensions",
			fmt.Sprintf("%d", e.embedder.Dimensions()))
	}
	if storedName != e.embedder.Name() {
		return fmt.Errorf("embedder mismatch: stored=%q current=%q — run memory_migrate_embedder first",
			storedName, e.embedder.Name())
	}
	// Skip dimension check if migration is in progress — the new model may have
	// different dimensions, and embedder_dimensions will be reset once re-embedding
	// completes.
	migrating, _, _ := e.backend.GetMeta(ctx, e.project, "embedding_migration_in_progress")
	if migrating == "true" {
		return nil
	}
	storedDimsStr, ok, err := e.backend.GetMeta(ctx, e.project, "embedder_dimensions")
	if err != nil {
		return err
	}
	if ok {
		storedDims, err := strconv.Atoi(storedDimsStr)
		if err != nil {
			return fmt.Errorf("embedder_dimensions metadata is corrupt: %w", err)
		}
		if storedDims != e.embedder.Dimensions() {
			return fmt.Errorf("embedder dimensions mismatch: stored %d, current %d — use memory_migrate_embedder to switch models",
				storedDims, e.embedder.Dimensions())
		}
	}
	return nil
}

func hoursSince(t time.Time) float64 {
	return time.Since(t).Hours()
}

// toConnectedMemories converts raw relationship rows into ConnectedMemory values
// relative to the given memory ID. ConnectedMemory.Memory is intentionally left
// nil to avoid a second DB round-trip per relationship.
func toConnectedMemories(rels []types.Relationship, memID string) []types.ConnectedMemory {
	out := make([]types.ConnectedMemory, 0, len(rels))
	for _, r := range rels {
		dir := "outgoing"
		if r.TargetID == memID {
			dir = "incoming"
		}
		out = append(out, types.ConnectedMemory{
			RelType:   r.RelType,
			Direction: dir,
			Strength:  r.Strength,
		})
	}
	return out
}

// sortResults sorts descending by composite score.
func sortResults(r []types.SearchResult) {
	sort.Slice(r, func(i, j int) bool { return r[i].Score > r[j].Score })
}

// List returns memories for the project matching optional filters.
func (e *SearchEngine) List(ctx context.Context, memType *string, tags []string,
	maxImportance *int, limit, offset int) ([]*types.Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	return e.backend.ListMemories(ctx, e.project, db.ListOptions{
		MemoryType:        memType,
		Tags:              tags,
		ImportanceCeiling: maxImportance,
		Limit:             limit,
		Offset:            offset,
	})
}

// Connect creates a directed relationship between two memories.
func (e *SearchEngine) Connect(ctx context.Context, srcID, dstID, relType string, strength float64) error {
	if !types.ValidateRelationType(relType) {
		return fmt.Errorf("invalid relation type %q", relType)
	}
	if strength <= 0 {
		strength = 1.0
	}
	rel := &types.Relationship{
		ID:       types.NewMemoryID(),
		SourceID: srcID,
		TargetID: dstID,
		RelType:  relType,
		Strength: strength,
		Project:  e.project,
	}
	return e.backend.StoreRelationship(ctx, rel)
}

// Correct updates mutable fields on an existing memory and returns the updated record.
// When content is non-nil, the old chunks are deleted and new chunks are created so
// the search index reflects the corrected text.
func (e *SearchEngine) Correct(ctx context.Context, id string, content *string, tags []string, importance *int) (*types.Memory, error) {
	mem, err := e.backend.UpdateMemory(ctx, id, content, tags, importance)
	if err != nil {
		return nil, err
	}

	// If content changed, re-chunk + re-embed first (outside any transaction so
	// a slow embedder call does not hold a lock), then atomically swap old chunks
	// for new ones inside a single transaction. This prevents orphaned memories
	// (no chunks, no vector) if the embedder fails after the delete.
	if content != nil {
		chunks, err := e.storeChunksForMemory(ctx, mem)
		if err != nil {
			return nil, fmt.Errorf("re-chunk after correct: %w", err)
		}

		tx, err := e.backend.Begin(ctx)
		if err != nil {
			return nil, err
		}
		if err := e.backend.DeleteChunksForMemoryTx(ctx, tx, mem.ID); err != nil {
			_ = tx.Rollback(ctx)
			return nil, fmt.Errorf("delete old chunks: %w", err)
		}
		if len(chunks) > 0 {
			if err := e.backend.StoreChunksTx(ctx, tx, chunks); err != nil {
				_ = tx.Rollback(ctx)
				return nil, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
	}

	return mem, nil
}

// Forget deletes a memory by ID. Returns true if deleted, false if not found.
func (e *SearchEngine) Forget(ctx context.Context, id string) (bool, error) {
	return e.backend.DeleteMemoryAtomic(ctx, e.project, id, false)
}

// Status returns aggregate statistics for the project.
func (e *SearchEngine) Status(ctx context.Context) (*types.MemoryStats, error) {
	return e.backend.GetStats(ctx, e.project)
}

// Feedback records a positive access signal by boosting edges and updating last-accessed
// for each ID. This is a best-effort operation: if one ID fails, the error is returned
// immediately and subsequent IDs in the slice are not processed. Callers that need
// all-or-nothing semantics should call this method once per ID and handle errors individually.
func (e *SearchEngine) Feedback(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("feedback: no memory IDs provided")
	}
	for _, id := range ids {
		if _, err := e.backend.BoostEdgesForMemory(ctx, id, 1.05); err != nil {
			return err
		}
		if err := e.backend.TouchMemory(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

const (
	consolidateStaleAgeHours   = 90 * 24
	consolidateColdAgeHours    = 60 * 24
	consolidateMaxImportance   = 3
	consolidateEdgeDecayFactor = 0.02
	consolidateEdgeMinStrength = 0.1
)

// Consolidate prunes stale and cold memories and decays graph edges.
// Returns a summary map of counts for each operation performed.
func (e *SearchEngine) Consolidate(ctx context.Context) (map[string]any, error) {
	// TODO: Jaccard near-duplicate merge (threshold 0.85) is a future task.
	pruned, err := e.backend.PruneStaleMemories(ctx, e.project, consolidateStaleAgeHours, consolidateMaxImportance)
	if err != nil {
		return nil, err
	}
	coldPruned, err := e.backend.PruneColdDocuments(ctx, e.project, consolidateColdAgeHours, consolidateMaxImportance)
	if err != nil {
		return nil, err
	}
	decayed, edgePruned, err := e.backend.DecayAllEdges(ctx, e.project, consolidateEdgeDecayFactor, consolidateEdgeMinStrength)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"pruned_memories":  pruned,
		"pruned_cold_docs": coldPruned,
		"edges_decayed":    decayed,
		"edges_pruned":     edgePruned,
	}, nil
}

// Verify checks hash coverage and corruption for the project.
// Returns a map with total, hashed, corrupt counts and coverage percentage.
func (e *SearchEngine) Verify(ctx context.Context) (map[string]any, error) {
	stats, err := e.backend.GetIntegrityStats(ctx, e.project)
	if err != nil {
		return nil, err
	}
	pct := 0.0
	if stats.Total > 0 {
		pct = float64(stats.Hashed) / float64(stats.Total) * 100
	}
	return map[string]any{
		"total":    stats.Total,
		"hashed":   stats.Hashed,
		"corrupt":  stats.Corrupt,
		"coverage": fmt.Sprintf("%.1f%%", pct),
	}, nil
}

// MigrateEmbedder initiates an embedding migration to a new model by nulling all
// existing embeddings and recording the new model name in project metadata.
// A background reembed worker will repopulate embeddings after this call.
func (e *SearchEngine) MigrateEmbedder(ctx context.Context, newModel string) (map[string]any, error) {
	// Null all embeddings first — if this fails, no metadata is written and state stays consistent.
	nulled, err := e.backend.NullAllEmbeddings(ctx, e.project)
	if err != nil {
		return nil, err
	}
	if err := e.backend.SetMeta(ctx, e.project, "embedding_migration_in_progress", "true"); err != nil {
		return nil, err
	}
	if err := e.backend.SetMeta(ctx, e.project, "embedder_name", newModel); err != nil {
		return nil, err
	}

	// Stop old reembed worker and create a new one with the new model.
	// Without this, the worker holds a stale reference to the original embedder
	// and never runs because its done channel was already closed at construction.
	e.reembedder.Stop()

	newEmbedder, err := embed.NewOllamaClient(ctx, e.ollamaURL, newModel)
	if err != nil {
		return nil, fmt.Errorf("create embedder for new model %q: %w", newModel, err)
	}
	e.embedder = newEmbedder

	e.reembedder = reembed.NewWorker(e.backend, newEmbedder, e.project, true)
	e.reembedder.Start()

	return map[string]any{
		"chunks_nulled": nulled,
		"new_model":     newModel,
		"status":        "migration started — reembed worker running with new model",
	}, nil
}

const consolidateJaccardThreshold = 0.85
const consolidateMaxMemories = 500
const consolidateCombinedThreshold = 0.80

// ConsolidateWithClaude runs base consolidation then finds near-duplicate memory
// pairs via MinHash/LSH candidate generation, scores them with a hybrid
// Jaccard + embedding cosine signal, batches them to reviewer for merge
// decisions, and applies the approved merges.
func (e *SearchEngine) ConsolidateWithClaude(ctx context.Context, reviewer MergeReviewer) (map[string]any, error) {
	// 1. Run base consolidation.
	result, err := e.Consolidate(ctx)
	if err != nil {
		return nil, err
	}

	// 2. Fetch up to consolidateMaxMemories memories.
	mems, err := e.backend.ListMemories(ctx, e.project, db.ListOptions{Limit: consolidateMaxMemories})
	if err != nil {
		return result, err
	}

	// 3. Filter: 50-4000 chars, not immutable.
	var filtered []*types.Memory
	memMap := make(map[string]*types.Memory)
	for _, m := range mems {
		if m.Immutable || len(m.Content) < 50 || len(m.Content) > 4000 {
			continue
		}
		filtered = append(filtered, m)
		memMap[m.ID] = m
	}

	// 4. Compute MinHash signatures — O(n).
	hasher := minhash.NewHasher(42)
	idx := minhash.NewIndex(16, 8)
	for _, m := range filtered {
		sig := hasher.Signature(m.Content)
		idx.Add(m.ID, sig)
	}

	// 5. Get candidate pairs from LSH — O(n).
	lshPairs := idx.Candidates()

	// 6. Score each candidate: exact Jaccard + embedding cosine.
	var candidates []MergeCandidate
	for _, pair := range lshPairs {
		memA, memB := memMap[pair[0]], memMap[pair[1]]
		if memA == nil || memB == nil {
			continue
		}

		jaccard := bigramJaccard(memA.Content, memB.Content)
		if jaccard < consolidateJaccardThreshold {
			continue // LSH false positive
		}

		// Embedding cosine distance → similarity.
		dist, err := e.backend.ChunkEmbeddingDistance(ctx, memA.ID, memB.ID)
		if err != nil {
			dist = 2.0 // treat as max distance on error
		}
		cosineSim := 1.0 - dist

		// Combined score: weighted Jaccard + cosine.
		combined := 0.7*jaccard + 0.3*cosineSim
		if combined < consolidateCombinedThreshold {
			continue
		}

		candidates = append(candidates, MergeCandidate{
			MemoryA:    memA,
			MemoryB:    memB,
			Similarity: combined,
		})
	}

	// 7. Batch candidates to Claude reviewer.
	const batchSize = 10
	var totalMerged, totalReviewed int
	for start := 0; start < len(candidates); start += batchSize {
		end := start + batchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]
		decisions, err := reviewer.ReviewMergeCandidates(ctx, batch)
		if err != nil {
			continue
		}
		totalReviewed += len(batch)
		for _, d := range decisions {
			if !d.ShouldMerge {
				continue
			}
			content := d.MergedContent
			if content != "" {
				if _, err := e.backend.UpdateMemory(ctx, d.MemoryAID, &content, nil, nil); err != nil {
					continue
				}
			}
			if deleted, err := e.backend.DeleteMemoryAtomic(ctx, e.project, d.MemoryBID, false); err != nil || !deleted {
				continue
			}
			totalMerged++
		}
	}

	result["merged_memories"] = totalMerged
	result["candidates_reviewed"] = totalReviewed
	result["lsh_candidates"] = len(lshPairs)
	result["jaccard_passed"] = len(candidates)
	return result, nil
}

// bigramJaccard computes character-bigram Jaccard similarity between two strings.
// Returns |A∩B| / |A∪B| where A and B are the bigram sets of a and b.
// Iterates over Unicode code points (runes), not bytes, to handle multibyte UTF-8.
func bigramJaccard(a, b string) float64 {
	bigrams := func(s string) map[[2]rune]struct{} {
		r := []rune(s)
		m := make(map[[2]rune]struct{}, len(r))
		for i := 0; i+1 < len(r); i++ {
			m[[2]rune{r[i], r[i+1]}] = struct{}{}
		}
		return m
	}
	setA := bigrams(a)
	setB := bigrams(b)
	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}
	var inter int
	for k := range setA {
		if _, ok := setB[k]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// SummarizeNow: handled directly by the MCP tool via summarize package (see tools.go).
