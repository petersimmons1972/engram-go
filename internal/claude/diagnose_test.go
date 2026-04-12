package claude_test

// Feature 7: Conflict-Aware Reasoning
// All tests written BEFORE implementation (TDD).
// They must fail (compile or runtime) until Feature 7 is implemented.

import (
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiagnoseMemories_NoConflicts verifies that DiagnoseMemories returns
// confidence=1.0 and empty conflicts when no contradicts edges exist.
func TestDiagnoseMemories_NoConflicts(t *testing.T) {
	memories := []*types.Memory{
		{ID: "m1", Content: "Go uses goroutines for concurrency", Project: "test"},
		{ID: "m2", Content: "Go channels enable safe communication", Project: "test"},
	}
	result := claude.DiagnoseMemories(memories, nil)
	assert.InDelta(t, 1.0, result.Confidence, 0.001,
		"confidence must be 1.0 when no conflicts exist")
	assert.Empty(t, result.Conflicts, "conflicts must be empty")
	assert.Empty(t, result.InvalidatedSources, "no invalidated sources")
}

// TestDiagnoseMemories_WithContradicts verifies that DiagnoseMemories detects
// contradicts edges and reduces confidence.
func TestDiagnoseMemories_WithContradicts(t *testing.T) {
	m1 := &types.Memory{ID: "m1", Content: "PostgreSQL uses MVCC", Project: "test"}
	m2 := &types.Memory{ID: "m2", Content: "PostgreSQL does not use MVCC", Project: "test"}
	m3 := &types.Memory{ID: "m3", Content: "Go is statically typed", Project: "test"}

	rels := []types.Relationship{
		{ID: "r1", SourceID: "m1", TargetID: "m2", RelType: types.RelTypeContradicts, Strength: 0.9, Project: "test"},
	}

	result := claude.DiagnoseMemories([]*types.Memory{m1, m2, m3}, rels)
	require.Len(t, result.Conflicts, 1, "must detect the contradicts edge")
	assert.Equal(t, "m1", result.Conflicts[0].MemoryAID)
	assert.Equal(t, "m2", result.Conflicts[0].MemoryBID)
	assert.InDelta(t, 0.9, result.Conflicts[0].Strength, 0.001)
	assert.InDelta(t, 0.5, result.Confidence, 0.001,
		"confidence with 1 conflict: 1/(1+1) = 0.5")
}

// TestDiagnoseMemories_InvalidatedSources verifies that memories with ValidTo set
// appear in InvalidatedSources.
func TestDiagnoseMemories_InvalidatedSources(t *testing.T) {
	invalidTime := time.Now().UTC()
	m1 := &types.Memory{ID: "m1", Content: "Active memory", Project: "test"}
	m2 := &types.Memory{ID: "m2", Content: "Invalidated memory", Project: "test", ValidTo: &invalidTime}

	result := claude.DiagnoseMemories([]*types.Memory{m1, m2}, nil)
	assert.Contains(t, result.InvalidatedSources, "m2",
		"invalidated memory ID must appear in InvalidatedSources")
	assert.NotContains(t, result.InvalidatedSources, "m1",
		"active memory must not appear in InvalidatedSources")
}

// TestConflictAwarePrompt_WithConflicts verifies that BuildConflictAwarePrompt
// includes conflict annotations when conflicts exist.
func TestConflictAwarePrompt_WithConflicts(t *testing.T) {
	m1 := &types.Memory{ID: "m1", Content: "Claim A about MVCC", Project: "test"}
	m2 := &types.Memory{ID: "m2", Content: "Claim B about MVCC (contradicts A)", Project: "test"}

	evidence := claude.EvidenceMap{
		Memories: []*types.Memory{m1, m2},
		Conflicts: []claude.ConflictPair{
			{MemoryAID: "m1", MemoryBID: "m2", Strength: 0.9},
		},
		Confidence: 0.5,
	}

	prompt := claude.BuildConflictAwarePrompt("What does MVCC do?", evidence)
	assert.Contains(t, prompt, "CONFLICT", "prompt must call out conflicts explicitly")
	assert.Contains(t, prompt, "m1", "prompt must name the conflicting memory IDs")
	assert.Contains(t, prompt, "m2", "prompt must name the conflicting memory IDs")
	assert.Contains(t, prompt, "What does MVCC do?", "question must appear in prompt")
}

// TestConflictAwarePrompt_IncludesRejectionInstruction verifies that
// BuildConflictAwarePrompt instructs Claude to explicitly name rejected
// alternatives when conflicts are present.
func TestConflictAwarePrompt_IncludesRejectionInstruction(t *testing.T) {
	m1 := &types.Memory{ID: "m1", Content: "X uses MVCC", Project: "test"}
	m2 := &types.Memory{ID: "m2", Content: "X does not use MVCC", Project: "test"}

	evidence := claude.EvidenceMap{
		Memories: []*types.Memory{m1, m2},
		Conflicts: []claude.ConflictPair{
			{MemoryAID: "m1", MemoryBID: "m2", Strength: 0.85},
		},
		Confidence: 0.5,
	}

	prompt := claude.BuildConflictAwarePrompt("Does X use MVCC?", evidence)
	assert.Contains(t, prompt, "explicitly name", "prompt must instruct Claude to explicitly name rejected alternatives")
}

// TestConflictAwarePrompt_IncludesConflictingContent verifies that
// BuildConflictAwarePrompt includes actual memory content for both sides of
// a conflict, not just their IDs.
func TestConflictAwarePrompt_IncludesConflictingContent(t *testing.T) {
	m1 := &types.Memory{ID: "m1", Content: "X uses MVCC", Project: "test"}
	m2 := &types.Memory{ID: "m2", Content: "X does not use MVCC", Project: "test"}

	evidence := claude.EvidenceMap{
		Memories: []*types.Memory{m1, m2},
		Conflicts: []claude.ConflictPair{
			{MemoryAID: "m1", MemoryBID: "m2", Strength: 0.85},
		},
		Confidence: 0.5,
	}

	prompt := claude.BuildConflictAwarePrompt("Does X use MVCC?", evidence)
	// Both content excerpts must appear in the conflict section, not just IDs.
	assert.Contains(t, prompt, "X uses MVCC", "prompt must include CLAIM A content")
	assert.Contains(t, prompt, "X does not use MVCC", "prompt must include CLAIM B content")
	// Content must be labeled as CLAIM A / CLAIM B so Claude can reference them.
	assert.Contains(t, prompt, "CLAIM A", "prompt must label the first conflicting claim")
	assert.Contains(t, prompt, "CLAIM B", "prompt must label the second conflicting claim")
}
