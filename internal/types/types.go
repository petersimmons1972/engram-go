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
	RelTypeCausedBy   = "caused_by"
	RelTypeRelatesTo  = "relates_to"
	RelTypeDependsOn  = "depends_on"
	RelTypeSupersedes = "supersedes"
	RelTypeUsedIn     = "used_in"
	RelTypeResolvedBy = "resolved_by"
)

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
	RelTypeCausedBy:   true,
	RelTypeRelatesTo:  true,
	RelTypeDependsOn:  true,
	RelTypeSupersedes: true,
	RelTypeUsedIn:     true,
	RelTypeResolvedBy: true,
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
	ID          string    `json:"id"`
	Content     string    `json:"content"`
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
}

// Chunk is a sub-unit of a Memory, holding one text window and its embedding.
type Chunk struct {
	ID         string `json:"id"`
	MemoryID   string `json:"memory_id"`
	ChunkText  string `json:"chunk_text"`
	ChunkIndex int    `json:"chunk_index"`
	ChunkHash  string `json:"chunk_hash"`

	// Embedding is the raw little-endian float32 blob from vector.ToBlob.
	Embedding []byte `json:"embedding,omitempty"`

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
