// Package types defines the core data structures and constants for the Engram
// memory system. All values are ported from the Python reference implementation
// in engram/src/engram/types.py and must remain wire-compatible with it.
package types

import (
	"time"

	"github.com/google/uuid"
)

// Memory type constants. Values must match Python MemoryType enum strings exactly.
const (
	MemoryTypeDecision     = "decision"
	MemoryTypePattern      = "pattern"
	MemoryTypeError        = "error"
	MemoryTypeContext      = "context"
	MemoryTypeArchitecture = "architecture"
	MemoryTypePreference   = "preference"
)

// Relationship type constants. Values must match Python RelationType enum strings exactly.
const (
	RelTypeCausedBy    = "caused_by"
	RelTypeRelatesTo   = "relates_to"
	RelTypeDependsOn   = "depends_on"
	RelTypeSupersedes  = "supersedes"
	RelTypeUsedIn      = "used_in"
	RelTypeResolvedBy  = "resolved_by"
	RelTypeContradicts = "contradicts" // set by sleep consolidation daemon

	// Semantic types from open-brain vocabulary (additive merge, v3.x).
	RelTypeSupports    = "supports"      // one memory strengthens another's evidence
	RelTypeDerivedFrom = "derived_from"  // citation chain — memory derived from source
	RelTypePartOf      = "part_of"       // hierarchical containment
	RelTypeFollows     = "follows"       // temporal or sequential ordering
)

// MemoryVersionChangeType constants for memory_versions.change_type.
const (
	VersionChangeUpdate     = "update"
	VersionChangeInvalidate = "invalidate"
)

// FailureClass constants classify why a retrieval event failed. The set is closed — add new values via types migration only.
const (
	FailureClassVocabularyMismatch = "vocabulary_mismatch"
	FailureClassAggregationFailure = "aggregation_failure"
	FailureClassStaleRanking       = "stale_ranking"
	FailureClassMissingContent     = "missing_content"
	FailureClassScopeMismatch      = "scope_mismatch"
	FailureClassOther              = "other"
)

var validFailureClasses = map[string]bool{
	FailureClassVocabularyMismatch: true,
	FailureClassAggregationFailure: true,
	FailureClassStaleRanking:       true,
	FailureClassMissingContent:     true,
	FailureClassScopeMismatch:      true,
	FailureClassOther:              true,
}

func ValidateFailureClass(s string) bool {
	if s == "" {
		return true
	}
	return validFailureClasses[s]
}

// MaxContentLength is the maximum allowed length of memory content in bytes.
// Mirrors Python MAX_CONTENT_LENGTH = 500_000.
const MaxContentLength = 500_000

// validMemoryTypes is the closed set of allowed memory type strings.
var validMemoryTypes = map[string]bool{
	MemoryTypeDecision:     true,
	MemoryTypePattern:      true,
	MemoryTypeError:        true,
	MemoryTypeContext:      true,
	MemoryTypeArchitecture: true,
	MemoryTypePreference:   true,
}

// validRelationTypes is the closed set of allowed relationship type strings.
var validRelationTypes = map[string]bool{
	RelTypeCausedBy:    true,
	RelTypeRelatesTo:   true,
	RelTypeDependsOn:   true,
	RelTypeSupersedes:  true,
	RelTypeUsedIn:      true,
	RelTypeResolvedBy:  true,
	RelTypeContradicts: true,
	RelTypeSupports:    true,
	RelTypeDerivedFrom: true,
	RelTypePartOf:      true,
	RelTypeFollows:     true,
}

// ValidateMemoryType reports whether s is a valid memory type constant.
func ValidateMemoryType(s string) bool {
	return validMemoryTypes[s]
}

// ValidateRelationType reports whether s is a valid relationship type constant.
func ValidateRelationType(s string) bool {
	return validRelationTypes[s]
}

// ValidateImportance clamps n to the valid importance range [0, 4].
// 0 = critical (never pruned), 4 = trivial (auto-pruned).
func ValidateImportance(n int) int {
	if n < 0 {
		return 0
	}
	if n > 4 {
		return 4
	}
	return n
}

// NewMemoryID generates a UUID v7 identifier. UUID v7 encodes a millisecond
// timestamp in the high bits, so IDs are time-sortable and monotonically
// increasing within the same millisecond window.
func NewMemoryID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// Fallback to v4 if the OS entropy source fails — should never occur in
		// production but we must not panic on an error path.
		id = uuid.New()
	}
	return id.String()
}

// Memory is the primary storage unit. One Memory maps to one or more Chunks.
// Fields are tagged for JSON serialization with snake_case keys to match the
// MCP tool wire format.
type Memory struct {
	// ID is a UUID v7 string, time-sortable.
	ID      string `json:"id"`
	Content string `json:"content"`

	// RawBody holds the original full content when a Tier-1 synopsis memory is
	// constructed in-process. It is never serialised (json:"-") because it is
	// only meaningful during the current Store call: once the memory is written
	// to the database, Content (the synopsis) is the authoritative field.
	//
	// Usage: set this before calling Store() when m.Content holds a synopsis
	// and you want chunks to be built from the full body. Store() passes
	// m.RawBody to StoreWithRawBody, eliminating the magic-value "" sentinel.
	// When RawBody is empty (normal memories), behaviour is unchanged.
	RawBody string `json:"-"`
	MemoryType  string    `json:"memory_type"`
	Project     string    `json:"project"`
	Tags        []string  `json:"tags"`
	Importance  int       `json:"importance"`
	AccessCount int       `json:"access_count"`
	LastAccessed time.Time `json:"last_accessed"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Immutable   bool      `json:"immutable"`

	// ExpiresAt is nil when the memory does not expire.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// Summary is populated asynchronously by the background summarizer.
	// nil until summarization has run.
	Summary *string `json:"summary,omitempty"`

	// ContentHash is a SHA-256 hex digest of the content, stored in DB and
	// validated on read.
	ContentHash *string `json:"content_hash,omitempty"`

	// StorageMode is "focused" (sentence-window chunks) or "document"
	// (semantic chunking via ChunkDocument).
	StorageMode string `json:"storage_mode"`

	// ValidFrom and ValidTo define the known-truth window (bi-temporal model).
	// ValidTo IS NULL while the memory is active. Set to NOW() on soft-delete.
	ValidFrom *time.Time `json:"valid_from,omitempty"`
	ValidTo   *time.Time `json:"valid_to,omitempty"`

	// InvalidationReason records why the memory was soft-deleted.
	InvalidationReason *string `json:"invalidation_reason,omitempty"`

	// DynamicImportance is the learned importance score updated via spaced repetition.
	// Starts at (5-Importance)/3 and drifts up on positive feedback, down on decay.
	DynamicImportance *float64 `json:"dynamic_importance,omitempty"`

	// RetrievalIntervalHrs is the spaced-repetition interval in hours.
	// Grows by 1.5× on positive feedback; reset toward default on negative.
	RetrievalIntervalHrs float64 `json:"retrieval_interval_hrs,omitempty"`

	// NextReviewAt is when the memory should next be retrieved to avoid decay.
	NextReviewAt *time.Time `json:"next_review_at,omitempty"`

	// TimesRetrieved counts how many recall events included this memory.
	TimesRetrieved int `json:"times_retrieved,omitempty"`

	// TimesUseful counts how many of those retrievals were marked useful by the caller.
	TimesUseful int `json:"times_useful,omitempty"`

	// RetrievalPrecision is times_useful / times_retrieved, set once TimesRetrieved >= 5.
	// nil during cold start (treated as 0.5 neutral in scoring).
	RetrievalPrecision *float64 `json:"retrieval_precision,omitempty"`

	// EpisodeID links this memory to the session episode during which it was stored.
	// nil if the memory was stored outside of a named episode.
	EpisodeID string `json:"episode_id,omitempty"`

	// DocumentID links a Tier-2 ingested memory to the raw document content
	// stored in the documents table. Empty for focused and document-mode
	// memories that fit within the standard chunking pipeline.
	DocumentID string `json:"document_id,omitempty"`
}

// Episode is a named session context that groups memories stored during one
// connected session. Created by memory_episode_start, closed by memory_episode_end.
type Episode struct {
	ID          string    `json:"id"`
	Project     string    `json:"project"`
	Description string    `json:"description"`
	StartedAt   time.Time `json:"started_at"`
	EndedAt     time.Time `json:"ended_at,omitempty"`
	Summary     string    `json:"summary,omitempty"`
}

// RetrievalEvent records one recall invocation and the caller's feedback.
type RetrievalEvent struct {
	ID           string     `json:"id"`
	Project      string     `json:"project"`
	Query        string     `json:"query"`
	ResultIDs    []string   `json:"result_ids"`
	FeedbackIDs  []string   `json:"feedback_ids,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	FeedbackAt   *time.Time `json:"feedback_at,omitempty"`
	FailureClass string     `json:"failure_class,omitempty"`
}

// MemoryVersion is one entry in the bi-temporal history of a Memory.
// A row is written to memory_versions before every update and on every
// soft-delete, giving a full audit trail.
type MemoryVersion struct {
	ID           string     `json:"id"`
	MemoryID     string     `json:"memory_id"`
	Content      string     `json:"content"`
	MemoryType   string     `json:"memory_type"`
	Tags         []string   `json:"tags"`
	Importance   int        `json:"importance"`
	SystemFrom   time.Time  `json:"system_from"`
	SystemTo     *time.Time `json:"system_to,omitempty"`
	ValidFrom    *time.Time `json:"valid_from,omitempty"`
	ValidTo      *time.Time `json:"valid_to,omitempty"`
	ChangeType   string     `json:"change_type"`
	ChangeReason *string    `json:"change_reason,omitempty"`
	Project      string     `json:"project"`
}

// Chunk is a sub-unit of a Memory, holding one text window and its embedding.
type Chunk struct {
	ID         string `json:"id"`
	MemoryID   string `json:"memory_id"`
	ChunkText  string `json:"chunk_text"`
	ChunkIndex int    `json:"chunk_index"`
	ChunkHash  string `json:"chunk_hash"`

	// Embedding is the float32 vector from the embedding model.
	// Stored as pgvector vector(768) in the database.
	Embedding []float32 `json:"embedding,omitempty"`

	// SectionHeading is the nearest level-1/2 Markdown heading ancestor, or nil
	// for non-document-mode chunks.
	SectionHeading *string `json:"section_heading,omitempty"`

	// ChunkType is "sentence_window", "paragraph", or "section".
	ChunkType string `json:"chunk_type"`

	// LastMatched records the last time this chunk was returned as a search hit.
	LastMatched *time.Time `json:"last_matched,omitempty"`

	// Project is denormalized from the parent Memory for efficient per-project queries.
	Project string `json:"project"`
}

// Relationship represents a directed link between two memories.
type Relationship struct {
	ID        string    `json:"id"`
	SourceID  string    `json:"source_id"`
	TargetID  string    `json:"target_id"`
	RelType   string    `json:"rel_type"`
	Strength  float64   `json:"strength"`
	Project   string    `json:"project"`
	CreatedAt time.Time `json:"created_at"`
}

// SearchResult is returned by the search engine for each ranked hit.
type SearchResult struct {
	Memory            *Memory            `json:"memory"`
	Score             float64            `json:"score"`
	ScoreBreakdown    map[string]float64 `json:"score_breakdown"`
	MatchedChunk      string             `json:"matched_chunk"`
	ChunkScore        float64            `json:"chunk_score"`
	MatchedChunkIndex int                `json:"matched_chunk_index"`

	// Connected holds memories linked via the knowledge graph.
	Connected []ConnectedMemory `json:"connected"`

	// MatchedChunkSection is the section_heading of the matched chunk, if any.
	MatchedChunkSection *string `json:"matched_chunk_section,omitempty"`
}

// ConnectedMemory is a graph neighbor attached to a SearchResult.
type ConnectedMemory struct {
	Memory    *Memory `json:"memory"`
	RelType   string  `json:"rel_type"`
	Direction string  `json:"direction"` // "outgoing" or "incoming"
	Strength  float64 `json:"strength"`
}

// Handle is a lightweight reference returned by handle-mode recall.
// The caller fetches content on demand via memory_fetch, keeping the transcript
// free of large memory payloads until they are actually needed.
type Handle struct {
	ID          string  `json:"id"`
	Project     string  `json:"project"`
	Summary     string  `json:"summary"`       // "" if not yet summarized
	Score       float64 `json:"score"`
	StorageMode string  `json:"storage_mode"`
	ChunkCount  int     `json:"chunk_count"`   // 0 if unknown at recall time
	Bytes       int     `json:"bytes"`         // len(content)
	IsHandle    bool    `json:"is_handle"`     // always true; signals paged-out result
	FetchHint   string  `json:"fetch_hint"`    // human-readable usage hint
}

// MemoryStats summarizes the contents of the store. Returned by the status endpoint.
type MemoryStats struct {
	TotalMemories       int            `json:"total_memories"`
	TotalChunks         int            `json:"total_chunks"`
	TotalRelationships  int            `json:"total_relationships"`
	ByType              map[string]int `json:"by_type"`
	ByImportance        map[string]int `json:"by_importance"`
	Oldest              *string        `json:"oldest,omitempty"`
	Newest              *string        `json:"newest,omitempty"`
	DBSizeBytes         int64          `json:"db_size_bytes"`
	PendingSummarization int           `json:"pending_summarization"`
	Summarization       map[string]any `json:"summarization"`
}

// FTSResult is an intermediate result from the full-text search layer, before
// merging with vector scores.
type FTSResult struct {
	Memory *Memory `json:"memory"`
	Score  float64 `json:"score"`
}

// ConflictingResult represents a memory that contradicts one of the recall
// results, discovered by following "contradicts" edges in the relationship
// graph. Returned in the conflicting_results array when include_conflicts=true
// is passed to memory_recall.
type ConflictingResult struct {
	Memory        *Memory `json:"memory"`
	ContradictsID string  `json:"contradicts_id"` // ID of the recalled memory this contradicts
	Strength      float64 `json:"strength"`        // contradiction edge strength
	MatchedChunk  string  `json:"matched_chunk"`   // first 500 bytes of content
}

// AggregateRow is one bucket in an aggregate query result.
type AggregateRow struct {
	Label  string    `json:"label"`
	Count  int       `json:"count"`
	Oldest time.Time `json:"oldest"`
	Newest time.Time `json:"newest"`
}
