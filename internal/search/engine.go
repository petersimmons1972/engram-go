package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"


	"github.com/petersimmons1972/engram/internal/chunk"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/minhash"
	"github.com/petersimmons1972/engram/internal/reembed"
	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/petersimmons1972/engram/internal/types"
)

// globalNotifier is satisfied by *reembed.GlobalReembedder. Declared here as an
// unexported interface so the search package does not import the reembed package
// (avoiding an import cycle) and tests can supply a stub.
type globalNotifier interface {
	Notify()
}

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
	// Mode controls response format. "" or "full" returns complete SearchResults;
	// "handle" returns lightweight Handle references (content omitted).
	Mode string
	// CurrentEpisodeID is the session episode; memories with a matching episode_id
	// score 1.15× higher via EpisodeMatch in ScoreInput. Empty string = no boost.
	CurrentEpisodeID string
}

// ToHandles projects a slice of SearchResults into lightweight Handle references.
// Handles carry only the summary and metadata needed to decide whether to fetch;
// full content is never loaded. Entries with a nil Memory are skipped.
func ToHandles(results []types.SearchResult) []types.Handle {
	if len(results) == 0 {
		return nil
	}
	const fetchHint = "call memory_fetch with this id and detail=summary|chunk|full"
	out := make([]types.Handle, 0, len(results))
	for _, r := range results {
		if r.Memory == nil {
			continue
		}
		sum := ""
		if r.Memory.Summary != nil {
			sum = *r.Memory.Summary
		}
		out = append(out, types.Handle{
			ID:          r.Memory.ID,
			Project:     r.Memory.Project,
			Summary:     sum,
			Score:       r.Score,
			StorageMode: r.Memory.StorageMode,
			Bytes:       len(r.Memory.Content),
			IsHandle:    true,
			FetchHint:   fetchHint,
		})
	}
	return out
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

// bestHit holds the best vector chunk match for a single memory ID.
// Consolidating five parallel maps into one struct reduces map lookups by 5×
// and reduces per-call heap allocations in the RecallWithOpts inner loop
// (at the cost of slightly larger per-call bytes due to pre-allocated bucket capacity).
//
// sectionHeading is a borrowed pointer — it points into the VectorHit returned
// by the database scan and must not outlive the enclosing Recall call. This is
// safe because bestHit is stack-scoped to RecallWithOpts and not retained after
// the function returns.
type bestHit struct {
	cosine         float64
	chunkText      string
	chunkIndex     int
	sectionHeading *string // borrowed; see struct comment
	chunkID        string
}

// SearchEngine is the core retrieval engine: it stores memories (chunked + embedded)
// and recalls them via composite vector+FTS scoring.
type SearchEngine struct {
	backend          db.Backend
	embedMu          sync.RWMutex // protects embedder; use getEmbedder() for all reads
	embedder         embed.Client
	project          string
	ollamaURL        string
	targetDims       int             // MRL truncation target; 0 = model native output
	ctx              context.Context // parent lifecycle context — passed to workers via StartWithContext
	summarizer       *summarize.Worker
	reembedder       *reembed.Worker
	decayer          *DecayWorker
	weightCache      *WeightCache    // nil when no pgxpool available (pre-migration or test)
	globalReembedder globalNotifier  // non-nil after SetGlobalReembedder; woken on chunk store
}

// getEmbedder safely reads the current embedder. Use this instead of e.embedder
// directly so concurrent MigrateEmbedder calls don't cause a data race.
func (e *SearchEngine) getEmbedder() embed.Client {
	e.embedMu.RLock()
	defer e.embedMu.RUnlock()
	return e.embedder
}

// Embedder returns the current embedding client. Used by callers (e.g. consolidate
// runner) that need the live embedder rather than a nil placeholder (#94).
func (e *SearchEngine) Embedder() embed.Client {
	return e.getEmbedder()
}

// SetGlobalReembedder wires the shared GlobalReembedder so StoreWithRawBody and
// StoreBatch can wake it immediately after writing chunks. Call once after
// constructing the engine in main; nil is safe (Notify is skipped).
func (e *SearchEngine) SetGlobalReembedder(n globalNotifier) {
	e.globalReembedder = n
}

// New constructs a SearchEngine and starts background workers (summarize, reembed,
// and spaced-repetition importance decay). claudeClient may be nil, in which case
// summarization falls back to Ollama. decayInterval controls how often the decay
// pass runs; pass 0 to use the default (8 hours).
func New(ctx context.Context, backend db.Backend, embedder embed.Client, project string,
	ollamaURL, summarizeModel string, summarizeEnabled bool,
	claudeClient summarize.ClaudeCompleter, decayInterval time.Duration, targetDims ...int) *SearchEngine {
	sum := summarize.NewWorkerWithClaude(backend, project, ollamaURL, summarizeModel, summarizeEnabled, claudeClient)
	sum.StartWithContext(ctx)

	// The per-project reembedder is not started in the server process. A
	// GlobalReembedder (cmd/engram/main.go) processes unembedded chunks across
	// all projects from a single goroutine using FOR UPDATE SKIP LOCKED (#359).
	// The Worker is kept so Notify() and Stop() calls remain valid no-ops, and
	// so the model-migration path in ChangeEmbedder can still restart a worker
	// temporarily when switching models for a specific project.
	reb := reembed.NewWorkerFromMeta(ctx, backend, embedder, project)
	// Do not call StartWithContext here — GlobalReembedder owns this work.

	dec := NewDecayWorker(backend, project, decayInterval)
	dec.StartWithContext(ctx)

	// Build a weight cache if the backend exposes a pgxpool.
	// PostgresBackend implements pgPooler; test stubs do not.
	var wc *WeightCache
	if pgb, ok := backend.(pgPooler); ok {
		wc = NewWeightCache(pgb.PgxPool())
	}

	var dims int
	if len(targetDims) > 0 {
		dims = targetDims[0]
	}
	return &SearchEngine{
		backend:     backend,
		embedder:    embedder,
		project:     project,
		ollamaURL:   ollamaURL,
		targetDims:  dims,
		ctx:         ctx,
		summarizer:  sum,
		reembedder:  reb,
		decayer:     dec,
		weightCache: wc,
	}
}

// Close shuts down the engine, stops all background workers, and releases
// the database connection pool. Must be called exactly once per engine.
func (e *SearchEngine) Close() {
	e.decayer.Stop()
	e.summarizer.Stop()
	e.reembedder.Stop()
	e.backend.Close()
}

// Backend exposes the underlying db.Backend for callers that need direct access
// (e.g. EnginePool, MCP tool handlers).
func (e *SearchEngine) Backend() db.Backend { return e.backend }

// Project returns the project slug this engine is scoped to.
func (e *SearchEngine) Project() string { return e.project }

// storeChunksForMemory chunks content, embeds each chunk, and returns the new
// chunk records ready for storage. It is used by both Store (new memories) and
// Correct (re-chunking after a content change). Embedding is done outside any
// transaction because it is slow; callers are responsible for writing the
// returned chunks inside a transaction.
func (e *SearchEngine) storeChunksForMemory(ctx context.Context, m *types.Memory, rawBody string) ([]*types.Chunk, error) {
	// A4 Tier-1 synopsis support: when rawBody is non-empty, chunks are built
	// from the full document body rather than the memory's (synopsis) Content.
	// This keeps recall grounded in the original text even though Memory.Content
	// is truncated to a synopsis for context-window friendliness.
	chunkSource := m.Content
	if rawBody != "" {
		chunkSource = rawBody
	}

	// Produce chunk candidates. ChunkDocument returns []ChunkCandidate (with heading
	// metadata). ChunkText returns plain []string which we wrap into candidates.
	var candidates []chunk.ChunkCandidate
	if m.StorageMode == "document" {
		candidates = chunk.ChunkDocument(chunkSource, 0 /* use package default */)
	} else {
		// Chunk using the configured model's context window so no chunk
		// exceeds what Ollama accepts for this embedding model (#361).
		for _, text := range chunk.ChunkText(chunkSource, embed.ModelMaxTokens(e.getEmbedder().Name()), 50) {
			candidates = append(candidates, chunk.ChunkCandidate{
				Text:      text,
				ChunkType: "sentence_window",
			})
		}
	}

	// If ChunkText produced nothing (empty content edge case), store content as one chunk.
	if len(candidates) == 0 {
		candidates = []chunk.ChunkCandidate{{Text: chunkSource, ChunkType: "sentence_window"}}
	}

	// Filter to new chunks only (deduplicate by hash) before embedding.
	type pendingChunk struct {
		idx       int
		candidate chunk.ChunkCandidate
		hash      string
	}
	var pending []pendingChunk
	for i, c := range candidates {
		hash := chunk.ChunkHash(c.Text)
		exists, err := e.backend.ChunkHashExists(ctx, hash, m.ID)
		if err != nil {
			return nil, fmt.Errorf("check chunk hash: %w", err)
		}
		if !exists {
			pending = append(pending, pendingChunk{idx: i, candidate: c, hash: hash})
		}
	}

	if len(pending) == 0 {
		return nil, nil
	}

	// All new chunks are stored with nil embeddings. The reembed worker backfills
	// them asynchronously, keeping the store path fully decoupled from Ollama.
	// This eliminates the 48-minute hang caused by Ollama blocking the MCP call.
	chunks := make([]*types.Chunk, len(pending))
	for j, p := range pending {
		ch := &types.Chunk{
			ID:         types.NewMemoryID(),
			MemoryID:   m.ID,
			ChunkText:  p.candidate.Text,
			ChunkIndex: p.idx,
			ChunkHash:  p.hash,
			ChunkType:  p.candidate.ChunkType,
			Project:    e.project,
			Embedding:  nil,
		}
		if p.candidate.HasHeading {
			heading := p.candidate.SectionHeading
			ch.SectionHeading = &heading
		}
		chunks[j] = ch
	}
	return chunks, nil
}

// Store persists a memory: sets defaults, chunks content, deduplicates by hash,
// embeds new chunks, and writes everything inside a single transaction.
//
// When m.RawBody is non-empty, Store uses it as the chunk source instead of
// m.Content. Set m.RawBody before calling Store when m.Content holds only a
// synopsis: this keeps recall grounded in the full text while the synopsis
// stays context-window friendly. When m.RawBody is empty the behaviour is
// identical to the previous version — chunks come from m.Content.
func (e *SearchEngine) Store(ctx context.Context, m *types.Memory) error {
	return e.StoreWithRawBody(ctx, m, m.RawBody)
}

// StoreWithRawBody is like Store, but chunks the given rawBody (rather than
// m.Content) when non-empty. Used by Tier-1 large-document ingestion: the
// memory carries a synopsis in Content while chunks are produced from the
// full body so semantic recall stays grounded in the original text.
//
// When to pass each value:
//   - Normal memories (focused, or document-mode that fits in Content):
//     pass rawBody="". Chunks are built from m.Content.
//   - Tier-1 synopsis ingestion (A4): pass rawBody=<full original body>.
//     m.Content carries the synopsis; chunks come from the full body.
//   - Tier-2 raw-document ingestion (A4): pass rawBody="". The full body is
//     already parked in the documents table; chunks are built from the
//     synopsis in m.Content and recall goes through memory_query_document.
//   - Correct() re-chunking: passes rawBody="" because the caller is updating
//     the authoritative content field. Callers that want to preserve a
//     synopsis/body split across corrections must re-issue StoreWithRawBody
//     with the full body themselves — there is no persisted raw body to
//     recover from once a memory is stored with only a synopsis in Content.
//
func (e *SearchEngine) StoreWithRawBody(ctx context.Context, m *types.Memory, rawBody string) error {
	if m.ID == "" {
		m.ID = types.NewMemoryID()
	}
	m.Project = e.project

	effectiveSize := len(m.Content)
	if rawBody != "" {
		effectiveSize = len(rawBody)
	}
	if m.StorageMode == "" {
		if effectiveSize > 10_000 {
			m.StorageMode = "document"
		} else {
			m.StorageMode = "focused"
		}
	}

	if err := e.checkEmbedderMeta(ctx); err != nil {
		return err
	}

	chunks, err := e.storeChunksForMemory(ctx, m, rawBody)
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
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if len(chunks) > 0 {
		e.reembedder.Notify()
		if e.globalReembedder != nil {
			e.globalReembedder.Notify()
		}
	}
	return nil
}

// StoreBatch stores multiple memories atomically (#115).
// All embeddings are computed first (outside any transaction), then the entire
// batch is written in a single DB transaction — either all succeed or none do.
func (e *SearchEngine) StoreBatch(ctx context.Context, memories []*types.Memory) error {
	if len(memories) == 0 {
		return nil
	}

	if err := e.checkEmbedderMeta(ctx); err != nil {
		return err
	}

	// Phase 1: compute embeddings for all items (external calls, outside tx).
	type memWithChunks struct {
		mem    *types.Memory
		chunks []*types.Chunk
	}
	prepared := make([]memWithChunks, 0, len(memories))
	for _, m := range memories {
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
		chunks, err := e.storeChunksForMemory(ctx, m, "")
		if err != nil {
			return fmt.Errorf("prepare memory %q: %w", m.ID, err)
		}
		prepared = append(prepared, memWithChunks{mem: m, chunks: chunks})
	}

	// Phase 2: write everything in one transaction.
	tx, err := e.backend.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	hasChunks := false
	for _, p := range prepared {
		if err := e.backend.StoreMemoryTx(ctx, tx, p.mem); err != nil {
			return err
		}
		if len(p.chunks) > 0 {
			if err := e.backend.StoreChunksTx(ctx, tx, p.chunks); err != nil {
				return err
			}
			hasChunks = true
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if hasChunks {
		e.reembedder.Notify()
		if e.globalReembedder != nil {
			e.globalReembedder.Notify()
		}
	}
	return nil
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

	// 2s deadline — Ollama must never block MCP calls. On failure, degrade
	// gracefully to BM25+recency only; the vector leg is skipped but recall
	// still returns useful results.
	embedCtx, embedCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer embedCancel()
	queryVec, err := e.getEmbedder().Embed(embedCtx, query)
	if err != nil {
		slog.Warn("embed query failed, degrading to BM25+recency only", "project", e.project, "err", err)
		queryVec = nil
	}

	// ANN vector search via pgvector HNSW index — skipped when embed degraded.
	var vecHits []db.VectorHit
	if queryVec != nil {
		vecHits, err = e.backend.VectorSearch(ctx, e.project, queryVec, topK*3)
		if err != nil {
			return nil, err
		}
	}

	// Fan-out FTS search concurrently.
	type ftsResult struct {
		results []db.FTSResult
		err     error
	}
	ftsCh := make(chan ftsResult, 1)
	go func() {
		res, err := e.backend.FTSSearch(ctx, e.project, query, topK*3, nil, nil)
		select {
		case ftsCh <- ftsResult{res, err}:
		case <-ctx.Done():
		}
	}()

	// Build per-memory best cosine from vector hits.
	// pgvector returns cosine distance (0-2); convert to similarity (1-0).
	// bestHits consolidates five parallel maps into one struct map, halving
	// hash lookups and reducing allocations in this hot inner loop.
	bestHits := make(map[string]bestHit, len(vecHits))
	uniqueIDs := make([]string, 0, len(vecHits))
	seen := make(map[string]bool, len(vecHits))

	for _, h := range vecHits {
		cosine := 1.0 - h.Distance
		if existing, ok := bestHits[h.MemoryID]; !ok || cosine > existing.cosine {
			bestHits[h.MemoryID] = bestHit{
				cosine:         cosine,
				chunkText:      h.ChunkText,
				chunkIndex:     h.ChunkIndex,
				sectionHeading: h.SectionHeading,
				chunkID:        h.ChunkID,
			}
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
	var ftsRes ftsResult
	select {
	case ftsRes = <-ftsCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
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

	// Detect preference-shaped queries once before the scoring loop (#364).
	prefQuery := isPreferenceQuery(query)

	// Composite scoring per memory.
	results := make([]types.SearchResult, 0)
	for id, m := range memories {
		bm25 := 0.0
		if maxBM25 > 0 {
			bm25 = ftsScores[id] / maxBM25
		}
		hit := bestHits[id]
		input := ScoreInput{
			Cosine:             hit.cosine,
			BM25:               bm25,
			HoursSince:         hoursSince(m.LastAccessed),
			Importance:         m.Importance,
			DynamicImportance:  m.DynamicImportance,
			RetrievalPrecision: m.RetrievalPrecision,
			EpisodeMatch:       opts.CurrentEpisodeID != "" && m.EpisodeID == opts.CurrentEpisodeID,
			MemoryType:         m.MemoryType,
			IsPreferenceQuery:  prefQuery,
		}
		var score float64
		if e.weightCache != nil {
			score = CompositeScoreWithWeights(input, e.weightCache.Get(ctx, e.project))
		} else {
			score = CompositeScore(input)
		}

		result := types.SearchResult{
			Memory:     m,
			Score:      score,
			ChunkScore: hit.cosine,
			ScoreBreakdown: func() map[string]float64 {
				bd := map[string]float64{
					"cosine":        hit.cosine,
					"bm25":          bm25,
					"recency":       RecencyDecay(input.HoursSince),
					"episode_boost": 1.0,
				}
				if input.EpisodeMatch {
					bd["episode_boost"] = 1.15
				}
				if m.DynamicImportance != nil {
					bd["dynamic_importance"] = *m.DynamicImportance
				} else {
					bd["importance_boost"] = ImportanceBoost(m.Importance)
				}
				if m.RetrievalPrecision != nil {
					bd["retrieval_precision"] = *m.RetrievalPrecision
				}
				return bd
			}(),
			MatchedChunk:        hit.chunkText,
			MatchedChunkIndex:   hit.chunkIndex,
			MatchedChunkSection: hit.sectionHeading,
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

	// Optional re-ranking: cap at topK candidates so the reranker only sees the
	// same set that will be returned, not the full pre-truncation pool (which can
	// be 3× larger and blows reranker latency and context window budgets).
	if opts.Reranker != nil && len(results) > 0 {
		rerankCandidates := results
		if len(rerankCandidates) > topK {
			rerankCandidates = rerankCandidates[:topK]
		}
		items := make([]RerankItem, len(rerankCandidates))
		for i, r := range rerankCandidates {
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

	// Fetch relationships for the final topK results in a single batch query,
	// replacing the prior N+1 loop (one GetRelationships call per result).
	topKIDs := make([]string, len(results))
	for i, r := range results {
		topKIDs[i] = r.Memory.ID
	}
	relsMap, err := e.backend.GetRelationshipsBatch(ctx, e.project, topKIDs)
	if err != nil {
		// best-effort: proceed with empty relationship sets rather than failing recall
		slog.Warn("GetRelationshipsBatch failed, proceeding without relationships", "err", err)
		relsMap = make(map[string][]types.Relationship)
	}

	var allRels [][]types.Relationship
	var neighborIDs []string
	neighborSet := make(map[string]struct{})
	for i := range results {
		rels := relsMap[results[i].Memory.ID]
		allRels = append(allRels, rels)
		for _, r := range rels {
			neighborID := r.TargetID
			if r.TargetID == results[i].Memory.ID {
				neighborID = r.SourceID
			}
			if _, seen := neighborSet[neighborID]; !seen {
				neighborSet[neighborID] = struct{}{}
				neighborIDs = append(neighborIDs, neighborID)
			}
		}
	}
	neighborMap := make(map[string]*types.Memory, len(neighborIDs))
	if len(neighborIDs) > 0 {
		fetched, err := e.backend.GetMemoriesByIDs(ctx, e.project, neighborIDs)
		if err == nil {
			for _, m := range fetched {
				neighborMap[m.ID] = m
			}
		}
	}
	for i := range results {
		results[i].Connected = toConnectedMemories(allRels[i], results[i].Memory.ID, neighborMap)
	}

	// Batch-update access timestamps (#117 — replaces N+1 TouchMemory calls).
	touchIDs := make([]string, len(results))
	for i, r := range results {
		touchIDs[i] = r.Memory.ID
	}
	if err := e.backend.TouchMemories(ctx, touchIDs); err != nil {
		slog.Warn("touch memories failed", "err", err)
	}
	for _, r := range results {
		// hit.chunkID != "" guards against calling UpdateChunkLastMatched with
		// an empty ID. FTS-only results (not in bestHits) are correctly skipped
		// via the ok check; vector hits with an empty ChunkID (unusual but valid)
		// are skipped via the chunkID check. This is a strict improvement over
		// the prior code, which would have called UpdateChunkLastMatched("").
		if hit, ok := bestHits[r.Memory.ID]; ok && hit.chunkID != "" {
			_ = e.backend.UpdateChunkLastMatched(ctx, hit.chunkID)
		}
	}

	return results, nil
}

// RecallWithinMemory returns up to topK chunks from a single memory's document
// most semantically similar to query, projected into minimal *types.Memory
// values so callers get the chunk text alongside the parent memory's ID.
// Used by the A5 memory_query_document tool's semantic path.
func (e *SearchEngine) RecallWithinMemory(ctx context.Context, query string, memoryID string, topK int, detail string) ([]*types.Memory, error) {
	if topK <= 0 {
		topK = 10
	}
	// 2s deadline — Ollama must never block MCP calls. RecallWithinMemory has
	// no BM25 fallback (document chunk search is vector-only), so it returns an
	// error on embed failure — but fails fast rather than hanging.
	embedCtx, embedCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer embedCancel()
	queryVec, err := e.getEmbedder().Embed(embedCtx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	chunks, err := e.backend.SearchChunksWithinMemory(ctx, queryVec, memoryID, topK)
	if err != nil {
		return nil, err
	}
	_ = detail // currently all modes return the chunk text verbatim
	out := make([]*types.Memory, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, &types.Memory{
			ID:       c.MemoryID,
			Content:  c.ChunkText,
			Project:  c.Project,
		})
	}
	return out, nil
}

// checkEmbedderMeta ensures the stored embedder name matches the current client,
// or registers it if this is the first store for the project.
func (e *SearchEngine) checkEmbedderMeta(ctx context.Context) error {
	storedName, ok, err := e.backend.GetMeta(ctx, e.project, "embedder_name")
	if err != nil {
		return err
	}
	emb := e.getEmbedder()
	if !ok {
		if err := e.backend.SetMeta(ctx, e.project, "embedder_name", emb.Name()); err != nil {
			return err
		}
		return e.backend.SetMeta(ctx, e.project, "embedder_dimensions",
			fmt.Sprintf("%d", emb.Dimensions()))
	}
	if storedName != emb.Name() {
		return fmt.Errorf("embedder mismatch: stored=%q current=%q — run memory_migrate_embedder first",
			storedName, emb.Name())
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
		if storedDims != emb.Dimensions() {
			return fmt.Errorf("embedder dimensions mismatch: stored %d, current %d — use memory_migrate_embedder to switch models",
				storedDims, emb.Dimensions())
		}
	}
	return nil
}

func hoursSince(t time.Time) float64 {
	return time.Since(t).Hours()
}

// toConnectedMemories converts raw relationship rows into ConnectedMemory values
// relative to the given memory ID. neighborMap is used to populate the Memory
// field on each result; missing entries are left nil rather than failing.
func toConnectedMemories(rels []types.Relationship, memID string, neighborMap map[string]*types.Memory) []types.ConnectedMemory {
	out := make([]types.ConnectedMemory, 0, len(rels))
	for _, r := range rels {
		neighborID := r.TargetID
		dir := "outgoing"
		if r.TargetID == memID {
			neighborID = r.SourceID
			dir = "incoming"
		}
		out = append(out, types.ConnectedMemory{
			Memory:    neighborMap[neighborID],
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
		chunks, err := e.storeChunksForMemory(ctx, mem, "")
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

// Forget soft-deletes a memory by setting valid_to=NOW() and snapshotting it
// into memory_versions. Returns true if deleted, false if not found or already invalidated.
func (e *SearchEngine) Forget(ctx context.Context, id, reason string) (bool, error) {
	return e.backend.SoftDeleteMemory(ctx, e.project, id, reason)
}

// MemoryHistory returns the full version chain for a memory in reverse
// chronological order (most recent change first).
func (e *SearchEngine) MemoryHistory(ctx context.Context, id string) ([]*types.MemoryVersion, error) {
	return e.backend.GetMemoryHistory(ctx, e.project, id)
}

// MemoryAsOf returns memories that were active at the given point in time.
func (e *SearchEngine) MemoryAsOf(ctx context.Context, asOf time.Time, limit int) ([]*types.Memory, error) {
	return e.backend.GetMemoriesAsOf(ctx, e.project, asOf, limit)
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
		// Spaced-repetition boost: grow dynamic_importance and advance next_review_at.
		if err := e.backend.UpdateDynamicImportance(ctx, id, 0.1, 1.5); err != nil {
			return err
		}
	}
	return nil
}

// FeedbackNegative records a negative access signal: dynamic_importance decreases.
func (e *SearchEngine) FeedbackNegative(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("feedback: no memory IDs provided")
	}
	for _, id := range ids {
		if err := e.backend.UpdateDynamicImportance(ctx, id, -0.05, 0); err != nil {
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
// For Claude-assisted near-duplicate merging, use ConsolidateWithClaude.
func (e *SearchEngine) Consolidate(ctx context.Context) (map[string]any, error) {
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
	// Wrap null + meta writes in a single transaction (#102).
	// Without this, a crash between NullAllEmbeddings and SetMeta leaves chunks
	// without embeddings but the migrator flag never set — reembed worker never runs.
	tx, err := e.backend.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	nulled, err := e.backend.NullAllEmbeddingsTx(ctx, tx, e.project)
	if err != nil {
		return nil, err
	}
	if err := e.backend.SetMetaTx(ctx, tx, e.project, "embedding_migration_in_progress", "true"); err != nil {
		return nil, err
	}
	if err := e.backend.SetMetaTx(ctx, tx, e.project, "embedder_name", newModel); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Stop old reembed worker and create a new one with the new model.
	// Without this, the worker holds a stale reference to the original embedder
	// and never runs because its done channel was already closed at construction.
	e.reembedder.Stop()

	// ctx is used only for the startup probe; the returned client is context-independent.
	newEmbedder, err := embed.NewOllamaClientWithDims(ctx, e.ollamaURL, newModel, e.targetDims)
	if err != nil {
		return nil, fmt.Errorf("create embedder for new model %q: %w", newModel, err)
	}
	e.embedMu.Lock()
	e.embedder = newEmbedder
	e.embedMu.Unlock()

	e.reembedder = reembed.NewWorker(e.backend, newEmbedder, e.project, true)
	e.reembedder.StartWithContext(e.ctx)

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
	hasher, err := minhash.NewHasher()
	if err != nil {
		return result, fmt.Errorf("consolidate: %w", err)
	}
	idx, err := minhash.NewIndex(16, 8)
	if err != nil {
		return result, fmt.Errorf("consolidate: %w", err)
	}
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
			// Atomic merge (#104): update winner content + delete loser in one tx.
			if err := e.backend.MergeMemoriesAtomic(ctx, e.project, d.MemoryAID, d.MemoryBID, d.MergedContent); err != nil {
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

// RecallWithEvent is like Recall but also stores a retrieval_event row and returns
// the event ID. Pass the event ID to FeedbackWithEvent to record which results were useful.
func (e *SearchEngine) RecallWithEvent(ctx context.Context, query string, topK int, detail string) ([]types.SearchResult, string, error) {
	results, err := e.RecallWithOpts(ctx, query, topK, detail, RecallOpts{})
	if err != nil {
		return nil, "", err
	}

	resultIDs := make([]string, 0, len(results))
	for _, r := range results {
		if r.Memory != nil {
			resultIDs = append(resultIDs, r.Memory.ID)
		}
	}

	event := &types.RetrievalEvent{
		ID:        types.NewMemoryID(),
		Project:   e.project,
		Query:     query,
		ResultIDs: resultIDs,
		CreatedAt: time.Now().UTC(),
	}
	if err := e.backend.StoreRetrievalEvent(ctx, event); err != nil {
		slog.Warn("store retrieval event failed", "project", e.project, "err", err) // #96
		return results, "", nil
	}

	// Auto-increment times_retrieved for all returned memories so the
	// retrieval precision signal warms up without requiring explicit
	// memory_feedback calls for the denominator.
	if len(resultIDs) > 0 {
		// Best-effort: do not fail the recall if the increment fails.
		if err := e.backend.IncrementTimesRetrieved(ctx, resultIDs); err != nil {
			slog.Warn("auto-increment times_retrieved failed", "project", e.project, "err", err)
		}
	}

	return results, event.ID, nil
}

// FeedbackWithEvent records which results from a retrieval event were useful.
// It calls RecordFeedback (which increments times_retrieved/times_useful and
// recomputes precision) and also applies the edge boost and spaced-repetition
// boost via Feedback.
func (e *SearchEngine) FeedbackWithEvent(ctx context.Context, eventID string, usefulIDs []string) error {
	if err := e.backend.RecordFeedback(ctx, eventID, usefulIDs); err != nil {
		return err
	}
	if len(usefulIDs) > 0 {
		return e.Feedback(ctx, usefulIDs)
	}
	return nil
}

// FeedbackWithEventAndClass records which results from a retrieval event were useful,
// annotating the event with an optional failure class. When failureClass is non-empty
// the edge boost and spaced-repetition boost are skipped — misfired memories must not
// be reinforced. When failureClass is empty the behaviour is identical to FeedbackWithEvent.
func (e *SearchEngine) FeedbackWithEventAndClass(ctx context.Context, eventID string, usefulIDs []string, failureClass string) error {
	if err := e.backend.RecordFeedbackWithClass(ctx, eventID, usefulIDs, failureClass); err != nil {
		return err
	}
	if failureClass != "" || len(usefulIDs) == 0 {
		return nil
	}
	return e.Feedback(ctx, usefulIDs)
}

// Aggregate returns aggregated memory statistics grouped by the given dimension.
// Supported values for by: "tag", "type", "failure_class".
// filter is an optional prefix/value filter (not applicable for failure_class).
// limit caps the number of rows returned; limit < 1 is treated as the default (20).
func (e *SearchEngine) Aggregate(ctx context.Context, by, filter string, limit int) ([]types.AggregateRow, error) {
	if limit < 1 {
		limit = 20
	}
	switch by {
	case "tag", "type":
		return e.backend.AggregateMemories(ctx, e.project, by, filter, limit)
	case "failure_class":
		if filter != "" {
			return nil, fmt.Errorf("aggregate: filter not supported for failure_class")
		}
		return e.backend.AggregateFailureClasses(ctx, e.project, limit)
	default:
		return nil, fmt.Errorf("aggregate: unsupported by %q (must be tag, type, or failure_class)", by)
	}
}

// SummarizeNow: handled directly by the MCP tool via summarize package (see tools.go).
