package search

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/petersimmons1972/engram/internal/chunk"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/types"
)

// SearchEngine is the core retrieval engine: it stores memories (chunked + embedded)
// and recalls them via composite vector+FTS scoring.
type SearchEngine struct {
	backend  db.Backend
	embedder embed.Client
	project  string
	cancel   context.CancelFunc
}

// New constructs a SearchEngine. Background workers are wired in Task 8; for now
// only the cancel context is established.
func New(ctx context.Context, backend db.Backend, embedder embed.Client, project string) *SearchEngine {
	_, cancel := context.WithCancel(ctx)
	return &SearchEngine{backend: backend, embedder: embedder, project: project, cancel: cancel}
}

// Close shuts down the engine and signals any background goroutines.
func (e *SearchEngine) Close() { e.cancel() }

// Backend exposes the underlying db.Backend for callers that need direct access
// (e.g. EnginePool, MCP tool handlers).
func (e *SearchEngine) Backend() db.Backend { return e.backend }

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

		exists, err := e.backend.ChunkHashExists(ctx, hash, e.project)
		if err != nil {
			return fmt.Errorf("check chunk hash: %w", err)
		}
		if exists {
			continue
		}

		embedding, err := e.embedder.Embed(ctx, c.Text)
		if err != nil {
			return fmt.Errorf("embed chunk %d: %w", i, err)
		}

		ch := &types.Chunk{
			ID:         types.NewMemoryID(),
			MemoryID:   m.ID,
			ChunkText:  c.Text,
			ChunkIndex: i,
			ChunkHash:  hash,
			ChunkType:  c.ChunkType,
			Project:    e.project,
			Embedding:  embedToBlob(embedding),
		}
		if c.HasHeading {
			heading := c.SectionHeading
			ch.SectionHeading = &heading
		}
		chunks = append(chunks, ch)
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

// Recall retrieves the topK most relevant memories for query, using composite
// vector+FTS scoring. detail controls content truncation: "id_only", "summary",
// or "full" (default).
func (e *SearchEngine) Recall(ctx context.Context, query string, topK int, detail string) ([]types.SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	queryVec, err := e.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	chunks, err := e.backend.GetAllChunksWithEmbeddings(ctx, e.project, 10_000)
	if err != nil {
		return nil, err
	}

	// Fan-out FTS search concurrently while scoring vectors.
	type ftsResult struct {
		results []db.FTSResult
		err     error
	}
	ftsCh := make(chan ftsResult, 1)
	go func() {
		res, err := e.backend.FTSSearch(ctx, e.project, query, topK*3, nil, nil)
		ftsCh <- ftsResult{res, err}
	}()

	// Score all chunks by cosine similarity.
	var scored []chunkScore
	for _, c := range chunks {
		if len(c.Embedding) == 0 {
			continue
		}
		chunkVec := blobToEmbed(c.Embedding)
		cos := cosineSimilarity(queryVec, chunkVec)
		if cos > 0 {
			scored = append(scored, chunkScore{c, cos})
		}
	}

	sortByScore(scored)
	if len(scored) > topK*3 {
		scored = scored[:topK*3]
	}

	// Resolve memory records for top vector hits in a single batch query.
	uniqueIDs := make([]string, 0, len(scored))
	seen := make(map[string]bool, len(scored))
	for _, s := range scored {
		if !seen[s.chunk.MemoryID] {
			seen[s.chunk.MemoryID] = true
			uniqueIDs = append(uniqueIDs, s.chunk.MemoryID)
		}
	}
	batchMems, err := e.backend.GetMemoriesByIDs(ctx, e.project, uniqueIDs)
	if err != nil {
		return nil, err
	}
	memories := make(map[string]*types.Memory, len(batchMems))
	for _, m := range batchMems {
		memories[m.ID] = m
	}

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

	// Per-memory: best cosine score and the chunk that produced it.
	bestCosine := make(map[string]float64)
	bestChunk := make(map[string]*types.Chunk)
	for _, s := range scored {
		if s.cosine > bestCosine[s.chunk.MemoryID] {
			bestCosine[s.chunk.MemoryID] = s.cosine
			bestChunk[s.chunk.MemoryID] = s.chunk
		}
	}

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

		mc := bestChunk[id]
		result := types.SearchResult{
			Memory:     m,
			Score:      score,
			ChunkScore: bestCosine[id],
			ScoreBreakdown: map[string]float64{
				"cosine":  bestCosine[id],
				"bm25":    bm25,
				"recency": RecencyDecay(input.HoursSince),
			},
		}
		if mc != nil {
			result.MatchedChunk = mc.ChunkText
			result.MatchedChunkIndex = mc.ChunkIndex
			result.MatchedChunkSection = mc.SectionHeading
		}
		switch detail {
		case "id_only":
			result.Memory = &types.Memory{ID: m.ID}
		case "summary":
			if m.Summary != nil {
				result.Memory.Content = *m.Summary
			} else {
				// Summary not yet generated; truncate content as preview.
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
	if len(results) > topK {
		results = results[:topK]
	}

	// Update access timestamps for returned results.
	for _, r := range results {
		_ = e.backend.TouchMemory(ctx, r.Memory.ID)
		if r.MatchedChunk != "" && bestChunk[r.Memory.ID] != nil {
			_ = e.backend.UpdateChunkLastMatched(ctx, bestChunk[r.Memory.ID].ID)
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

// embedToBlob encodes a float32 vector as a little-endian byte slice.
func embedToBlob(vec []float32) []byte {
	b := make([]byte, 4*len(vec))
	for i, f := range vec {
		u := math.Float32bits(f)
		b[4*i] = byte(u)
		b[4*i+1] = byte(u >> 8)
		b[4*i+2] = byte(u >> 16)
		b[4*i+3] = byte(u >> 24)
	}
	return b
}

// blobToEmbed decodes a little-endian byte slice back to float32 vector.
func blobToEmbed(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	vec := make([]float32, len(b)/4)
	for i := range vec {
		u := uint32(b[4*i]) | uint32(b[4*i+1])<<8 | uint32(b[4*i+2])<<16 | uint32(b[4*i+3])<<24
		vec[i] = math.Float32frombits(u)
	}
	return vec
}

// cosineSimilarity returns the cosine similarity between two equal-length vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func hoursSince(t time.Time) float64 {
	return time.Since(t).Hours()
}

// sortByScore sorts descending by cosine (insertion sort — small N, cache-friendly).
func sortByScore(s []chunkScore) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].cosine > s[j-1].cosine; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// sortResults sorts descending by composite score.
func sortResults(r []types.SearchResult) {
	for i := 1; i < len(r); i++ {
		for j := i; j > 0 && r[j].Score > r[j-1].Score; j-- {
			r[j], r[j-1] = r[j-1], r[j]
		}
	}
}

type chunkScore struct {
	chunk  *types.Chunk
	cosine float64
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
func (e *SearchEngine) Correct(ctx context.Context, id string, content *string, tags []string, importance *int) (*types.Memory, error) {
	return e.backend.UpdateMemory(ctx, id, content, tags, importance)
}

// Forget deletes a memory by ID. Returns true if deleted, false if not found.
func (e *SearchEngine) Forget(ctx context.Context, id string) (bool, error) {
	return e.backend.DeleteMemoryAtomic(ctx, e.project, id, false)
}

// Status returns aggregate statistics for the project.
func (e *SearchEngine) Status(ctx context.Context) (*types.MemoryStats, error) {
	return e.backend.GetStats(ctx, e.project)
}

// Feedback boosts edge weights and touches access timestamps for the given memory IDs,
// signaling that these memories were useful to the caller.
func (e *SearchEngine) Feedback(ctx context.Context, ids []string) error {
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

// jaccardMergeThreshold would be 0.85 when Jaccard near-duplicate merge is implemented (future task).

// Consolidate prunes stale and cold memories and decays graph edges.
// Returns a summary map of counts for each operation performed.
func (e *SearchEngine) Consolidate(ctx context.Context) (map[string]any, error) {
	pruned, err := e.backend.PruneStaleMemories(ctx, e.project, 90*24, 3)
	if err != nil {
		return nil, err
	}
	coldPruned, err := e.backend.PruneColdDocuments(ctx, e.project, 60*24, 3)
	if err != nil {
		return nil, err
	}
	decayed, edgePruned, err := e.backend.DecayAllEdges(ctx, e.project, 0.02, 0.1)
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
	if err := e.backend.SetMeta(ctx, e.project, "embedding_migration_in_progress", "true"); err != nil {
		return nil, err
	}
	if err := e.backend.SetMeta(ctx, e.project, "embedder_name", newModel); err != nil {
		return nil, err
	}
	nulled, err := e.backend.NullAllEmbeddings(ctx, e.project)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"chunks_nulled": nulled,
		"new_model":     newModel,
		"status":        "migration started — reembed worker will complete in background",
	}, nil
}

// SummarizeNow: handled directly by the MCP tool via summarize package (see tools.go).
