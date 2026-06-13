package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/petersimmons1972/engram/internal/chunk"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/embedmodel"
	"github.com/petersimmons1972/engram/internal/envconf"
	"github.com/petersimmons1972/engram/internal/metrics"
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

type embedGateway interface {
	Enqueue(chunkIDs []string)
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
	// EmbedDegraded receives the embedder health status if the caller provides a
	// non-nil pointer. It is set to true if the embed operation timed out or
	// returned an error, causing fallback to BM25+recency. Nil = do not populate.
	EmbedDegraded *bool
	// EmbedDegradedReason receives the actual degradation cause when a non-nil
	// pointer is provided. Populated alongside EmbedDegraded; values are
	// "embed_timeout" (the embed deadline fired) or "embed_error" (hard failure
	// such as model crash or connection refused). Empty string when not degraded.
	// Nil = do not populate. (#989)
	EmbedDegradedReason *string
	// DateSince and DateBefore scope retrieval by memory valid_from timestamps.
	// Nil bounds leave that side unbounded.
	DateSince  *time.Time
	DateBefore *time.Time
	// TopicAnchorBoost enables the H-TAB scoring boost (LME experiment #3).
	// When true and the query is preference-shaped, topic-anchor tokens are
	// extracted from the query; preference memories whose content contains at
	// least one of those tokens receive an additional 1.25× score multiplier.
	// This targets multi-preference-session distraction: identical generic
	// preference language ("I like X") no longer causes majority-vote pull
	// toward off-topic sessions. Default false — composable with DualPreferenceRecall.
	TopicAnchorBoost bool
	// TopClusters controls the coarse-level fanout for MemPalace hierarchical recall
	// (ENGRAM_MEMPALACE_HIERARCHICAL_RECALL=true). 0 uses defaultTopClusters (3).
	// Ignored when the flag is off. (LME experiment #9)
	TopClusters int
	// ExactFactBoost enables the exact-fact / entity-identifier scoring boost
	// (LME experiment #4, issue #938 improvement #3). When true, memories whose
	// content contains a verbatim identifier from the query (named entity, URL,
	// phone number) receive an additive score boost calibrated to surface them
	// above high-cosine near-misses. Default false (ablation-safe).
	ExactFactBoost bool
	// Fusion enables Reciprocal Rank Fusion (RRF) candidate merging before
	// composite scoring (LME experiment #6, issue #938 improvement #1). When
	// true, the engine pre-computes rank positions from the vector and BM25 legs
	// and passes them to CompositeScoreRRF instead of CompositeScoreWithWeights.
	// Targets the gold_missing failure class (~168/271 incorrect where gold is
	// absent from top-10 entirely). Default false (ablatable).
	Fusion bool
	// ParaphraseUnion enables multi-pass paraphrased query union retrieval
	// (LME experiment #2, issue #938 improvement #4). Default false (ablatable).
	ParaphraseUnion bool

	// SessionNDCGAgg enables LEVER-8 session-DCG aggregation re-ranking.
	// When true, recalled memories are grouped by their "sid:<id>" tag, each
	// group's chunk cosine scores are aggregated via DCG, and sessions are
	// re-ranked by that aggregate. Within each session group, members are sorted
	// by composite score descending (P1 policy). This surfaces sessions where
	// multiple chunks each score mid-pack over sessions with a single high-scoring
	// chunk — directly targeting multi-session and temporal question types.
	//
	// Applied after composite scoring and before the topK truncation. Cleanly
	// ablatable: when false the code path is entirely bypassed and results are
	// identical to the baseline. Default false. (LEVER-8)
	SessionNDCGAgg bool

	// PreferenceMMR enables the H-NEW-2 centroid-MMR diversity pass for preference
	// queries. When true and the query is preference-shaped, the engine fetches
	// best-chunk embeddings for top candidates, computes a centroid of the top-10
	// results (the "dominant topic cluster"), and re-scores all candidates using
	// MMR: score = λ·relevance - (1-λ)·sim(doc, centroid). This surfaces
	// domain-specific preference sessions buried under the dominant topic.
	// Default OFF (false). Ablatable: flag-off → baseline identical.
	// Composable with DualPreferenceRecall (H15). Server-side only. (#H-NEW-2)
	PreferenceMMR bool
	// TemporalWindowRecall enables H-NEW-1 server-side two-pass date-windowed
	// recall. When true and QuestionText resolves to a temporal window via
	// ParseTemporalWindow(QuestionText, QuestionDate), Recall runs a second,
	// date-filtered pass and unions it with the unfiltered pass so the caller
	// receives a temporally-scoped candidate set. Default false = baseline.
	TemporalWindowRecall bool
	// QuestionText is the raw natural-language question used only for temporal
	// anchor parsing when TemporalWindowRecall is true. When empty the recall
	// query itself is used. Has no effect unless TemporalWindowRecall is true.
	QuestionText string
	// QuestionDate is the asked-on reference date (LongMemEval question_date
	// format, e.g. "2023/06/09 (Fri)") used to resolve relative anchors such as
	// "3 days ago". Has no effect unless TemporalWindowRecall is true.
	QuestionDate string

	// EvidenceFirstPack reorders the returned SearchResults by exact-signal score
	// before returning them to the caller (LME Phase 3, issue #938 improvement).
	// Memories whose content contains a verbatim identifier from the query (URL,
	// phone number, or quoted phrase) are moved to the front of the result slice.
	// Stable sort: equal-scoring entries keep their relative order.
	//
	// Enabled server-wide via ENGRAM_EVIDENCE_FIRST_PACK=true env var,
	// or per-request via the MCP evidence_first_pack argument.
	// Suppressed automatically for temporal-reasoning questions where chronological
	// ordering is load-bearing (mirrors the run.go precedence rule).
	// Default false (ablatable — no behavior change until explicitly enabled).
	EvidenceFirstPack bool
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
	rrfScore       float64 // populated by applyFusion when RecallOpts.Fusion=true; 0 otherwise
}

// defaultEmbedRecallTimeoutMS is the bounded timeout for the embed call during
// recall. On expiry the call degrades to BM25+recency; the parent context
// deadline is untouched. Configurable via ENGRAM_EMBED_RECALL_TIMEOUT_MS.
// Embed recall timeout used when the env var is unset. This value was restored
// to 500ms to preserve the production recall SLA contract.
const defaultEmbedRecallTimeoutMS = 500

// noEmbedTimeout is a sentinel stored in embedRecallTimeout to indicate that
// no per-embed deadline should be applied. The parent context's deadline (if
// any) governs the embed call instead. Set by SetEmbedRecallTimeout(0) which
// is the documented contract for "--embed-recall-timeout-ms=0 = no timeout".
const noEmbedTimeout = time.Duration(-1)

// SearchEngine is the core retrieval engine: it stores memories (chunked + embedded)
// and recalls them via composite vector+FTS scoring.
type SearchEngine struct {
	backend             db.Backend
	embedMu             sync.RWMutex // protects embedder; use getEmbedder() for all reads
	embedder            embed.Client
	project             string
	ollamaURL           string
	targetDims          int             // MRL truncation target; 0 = model native output
	ctx                 context.Context // parent lifecycle context — passed to workers via StartWithContext
	summarizer          *summarize.Worker
	reembedder          *reembed.Worker
	decayer             *DecayWorker
	weightCache         *WeightCache        // nil when no pgxpool available (pre-migration or test)
	globalReembedder    globalNotifier      // non-nil after SetGlobalReembedder; woken on chunk store
	embedGateway        embedGateway        // non-nil when ENGRAM_EMBED_GW_ENABLED=true
	PreferenceExtractor PreferenceExtractor // nil = extraction disabled; default PatternPreferenceExtractor{}
	hydeIndexer         *HydeIndexer        // nil when ENGRAM_HYDE_ENABLED=false or backend lacks hydeBackend
	// Paraphraser generates query variants for the ParaphraseUnion recall path.
	// Defaults to RuleBasedParaphraser{} when nil. Override via SetParaphraser.
	Paraphraser QueryParaphraser
	// embedRecallTimeout is the bounded embed deadline for recall. Read from
	// ENGRAM_EMBED_RECALL_TIMEOUT_MS at engine construction; default 500ms.
	embedRecallTimeout time.Duration

	// embedder metadata cache — eliminates 2-3 DB round-trips per recall/store.
	// Protected by embedderMetaMu, which is SEPARATE from embedMu to avoid
	// coupling the embedder-swap lock to the metadata-read lock.
	embedderMetaMu            sync.RWMutex
	embedderMetaCacheValid    bool
	cachedEmbedderName        string
	cachedEmbedderDims        int
	cachedMigrationInProgress bool
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

func (e *SearchEngine) SetEmbedGateway(g embedGateway) {
	e.embedGateway = g
}

// SetHydeIndexer wires a HydeIndexer so that StoreWithRawBody generates and
// stores a HyDE embedding for each memory when ENGRAM_HYDE_ENABLED=true.
// Call once after New(), with a nil guard: passing nil disables HyDE indexing.
func (e *SearchEngine) SetHydeIndexer(idx *HydeIndexer) {
	e.hydeIndexer = idx
}

// SetEmbedRecallTimeout overrides the embed call budget used during recall.
// When ms > 0 the engine uses that value instead of the 500ms default.
// When ms == 0, the embed timeout is disabled entirely: the parent context's
// deadline governs the embed call (documented contract for --embed-recall-timeout-ms=0).
// Call this after New() to thread the value from the --embed-recall-timeout-ms
// CLI flag (#932, #973).
func (e *SearchEngine) SetEmbedRecallTimeout(ms int) {
	if ms > 0 {
		e.embedRecallTimeout = time.Duration(ms) * time.Millisecond
	} else if ms == 0 {
		e.embedRecallTimeout = noEmbedTimeout
	}
	// ms < 0: no-op; leave the current value (env-var default or prior set).
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
		backend:             backend,
		embedder:            embedder,
		project:             project,
		ollamaURL:           ollamaURL,
		targetDims:          dims,
		ctx:                 ctx,
		summarizer:          sum,
		reembedder:          reb,
		decayer:             dec,
		weightCache:         wc,
		PreferenceExtractor: PatternPreferenceExtractor{}, // default; swap for Ollama-backed impl without changing callers
		// env var is the engine-level default; the --embed-recall-timeout-ms flag overrides it via SetEmbedRecallTimeout (called by main.go).
		// Apply the same 0 → noEmbedTimeout mapping as SetEmbedRecallTimeout so
		// ENGRAM_EMBED_RECALL_TIMEOUT_MS=0 disables the embed deadline at startup
		// (0*ms == 0, which would cause context.WithTimeout(ctx, 0) = immediate cancel, #973).
		embedRecallTimeout: func() time.Duration {
			ms := envconf.Int("ENGRAM_EMBED_RECALL_TIMEOUT_MS", defaultEmbedRecallTimeoutMS)
			if ms == 0 {
				return noEmbedTimeout
			}
			return time.Duration(ms) * time.Millisecond
		}(),
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

// storeChunksForMemory chunks content and returns records with NULL embeddings.
// It is used by both Store (new memories) and Correct (re-chunking after a
// content change). Background workers fill vectors after callers commit chunks.
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

	// Build chunk records with nil embeddings. Background embedding is handled by
	// GlobalReembedder or EmbedGateway; Store must never block on the embedder.
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
		// Enqueue chunks for the enabled background embedding path.
		chunkIDs := make([]string, len(chunks))
		for i, c := range chunks {
			chunkIDs[i] = c.ID
		}
		if err := e.backend.EnqueueChunkLeases(ctx, chunkIDs); err != nil {
			// Log but do not fail — reembed worker will find chunks via NULL embedding.
			slog.Warn("failed to enqueue chunk leases", "count", len(chunkIDs), "err", err)
		}
		if e.embedGateway != nil {
			e.embedGateway.Enqueue(chunkIDs)
			e.notifyEmbedQueue(ctx)
		} else {
			e.reembedder.Notify()
		}
		if e.globalReembedder != nil && e.embedGateway == nil {
			e.globalReembedder.Notify()
		}
	}
	// HyDE indexing: generate and store a hypothetical question embedding for
	// the memory. Best-effort: log but do not fail if this step errors.
	// Gated on ENGRAM_HYDE_ENABLED=true and hydeIndexer being set.
	if HydeEnabled() && e.hydeIndexer != nil {
		if idxErr := e.hydeIndexer.IndexHydeForMemory(ctx, m); idxErr != nil {
			slog.Warn("hyde: failed to index memory", "memory_id", m.ID, "err", idxErr)
		}
	}
	// Count each store call that returned before embedding completed.
	metrics.StoreEmbedAsyncTotal.Inc()
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
	var allChunkIDs []string
	for _, p := range prepared {
		if err := e.backend.StoreMemoryTx(ctx, tx, p.mem); err != nil {
			return err
		}
		if len(p.chunks) > 0 {
			if err := e.backend.StoreChunksTx(ctx, tx, p.chunks); err != nil {
				return err
			}
			for _, c := range p.chunks {
				allChunkIDs = append(allChunkIDs, c.ID)
			}
			hasChunks = true
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if hasChunks {
		// Enqueue all chunks for async re-embedding by setting initial leases.
		if err := e.backend.EnqueueChunkLeases(ctx, allChunkIDs); err != nil {
			// Log but do not fail — reembed worker will find chunks via NULL embedding.
			slog.Warn("failed to enqueue batch chunk leases", "count", len(allChunkIDs), "err", err)
		}
		if e.embedGateway != nil {
			e.embedGateway.Enqueue(allChunkIDs)
			e.notifyEmbedQueue(ctx)
		} else {
			e.reembedder.Notify()
		}
		if e.globalReembedder != nil && e.embedGateway == nil {
			e.globalReembedder.Notify()
		}
	}
	// Count each store-batch call that returned before embedding completed.
	// One increment per StoreBatch call (not per memory) — tracks call volume.
	metrics.StoreEmbedAsyncTotal.Inc()
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

	// H-NEW-1: server-side two-pass date-windowed temporal recall. Delegated here
	// (before any work) so the existing single-pass scoring path below stays
	// untouched and fully ablatable. The branch is taken only when the caller opts
	// in AND a temporal window resolves; otherwise recall is byte-for-byte baseline.
	if opts.TemporalWindowRecall {
		anchorText := opts.QuestionText
		if anchorText == "" {
			anchorText = query
		}
		if since, before := ParseTemporalWindow(anchorText, opts.QuestionDate); since != nil || before != nil {
			return e.recallTwoPassTemporal(ctx, query, topK, detail, opts, since, before)
		}
		// No window resolved (e.g. "how many X ago", non-temporal): fall through
		// to the unfiltered single-pass path below. TemporalWindowRecall=true is
		// harmless at this point — the flag is not re-checked on this path and
		// RecallWithOpts is not called recursively here.
	}

	// Guard: ensure the current embedder matches the stored metadata before
	// issuing any query. Without this check a dimension mismatch would produce
	// silent nil/empty results (or a pgvector cross-dim panic) instead of the
	// structured embed.PermanentError callers can inspect and act on.
	if err := e.checkEmbedderMeta(ctx); err != nil {
		return nil, err
	}

	// Bound the embed call to embedRecallTimeout (default 500ms; configurable via
	// ENGRAM_EMBED_RECALL_TIMEOUT_MS). On timeout or error, degrade gracefully to
	// BM25+recency only — the vector leg is skipped but recall still returns useful
	// results. The timeout applies to the embed call ONLY; the parent ctx's deadline
	// governs the rest of the operation (VectorSearch, FTS, etc.). This prevents a
	// hanging embedder from cancelling the parent context and surfacing a
	// "context canceled" error to callers.
	//
	// When embedRecallTimeout == noEmbedTimeout (set by SetEmbedRecallTimeout(0)),
	// no per-embed deadline is applied and the parent context governs the call
	// directly. This is the documented contract for --embed-recall-timeout-ms=0.
	var embedCtx context.Context
	var embedCancel context.CancelFunc
	if e.embedRecallTimeout == noEmbedTimeout {
		embedCtx, embedCancel = ctx, func() {}
	} else {
		embedCtx, embedCancel = context.WithTimeout(ctx, e.embedRecallTimeout)
	}
	defer embedCancel()
	queryVec, err := e.getEmbedder().Embed(embedCtx, query)
	embedDegraded := false
	embedDegradeReason := ""
	if err != nil {
		// Distinguish a deadline-exceeded embed (backend saturated or slow) from a
		// hard embed error (model crash, connection refused). Saturation is the
		// primary driver of recall quality degradation under batch reembed load
		// (#917); surfacing the reason at WARN lets operators act on it.
		degradeReason := "embed_error"
		if embedCtx.Err() != nil {
			degradeReason = "embed_timeout"
		}
		timeoutField := slog.Int64("timeout_ms", e.embedRecallTimeout.Milliseconds())
		if e.embedRecallTimeout == noEmbedTimeout {
			timeoutField = slog.String("timeout_ms", "disabled")
		}
		slog.Warn("embed query failed, degrading to BM25+recency only",
			"project", e.project, timeoutField,
			"reason", degradeReason, "err", err)
		queryVec = nil
		embedDegraded = true
		embedDegradeReason = degradeReason
		metrics.RecallEmbedTimeoutTotal.Inc()
		metrics.RecallDegradedTotal.WithLabelValues(degradeReason).Inc()
	}
	// Populate the caller's embed degradation signal if they provided a pointer.
	if opts.EmbedDegraded != nil {
		*opts.EmbedDegraded = embedDegraded
	}
	// Populate the caller's degradation reason if they provided a pointer (#989).
	if opts.EmbedDegradedReason != nil {
		*opts.EmbedDegradedReason = embedDegradeReason
	}

	// ANN vector search via pgvector HNSW index — skipped when embed degraded.
	var vecHits []db.VectorHit
	if queryVec != nil {
		// MemPalace hierarchical recall (LME experiment #9): when the env flag is
		// set, run coarse→fine cluster-filtered search; otherwise flat search.
		if HierarchicalRecallEnabled() {
			vecHits, err = hierarchicalVectorSearch(ctx, e.backend, e.project, queryVec, topK*3, opts.TopClusters, opts.DateSince, opts.DateBefore)
		} else {
			vecHits, err = e.backend.VectorSearchWithDateRange(ctx, e.project, queryVec, topK*3, opts.DateSince, opts.DateBefore)
		}
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
		res, err := e.backend.FTSSearch(ctx, e.project, query, topK*3, opts.DateSince, opts.DateBefore)
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

	// allChunkCosines tracks every chunk cosine per memory for LEVER-8 session-DCG
	// aggregation. Only populated when SessionNDCGAgg is enabled to avoid allocating
	// a second map on every recall call.
	var allChunkCosines map[string][]float64
	if opts.SessionNDCGAgg {
		allChunkCosines = make(map[string][]float64, len(vecHits))
	}

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
		if opts.SessionNDCGAgg {
			allChunkCosines[h.MemoryID] = append(allChunkCosines[h.MemoryID], cosine)
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

	// ParaphraseUnion path (LME experiment #2): run additional vector+FTS passes
	// for each paraphrase variant and union candidate sets before scoring.
	// Targets missing_recall failures where gold is an incidental mention.
	// When opts.ParaphraseUnion is false this block is entirely skipped.
	if opts.ParaphraseUnion {
		parer := e.Paraphraser
		if parer == nil {
			parer = RuleBasedParaphraser{}
		}
		paraphrases := parer.Paraphrase(query, 3)
		for _, pq := range paraphrases {
			// Vector pass for this variant (best-effort; errors are logged, not fatal).
			var pEmbedCtx context.Context
			var pEmbedCancel context.CancelFunc
			if e.embedRecallTimeout == noEmbedTimeout {
				pEmbedCtx, pEmbedCancel = ctx, func() {}
			} else {
				pEmbedCtx, pEmbedCancel = context.WithTimeout(ctx, e.embedRecallTimeout)
			}
			pVec, pErr := e.getEmbedder().Embed(pEmbedCtx, pq)
			pEmbedCancel()
			if pErr == nil && pVec != nil {
				pVecHits, pVErr := e.backend.VectorSearchWithDateRange(ctx, e.project, pVec, topK*3, opts.DateSince, opts.DateBefore)
				if pVErr == nil {
					for _, h := range pVecHits {
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
				} else {
					slog.Debug("paraphrase union: vector pass failed", "paraphrase", pq, "err", pVErr)
				}
			} else if pErr != nil {
				slog.Debug("paraphrase union: embed failed", "paraphrase", pq, "err", pErr)
			}
			// FTS pass for this variant.
			pFTSResults, pFErr := e.backend.FTSSearch(ctx, e.project, pq, topK*3, opts.DateSince, opts.DateBefore)
			if pFErr == nil {
				for _, r := range pFTSResults {
					// Prefer higher BM25 score across passes.
					if r.Score > ftsScores[r.Memory.ID] {
						ftsScores[r.Memory.ID] = r.Score
						if r.Score > maxBM25 {
							maxBM25 = r.Score
						}
					}
					if _, exists := memories[r.Memory.ID]; !exists {
						memories[r.Memory.ID] = r.Memory
					}
				}
			} else {
				slog.Debug("paraphrase union: FTS pass failed", "paraphrase", pq, "err", pFErr)
			}
		}
		// Batch-fetch any new vector-only candidates surfaced by paraphrase passes.
		if len(uniqueIDs) > 0 {
			newIDs := make([]string, 0, 8)
			for _, id := range uniqueIDs {
				if _, exists := memories[id]; !exists {
					newIDs = append(newIDs, id)
				}
			}
			if len(newIDs) > 0 {
				newMems, fetchErr := e.backend.GetMemoriesByIDs(ctx, e.project, newIDs)
				if fetchErr == nil {
					for _, m := range newMems {
						memories[m.ID] = m
					}
				} else {
					slog.Debug("paraphrase union: batch fetch failed", "err", fetchErr)
				}
			}
		}
	}

	// Detect query type once before the scoring loop.
	prefQuery := isPreferenceQuery(query)
	tempQuery := isTemporalQuery(query)
	kuQuery := isKnowledgeUpdateQuery(query)
	// Identifier query detection for exact-fact boost (LME #938 improvement #3).
	// Only active when the caller enables the flag; detection is cheap (regex).
	exactBoostEnabled := opts.ExactFactBoost && isIdentifierQuery(query)

	// RRF setup (LME experiment #6, issue #938 improvement #1): pre-compute
	// per-leg rank positions. applyFusion is a strict no-op when Fusion=false.
	vecRanks := rankVectorHits(vecHits)
	ftsRanks := rankFTSResults(ftsRes.results)
	applyFusion(opts, memories, bestHits, vecRanks, ftsRanks)

	// Resolve base weights: per-project cache when available, otherwise defaults.
	// Type-specific profiles override the cache — recency dominance is correct
	// regardless of per-project tuning for temporal and knowledge-update queries.
	baseWeights := DefaultWeights()
	if e.weightCache != nil {
		baseWeights = e.weightCache.Get(ctx, e.project)
	}
	switch {
	case tempQuery:
		baseWeights = TemporalWeights()
	case kuQuery:
		baseWeights = KnowledgeUpdateWeights()
	}

	// H-TAB (LME exp #3): pre-compute topic-anchor tokens from the query when
	// TopicAnchorBoost is set and the query is preference-shaped. Done once
	// outside the scoring loop so every candidate can be checked in O(1) space.
	var topicAnchorTokens []string
	if opts.TopicAnchorBoost && prefQuery {
		topicAnchorTokens = extractTopicAnchorTokens(query)
	}

	// For temporal queries, pre-collect the candidate slice once so
	// CompositeScoreWithRankNorm can compute the date range across the full set.
	// This avoids an O(n²) re-scan: candidateDateRange runs once via the slice.
	var temporalCandidates []*types.Memory
	if tempQuery {
		temporalCandidates = make([]*types.Memory, 0, len(memories))
		for _, m := range memories {
			temporalCandidates = append(temporalCandidates, m)
		}
	}

	// Composite scoring per memory.
	results := make([]types.SearchResult, 0)
	for id, m := range memories {
		// Extracted preference memories are ingest-time fragments tagged
		// "extracted-preference". Surface them only for preference-shaped queries;
		// for all other query types they add noise and dilute recall quality.
		if !prefQuery {
			skip := false
			for _, t := range m.Tags {
				if t == "extracted-preference" {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}
		bm25 := 0.0
		if maxBM25 > 0 {
			bm25 = ftsScores[id] / maxBM25
		}
		hit := bestHits[id]
		exactMatch := exactBoostEnabled && ExactIdentifierHit(m.Content, query)
		input := ScoreInput{
			Cosine:             hit.cosine,
			BM25:               bm25,
			HoursSince:         temporalAnchorHours(*m),
			Importance:         m.Importance,
			DynamicImportance:  m.DynamicImportance,
			RetrievalPrecision: m.RetrievalPrecision,
			EpisodeMatch:       opts.CurrentEpisodeID != "" && m.EpisodeID == opts.CurrentEpisodeID,
			MemoryType:         m.MemoryType,
			IsPreferenceQuery:  prefQuery,
			// H-TAB: set when TopicAnchorBoost is on and the memory content
			// contains at least one domain token from the recall query.
			TopicAnchorMatch:     len(topicAnchorTokens) > 0 && contentContainsTopicAnchor(m.Content, topicAnchorTokens),
			ExactIdentifierMatch: exactMatch,
		}
		var score float64
		if opts.Fusion {
			// RRF path: use pre-computed rank fusion score in place of raw cosine/BM25.
			score = CompositeScoreRRF(input, baseWeights, hit.rrfScore)
		} else if tempQuery {
			var validFrom time.Time
			if m.ValidFrom != nil {
				validFrom = *m.ValidFrom
			}
			score = CompositeScoreWithRankNorm(input, baseWeights, validFrom, temporalCandidates)
		} else {
			score = CompositeScoreWithWeights(input, baseWeights)
		}

		// LME Experiment #5: apply validity window boost when ENGRAM_VALIDITY_WINDOW_FILTER=true.
		// Multiplies the composite score by a valid_from recency factor so that
		// more-recently-dated memories rank above older ones for the same query.
		// Flag OFF (default) → vwBoost = 1.0, score unchanged (baseline-safe).
		//
		// Guard: do NOT apply on temporal queries (tempQuery=true). CompositeScoreWithRankNorm
		// already incorporates valid_from via RankNormalizedRecency; applying vwBoost there
		// would double-count the recency signal. The experiment target is knowledge-update
		// queries (kuQuery), which use CompositeScoreWithWeights and have no rank-norm path.
		vwBoost := ValidFromBoost(m.ValidFrom)
		if !tempQuery {
			score *= vwBoost
		} else {
			// Record 1.0 in the breakdown so observability accurately reflects that
			// no extra boost was applied on this path.
			vwBoost = 1.0
		}

		result := types.SearchResult{
			Memory:     m,
			Score:      score,
			ChunkScore: hit.cosine,
			ScoreBreakdown: func() map[string]float64 {
				bd := map[string]float64{
					"cosine":           hit.cosine,
					"bm25":             bm25,
					"recency":          RecencyDecay(input.HoursSince),
					"episode_boost":    1.0,
					"valid_from_boost": vwBoost,
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
				if input.ExactIdentifierMatch {
					bd["exact_identifier_boost"] = exactIdentifierBoost()
				}
				return bd
			}(),
			MatchedChunk:        hit.chunkText,
			MatchedChunkIndex:   hit.chunkIndex,
			MatchedChunkSection: hit.sectionHeading,
		}
		switch detail {
		case "id_only":
			// Intentionally minimal: only the ID is returned. All other fields
			// (Content, PatternConfidence, Tags, etc.) are stripped by design.
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

	// LEVER-8: session-DCG aggregation re-ranking.
	// When SessionNDCGAgg is enabled, group results by their "sid:" tag and
	// re-rank sessions by the DCG of their constituent chunk cosines. Applied
	// immediately after composite scoring so all other post-processing
	// (preference-first, re-ranker, topK truncation) sees the session-packed order.
	// When disabled this is a no-op (sessionNDCGRerank returns results unchanged).
	//
	// Bug 2 guard: session-NDCG is skipped when prefQuery is active.
	// The preference-first split/reassemble (below) and session-DCG aggregation
	// have conflicting ordering semantics: running session-NDCG first promotes a
	// low-DCG session's preference memory above a high-DCG session's general
	// memories, breaking the preference-first contract. Skipping session-NDCG for
	// preference queries is correct because preference memories are typically
	// singletons (no sid: tag) — session packing adds no value here — and the
	// preference-first path already produces coherent ordering.
	results = sessionNDCGRerank(results, allChunkCosines, opts.SessionNDCGAgg && !prefQuery)

	// Preference-first recall path: when the query is preference-shaped, ensure
	// preference-typed memories are represented in the top results even if their
	// raw composite scores are lower than irrelevant context memories.
	// Strategy: split sorted results into preference-typed and general pools;
	// take up to min(topK/2, 10) from the preference pool first, fill remaining
	// slots from the general pool, deduplicate by ID, reassemble.
	//
	// topicPool captures the relevance-ranked general (non-preference) pool before
	// the preference-front-load merge. Used by applyPreferenceMMR (H-NEW-2) as the
	// dominant-topic centroid source so the centroid reflects the actual topic
	// cluster rather than the preference-front-loaded order.
	var topicPool []types.SearchResult
	if prefQuery {
		prefSlots := topK / 2
		if prefSlots > 10 {
			prefSlots = 10
		}
		if prefSlots < 1 {
			prefSlots = 1
		}
		var prefResults, generalResults []types.SearchResult
		for _, r := range results {
			if r.Memory != nil && r.Memory.MemoryType == "preference" {
				prefResults = append(prefResults, r)
			} else {
				generalResults = append(generalResults, r)
			}
		}
		// Capture the general pool (relevance-ranked, no preference front-load) for
		// use as the dominant-topic centroid source in the MMR pass (Bug 2 fix).
		topicPool = generalResults
		if len(prefResults) > 0 {
			// Take up to prefSlots from preference pool.
			taken := prefSlots
			if taken > len(prefResults) {
				taken = len(prefResults)
			}
			merged := make([]types.SearchResult, 0, topK)
			seen := make(map[string]struct{}, topK)
			for i := 0; i < taken; i++ {
				merged = append(merged, prefResults[i])
				if prefResults[i].Memory != nil {
					seen[prefResults[i].Memory.ID] = struct{}{}
				}
			}
			// Fill remaining slots from general pool (already sorted by score).
			for _, r := range generalResults {
				if len(merged) >= topK {
					break
				}
				id := ""
				if r.Memory != nil {
					id = r.Memory.ID
				}
				if _, dup := seen[id]; dup {
					continue
				}
				merged = append(merged, r)
				if id != "" {
					seen[id] = struct{}{}
				}
			}
			// Also add any remaining preference results that fit.
			for i := taken; i < len(prefResults); i++ {
				if len(merged) >= topK {
					break
				}
				id := ""
				if prefResults[i].Memory != nil {
					id = prefResults[i].Memory.ID
				}
				if _, dup := seen[id]; dup {
					continue
				}
				merged = append(merged, prefResults[i])
				if id != "" {
					seen[id] = struct{}{}
				}
			}
			results = merged
		}
	}

	// H-NEW-2: Centroid-MMR diversity pass for preference queries.
	// When PreferenceMMR is enabled and the query is preference-shaped, re-score
	// all candidates by penalising similarity to the dominant-topic centroid.
	// This surfaces domain-specific preference sessions buried under the dominant
	// topic (e.g. books/reading flooding a generic preference query).
	//
	// The pass is entirely post-processing — no schema changes, no re-ingest.
	// It runs AFTER the preference-first pool split (above) so both paths benefit.
	// Flag-off (PreferenceMMR=false): this block is skipped → baseline identical.
	//
	// topicPool is passed so the centroid reflects the dominant topic (general pool)
	// rather than the preference-front-loaded merged order (Bug 2 fix).
	if opts.PreferenceMMR && prefQuery && len(results) > 1 {
		results = e.applyPreferenceMMR(ctx, results, topicPool)
	}

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

	if opts.Mode != "handle" {
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
	}

	// Preserve access heat for pruning/retention logic even when handle mode
	// skips the heavier graph-enrichment path.
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
			if err := e.backend.UpdateChunkLastMatched(ctx, hit.chunkID); err != nil {
				slog.Warn("recall: update last-matched failed", "err", err)
			}
		}
	}

	return results, nil
}

// recallTwoPassTemporal implements H-NEW-1: a date-windowed two-pass recall.
//
// Pass 1 is the standard unfiltered recall (topical scope). Pass 2 re-runs recall
// constrained to [since, before) via the indexed valid_from filter (temporal scope).
// The two result sets are unioned with pass-1 order preserved first, deduplicated by
// memory ID, and truncated to topK. This gives the model a candidate set that is both
// topically relevant AND temporally scoped, so it can resolve which retrieved session
// belongs to the asked-about time window — the failure mode behind temporal PARTIALs.
//
// Both passes clear TemporalWindowRecall to prevent re-entry, and pass 1 clears any
// DateSince/DateBefore so it stays a true unfiltered baseline pass.
//
// Re-ranking: both internal passes suppress opts.Reranker so the reranker runs
// exactly once — on the merged, deduplicated, topK-truncated result set. Running
// it per-pass would produce two independently-ranked lists merged in incoherent
// order and double the latency/cost.
func (e *SearchEngine) recallTwoPassTemporal(ctx context.Context, query string, topK int, detail string, opts RecallOpts, since, before *time.Time) ([]types.SearchResult, error) {
	pass1Opts := opts
	pass1Opts.TemporalWindowRecall = false
	pass1Opts.QuestionText = ""
	pass1Opts.QuestionDate = ""
	pass1Opts.DateSince = nil
	pass1Opts.DateBefore = nil
	// Suppress reranker inside both passes; apply it once to the merged result below.
	pass1Opts.Reranker = nil
	pass1, err := e.RecallWithOpts(ctx, query, topK, detail, pass1Opts)
	if err != nil {
		return nil, err
	}

	pass2Opts := pass1Opts
	pass2Opts.DateSince = since
	pass2Opts.DateBefore = before
	// EmbedDegraded pointers were populated by pass 1; do not let pass 2 overwrite
	// the caller-visible signal (a degraded pass 1 is the meaningful event).
	pass2Opts.EmbedDegraded = nil
	pass2Opts.EmbedDegradedReason = nil
	// pass2Opts.Reranker is already nil (inherited from pass1Opts above).
	pass2, err := e.RecallWithOpts(ctx, query, topK, detail, pass2Opts)
	if err != nil {
		// Pass 2 is additive. If the date-filtered pass fails, return pass 1 rather
		// than failing the whole recall — a topically-scoped set still beats nothing.
		// Apply reranker to pass 1 alone before returning so the caller still gets
		// reranked output when pass 2 is unavailable.
		slog.Warn("temporal-window recall: pass 2 failed, returning pass 1 only",
			"project", e.project, "err", err)
		return e.maybeRerank(ctx, query, topK, pass1, opts.Reranker), nil
	}

	merged := mergeSearchResults(pass1, pass2, topK)
	// Apply reranker once to the coherent merged set.
	return e.maybeRerank(ctx, query, topK, merged, opts.Reranker), nil
}

// maybeRerank applies reranker to results if reranker is non-nil, otherwise returns
// results unchanged. It mirrors the reranking block in RecallWithOpts so the
// temporal two-pass path and the single-pass path produce identically-sorted output.
func (e *SearchEngine) maybeRerank(ctx context.Context, query string, topK int, results []types.SearchResult, reranker ResultReranker) []types.SearchResult {
	if reranker == nil || len(results) == 0 {
		return results
	}
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
	reranked, err := reranker.RerankResults(ctx, query, items)
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
	return results
}

// mergeSearchResults unions two result slices, preserving the order of a first and
// appending only the IDs from b that a did not already contain, then truncates to
// limit. Results with a nil Memory are dropped. Pass-1 order is preserved because the
// unfiltered pass carries the composite-scored ranking the caller expects on top.
func mergeSearchResults(a, b []types.SearchResult, limit int) []types.SearchResult {
	merged := make([]types.SearchResult, 0, len(a)+len(b))
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, r := range a {
		if r.Memory == nil {
			continue
		}
		if _, dup := seen[r.Memory.ID]; dup {
			continue
		}
		seen[r.Memory.ID] = struct{}{}
		merged = append(merged, r)
	}
	for _, r := range b {
		if r.Memory == nil {
			continue
		}
		if _, dup := seen[r.Memory.ID]; dup {
			continue
		}
		seen[r.Memory.ID] = struct{}{}
		merged = append(merged, r)
	}
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

// RecallWithinMemory returns up to topK chunks from a single memory's document
// most semantically similar to query, projected into minimal *types.Memory
// values so callers get the chunk text alongside the parent memory's ID.
// Used by the A5 memory_query_document tool's semantic path.
func (e *SearchEngine) RecallWithinMemory(ctx context.Context, query string, memoryID string, topK int, detail string) ([]*types.Memory, error) {
	if topK <= 0 {
		topK = 10
	}
	// Guard: reject calls when the stored embedder name does not match the
	// current client — same check wired into RecallWithOpts and StoreBatch.
	// Without this, dimension or model mismatches return garbage silently.
	if err := e.checkEmbedderMeta(ctx); err != nil {
		return nil, err
	}

	// Embed call bounded by embedRecallTimeout, consistent with RecallWithOpts.
	// On embed timeout or error, degrade to keyword-scored chunk search using
	// GetChunksForMemory — mirrors the BM25+recency fallback in RecallWithOpts.
	// When embedRecallTimeout == noEmbedTimeout, the parent context governs
	// the embed call directly (--embed-recall-timeout-ms=0 contract, #973).
	var embedCtx context.Context
	var embedCancel context.CancelFunc
	if e.embedRecallTimeout == noEmbedTimeout {
		embedCtx, embedCancel = ctx, func() {}
	} else {
		embedCtx, embedCancel = context.WithTimeout(ctx, e.embedRecallTimeout)
	}
	defer embedCancel()
	queryVec, embedErr := e.getEmbedder().Embed(embedCtx, query)
	if embedErr != nil {
		// If the PARENT ctx was cancelled or expired (not just the embed child ctx),
		// propagate the parent error rather than silently degrading to BM25 results.
		// Only degrade when the embed itself failed while the parent ctx is still alive.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		// Distinguish timeout (backend saturated) from hard error (#917).
		degradeReason := "embed_error"
		if embedCtx.Err() != nil {
			degradeReason = "embed_timeout"
		}
		metrics.RecallEmbedTimeoutTotal.Inc()
		metrics.RecallDegradedTotal.WithLabelValues(degradeReason).Inc()
		withinTimeoutField := slog.Int64("timeout_ms", e.embedRecallTimeout.Milliseconds())
		if e.embedRecallTimeout == noEmbedTimeout {
			withinTimeoutField = slog.String("timeout_ms", "disabled")
		}
		slog.Warn("RecallWithinMemory embed failed, degrading to keyword search",
			"memory_id", memoryID, withinTimeoutField,
			"reason", degradeReason, "err", embedErr)
		return e.recallWithinMemoryKeywordFallback(ctx, query, memoryID, topK)
	}
	chunks, err := e.backend.SearchChunksWithinMemory(ctx, queryVec, memoryID, topK)
	if err != nil {
		return nil, err
	}
	_ = detail // currently all modes return the chunk text verbatim
	out := make([]*types.Memory, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, &types.Memory{
			ID:      c.MemoryID,
			Content: c.ChunkText,
			Project: c.Project,
		})
	}
	return out, nil
}

// recallWithinMemoryKeywordFallback is the degraded-signal path for
// RecallWithinMemory when the embedder is unavailable. It fetches all chunks
// for the memory and ranks them by simple term-overlap against the query
// (case-insensitive unigrams), returning up to topK results. This preserves
// usefulness of the document-query tool even when the vector index is cold.
func (e *SearchEngine) recallWithinMemoryKeywordFallback(ctx context.Context, query, memoryID string, topK int) ([]*types.Memory, error) {
	all, err := e.backend.GetChunksForMemory(ctx, memoryID)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return []*types.Memory{}, nil
	}

	// Build query term set (lowercase unigrams, ignore single-char tokens).
	queryTerms := make(map[string]struct{})
	for _, tok := range strings.Fields(strings.ToLower(query)) {
		tok = strings.Trim(tok, ".,;:!?\"'()[]{}") // strip punctuation
		if len(tok) > 1 {
			queryTerms[tok] = struct{}{}
		}
	}

	type scored struct {
		chunk *types.Chunk
		score float64
	}
	scoredChunks := make([]scored, 0, len(all))
	for _, c := range all {
		if len(queryTerms) == 0 {
			scoredChunks = append(scoredChunks, scored{chunk: c, score: 0})
			continue
		}
		text := strings.ToLower(c.ChunkText)
		var hits int
		for term := range queryTerms {
			if strings.Contains(text, term) {
				hits++
			}
		}
		scoredChunks = append(scoredChunks, scored{
			chunk: c,
			score: float64(hits) / float64(len(queryTerms)),
		})
	}

	// Sort descending by score, then ascending by chunk_index for ties.
	sort.SliceStable(scoredChunks, func(i, j int) bool {
		if scoredChunks[i].score != scoredChunks[j].score {
			return scoredChunks[i].score > scoredChunks[j].score
		}
		return scoredChunks[i].chunk.ChunkIndex < scoredChunks[j].chunk.ChunkIndex
	})

	if topK > 0 && len(scoredChunks) > topK {
		scoredChunks = scoredChunks[:topK]
	}
	out := make([]*types.Memory, 0, len(scoredChunks))
	for _, s := range scoredChunks {
		out = append(out, &types.Memory{
			ID:      s.chunk.MemoryID,
			Content: s.chunk.ChunkText,
			Project: s.chunk.Project,
		})
	}
	return out, nil
}

// embedderAliases maps known alternative name strings for the same underlying
// model to a single canonical form. This allows GGUF filenames (as reported by
// llama.cpp / olla) and HuggingFace IDs (as reported by Infinity / LiteLLM) to
// be treated as compatible without triggering a false embedder_mismatch error.
//
// Rules: every alias for a model family must map to the same canonical string.
// When adding a new quant variant, add it here — do not add fuzzy matching.
var embedderAliases = map[string]string{
	// BAAI/bge-m3 — official embedding model (2026-05-08+)
	"bge-m3-Q8_0.gguf":   "BAAI/bge-m3",
	"bge-m3-Q4_K_M.gguf": "BAAI/bge-m3",
	"bge-m3":             "BAAI/bge-m3",
	"BAAI/bge-m3":        "BAAI/bge-m3",
	"bge-m3:latest":      "BAAI/bge-m3",
	"BAAI/bge-m3:latest": "BAAI/bge-m3",
}

// canonicalEmbedderName returns the canonical model identifier for s.
// If s has no alias entry, it is returned unchanged.
func canonicalEmbedderName(s string) string {
	if c := embedmodel.CanonicalName(s); c != "" {
		return c
	}
	if c, ok := embedderAliases[s]; ok {
		return c
	}
	return s
}

func (e *SearchEngine) notifyEmbedQueue(ctx context.Context) {
	pgb, ok := e.backend.(pgPooler)
	if !ok || pgb.PgxPool() == nil {
		return
	}
	go func() {
		notifyCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if _, err := pgb.PgxPool().Exec(notifyCtx, "SELECT pg_notify($1, '')", embedmodel.PGNotifyChannel); err != nil {
			slog.Warn("failed to notify embed queue", "err", err)
		}
	}()
}

// checkEmbedderMeta ensures the stored embedder name matches the current client,
// or registers it if this is the first store for the project.
//
// Fast path: when the in-process cache is warm and no migration is in progress,
// the three checks (name match, migration flag, dims match) are satisfied from
// memory without any DB round-trips (#929).
func (e *SearchEngine) checkEmbedderMeta(ctx context.Context) error {
	emb := e.getEmbedder()

	// ── Fast path ────────────────────────────────────────────────────────────
	e.embedderMetaMu.RLock()
	if e.embedderMetaCacheValid && !e.cachedMigrationInProgress {
		cachedName := e.cachedEmbedderName
		cachedDims := e.cachedEmbedderDims
		e.embedderMetaMu.RUnlock()

		if canonicalEmbedderName(cachedName) != canonicalEmbedderName(emb.Name()) {
			return &embed.PermanentError{
				Code:        "embedder_mismatch",
				Stored:      cachedName,
				Current:     emb.Name(),
				Remediation: "run memory_migrate_embedder",
			}
		}
		if cachedDims != 0 && cachedDims != emb.Dimensions() {
			return &embed.PermanentError{
				Code:        "embedder_mismatch",
				Stored:      fmt.Sprintf("%d-dim", cachedDims),
				Current:     fmt.Sprintf("%d-dim", emb.Dimensions()),
				Remediation: "run memory_migrate_embedder",
			}
		}
		return nil
	}
	e.embedderMetaMu.RUnlock()

	// ── Slow path: read from DB ───────────────────────────────────────────────
	storedName, ok, err := e.backend.GetMeta(ctx, e.project, "embedder_name")
	if err != nil {
		return err
	}
	if !ok {
		// First store for this project — register the embedder.
		if err := e.backend.SetMeta(ctx, e.project, "embedder_name", canonicalEmbedderName(emb.Name())); err != nil {
			return err
		}
		if err := e.backend.SetMeta(ctx, e.project, "embedder_dimensions",
			fmt.Sprintf("%d", emb.Dimensions())); err != nil {
			return err
		}
		// Populate cache after successful registration.
		e.embedderMetaMu.Lock()
		e.cachedEmbedderName = canonicalEmbedderName(emb.Name())
		e.cachedEmbedderDims = emb.Dimensions()
		e.cachedMigrationInProgress = false
		e.embedderMetaCacheValid = true
		e.embedderMetaMu.Unlock()
		return nil
	}
	if canonicalEmbedderName(storedName) != canonicalEmbedderName(emb.Name()) {
		return &embed.PermanentError{
			Code:        "embedder_mismatch",
			Stored:      storedName,
			Current:     emb.Name(),
			Remediation: "run memory_migrate_embedder",
		}
	}
	// Skip dimension check if migration is in progress — the new model may have
	// different dimensions, and embedder_dimensions will be reset once re-embedding
	// completes.
	migrating, _, _ := e.backend.GetMeta(ctx, e.project, "embedding_migration_in_progress")
	if migrating == "true" {
		// Cache the migration-in-progress state so subsequent calls skip the
		// dimension check without hitting the DB, but keep cacheValid=false so
		// the slow path re-runs once migration completes and the flag clears.
		e.embedderMetaMu.Lock()
		e.cachedMigrationInProgress = true
		e.embedderMetaCacheValid = false
		e.embedderMetaMu.Unlock()
		return nil
	}
	storedDimsStr, ok, err := e.backend.GetMeta(ctx, e.project, "embedder_dimensions")
	if err != nil {
		return err
	}
	var storedDims int
	if ok {
		storedDims, err = strconv.Atoi(storedDimsStr)
		if err != nil {
			return fmt.Errorf("embedder_dimensions metadata is corrupt: %w", err)
		}
		if storedDims != emb.Dimensions() {
			return &embed.PermanentError{
				Code:        "embedder_mismatch",
				Stored:      fmt.Sprintf("%d-dim", storedDims),
				Current:     fmt.Sprintf("%d-dim", emb.Dimensions()),
				Remediation: "run memory_migrate_embedder",
			}
		}
	}
	// All checks passed — populate cache.
	e.embedderMetaMu.Lock()
	e.cachedEmbedderName = canonicalEmbedderName(storedName)
	e.cachedEmbedderDims = storedDims
	e.cachedMigrationInProgress = false
	e.embedderMetaCacheValid = true
	e.embedderMetaMu.Unlock()
	return nil
}

func hoursSince(t time.Time) float64 {
	return time.Since(t).Hours()
}

// temporalAnchorHours returns the hours since the memory's effective date.
// When ValidFrom is set (populated from a date: tag at ingest), it reflects
// the actual session date rather than the arbitrary last-access time, giving
// the recency scorer accurate temporal ordering across ingested sessions.
func temporalAnchorHours(m types.Memory) float64 {
	if m.ValidFrom != nil {
		return hoursSince(*m.ValidFrom)
	}
	return hoursSince(m.LastAccessed)
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
// patternConfidence: nil means "do not touch this field".
func (e *SearchEngine) Correct(ctx context.Context, id string, content *string, tags []string, importance *int, patternConfidence *float64) (*types.Memory, error) {
	mem, err := e.backend.UpdateMemory(ctx, id, content, tags, importance, patternConfidence)
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
		if len(chunks) > 0 {
			chunkIDs := make([]string, len(chunks))
			for i, c := range chunks {
				chunkIDs[i] = c.ID
			}
			if err := e.backend.EnqueueChunkLeases(ctx, chunkIDs); err != nil {
				slog.Warn("failed to enqueue corrected chunk leases", "count", len(chunkIDs), "err", err)
			}
			if e.embedGateway != nil {
				e.embedGateway.Enqueue(chunkIDs)
				e.notifyEmbedQueue(ctx)
			} else {
				e.reembedder.Notify()
			}
			if e.globalReembedder != nil && e.embedGateway == nil {
				e.globalReembedder.Notify()
			}
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

// migrateConfirmThreshold is the chunk count above which an explicit confirm
// is required before MigrateEmbedder will null all embeddings.
// At the default 1000-chunk threshold an accidental alias-rename is blocked;
// a deliberate large migration passes confirm=true.
const migrateConfirmThreshold = 1000

// MigrateParams holds the arguments for MigrateEmbedder.
// Separating params from the method signature lets us add guards without
// changing every call site each time a new safeguard is added.
type MigrateParams struct {
	// NewModel is the raw model name (alias or canonical) to migrate to.
	NewModel string
	// NewDims is the embedding dimension of the new model.
	// When non-zero and equal to the stored dimension, Force must be true.
	// When zero the dimension guard is skipped (dimension pre-flight in the
	// MCP handler already validated it before calling MigrateEmbedder).
	NewDims int
	// Force allows a same-dimension migration to proceed. Without it, a
	// same-dim migrate is refused to prevent accidental re-embedding when
	// vectors are still reusable.
	Force bool
	// DryRun returns chunks_would_null without nulling anything.
	DryRun bool
	// Confirm must be true when the affected chunk count exceeds
	// migrateConfirmThreshold. Prevents accidental mass-null on large corpora.
	Confirm bool
}

// MigrateEmbedder initiates an embedding migration to a new model by nulling all
// existing embeddings and recording the new model name in project metadata.
// A background reembed worker will repopulate embeddings after this call.
//
// Safety guards (applied in order, highest priority first):
//  G1 — Same-canonical-identity: returns no-op with chunks_nulled=0.
//  G2 — Same dimension without force: soft-refused (result["error"] set).
//  G3 — dry_run: counts affected chunks without nulling.
//       Large volume without confirm: soft-refused.
//  G4 — Canonical stamp: stores canonicalEmbedderName(NewModel) in meta.
func (e *SearchEngine) MigrateEmbedder(ctx context.Context, p MigrateParams) (map[string]any, error) {
	newModel := p.NewModel

	// ── G1: Same-canonical-identity guard ────────────────────────────────────
	// Read the currently stored embedder name from meta.
	storedName, _, _ := e.backend.GetMeta(ctx, e.project, "embedder_name")
	if storedName != "" && canonicalEmbedderName(newModel) == canonicalEmbedderName(storedName) {
		return map[string]any{
			"chunks_nulled": 0,
			"new_model":     canonicalEmbedderName(newModel),
			"status":        "identity unchanged",
		}, nil
	}

	// ── G2: Same-dimension guard ──────────────────────────────────────────────
	// If caller supplied NewDims and they match the stored dims, require Force.
	if p.NewDims > 0 && !p.Force {
		storedDimsStr, ok, _ := e.backend.GetMeta(ctx, e.project, "embedder_dimensions")
		if ok && storedDimsStr != "" {
			var storedDims int
			if _, scanErr := fmt.Sscanf(storedDimsStr, "%d", &storedDims); scanErr == nil && storedDims > 0 {
				if p.NewDims == storedDims {
					return map[string]any{
						"error":  fmt.Sprintf("new model produces %d-dim vectors (same as current) — existing vectors are still reusable; pass force=true to force a full re-embed", p.NewDims),
						"status": "refused",
					}, nil
				}
			}
		}
	}

	// ── G3a: dry_run — count without nulling ──────────────────────────────────
	chunkCount, countErr := e.backend.CountProjectChunks(ctx, e.project)
	if countErr != nil {
		return nil, fmt.Errorf("count chunks for volume guard: %w", countErr)
	}
	if p.DryRun {
		return map[string]any{
			"chunks_would_null": chunkCount,
			"new_model":         canonicalEmbedderName(newModel),
			"status":            "dry_run",
		}, nil
	}

	// ── G3b: Volume guard — require confirm for large corpus ──────────────────
	if chunkCount >= migrateConfirmThreshold && !p.Confirm {
		return map[string]any{
			"error":             fmt.Sprintf("%d chunks would be nulled — this is a large corpus; pass confirm=true to proceed", chunkCount),
			"chunks_would_null": chunkCount,
			"status":            "refused",
		}, nil
	}

	// ── Execute migration ─────────────────────────────────────────────────────
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
	// ── G4: Stamp canonical, not raw ─────────────────────────────────────────
	canonicalNewModel := canonicalEmbedderName(newModel)
	if err := e.backend.SetMetaTx(ctx, tx, e.project, "embedder_name", canonicalNewModel); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Invalidate embedder metadata cache — the DB now holds a different
	// embedder_name and the migration_in_progress flag is set (#929).
	e.embedderMetaMu.Lock()
	e.embedderMetaCacheValid = false
	e.embedderMetaMu.Unlock()

	if e.embedGateway != nil {
		e.embedGateway.Enqueue([]string{"migration"})
		e.notifyEmbedQueue(ctx)
		// Invalidate cache before early return so the embed-gateway path also
		// gets a clean slate on the next checkEmbedderMeta call (#929).
		return map[string]any{
			"chunks_nulled": nulled,
			"new_model":     canonicalNewModel,
			"status":        "migration queued — embed gateway will drain NULL embeddings",
		}, nil
	}

	// Stop old reembed worker and create a new one with the new model.
	// Without this, the worker holds a stale reference to the original embedder
	// and never runs because its done channel was already closed at construction.
	e.reembedder.Stop()

	// ctx is used only for the startup probe; the returned client is context-independent.
	newEmbedder, err := embed.NewLiteLLMClient(ctx, e.ollamaURL, canonicalNewModel, "", e.targetDims)
	if err != nil {
		return nil, fmt.Errorf("create embedder for new model %q: %w", canonicalNewModel, err)
	}
	e.embedMu.Lock()
	e.embedder = newEmbedder
	e.embedMu.Unlock()

	e.reembedder = reembed.NewWorker(e.backend, newEmbedder, e.project, true)
	e.reembedder.StartWithContext(e.ctx)

	return map[string]any{
		"chunks_nulled": nulled,
		"new_model":     canonicalNewModel,
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
	var totalMerged, totalReviewed, batchErrors int
	for start := 0; start < len(candidates); start += batchSize {
		end := start + batchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]
		decisions, err := reviewer.ReviewMergeCandidates(ctx, batch)
		if err != nil {
			slog.Warn("consolidate: reviewer batch failed", "err", err)
			batchErrors++
			metrics.ConsolidateBatchErrors.Inc()
			continue
		}
		totalReviewed += len(batch)
		for _, d := range decisions {
			if !d.ShouldMerge {
				continue
			}
			// Atomic merge (#104): update winner content + delete loser in one tx.
			if err := e.backend.MergeMemoriesAtomic(ctx, e.project, d.MemoryAID, d.MemoryBID, d.MergedContent); err != nil {
				slog.Warn("consolidate: merge atomic failed", "err", err)
				batchErrors++
				metrics.ConsolidateBatchErrors.Inc()
				continue
			}
			totalMerged++
		}
	}

	result["merged_memories"] = totalMerged
	result["candidates_reviewed"] = totalReviewed
	result["lsh_candidates"] = len(lshPairs)
	result["jaccard_passed"] = len(candidates)
	result["batch_errors"] = batchErrors
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
	return e.RecallWithEventAndOpts(ctx, query, topK, detail, RecallOpts{})
}

// RecallWithEventAndOpts is like RecallWithEvent but preserves RecallOpts such
// as handle mode while still emitting retrieval telemetry.
func (e *SearchEngine) RecallWithEventAndOpts(ctx context.Context, query string, topK int, detail string, opts RecallOpts) ([]types.SearchResult, string, error) {
	results, err := e.RecallWithOpts(ctx, query, topK, detail, opts)
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

// applyPreferenceMMR executes the H-NEW-2 centroid-MMR diversity pass on the
// given result slice. It fetches best-chunk embeddings for candidate memories,
// computes the dominant-topic centroid, re-scores all candidates via mmrReScore,
// and returns the re-sorted slice with MMR scores written back into r.Score.
//
// topicPool is the relevance-ranked general (non-preference) pool captured before
// the preference-first split. The centroid is computed from topicPool rather than
// from the preference-front-loaded results slice, so the centroid represents the
// dominant topic the query actually hits — not the preference memories that were
// artificially promoted. If topicPool is empty, results is used as a fallback.
// (Bug 2 fix: centroid must reflect dominant topic, not preference front-load.)
//
// r.Score is overwritten with the MMR score for each result. This ensures that
// any downstream sortResults call (e.g. from the reranker block) preserves the
// MMR ordering rather than reverting to the original composite score order.
// (Bug 1 fix: MMR ordering must survive when a reranker is active.)
//
// On any DB error the original (unmodified) results are returned so the recall
// path never fails due to the MMR pass. Best-effort: partial embedding coverage
// is handled gracefully (candidates without embeddings receive a zero penalty).
func (e *SearchEngine) applyPreferenceMMR(ctx context.Context, results []types.SearchResult, topicPool []types.SearchResult) []types.SearchResult {
	if len(results) == 0 {
		return results
	}

	// Collect all memory IDs we need embeddings for: the full result set plus the
	// topic pool (for centroid computation). De-duplicate to minimise DB round-trips.
	allIDs := make([]string, 0, len(results)+len(topicPool))
	seen := make(map[string]struct{}, len(results)+len(topicPool))
	for _, r := range results {
		if r.Memory != nil {
			if _, dup := seen[r.Memory.ID]; !dup {
				allIDs = append(allIDs, r.Memory.ID)
				seen[r.Memory.ID] = struct{}{}
			}
		}
	}
	for _, r := range topicPool {
		if r.Memory != nil {
			if _, dup := seen[r.Memory.ID]; !dup {
				allIDs = append(allIDs, r.Memory.ID)
				seen[r.Memory.ID] = struct{}{}
			}
		}
	}
	if len(allIDs) == 0 {
		return results
	}

	// Batch-fetch best-chunk embeddings for all candidate memories.
	chunks, err := e.backend.GetChunksForMemories(ctx, allIDs)
	if err != nil {
		slog.Warn("preference-mmr: GetChunksForMemories failed, skipping MMR pass",
			"project", e.project, "err", err)
		return results
	}

	// Build a map from memory_id → best-chunk embedding (highest-index chunk
	// as a simple proxy for the most representative chunk). GetChunksForMemories
	// returns chunks ordered by chunk_index so the last entry per memory_id is
	// the last chunk; use the first (index 0) instead — it's most general.
	embByMemID := make(map[string][]float32, len(allIDs))
	for _, ch := range chunks {
		if len(ch.Embedding) > 0 {
			if _, exists := embByMemID[ch.MemoryID]; !exists {
				embByMemID[ch.MemoryID] = ch.Embedding
			}
		}
	}

	// Build mmrCandidate slice preserving original score as relevance.
	candidates := make([]mmrCandidate, 0, len(results))
	for _, r := range results {
		if r.Memory == nil {
			continue
		}
		candidates = append(candidates, mmrCandidate{
			memoryID:  r.Memory.ID,
			relevance: r.Score,
			embedding: embByMemID[r.Memory.ID], // nil when no embedding available
		})
	}

	// Bug 2 fix: compute centroid from topicPool (general/non-preference pool,
	// relevance-ranked before the preference-first split). This represents the
	// dominant topic cluster the query is about — not the preference memories
	// that were front-loaded. If topicPool is empty (no general results, or the
	// preference split did not fire), fall back to the full candidate pool.
	centroidSource := topicPool
	if len(centroidSource) == 0 {
		// Fallback: use results directly (preference split did not produce a general pool).
		centroidSource = results
	}
	poolSize := mmrCentroidPoolSize
	if poolSize > len(centroidSource) {
		poolSize = len(centroidSource)
	}
	centroidVecs := make([][]float32, 0, poolSize)
	for i := 0; i < poolSize; i++ {
		r := centroidSource[i]
		if r.Memory == nil {
			continue
		}
		if emb := embByMemID[r.Memory.ID]; emb != nil {
			centroidVecs = append(centroidVecs, emb)
		}
	}
	centroid := computeCentroid(centroidVecs)
	if centroid == nil {
		// No embeddings available in topic pool — skip MMR (no centroid to penalise against).
		slog.Debug("preference-mmr: no embeddings in topic pool, skipping MMR pass",
			"project", e.project)
		return results
	}

	// Re-score all candidates with MMR formula.
	reScored := mmrReScore(candidates, centroid, mmrLambdaDefault)

	// Build a score map from the MMR re-scoring so we can update r.Score.
	// Bug 1 fix: write the MMR score back into r.Score so that any subsequent
	// sortResults call (e.g. in the reranker block) preserves the MMR ordering
	// rather than reverting to the original composite score.
	mmrScoreByID := make(map[string]float64, len(reScored))
	for i, c := range reScored {
		// Use rank-derived score: highest-ranked gets the largest value.
		// We use the mmrCandidate relevance field (which holds the per-candidate MMR
		// score) but that is not directly accessible after mmrReScore returns the
		// candidates in order without exposing the computed score. Instead, encode
		// rank position as a descending float so sortResults keeps the MMR order:
		// rank 0 → N, rank 1 → N-1, …, rank N-1 → 1. Integer offsets are sufficient
		// since sortResults only requires a consistent total order.
		mmrScoreByID[c.memoryID] = float64(len(reScored) - i)
	}

	// Re-map back to SearchResults in the MMR-determined order, updating Score.
	resultByID := make(map[string]types.SearchResult, len(results))
	for _, r := range results {
		if r.Memory != nil {
			resultByID[r.Memory.ID] = r
		}
	}
	out := make([]types.SearchResult, 0, len(reScored))
	for _, c := range reScored {
		if r, ok := resultByID[c.memoryID]; ok {
			r.Score = mmrScoreByID[c.memoryID] // write MMR rank score (Bug 1 fix)
			out = append(out, r)
		}
	}
	// Preserve any results that had no memory ID (shouldn't happen but be safe).
	if len(out) < len(results) {
		seenOut := make(map[string]struct{}, len(out))
		for _, r := range out {
			if r.Memory != nil {
				seenOut[r.Memory.ID] = struct{}{}
			}
		}
		for _, r := range results {
			id := ""
			if r.Memory != nil {
				id = r.Memory.ID
			}
			if _, found := seenOut[id]; !found {
				out = append(out, r)
				if id != "" {
					seenOut[id] = struct{}{}
				}
			}
		}
	}
	return out
}

// SummarizeNow: handled directly by the MCP tool via summarize package (see tools.go).
