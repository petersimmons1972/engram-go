package types_test

import (
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

func TestValidateMemoryType(t *testing.T) {
	valid := []string{"decision", "pattern", "error", "context", "architecture", "preference"}
	for _, v := range valid {
		if !types.ValidateMemoryType(v) {
			t.Errorf("expected %q to be valid memory type", v)
		}
	}
	invalid := []string{"", "unknown", "DECISION", "Decision"}
	for _, v := range invalid {
		if types.ValidateMemoryType(v) {
			t.Errorf("expected %q to be invalid memory type", v)
		}
	}
}

func TestValidateRelationType(t *testing.T) {
	valid := []string{"caused_by", "relates_to", "depends_on", "supersedes", "used_in", "resolved_by"}
	for _, v := range valid {
		if !types.ValidateRelationType(v) {
			t.Errorf("expected %q to be valid relation type", v)
		}
	}
	invalid := []string{"", "unknown", "CAUSED_BY", "causedby"}
	for _, v := range invalid {
		if types.ValidateRelationType(v) {
			t.Errorf("expected %q to be invalid relation type", v)
		}
	}
}

func TestValidateImportance(t *testing.T) {
	cases := []struct {
		input    int
		expected int
	}{
		{0, 0},  // critical — unchanged
		{4, 4},  // trivial — unchanged
		{2, 2},  // medium — unchanged
		{-1, 0}, // below floor → clamped to 0
		{5, 4},  // above ceiling → clamped to 4
		{99, 4}, // way over → clamped to 4
	}
	for _, c := range cases {
		got := types.ValidateImportance(c.input)
		if got != c.expected {
			t.Errorf("ValidateImportance(%d) = %d, want %d", c.input, got, c.expected)
		}
	}
}

func TestNewMemoryID(t *testing.T) {
	id1 := types.NewMemoryID()
	id2 := types.NewMemoryID()

	if id1 == "" {
		t.Error("NewMemoryID returned empty string")
	}
	if id1 == id2 {
		t.Error("NewMemoryID returned duplicate IDs")
	}
	// UUID v7 is hex with dashes; verify reasonable length (36 chars with dashes)
	if len(id1) != 36 {
		t.Errorf("NewMemoryID returned ID with unexpected length %d (want 36): %q", len(id1), id1)
	}
	// Version nibble must be '7'
	// UUID format: xxxxxxxx-xxxx-7xxx-xxxx-xxxxxxxxxxxx
	// version nibble is at index 14
	if id1[14] != '7' {
		t.Errorf("NewMemoryID returned non-v7 UUID: %q", id1)
	}
}

func TestMemoryDefaults(t *testing.T) {
	m := types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "test content",
		MemoryType:  types.MemoryTypeContext,
		Project:     "default",
		StorageMode: "focused",
		Importance:  2,
	}

	if m.Content != "test content" {
		t.Errorf("unexpected content: %q", m.Content)
	}
	if m.Immutable {
		t.Error("new Memory should not be immutable by default")
	}
	if m.ExpiresAt != nil {
		t.Error("new Memory ExpiresAt should be nil by default")
	}
	if m.Summary != nil {
		t.Error("new Memory Summary should be nil by default")
	}
}

func TestChunkStruct(t *testing.T) {
	now := time.Now()
	heading := "Introduction"
	c := types.Chunk{
		ID:             types.NewMemoryID(),
		MemoryID:       "mem-123",
		ChunkText:      "some chunk text",
		ChunkIndex:     0,
		ChunkHash:      "abc123",
		ChunkType:      "sentence_window",
		SectionHeading: &heading,
		LastMatched:    &now,
		Project:        "testproject",
	}
	if c.ChunkIndex != 0 {
		t.Errorf("unexpected chunk index: %d", c.ChunkIndex)
	}
	if *c.SectionHeading != "Introduction" {
		t.Errorf("unexpected section heading: %q", *c.SectionHeading)
	}
}

func TestRelationshipStruct(t *testing.T) {
	r := types.Relationship{
		ID:       types.NewMemoryID(),
		SourceID: "src-1",
		TargetID: "tgt-1",
		RelType:  types.RelTypeRelatesTo,
		Strength: 0.8,
		Project:  "myproject",
	}
	if r.Strength != 0.8 {
		t.Errorf("unexpected strength: %f", r.Strength)
	}
}

func TestSearchResultStruct(t *testing.T) {
	mem := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    "test",
		MemoryType: types.MemoryTypeDecision,
		Project:    "p",
	}
	sr := types.SearchResult{
		Memory:            mem,
		Score:             0.95,
		ScoreBreakdown:    map[string]float64{"bm25": 0.5, "vector": 0.45},
		MatchedChunk:      "matched text here",
		ChunkScore:        0.87,
		MatchedChunkIndex: 2,
	}
	if sr.Score != 0.95 {
		t.Errorf("unexpected score: %f", sr.Score)
	}
	if len(sr.Connected) != 0 {
		t.Error("expected empty Connected slice by default")
	}
}

func TestMemoryTypeConstants(t *testing.T) {
	// Verify the constant values match Python enum values exactly
	expected := map[string]string{
		"decision":     types.MemoryTypeDecision,
		"pattern":      types.MemoryTypePattern,
		"error":        types.MemoryTypeError,
		"context":      types.MemoryTypeContext,
		"architecture": types.MemoryTypeArchitecture,
		"preference":   types.MemoryTypePreference,
	}
	for want, got := range expected {
		if want != got {
			t.Errorf("constant mismatch: want %q got %q", want, got)
		}
	}
}

func TestRelationTypeConstants(t *testing.T) {
	expected := map[string]string{
		"caused_by":   types.RelTypeCausedBy,
		"relates_to":  types.RelTypeRelatesTo,
		"depends_on":  types.RelTypeDependsOn,
		"supersedes":  types.RelTypeSupersedes,
		"used_in":     types.RelTypeUsedIn,
		"resolved_by": types.RelTypeResolvedBy,
	}
	for want, got := range expected {
		if want != got {
			t.Errorf("constant mismatch: want %q got %q", want, got)
		}
	}
}

func TestFailureClassConstants(t *testing.T) {
	cases := []struct {
		got  string
		want string
	}{
		{types.FailureClassVocabularyMismatch, "vocabulary_mismatch"},
		{types.FailureClassAggregationFailure, "aggregation_failure"},
		{types.FailureClassStaleRanking, "stale_ranking"},
		{types.FailureClassMissingContent, "missing_content"},
		{types.FailureClassScopeMismatch, "scope_mismatch"},
		{types.FailureClassOther, "other"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestMaxContentLength(t *testing.T) {
	if types.MaxContentLength != 500_000 {
		t.Errorf("MaxContentLength = %d, want 500000", types.MaxContentLength)
	}
}

func TestMemoryIDFormat(t *testing.T) {
	// Generate 100 IDs and verify they're all unique and well-formed
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := types.NewMemoryID()
		if seen[id] {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = true
		parts := strings.Split(id, "-")
		if len(parts) != 5 {
			t.Errorf("UUID not in 8-4-4-4-12 format: %q", id)
		}
	}
}

func TestValidateFailureClass(t *testing.T) {
	valid := []string{
		"",
		types.FailureClassVocabularyMismatch,
		types.FailureClassAggregationFailure,
		types.FailureClassStaleRanking,
		types.FailureClassMissingContent,
		types.FailureClassScopeMismatch,
		types.FailureClassOther,
	}
	for _, v := range valid {
		if !types.ValidateFailureClass(v) {
			t.Errorf("expected %q to be valid failure class", v)
		}
	}
	invalid := []string{"unknown_class", "VOCABULARY_MISMATCH", "Vocabulary_Mismatch"}
	for _, v := range invalid {
		if types.ValidateFailureClass(v) {
			t.Errorf("expected %q to be invalid failure class", v)
		}
	}
}
