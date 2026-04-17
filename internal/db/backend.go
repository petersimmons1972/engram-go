// Package db defines the database backend interface and provides a PostgreSQL
// implementation. All methods accept context.Context as the first parameter.
package db

import (
	"context"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

// Backend is the storage interface for the Engram memory system.
// All implementations must be safe for concurrent use from multiple goroutines.
type Backend interface {
	// Close releases all resources held by the backend (connection pool, etc.).
	Close()

	// ── Project metadata ────────────────────────────────────────────────────

	// GetMeta returns the value for key in the given project, or ("", false) if absent.
	GetMeta(ctx context.Context, project, key string) (string, bool, error)
	// SetMeta upserts a key/value pair for the given project.
	SetMeta(ctx context.Context, project, key, value string) error
	// SetMetaTx is like SetMeta but runs inside an existing transaction.
	SetMetaTx(ctx context.Context, tx Tx, project, key, value string) error

	// ── Memory CRUD ─────────────────────────────────────────────────────────

	// StoreMemory persists a new memory. Sets created_at, updated_at, content_hash.
	StoreMemory(ctx context.Context, m *types.Memory) error
	// StoreMemoryTx is like StoreMemory but runs inside an existing transaction.
	StoreMemoryTx(ctx context.Context, tx Tx, m *types.Memory) error
	// GetMemory retrieves a memory by ID. Returns nil, nil if not found.
	GetMemory(ctx context.Context, id string) (*types.Memory, error)
	// GetMemoriesByIDs retrieves multiple memories by ID in a single query.
	// Only memories belonging to project are returned. Missing IDs are silently omitted.
	GetMemoriesByIDs(ctx context.Context, project string, ids []string) ([]*types.Memory, error)
	// UpdateMemory updates mutable fields on an existing memory.
	// Returns nil, nil if not found. Returns error if immutable.
	UpdateMemory(ctx context.Context, id string, content *string, tags []string, importance *int) (*types.Memory, error)
	// DeleteMemory hard-deletes a memory and its chunks/relationships by ID.
	// Prefer SoftDeleteMemory for normal use — it preserves history and respects
	// immutability. DeleteMemory is retained for internal rollback paths only
	// (e.g. MergeMemoriesAtomic loser cleanup). Returns false if not found.
	DeleteMemory(ctx context.Context, id string) (bool, error)
	// DeleteMemoryAtomic atomically locks, validates, and hard-deletes a memory.
	// force=true bypasses the immutability check (rollback path only).
	// Prefer SoftDeleteMemory for all caller-initiated deletes.
	DeleteMemoryAtomic(ctx context.Context, project, id string, force bool) (bool, error)
	// MergeMemoriesAtomic updates winnerID content (if newContent non-empty) and
	// deletes loserID in a single transaction. Prevents partial-merge state on crash.
	MergeMemoriesAtomic(ctx context.Context, project, winnerID, loserID, newContent string) error
	// ListMemories returns memories for project matching the given filters.
	ListMemories(ctx context.Context, project string, opts ListOptions) ([]*types.Memory, error)
	// TouchMemory increments access_count and sets last_accessed = now.
	TouchMemory(ctx context.Context, id string) error
	// TouchMemories batch-increments access_count and sets last_accessed = now for multiple IDs.
	TouchMemories(ctx context.Context, ids []string) error

	// ── Chunk CRUD ──────────────────────────────────────────────────────────

	// StoreChunks bulk-inserts chunks. ON CONFLICT DO NOTHING.
	StoreChunks(ctx context.Context, chunks []*types.Chunk) error
	// StoreChunksTx is like StoreChunks but runs inside an existing transaction.
	StoreChunksTx(ctx context.Context, tx Tx, chunks []*types.Chunk) error
	// GetChunksForMemory returns all chunks for a memory, ordered by chunk_index.
	GetChunksForMemory(ctx context.Context, memoryID string) ([]*types.Chunk, error)
	// GetAllChunksWithEmbeddings returns up to limit chunks that have embeddings,
	// ordered by the parent memory's last_accessed DESC.
	GetAllChunksWithEmbeddings(ctx context.Context, project string, limit int) ([]*types.Chunk, error)
	// GetAllChunkTexts returns all chunk_text values for a project (no embeddings).
	GetAllChunkTexts(ctx context.Context, project string, limit int) ([]string, error)
	// GetChunksForMemories returns embedded chunks for specific memory IDs.
	GetChunksForMemories(ctx context.Context, memoryIDs []string) ([]*types.Chunk, error)
	// ChunkHashExists returns true if a chunk with this hash exists for this memory.
	ChunkHashExists(ctx context.Context, chunkHash, memoryID string) (bool, error)
	// DeleteChunksForMemory deletes all chunks for a memory.
	DeleteChunksForMemory(ctx context.Context, memoryID string) error
	// DeleteChunksForMemoryTx is like DeleteChunksForMemory but runs inside an existing transaction.
	DeleteChunksForMemoryTx(ctx context.Context, tx Tx, memoryID string) error
	// DeleteChunksByIDs deletes specific chunks by ID. Returns count deleted.
	DeleteChunksByIDs(ctx context.Context, chunkIDs []string) (int, error)
	// NullAllEmbeddings sets embedding=NULL on all chunks for a project.
	NullAllEmbeddings(ctx context.Context, project string) (int, error)
	// NullAllEmbeddingsTx is like NullAllEmbeddings but runs inside an existing transaction.
	NullAllEmbeddingsTx(ctx context.Context, tx Tx, project string) (int, error)
	// GetChunksPendingEmbedding returns chunks with NULL embedding for a project.
	GetChunksPendingEmbedding(ctx context.Context, project string, limit int) ([]*types.Chunk, error)
	// UpdateChunkEmbedding sets the embedding on a chunk. Returns rows updated.
	UpdateChunkEmbedding(ctx context.Context, chunkID string, embedding []float32) (int, error)
	// VectorSearch returns the nearest chunks to queryVec by cosine distance,
	// using the HNSW index. Returns at most limit results.
	VectorSearch(ctx context.Context, project string, queryVec []float32, limit int) ([]VectorHit, error)
	// ChunkEmbeddingDistance returns the minimum cosine distance between any
	// chunk of memA and any chunk of memB. Returns 2.0 (max distance) if
	// either memory has no embedded chunks.
	ChunkEmbeddingDistance(ctx context.Context, memAID, memBID string) (float64, error)
	// UpdateChunkLastMatched sets last_matched = now on a chunk.
	UpdateChunkLastMatched(ctx context.Context, chunkID string) error
	// GetPendingEmbeddingCount returns the count of chunks with NULL embedding.
	GetPendingEmbeddingCount(ctx context.Context, project string) (int, error)

	// ── Relationship CRUD ───────────────────────────────────────────────────

	// StoreRelationship upserts a directed relationship between two memories.
	StoreRelationship(ctx context.Context, rel *types.Relationship) error
	// GetConnected performs a BFS from memoryID and returns connected memories
	// up to maxHops hops away.
	GetConnected(ctx context.Context, memoryID string, maxHops int) ([]ConnectedResult, error)
	// BoostEdgesForMemory increases strength on all edges touching memoryID by factor.
	BoostEdgesForMemory(ctx context.Context, memoryID string, factor float64) (int, error)
	// DecayEdgesForMemory decreases strength on all edges touching memoryID by factor.
	DecayEdgesForMemory(ctx context.Context, memoryID string, factor float64) (int, error)
	// GetConnectionCount returns the number of edges touching memoryID in project.
	GetConnectionCount(ctx context.Context, memoryID, project string) (int, error)
	// DecayAllEdges decays all edges for a project and prunes below minStrength.
	// Returns (decayed, pruned) counts.
	DecayAllEdges(ctx context.Context, project string, decayFactor, minStrength float64) (int, int, error)
	// DeleteRelationshipsForMemory deletes all edges touching memoryID.
	DeleteRelationshipsForMemory(ctx context.Context, memoryID string) error
	// GetRelationships returns all relationship edges where memoryID is either
	// the source or the target, scoped to project.
	GetRelationships(ctx context.Context, project, memoryID string) ([]types.Relationship, error)
	// GetRelationshipsBatch returns all relationship edges for a set of memory IDs
	// in a single query, grouped by the requested ID (each ID appears as both
	// source and target lookup, matching GetRelationships semantics). The returned
	// map contains an entry for every requested ID; IDs with no relationships have
	// an empty slice. An empty ids input returns an empty map immediately.
	GetRelationshipsBatch(ctx context.Context, project string, ids []string) (map[string][]types.Relationship, error)

	// ── Temporal versioning ─────────────────────────────────────────────────

	// GetMemoryHistory returns all version snapshots for memoryID in reverse
	// chronological order (most recent change first).
	GetMemoryHistory(ctx context.Context, project, memoryID string) ([]*types.MemoryVersion, error)

	// SoftDeleteMemory marks a memory as invalid by setting valid_to=NOW() and
	// storing the final state in memory_versions with change_type="invalidate".
	// The memory and its chunks are NOT removed from the database; they remain
	// for history queries. Returns false if not found, error if immutable.
	SoftDeleteMemory(ctx context.Context, project, id, reason string) (bool, error)

	// GetMemoriesAsOf returns memories that were active at the given point in time:
	// created_at <= asOf AND (valid_to IS NULL OR valid_to > asOf).
	GetMemoriesAsOf(ctx context.Context, project string, asOf time.Time, limit int) ([]*types.Memory, error)

	// ── Retrieval outcome tracking ──────────────────────────────────────────

	// StoreRetrievalEvent persists a new retrieval event. result_ids holds the
	// memory IDs returned by the recall call.
	StoreRetrievalEvent(ctx context.Context, event *types.RetrievalEvent) error

	// GetRetrievalEvent fetches a retrieval event by ID.
	GetRetrievalEvent(ctx context.Context, id string) (*types.RetrievalEvent, error)

	// RecordFeedback updates the retrieval event with feedback_ids, sets
	// feedback_at=NOW(), increments times_retrieved on all result memories, and
	// increments times_useful on feedback memories. Recomputes retrieval_precision
	// once times_retrieved >= 5.
	RecordFeedback(ctx context.Context, eventID string, usefulIDs []string) error

	// IncrementTimesRetrieved increments times_retrieved on the given memory IDs.
	IncrementTimesRetrieved(ctx context.Context, ids []string) error

	// ── Adaptive importance ─────────────────────────────────────────────────

	// UpdateDynamicImportance atomically adjusts dynamic_importance by delta
	// and, if positive, advances retrieval_interval_hrs by the given factor and
	// sets next_review_at = now + new_interval. delta may be negative.
	UpdateDynamicImportance(ctx context.Context, id string, delta float64, intervalFactor float64) error

	// SetNextReviewAt overrides the next_review_at timestamp for a memory.
	// Used by tests and the decay worker to force stale state.
	SetNextReviewAt(ctx context.Context, id string, t time.Time) error

	// DecayStaleImportance multiplies dynamic_importance by factor (<1.0) on all
	// active memories whose next_review_at is in the past. Returns rows updated.
	DecayStaleImportance(ctx context.Context, project string, factor float64) (int, error)

	// ── Pruning ─────────────────────────────────────────────────────────────

	// PruneStaleMemories deletes old low-importance and expired memories. Returns count.
	PruneStaleMemories(ctx context.Context, project string, maxAgeHours float64, maxImportance int) (int, error)
	// PruneColdDocuments deletes document-mode memories whose chunks were never matched.
	PruneColdDocuments(ctx context.Context, project string, maxAgeHours float64, maxImportance int) (int, error)

	// ── Full-text search ────────────────────────────────────────────────────

	// FTSSearch runs a PostgreSQL plainto_tsquery search. Returns (memory, score) pairs.
	FTSSearch(ctx context.Context, project, query string, limit int, since, before *time.Time) ([]FTSResult, error)
	// RebuildFTS reindexes the GIN search_vector index (runs outside a transaction).
	RebuildFTS(ctx context.Context) error

	// ── Stats and integrity ─────────────────────────────────────────────────

	// GetStats returns aggregate statistics for a project.
	GetStats(ctx context.Context, project string) (*types.MemoryStats, error)
	// ListAllProjects returns all distinct project names.
	ListAllProjects(ctx context.Context) ([]string, error)
	// GetAllMemoryIDs returns all memory IDs for a project.
	GetAllMemoryIDs(ctx context.Context, project string) (map[string]struct{}, error)
	// GetMemoriesPendingSummary returns (id, content) for memories where summary IS NULL.
	GetMemoriesPendingSummary(ctx context.Context, project string, limit int) ([]IDContent, error)
	// StoreSummary sets the summary field for a memory.
	StoreSummary(ctx context.Context, memoryID, summary string) error
	// GetPendingSummaryCount returns the count of memories with summary IS NULL.
	GetPendingSummaryCount(ctx context.Context, project string) (int, error)
	// ClearSummaries sets summary = NULL for all active memories in a project,
	// causing the background summarize worker to regenerate them on its next tick.
	// Returns the number of rows affected.
	ClearSummaries(ctx context.Context, project string) (int, error)
	// GetMemoriesMissingHash returns (id, content) for memories with no content_hash.
	GetMemoriesMissingHash(ctx context.Context, project string, limit int) ([]IDContent, error)
	// UpdateMemoryHash sets the content_hash field for a memory.
	UpdateMemoryHash(ctx context.Context, memoryID, contentHash string) error
	// ExistsWithContentHash returns true if a non-invalidated memory with the
	// given SHA-256 hex content hash exists in the project.
	ExistsWithContentHash(ctx context.Context, project, hash string) (bool, error)
	// GetIntegrityStats returns total, hashed, and corrupt counts for a project.
	GetIntegrityStats(ctx context.Context, project string) (IntegrityStats, error)

	// ── Episodes ────────────────────────────────────────────────────────────

	// StartEpisode creates a new episode record for the project and returns it.
	StartEpisode(ctx context.Context, project, description string) (*types.Episode, error)

	// EndEpisode marks an episode as ended and records an optional summary.
	EndEpisode(ctx context.Context, id, summary string) error

	// ListEpisodes returns up to limit episodes for the project, most recent first.
	ListEpisodes(ctx context.Context, project string, limit int) ([]*types.Episode, error)

	// RecallEpisode returns all memories associated with the given episode,
	// ordered by created_at ascending (chronological).
	RecallEpisode(ctx context.Context, episodeID string) ([]*types.Memory, error)

	// ── Raw document storage (Tier-2 ingestion, A4) ─────────────────────────

	// StoreDocument stores raw document content and returns the new document ID.
	// The content is hashed with SHA-256 and stored alongside size and project.
	StoreDocument(ctx context.Context, project, content string) (string, error)

	// GetDocument retrieves raw document content by ID. Returns "" if not found.
	GetDocument(ctx context.Context, id string) (string, error)

	// SetMemoryDocumentID links a memory to a document by setting
	// memories.document_id = documentID.
	SetMemoryDocumentID(ctx context.Context, memoryID, documentID string) error

	// ── Transactions ────────────────────────────────────────────────────────

	// Begin starts a new transaction.
	Begin(ctx context.Context) (Tx, error)
}

// Tx is an opaque transaction handle passed to *Tx methods.
// Call Commit or Rollback exactly once.
type Tx interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// ListOptions filters for ListMemories.
type ListOptions struct {
	MemoryType       *string  // nil means all types
	Tags             []string // match ANY tag
	ImportanceCeiling *int    // include importance <= this value
	Limit            int
	Offset           int
}

// ConnectedResult is one row from GetConnected.
type ConnectedResult struct {
	Memory    *types.Memory
	RelType   string
	Direction string // "outgoing" or "incoming"
	Strength  float64
}

// FTSResult is one row from FTSSearch.
type FTSResult struct {
	Memory *types.Memory
	Score  float64
}

// VectorHit is a single result from a pgvector ANN search.
type VectorHit struct {
	ChunkID        string
	MemoryID       string
	Distance       float64 // cosine distance (0 = identical, 2 = opposite)
	ChunkText      string
	ChunkIndex     int
	SectionHeading *string
}

// IDContent pairs a memory ID with its content (used for batch summary/hash operations).
type IDContent struct {
	ID      string
	Content string
}

// IntegrityStats summarizes hash coverage and corruption for a project.
type IntegrityStats struct {
	Total   int
	Hashed  int
	Corrupt int
}
