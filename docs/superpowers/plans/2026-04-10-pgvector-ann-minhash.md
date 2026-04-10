# pgvector ANN Recall + MinHash/LSH Consolidation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace in-process 10k-chunk brute-force recall with pgvector HNSW ANN, and replace O(n^2) consolidation pairwise comparison with MinHash/LSH candidate generation + hybrid scoring.

**Architecture:** Multi-step schema migration converts `chunks.embedding` from `BYTEA` to `vector(768)` with HNSW index. Recall delegates cosine distance to PostgreSQL. New `internal/minhash/` package provides MinHash signatures + LSH banding for sublinear near-duplicate candidate generation. Consolidation uses hybrid Jaccard + embedding cosine scoring.

**Tech Stack:** Go 1.24, PostgreSQL + pgvector, `github.com/pgvector/pgvector-go`, `github.com/jackc/pgx/v5`

**Spec:** `docs/superpowers/specs/2026-04-10-pgvector-ann-minhash-design.md`

---

### Task 1: Add pgvector-go dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add the pgvector-go module**

```bash
cd /home/psimmons/projects/engram-go/.worktrees/get-well-phase0
go get github.com/pgvector/pgvector-go
```

- [ ] **Step 2: Verify it resolved**

```bash
grep pgvector go.mod
```

Expected: `github.com/pgvector/pgvector-go v0.x.x`

- [ ] **Step 3: Verify build still passes**

```bash
go build ./...
```

Expected: exit 0, no errors

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add pgvector-go for native vector column support"
```

---

### Task 2: Write the pgvector SQL migration

**Files:**
- Create: `internal/db/migrations/003_pgvector.sql`

- [ ] **Step 1: Create the migration file**

Write `internal/db/migrations/003_pgvector.sql`:

```sql
-- pgvector migration: BYTEA → vector(768) with HNSW index.
-- Multi-step for zero-downtime rollback safety.

-- 1. Ensure pgvector extension.
CREATE EXTENSION IF NOT EXISTS vector;

-- 2. Add new vector column alongside existing BYTEA.
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS embedding_vec vector(768);

-- 3. Mark backfill pending — Go code handles BYTEA→vector conversion
-- because the BYTEA format is little-endian float32 (needs byte-swap).
INSERT INTO project_meta (project, key, value)
VALUES ('_engram', 'pgvector_backfill_pending', 'true')
ON CONFLICT (project, key) DO NOTHING;
```

**Note:** Steps 4-6 (drop old column, rename, create HNSW index) happen in a separate migration file `004_pgvector_finalize.sql` that runs AFTER the Go backfill completes. This ensures no data loss if the backfill is interrupted.

- [ ] **Step 2: Create the finalize migration**

Write `internal/db/migrations/004_pgvector_finalize.sql`:

```sql
-- Finalize pgvector migration. Only runs after Go backfill is complete
-- (pgvector_backfill_pending = 'false' in project_meta).
-- The runMigrations code checks this before applying this file.

-- 4. Drop old BYTEA column.
ALTER TABLE chunks DROP COLUMN IF EXISTS embedding;

-- 5. Rename vector column to canonical name.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name='chunks' AND column_name='embedding_vec') THEN
        ALTER TABLE chunks RENAME COLUMN embedding_vec TO embedding;
    END IF;
END $$;

-- 6. Create HNSW index for cosine distance.
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw
    ON chunks USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
```

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

Expected: exit 0 (embedded FS picks up new .sql files automatically)

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/003_pgvector.sql internal/db/migrations/004_pgvector_finalize.sql
git commit -m "schema: add 003/004 pgvector migrations — BYTEA to vector(768) with HNSW"
```

---

### Task 3: Add backfill logic and migration gating in postgres.go

**Files:**
- Modify: `internal/db/postgres.go:81-115` (runMigrations)

The `runMigrations` function needs two additions:
1. After applying `003_pgvector.sql`, run the Go-side BYTEA→vector backfill.
2. Gate `004_pgvector_finalize.sql` on the backfill being complete.

- [ ] **Step 1: Add the backfill function**

Add to `internal/db/postgres.go`, after `runMigrations`:

```go
// backfillVectors converts existing BYTEA embeddings to the new embedding_vec
// vector(768) column. Called once after 003_pgvector.sql creates the column.
// Idempotent: skips rows where embedding_vec is already populated.
func (b *PostgresBackend) backfillVectors(ctx context.Context) error {
	rows, err := b.pool.Query(ctx, `
		SELECT id, embedding FROM chunks
		WHERE embedding IS NOT NULL AND embedding_vec IS NULL`)
	if err != nil {
		return fmt.Errorf("backfill query: %w", err)
	}
	defer rows.Close()

	var converted int
	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return fmt.Errorf("backfill scan: %w", err)
		}
		if len(blob)%4 != 0 || len(blob) == 0 {
			slog.Warn("backfill: skipping chunk with invalid BYTEA length", "id", id, "len", len(blob))
			continue
		}
		// Decode little-endian float32 blob (same format as blobToEmbed).
		vec := make([]float32, len(blob)/4)
		for i := range vec {
			u := uint32(blob[4*i]) | uint32(blob[4*i+1])<<8 | uint32(blob[4*i+2])<<16 | uint32(blob[4*i+3])<<24
			vec[i] = math.Float32frombits(u)
		}
		// Write to the new vector column using pgvector encoding.
		if _, err := b.pool.Exec(ctx,
			"UPDATE chunks SET embedding_vec = $1 WHERE id = $2",
			pgvector.NewVector(vec), id,
		); err != nil {
			return fmt.Errorf("backfill update chunk %s: %w", id, err)
		}
		converted++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backfill iteration: %w", err)
	}

	slog.Info("pgvector backfill complete", "chunks_converted", converted)

	// Mark backfill done so 004 can run.
	_, err = b.pool.Exec(ctx,
		`UPDATE project_meta SET value='false' WHERE project='_engram' AND key='pgvector_backfill_pending'`)
	return err
}
```

Add these imports to postgres.go:

```go
import (
	pgvector "github.com/pgvector/pgvector-go"
)
```

- [ ] **Step 2: Update runMigrations to call backfill and gate 004**

In `runMigrations`, after the migration apply loop, add the backfill trigger and 004 gate. Replace the existing migration loop body to add gating logic:

```go
func (b *PostgresBackend) runMigrations(ctx context.Context) error {
	// ... existing schema_migrations table creation ...

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		name := e.Name()

		// Skip already-applied migrations.
		var applied bool
		err := b.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`, name,
		).Scan(&applied)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		// Gate 004: only apply if backfill is complete.
		if name == "004_pgvector_finalize.sql" {
			var pending string
			err := b.pool.QueryRow(ctx,
				`SELECT COALESCE(
					(SELECT value FROM project_meta WHERE project='_engram' AND key='pgvector_backfill_pending'),
					'false'
				)`).Scan(&pending)
			if err != nil {
				return fmt.Errorf("check backfill status: %w", err)
			}
			if pending == "true" {
				slog.Info("skipping 004_pgvector_finalize.sql — backfill still pending")
				continue
			}
		}

		sql, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := b.pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := b.pool.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, name,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		slog.Info("applied migration", "file", name)

		// After 003: run the Go-side backfill.
		if name == "003_pgvector.sql" {
			if err := b.backfillVectors(ctx); err != nil {
				return fmt.Errorf("pgvector backfill failed: %w", err)
			}
		}
	}
	return nil
}
```

**Important:** The code block above shows the FULL replacement for the `runMigrations` loop body. The `schema_migrations` table creation at the top stays as-is from Phase 0.

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add internal/db/postgres.go
git commit -m "feat: add pgvector backfill + migration gating for 003/004"
```

---

### Task 4: Change Chunk.Embedding type from []byte to []float32

**Files:**
- Modify: `internal/types/types.go:125-147`
- Modify: `internal/db/postgres.go` (rowToChunk, storeChunksExec, UpdateChunkEmbedding)
- Modify: `internal/db/backend.go:75`
- Modify: `internal/reembed/worker.go:105-132`
- Modify: `internal/search/engine.go:172`

- [ ] **Step 1: Update the Chunk struct**

In `internal/types/types.go`, change:

```go
// Old:
// Embedding is the raw little-endian float32 blob from vector.ToBlob.
Embedding []byte `json:"embedding,omitempty"`

// New:
// Embedding is the float32 vector from the embedding model.
// Stored as pgvector vector(768) in the database.
Embedding []float32 `json:"embedding,omitempty"`
```

- [ ] **Step 2: Update the Backend interface**

In `internal/db/backend.go`, change `UpdateChunkEmbedding` signature:

```go
// Old:
UpdateChunkEmbedding(ctx context.Context, chunkID string, embedding []byte) (int, error)

// New:
UpdateChunkEmbedding(ctx context.Context, chunkID string, embedding []float32) (int, error)
```

- [ ] **Step 3: Update rowToChunk in postgres.go**

Replace the raw struct scan for the embedding field. The `rowToChunk` function (around line 1183) currently scans `Embedding` as `[]byte`. After 003+004 migrations, the column is `vector(768)`. Use `pgvector.NewVector` for scanning:

```go
// In the raw struct inside rowToChunk, change:
// Old:
Embedding      []byte

// New:
Embedding      pgvector.Vector
```

And in the return statement, convert:

```go
// Old:
Embedding:      r.Embedding,

// New:
Embedding:      r.Embedding.Slice(),
```

Note: `pgvector.Vector.Slice()` returns `[]float32`.

- [ ] **Step 4: Update storeChunksExec**

In `internal/db/postgres.go` (around line 444), the INSERT uses `c.Embedding` as parameter `$7`. After the migration, this column is `vector(768)`. Wrap the value:

```go
// Old (in the Exec call):
c.Embedding,

// New:
pgvector.NewVector(c.Embedding),
```

Handle nil case: if `c.Embedding` is nil (pending embedding), pass `nil` directly.

```go
var embParam any
if len(c.Embedding) > 0 {
    embParam = pgvector.NewVector(c.Embedding)
}
_, err := ex.Exec(ctx, chunkSQL,
    c.ID, c.MemoryID, c.Project,
    c.ChunkText, c.ChunkIndex, c.ChunkHash,
    embParam, sectionHeading, c.ChunkType,
)
```

- [ ] **Step 5: Update UpdateChunkEmbedding**

In `internal/db/postgres.go` (around line 576):

```go
// Old:
func (b *PostgresBackend) UpdateChunkEmbedding(ctx context.Context, chunkID string, embedding []byte) (int, error) {
	tag, err := b.pool.Exec(ctx,
		"UPDATE chunks SET embedding=$1 WHERE id=$2", embedding, chunkID,
	)
	return int(tag.RowsAffected()), err
}

// New:
func (b *PostgresBackend) UpdateChunkEmbedding(ctx context.Context, chunkID string, embedding []float32) (int, error) {
	tag, err := b.pool.Exec(ctx,
		"UPDATE chunks SET embedding=$1 WHERE id=$2", pgvector.NewVector(embedding), chunkID,
	)
	return int(tag.RowsAffected()), err
}
```

- [ ] **Step 6: Update reembed worker**

In `internal/reembed/worker.go`, the `runBatch` method (line 105-119) calls `toBlob(vec)` to convert `[]float32` → `[]byte`, then passes to `UpdateChunkEmbedding`. After the type change, pass `vec` directly:

```go
// Old (lines 114-115):
blob := toBlob(vec)
if n, err := w.backend.UpdateChunkEmbedding(ctx, c.ID, blob); err != nil || n == 0 {

// New:
if n, err := w.backend.UpdateChunkEmbedding(ctx, c.ID, vec); err != nil || n == 0 {
```

Remove the `toBlob` function (lines 122-132) and the `"math"` import.

- [ ] **Step 7: Update Store in engine.go**

In `internal/search/engine.go` (line 172), change:

```go
// Old:
Embedding:  embedToBlob(embedding),

// New:
Embedding:  embedding,
```

The `embedding` variable from `e.embedder.Embed()` is already `[]float32`.

- [ ] **Step 8: Verify build**

```bash
go build ./...
```

Expected: Compile errors pointing to remaining uses of `embedToBlob`/`blobToEmbed`/`cosineSimilarity` in engine.go. These are removed in Task 6. For now, temporarily comment them out or leave them — they'll be replaced in the recall rewrite.

Actually: the build WILL fail here because `blobToEmbed` is still called in `RecallWithOpts` (line 238). We handle this by implementing Tasks 4 and 5 together as one commit. The `embedToBlob` and `blobToEmbed` functions themselves can be deleted at the same time.

- [ ] **Step 9: Commit (combined with Task 5 below)**

Deferred to end of Task 5.

---

### Task 5: Implement VectorSearch and rewrite RecallWithOpts

**Files:**
- Modify: `internal/db/backend.go` (add VectorSearch, VectorHit to interface)
- Modify: `internal/db/postgres.go` (implement VectorSearch)
- Modify: `internal/search/engine.go` (rewrite RecallWithOpts, remove blob/cosine helpers)

- [ ] **Step 1: Add VectorHit type and VectorSearch to backend.go**

In `internal/db/backend.go`, add after the `FTSResult` type:

```go
// VectorHit is a single result from a pgvector ANN search.
type VectorHit struct {
	ChunkID        string
	MemoryID       string
	Distance       float64 // cosine distance (0 = identical, 2 = opposite)
	ChunkText      string
	ChunkIndex     int
	SectionHeading *string
}
```

Add to the Backend interface, in the Chunk section:

```go
// VectorSearch returns the nearest chunks to queryVec by cosine distance,
// using the HNSW index. Returns at most limit results.
VectorSearch(ctx context.Context, project string, queryVec []float32, limit int) ([]VectorHit, error)
```

- [ ] **Step 2: Implement VectorSearch in postgres.go**

Add to `internal/db/postgres.go`:

```go
func (b *PostgresBackend) VectorSearch(ctx context.Context, project string, queryVec []float32, limit int) ([]db.VectorHit, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT c.id, c.memory_id,
		       c.embedding <=> $1::vector AS distance,
		       c.chunk_text, c.chunk_index, c.section_heading
		FROM chunks c
		WHERE c.project = $2 AND c.embedding IS NOT NULL
		ORDER BY c.embedding <=> $1::vector
		LIMIT $3`,
		pgvector.NewVector(queryVec), project, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []db.VectorHit
	for rows.Next() {
		var h db.VectorHit
		if err := rows.Scan(&h.ChunkID, &h.MemoryID, &h.Distance,
			&h.ChunkText, &h.ChunkIndex, &h.SectionHeading); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}
```

Note: import `db "github.com/petersimmons1972/engram/internal/db"` is not needed here since this IS the db package. The return type is just `[]VectorHit`.

- [ ] **Step 3: Rewrite RecallWithOpts in engine.go**

Replace the chunk-fetch + cosine-scoring section (lines 214-247) with VectorSearch. The new flow:

```go
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

	// Optional re-ranking (unchanged from original).
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

	// Update access timestamps.
	for _, r := range results {
		_ = e.backend.TouchMemory(ctx, r.Memory.ID)
		if chunkID, ok := bestChunkID[r.Memory.ID]; ok {
			_ = e.backend.UpdateChunkLastMatched(ctx, chunkID)
		}
	}

	return results, nil
}
```

- [ ] **Step 4: Remove dead code from engine.go**

Delete these functions from `internal/search/engine.go`:
- `embedToBlob` (lines 436-447)
- `blobToEmbed` (lines 449-460)
- `cosineSimilarity` (lines 462-476)

Also remove the `chunkScore` struct (line 500-503) and the `sortByScore` function since they are no longer used by the recall path.

Check if `sortByScore` or `chunkScore` is used anywhere else:

```bash
grep -rn 'chunkScore\|sortByScore' internal/search/
```

If only used in the old recall path, remove them.

- [ ] **Step 5: Verify build and tests**

```bash
go build ./...
go test ./...
```

Expected: build passes. Tests pass (existing tests don't hit a real DB so the old backend mock still works).

- [ ] **Step 6: Commit Tasks 4+5 together**

```bash
git add internal/types/types.go internal/db/backend.go internal/db/postgres.go \
        internal/search/engine.go internal/reembed/worker.go
git commit -m "feat: pgvector ANN recall — VectorSearch + embedding type migration

- Chunk.Embedding: []byte → []float32 across all packages
- VectorSearch: DB-side cosine via HNSW index replaces 10k chunk in-process scan
- RecallWithOpts: rewritten to use VectorSearch + distance→similarity conversion
- Removed: embedToBlob, blobToEmbed, cosineSimilarity, chunkScore, sortByScore
- Reembed worker: passes []float32 directly, removed toBlob helper"
```

---

### Task 6: Implement MinHash signatures

**Files:**
- Create: `internal/minhash/minhash.go`
- Create: `internal/minhash/minhash_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/minhash/minhash_test.go`:

```go
package minhash_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/minhash"
	"github.com/stretchr/testify/require"
)

func TestSignature_IdenticalStrings(t *testing.T) {
	h := minhash.NewHasher(42)
	sig1 := h.Signature("the quick brown fox jumps over the lazy dog")
	sig2 := h.Signature("the quick brown fox jumps over the lazy dog")
	require.Equal(t, sig1, sig2)
	require.InDelta(t, 1.0, minhash.EstimatedJaccard(sig1, sig2), 0.001)
}

func TestSignature_CompletelyDifferent(t *testing.T) {
	h := minhash.NewHasher(42)
	sig1 := h.Signature("aaaaaaaaaa bbbbbbbbbb cccccccccc")
	sig2 := h.Signature("xxxxxxxxxx yyyyyyyyyy zzzzzzzzzz")
	est := minhash.EstimatedJaccard(sig1, sig2)
	require.Less(t, est, 0.15, "completely different strings should have near-zero Jaccard")
}

func TestSignature_Deterministic(t *testing.T) {
	h1 := minhash.NewHasher(42)
	h2 := minhash.NewHasher(42)
	sig1 := h1.Signature("test content here")
	sig2 := h2.Signature("test content here")
	require.Equal(t, sig1, sig2, "same seed + same input must produce same signature")
}

func TestSignature_DifferentSeeds(t *testing.T) {
	h1 := minhash.NewHasher(42)
	h2 := minhash.NewHasher(99)
	sig1 := h1.Signature("test content here")
	sig2 := h2.Signature("test content here")
	require.NotEqual(t, sig1, sig2, "different seeds should produce different signatures")
}

func TestSignature_NearDuplicate(t *testing.T) {
	h := minhash.NewHasher(42)
	base := "kubernetes deployment patterns for production workloads with high availability"
	sig1 := h.Signature(base)
	sig2 := h.Signature(base + " and disaster recovery")
	est := minhash.EstimatedJaccard(sig1, sig2)
	require.Greater(t, est, 0.5, "near-duplicate should have moderate-to-high Jaccard")
}

func TestSignature_EmptyString(t *testing.T) {
	h := minhash.NewHasher(42)
	sig := h.Signature("")
	// Empty string has no bigrams; all signature slots stay at max.
	est := minhash.EstimatedJaccard(sig, sig)
	require.InDelta(t, 1.0, est, 0.001, "empty signature compared to itself is 1.0")
}

func TestSignature_UTF8(t *testing.T) {
	h := minhash.NewHasher(42)
	sig1 := h.Signature("日本語テスト")
	sig2 := h.Signature("日本語テスト")
	require.Equal(t, sig1, sig2, "UTF-8 strings must produce identical signatures")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/minhash/... -v
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement the MinHash Hasher**

Create `internal/minhash/minhash.go`:

```go
// Package minhash provides MinHash signature computation for near-duplicate
// detection via Jaccard similarity estimation.
package minhash

import (
	"hash/fnv"
	"math"
	"math/rand"
)

// NumHashes is the number of hash functions in a MinHash signature.
const NumHashes = 128

// Signature is a MinHash signature — the minimum hash value per hash function.
type Signature [NumHashes]uint64

// prime is a large Mersenne prime used as the hash modulus (2^61 - 1).
const prime = (1 << 61) - 1

// Hasher computes MinHash signatures from string content using character bigrams.
type Hasher struct {
	a [NumHashes]uint64
	b [NumHashes]uint64
}

// NewHasher creates a Hasher with deterministic coefficients from seed.
func NewHasher(seed int64) *Hasher {
	rng := rand.New(rand.NewSource(seed))
	var h Hasher
	for i := range NumHashes {
		h.a[i] = rng.Uint64()%prime + 1 // a must be non-zero
		h.b[i] = rng.Uint64() % prime
	}
	return &h
}

// Signature computes the MinHash signature for content using rune-based
// character bigrams. An empty string returns a signature with all slots
// set to math.MaxUint64.
func (h *Hasher) Signature(content string) Signature {
	var sig Signature
	for i := range sig {
		sig[i] = math.MaxUint64
	}

	runes := []rune(content)
	if len(runes) < 2 {
		return sig
	}

	for i := 0; i+1 < len(runes); i++ {
		// Hash the bigram to a uint64.
		bg := bigramHash(runes[i], runes[i+1])

		// For each hash function, compute h_i(bg) = (a_i * bg + b_i) mod p
		// and keep the minimum.
		for j := range NumHashes {
			val := (h.a[j]*bg + h.b[j]) % prime
			if val < sig[j] {
				sig[j] = val
			}
		}
	}
	return sig
}

// bigramHash hashes a rune pair to a uint64 using FNV-1a.
func bigramHash(a, b rune) uint64 {
	h := fnv.New64a()
	buf := [8]byte{
		byte(a), byte(a >> 8), byte(a >> 16), byte(a >> 24),
		byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24),
	}
	h.Write(buf[:])
	return h.Sum64()
}

// EstimatedJaccard returns the estimated Jaccard similarity from two signatures.
// The estimate is the fraction of hash slots where both signatures agree.
func EstimatedJaccard(a, b Signature) float64 {
	matches := 0
	for i := range NumHashes {
		if a[i] == b[i] {
			matches++
		}
	}
	return float64(matches) / float64(NumHashes)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/minhash/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/minhash/minhash.go internal/minhash/minhash_test.go
git commit -m "feat: add MinHash signature computation (internal/minhash)"
```

---

### Task 7: Implement LSH banding

**Files:**
- Create: `internal/minhash/lsh.go`
- Modify: `internal/minhash/minhash_test.go` (add LSH tests)

- [ ] **Step 1: Write the failing tests**

Append to `internal/minhash/minhash_test.go`:

```go
func TestLSH_IdenticalStrings_AreCandidates(t *testing.T) {
	h := minhash.NewHasher(42)
	idx := minhash.NewIndex(16, 8)

	sig1 := h.Signature("the quick brown fox jumps over the lazy dog")
	sig2 := h.Signature("the quick brown fox jumps over the lazy dog")

	idx.Add("mem-1", sig1)
	idx.Add("mem-2", sig2)

	candidates := idx.Candidates()
	require.Len(t, candidates, 1)
	require.ElementsMatch(t, candidates[0][:], []string{"mem-1", "mem-2"})
}

func TestLSH_DifferentStrings_NotCandidates(t *testing.T) {
	h := minhash.NewHasher(42)
	idx := minhash.NewIndex(16, 8)

	sig1 := h.Signature("aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd")
	sig2 := h.Signature("xxxxxxxxxx yyyyyyyyyy zzzzzzzzzz wwwwwwwwww")

	idx.Add("mem-1", sig1)
	idx.Add("mem-2", sig2)

	candidates := idx.Candidates()
	require.Empty(t, candidates, "completely different strings should not be candidates")
}

func TestLSH_ThreeMemories_OnlyNearPairMatches(t *testing.T) {
	h := minhash.NewHasher(42)
	idx := minhash.NewIndex(16, 8)

	base := "kubernetes deployment patterns for production workloads with high availability"
	sig1 := h.Signature(base)
	sig2 := h.Signature(base + " and resilience")
	sig3 := h.Signature("completely unrelated text about cooking recipes and kitchen tips")

	idx.Add("mem-1", sig1)
	idx.Add("mem-2", sig2)
	idx.Add("mem-3", sig3)

	candidates := idx.Candidates()
	// mem-1 and mem-2 should be candidates; mem-3 should not pair with either.
	found := false
	for _, pair := range candidates {
		if (pair[0] == "mem-3") || (pair[1] == "mem-3") {
			t.Error("mem-3 should not be a candidate with anything")
		}
		if (pair[0] == "mem-1" && pair[1] == "mem-2") || (pair[0] == "mem-2" && pair[1] == "mem-1") {
			found = true
		}
	}
	require.True(t, found, "mem-1 and mem-2 should be candidates")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/minhash/... -v -run TestLSH
```

Expected: FAIL — `NewIndex` and `Candidates` not defined.

- [ ] **Step 3: Implement LSH banding**

Create `internal/minhash/lsh.go`:

```go
package minhash

import (
	"encoding/binary"
	"hash/fnv"
)

// Index stores MinHash signatures and finds candidate pairs via LSH banding.
// Two memories are candidates if their signatures hash to the same bucket
// in any band.
type Index struct {
	bands   int
	rows    int
	buckets []map[uint64][]string // band → bucket_hash → memory IDs
}

// NewIndex creates an LSH index. bands * rowsPerBand must equal NumHashes.
func NewIndex(bands, rowsPerBand int) *Index {
	if bands*rowsPerBand != NumHashes {
		panic("minhash: bands * rowsPerBand must equal NumHashes")
	}
	b := make([]map[uint64][]string, bands)
	for i := range b {
		b[i] = make(map[uint64][]string)
	}
	return &Index{bands: bands, rows: rowsPerBand, buckets: b}
}

// Add inserts a memory's signature into all band buckets.
func (idx *Index) Add(id string, sig Signature) {
	for band := range idx.bands {
		start := band * idx.rows
		key := bandHash(sig[start : start+idx.rows])
		idx.buckets[band][key] = append(idx.buckets[band][key], id)
	}
}

// Candidates returns all unique pairs of memory IDs that share at least
// one LSH bucket. Each pair appears exactly once as [2]string{idA, idB}
// where idA < idB lexicographically.
func (idx *Index) Candidates() [][2]string {
	seen := make(map[[2]string]bool)
	var pairs [][2]string

	for _, bucket := range idx.buckets {
		for _, ids := range bucket {
			for i := 0; i < len(ids); i++ {
				for j := i + 1; j < len(ids); j++ {
					a, b := ids[i], ids[j]
					if a > b {
						a, b = b, a
					}
					pair := [2]string{a, b}
					if !seen[pair] {
						seen[pair] = true
						pairs = append(pairs, pair)
					}
				}
			}
		}
	}
	return pairs
}

// bandHash hashes a slice of signature values into a single bucket key.
func bandHash(vals []uint64) uint64 {
	h := fnv.New64a()
	buf := make([]byte, 8)
	for _, v := range vals {
		binary.LittleEndian.PutUint64(buf, v)
		h.Write(buf)
	}
	return h.Sum64()
}
```

- [ ] **Step 4: Run all minhash tests**

```bash
go test ./internal/minhash/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/minhash/lsh.go internal/minhash/minhash_test.go
git commit -m "feat: add LSH banding for candidate pair generation (internal/minhash)"
```

---

### Task 8: Implement ChunkEmbeddingDistance and rewrite ConsolidateWithClaude

**Files:**
- Modify: `internal/db/backend.go` (add ChunkEmbeddingDistance)
- Modify: `internal/db/postgres.go` (implement ChunkEmbeddingDistance)
- Modify: `internal/search/engine.go` (rewrite ConsolidateWithClaude)

- [ ] **Step 1: Add ChunkEmbeddingDistance to backend.go**

In `internal/db/backend.go`, add to the Chunk section:

```go
// ChunkEmbeddingDistance returns the minimum cosine distance between any
// chunk of memA and any chunk of memB. Returns 2.0 (max distance) if
// either memory has no embedded chunks.
ChunkEmbeddingDistance(ctx context.Context, memAID, memBID string) (float64, error)
```

- [ ] **Step 2: Implement in postgres.go**

```go
func (b *PostgresBackend) ChunkEmbeddingDistance(ctx context.Context, memAID, memBID string) (float64, error) {
	var dist *float64
	err := b.pool.QueryRow(ctx, `
		SELECT MIN(ca.embedding <=> cb.embedding)
		FROM chunks ca, chunks cb
		WHERE ca.memory_id = $1 AND cb.memory_id = $2
		  AND ca.embedding IS NOT NULL AND cb.embedding IS NOT NULL`,
		memAID, memBID,
	).Scan(&dist)
	if err != nil {
		return 2.0, err
	}
	if dist == nil {
		return 2.0, nil // no embedded chunks
	}
	return *dist, nil
}
```

- [ ] **Step 3: Rewrite ConsolidateWithClaude in engine.go**

Replace the existing `ConsolidateWithClaude` function (lines 643-727) with:

```go
const consolidateJaccardThreshold = 0.85
const consolidateCombinedThreshold = 0.80
const consolidateMaxMemories = 500

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
	sigs := make(map[string]minhash.Signature, len(filtered))
	idx := minhash.NewIndex(16, 8)
	for _, m := range filtered {
		sig := hasher.Signature(m.Content)
		sigs[m.ID] = sig
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
```

Add the minhash import to engine.go:

```go
import (
	"github.com/petersimmons1972/engram/internal/minhash"
)
```

- [ ] **Step 4: Verify build and tests**

```bash
go build ./...
go test ./...
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/db/backend.go internal/db/postgres.go internal/search/engine.go
git commit -m "feat: MinHash/LSH hybrid consolidation with embedding cosine tiebreaker

- ChunkEmbeddingDistance: pgvector cross-join for pairwise chunk distance
- ConsolidateWithClaude: MinHash signatures + LSH banding for O(n) candidate
  generation, hybrid scoring (0.7*Jaccard + 0.3*cosine), threshold 0.80"
```

---

### Task 9: Add dimensional guard at startup

**Files:**
- Modify: `cmd/engram/main.go`

- [ ] **Step 1: Add the dimension check**

In `cmd/engram/main.go`, after the embedder is created (line ~58) and before the factory function, add:

```go
// Verify embedding dimensions match the pgvector column width (768).
const expectedDims = 768
testVec, err := embedder.Embed(ctx, "dimensional guard test")
if err != nil {
	return fmt.Errorf("dimensional guard: embed test failed: %w", err)
}
if len(testVec) != expectedDims {
	return fmt.Errorf("dimensional guard: embedding model produces %d dimensions, but pgvector column is vector(%d) — use a %d-dimension model or run a schema migration", len(testVec), expectedDims, expectedDims)
}
slog.Info("dimensional guard passed", "dims", expectedDims)
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add cmd/engram/main.go
git commit -m "safety: add dimensional guard — refuse to start if embedding dims != 768"
```

---

### Task 10: Final verification and push

- [ ] **Step 1: Full build + test**

```bash
go build ./...
go test ./... -v
```

Expected: all packages build, all tests pass.

- [ ] **Step 2: Review the full diff**

```bash
git log --oneline get-well/phase0 ^main
git diff --stat main..get-well/phase0
```

Verify: all expected files modified, no stray changes.

- [ ] **Step 3: Push and update PR**

```bash
git push origin get-well/phase0
```

The existing PR #51 will update automatically with the new commits.
