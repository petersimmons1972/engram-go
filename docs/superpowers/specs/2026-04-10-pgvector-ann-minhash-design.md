# pgvector ANN Recall + MinHash/LSH Consolidation — Design Spec

**Date:** 2026-04-10
**Status:** Draft
**Scope:** P1 (pgvector ANN index for recall) + P4 (MinHash/LSH hybrid for consolidation)
**Predecessor:** Get Well Phase 0 (security), Phase 1 (data correctness), Phases 2-3 (sort/UTF-8/cleanup)

---

## 1. Problem Statement

Two performance-architecture issues remain from the adversarial code review:

**P1 — Recall path scans all chunks in-process.** `RecallWithOpts` calls `GetAllChunksWithEmbeddings(project, 10_000)`, transfers up to 10k BYTEA blobs over the wire, then computes cosine similarity for each in Go. At current scale (<1k memories) this is tolerable; at 10k+ it becomes a latency and memory bottleneck.

**P4 — Consolidation compares all memory pairs.** `ConsolidateWithClaude` runs O(n^2) pairwise `bigramJaccard` on up to 500 memories (~125k comparisons). At 10k memories the cap would need to increase, making this impractical without a sublinear candidate-generation step.

---

## 2. Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| ANN index type | HNSW | Better recall quality than IVFFlat at all scales; no periodic reindex needed; memory overhead negligible at 768d |
| Column migration | Multi-step (add, backfill, drop, rename) | Zero downtime, rollback-safe; table lock only on final rename |
| Consolidation candidate generation | MinHash/LSH | Canonical O(n) solution for near-duplicate detection; independent of recall path |
| Consolidation scoring | Hybrid: Jaccard + embedding cosine | Embedding distance catches semantic duplicates that text overlap misses; Jaccard catches verbatim overlap that embeddings abstract away |
| Embedding type in Go | `[]float32` (replacing `[]byte`) | Matches what `embed.Client.Embed()` already returns; pgvector-go handles DB encoding |

---

## 3. Schema Migration: 003_pgvector.sql

Multi-step migration from `BYTEA` to `vector(768)`:

```sql
-- Step 1: Ensure pgvector extension exists.
CREATE EXTENSION IF NOT EXISTS vector;

-- Step 2: Add new vector column alongside existing BYTEA.
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS embedding_vec vector(768);

-- Step 3: Backfill — convert BYTEA (little-endian float32) to vector.
-- The Go code stores embeddings as little-endian float32 blobs via embedToBlob().
-- PostgreSQL float4 is big-endian (network byte order). We must byte-swap each
-- 4-byte group before casting. We do this in Go via a one-time migration helper
-- rather than in PL/pgSQL, because Go already has the blobToEmbed() function
-- that correctly decodes the format. The migration SQL sets a marker; the Go
-- migration code handles the actual conversion.
--
-- See: PostgresBackend.backfillVectors() in postgres.go
--
-- Marker for Go to detect and run the backfill:
INSERT INTO project_meta (project, key, value)
VALUES ('_engram', 'pgvector_backfill_pending', 'true')
ON CONFLICT (project, key) DO NOTHING;

-- Step 4: Drop old BYTEA column.
ALTER TABLE chunks DROP COLUMN IF EXISTS embedding;

-- Step 5: Rename new column to canonical name.
ALTER TABLE chunks RENAME COLUMN embedding_vec TO embedding;

-- Step 6: Create HNSW index for cosine distance.
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw
    ON chunks USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
```

**Backfill note:** The PL/pgSQL conversion handles the existing little-endian float32 BYTEA format. At <1k chunks this completes in seconds. At 100k chunks, budget ~30 seconds. The migration is idempotent (`WHERE embedding_vec IS NULL`).

**Rollback path:** Before Step 4, both columns coexist. If the migration fails at any step, the old `embedding BYTEA` column is still intact and the application code (on the old branch) continues to work.

**Dimensional guard:** At startup, the application embeds a test string and verifies `len(vec) == 768`. If the dimension doesn't match the column width, the server refuses to start with a clear error message. This catches model changes that would silently produce wrong-dimension vectors.

---

## 4. Recall Path Rewrite

### 4.1 New Backend Method

```go
// VectorHit represents a single ANN search result from pgvector.
type VectorHit struct {
    ChunkID        string
    MemoryID       string
    Distance       float64  // cosine distance (0 = identical, 2 = opposite)
    ChunkText      string
    ChunkIndex     int
    SectionHeading *string
}

// VectorSearch returns the top-limit chunks nearest to queryVec by cosine distance.
func (b *PostgresBackend) VectorSearch(
    ctx context.Context, project string, queryVec []float32, limit int,
) ([]VectorHit, error)
```

**SQL:**
```sql
SELECT c.id, c.memory_id,
       c.embedding <=> $1::vector AS distance,
       c.chunk_text, c.chunk_index, c.section_heading
FROM chunks c
WHERE c.project = $2 AND c.embedding IS NOT NULL
ORDER BY c.embedding <=> $1::vector
LIMIT $3
```

The `<=>` operator uses the HNSW index automatically when available. pgvector returns cosine *distance* (0-2), not similarity (1-0). Conversion: `cosine_similarity = 1.0 - distance`.

### 4.2 RecallWithOpts Changes

**Removed:**
- `GetAllChunksWithEmbeddings()` call (10k chunk fetch)
- In-process `cosineSimilarity()` loop
- `blobToEmbed()` conversion

**Added:**
- `VectorSearch(ctx, project, queryVec, topK*3)` call
- Distance-to-similarity conversion: `cosine = 1.0 - hit.Distance`

**Unchanged:**
- FTS fan-out goroutine (parallel BM25 search)
- `CompositeScore()` function and weight formula
- Memory batch resolution (`GetMemoriesByIDs`)
- Claude re-ranking (operates on scored results, not vectors)
- Access timestamp updates

### 4.3 Embedding Type Change

| Location | Before | After |
|----------|--------|-------|
| `types.Chunk.Embedding` | `[]byte` | `[]float32` |
| `embed.Client.Embed()` return | `[]float32` | `[]float32` (no change) |
| `UpdateChunkEmbedding(ctx, id, emb)` | `emb []byte` | `emb []float32` |
| `rowToChunk` scanner | `Scan(&r.Embedding)` as `[]byte` | `Scan` via `pgvector.Vector` → `[]float32` |
| `embedToBlob(vec)` | float32 → little-endian bytes | Removed |
| `blobToEmbed(b)` | little-endian bytes → float32 | Removed |

**Dependency added:** `github.com/pgvector/pgvector-go` (provides `pgvector.NewVector()` and pgx scan support).

---

## 5. MinHash/LSH Hybrid Consolidation

### 5.1 New Package: `internal/minhash/`

```
internal/minhash/
    minhash.go      — signature computation
    lsh.go          — banding + candidate generation
    minhash_test.go — unit tests
```

### 5.2 MinHash Signatures (`minhash.go`)

**Parameters:**
- `NumHashes = 128` — number of hash functions in the signature
- Hash family: `h_i(x) = (a_i * x + b_i) mod p` where `p` = large prime (2^61 - 1)
- `a_i`, `b_i` are deterministic from a fixed seed (reproducible across restarts)

**Input:** String content → rune-based character bigram set (same bigram logic as the fixed `bigramJaccard`)

**Output:** `type Signature [128]uint64`

**API:**
```go
// Hasher computes MinHash signatures from string content.
type Hasher struct {
    a, b [NumHashes]uint64  // hash function coefficients
    p    uint64              // prime modulus
}

// NewHasher creates a Hasher with deterministic coefficients from seed.
func NewHasher(seed int64) *Hasher

// Signature computes the MinHash signature for content.
func (h *Hasher) Signature(content string) Signature

// EstimatedJaccard returns the estimated Jaccard similarity from two signatures.
func EstimatedJaccard(a, b Signature) float64
```

### 5.3 LSH Banding (`lsh.go`)

**Parameters:**
- `NumBands = 16`
- `RowsPerBand = 8` (16 x 8 = 128 = NumHashes)

**Probability analysis at threshold 0.85:**
- P(candidate | sim=0.85) = 1 - (1 - 0.85^8)^16 = 0.97 (97% true positive rate)
- P(candidate | sim=0.50) = 1 - (1 - 0.50^8)^16 = 0.06 (6% false positive rate)
- P(candidate | sim=0.30) = 1 - (1 - 0.30^8)^16 < 0.001 (negligible)

**API:**
```go
// Index stores signatures and finds candidate pairs via LSH banding.
type Index struct {
    bands   int
    rows    int
    buckets []map[uint64][]string  // band → bucket_hash → list of memory IDs
}

// NewIndex creates an LSH index with the given band/row configuration.
func NewIndex(bands, rowsPerBand int) *Index

// Add inserts a memory's signature into the index.
func (idx *Index) Add(id string, sig Signature)

// Candidates returns all pairs of memory IDs that share at least one LSH bucket.
func (idx *Index) Candidates() [][2]string
```

### 5.4 Hybrid Scoring in ConsolidateWithClaude

**New flow:**

```
1. Fetch up to consolidateMaxMemories memories (unchanged)
2. Filter: 50-4000 chars, not immutable (unchanged)
3. Compute MinHash signature per memory — O(n)
4. Build LSH index, extract candidate pairs — O(n)
5. For each candidate pair:
   a. Exact bigramJaccard — must be >= consolidateJaccardThreshold (0.85)
   b. Embedding cosine lookup via VectorSearch — convert to similarity
   c. Combined score: 0.7 * jaccard + 0.3 * cosine_similarity
6. Pairs above combined threshold (0.80) → batch to Claude reviewer
```

**Combined threshold reasoning:** The 0.80 combined threshold is lower than the 0.85 Jaccard-only threshold because the embedding signal adds information. A pair with Jaccard=0.82 and cosine=0.95 is likely a genuine duplicate even though pure Jaccard would miss it. The Claude reviewer is the final arbiter.

**Embedding lookup:** For each candidate pair `(A, B)`, we need memory A's best chunk embedding and memory B's best chunk embedding. Rather than running a full `VectorSearch`, we compute cosine distance directly:

```go
// ChunkEmbeddingDistance returns the minimum cosine distance between any
// chunk of memA and any chunk of memB.
func (b *PostgresBackend) ChunkEmbeddingDistance(
    ctx context.Context, memAID, memBID string,
) (float64, error)
```

```sql
SELECT MIN(ca.embedding <=> cb.embedding) AS min_distance
FROM chunks ca, chunks cb
WHERE ca.memory_id = $1 AND cb.memory_id = $2
  AND ca.embedding IS NOT NULL AND cb.embedding IS NOT NULL
```

This is a cross-join between two small chunk sets (typically 1-5 chunks per memory), so it's fast.

---

## 6. Files Modified

| File | Changes |
|------|---------|
| `internal/db/migrations/003_pgvector.sql` | New: multi-step BYTEA→vector migration |
| `internal/db/postgres.go` | Add `VectorSearch`, `ChunkEmbeddingDistance`; update `UpdateChunkEmbedding`, `rowToChunk`; remove BYTEA helpers |
| `internal/db/backend.go` | Add `VectorSearch`, `ChunkEmbeddingDistance` to interface |
| `internal/search/engine.go` | Rewrite recall path in `RecallWithOpts`; update `ConsolidateWithClaude` to use MinHash/LSH hybrid; remove `cosineSimilarity`, `embedToBlob`, `blobToEmbed` |
| `internal/types/types.go` | `Chunk.Embedding`: `[]byte` → `[]float32` |
| `internal/minhash/minhash.go` | New: MinHash signature computation |
| `internal/minhash/lsh.go` | New: LSH banding and candidate generation |
| `internal/minhash/minhash_test.go` | New: unit tests |
| `internal/reembed/worker.go` | Update embedding write calls for `[]float32` |
| `cmd/engram/main.go` | Add dimensional guard at startup |
| `go.mod` | Add `github.com/pgvector/pgvector-go` |

---

## 7. Testing Strategy

### Unit Tests

**MinHash/LSH:**
- Two identical strings → Jaccard estimate = 1.0
- Two completely different strings → Jaccard estimate near 0.0
- Threshold 0.85: pairs above threshold appear as candidates; pairs below 0.50 do not
- Deterministic: same seed + same input → same signature

**VectorSearch:**
- Requires integration test with real pgvector (or mock). Test that closer vectors rank higher.

### Integration Tests

**Migration idempotency:**
- Run 003_pgvector.sql twice → no error, same schema
- Verify BYTEA data correctly converted to vector

**Recall path end-to-end:**
- Store 3 memories with known content
- Recall with a query close to memory #2
- Verify memory #2 ranks highest
- Verify score breakdown includes cosine from pgvector (not in-process)

**Consolidation hybrid:**
- Store two near-duplicate memories (Jaccard > 0.85)
- Store two unrelated memories (Jaccard < 0.30)
- Run ConsolidateWithClaude with mock reviewer
- Verify only the near-duplicate pair is sent for review
- Verify the combined score includes both Jaccard and cosine components

### Dimensional Guard

- Startup with matching model (768d) → server starts
- Startup with mismatched model → server refuses to start with clear error

---

## 8. Sequencing

1. **Add pgvector-go dependency** — `go get github.com/pgvector/pgvector-go`
2. **Write 003_pgvector.sql migration** — multi-step conversion
3. **Change Chunk.Embedding type** — `[]byte` → `[]float32`; update all touchpoints
4. **Implement VectorSearch** — new backend method + SQL
5. **Rewrite RecallWithOpts** — swap in VectorSearch, remove in-process scoring
6. **Implement minhash package** — Hasher + LSH Index
7. **Implement ChunkEmbeddingDistance** — new backend method
8. **Rewrite ConsolidateWithClaude** — MinHash candidates → hybrid scoring
9. **Add dimensional guard** — startup check in main.go
10. **Run full test suite** — unit + integration
