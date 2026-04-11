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
	assert.Less(t, result.Confidence, 1.0, "confidence must drop when conflicts exist")
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
