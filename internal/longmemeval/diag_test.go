package longmemeval_test

// TDD suite for H-DIAG gold visibility diagnostic fields.
// These tests define the contract for ComputeDiag before implementation.

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// ---------------------------------------------------------------------------
// ComputeDiag
// ---------------------------------------------------------------------------

// TestComputeDiag_GoldInTopK_Hit: gold session appears within context window.
func TestComputeDiag_GoldInTopK_Hit(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sess-noise",
		"mem-2": "sess-gold",
		"mem-3": "sess-noise2",
	}
	retrieved := []string{"mem-1", "mem-2", "mem-3"}
	gold := []string{"sess-gold"}
	// contextTopK=3 → all 3 in context, gold at rank 2
	d := longmemeval.ComputeDiag(retrieved, memoryMap, gold, 3)
	if !d.GoldVisibleInContext {
		t.Error("GoldVisibleInContext: expected true (gold in top 3)")
	}
	if d.RetrievedGoldRank != 2 {
		t.Errorf("RetrievedGoldRank: got %d, want 2", d.RetrievedGoldRank)
	}
}

// TestComputeDiag_GoldBeyondContext: gold is retrieved but outside context window.
func TestComputeDiag_GoldBeyondContext(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sess-noise",
		"mem-2": "sess-noise2",
		"mem-3": "sess-gold",
	}
	retrieved := []string{"mem-1", "mem-2", "mem-3"}
	gold := []string{"sess-gold"}
	// contextTopK=2 → only first 2 in context, gold at rank 3 is outside
	d := longmemeval.ComputeDiag(retrieved, memoryMap, gold, 2)
	if d.GoldVisibleInContext {
		t.Error("GoldVisibleInContext: expected false (gold at rank 3 > contextTopK 2)")
	}
	if d.RetrievedGoldRank != 3 {
		t.Errorf("RetrievedGoldRank: got %d, want 3", d.RetrievedGoldRank)
	}
}

// TestComputeDiag_GoldNotRetrieved: gold not in retrieved list at all.
func TestComputeDiag_GoldNotRetrieved(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sess-noise",
		"mem-2": "sess-noise2",
	}
	retrieved := []string{"mem-1", "mem-2"}
	gold := []string{"sess-gold"}
	d := longmemeval.ComputeDiag(retrieved, memoryMap, gold, 10)
	if d.GoldVisibleInContext {
		t.Error("GoldVisibleInContext: expected false (gold not retrieved)")
	}
	if d.RetrievedGoldRank != 0 {
		t.Errorf("RetrievedGoldRank: got %d, want 0 (not found)", d.RetrievedGoldRank)
	}
}

// TestComputeDiag_SessionDominance_Single: all retrieved from one session.
func TestComputeDiag_SessionDominance_Single(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sess-a",
		"mem-2": "sess-a",
		"mem-3": "sess-a",
		"mem-4": "sess-a",
	}
	retrieved := []string{"mem-1", "mem-2", "mem-3", "mem-4"}
	d := longmemeval.ComputeDiag(retrieved, memoryMap, []string{"sess-a"}, 4)
	if d.SessionDominanceRatio != 1.0 {
		t.Errorf("SessionDominanceRatio: got %f, want 1.0", d.SessionDominanceRatio)
	}
}

// TestComputeDiag_SessionDominance_Even: equal distribution across sessions.
func TestComputeDiag_SessionDominance_Even(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sess-a",
		"mem-2": "sess-b",
		"mem-3": "sess-c",
		"mem-4": "sess-d",
	}
	retrieved := []string{"mem-1", "mem-2", "mem-3", "mem-4"}
	d := longmemeval.ComputeDiag(retrieved, memoryMap, []string{"sess-a"}, 4)
	if d.SessionDominanceRatio != 0.25 {
		t.Errorf("SessionDominanceRatio: got %f, want 0.25", d.SessionDominanceRatio)
	}
}

// TestComputeDiag_SessionDominance_Mixed: dominant session gets 3/4.
func TestComputeDiag_SessionDominance_Mixed(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sess-a",
		"mem-2": "sess-a",
		"mem-3": "sess-a",
		"mem-4": "sess-b",
	}
	retrieved := []string{"mem-1", "mem-2", "mem-3", "mem-4"}
	d := longmemeval.ComputeDiag(retrieved, memoryMap, []string{"sess-a"}, 4)
	want := 0.75
	if d.SessionDominanceRatio != want {
		t.Errorf("SessionDominanceRatio: got %f, want %f", d.SessionDominanceRatio, want)
	}
}

// TestComputeDiag_Empty: nil/empty inputs return zero-value DiagFields.
func TestComputeDiag_Empty(t *testing.T) {
	d := longmemeval.ComputeDiag(nil, nil, nil, 15)
	if d.GoldVisibleInContext {
		t.Error("empty: GoldVisibleInContext should be false")
	}
	if d.RetrievedGoldRank != 0 {
		t.Error("empty: RetrievedGoldRank should be 0")
	}
	if d.SessionDominanceRatio != 0 {
		t.Error("empty: SessionDominanceRatio should be 0")
	}
}

// TestComputeDiag_ContextTopKZero: zero contextTopK means unlimited (all retrieved).
func TestComputeDiag_ContextTopKZero(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sess-noise",
		"mem-2": "sess-gold",
	}
	retrieved := []string{"mem-1", "mem-2"}
	gold := []string{"sess-gold"}
	// contextTopK=0 means unlimited — gold at rank 2 is visible
	d := longmemeval.ComputeDiag(retrieved, memoryMap, gold, 0)
	if !d.GoldVisibleInContext {
		t.Error("contextTopK=0: GoldVisibleInContext should be true (unlimited context)")
	}
}

// TestComputeDiag_SessionDominance_OnlyContextWindow: dominance uses full retrieved list.
func TestComputeDiag_SessionDominance_FullList(t *testing.T) {
	// Dominance computed over all retrieved IDs (not just context window),
	// to capture the diversity of the recall output.
	memoryMap := map[string]string{
		"mem-1": "sess-a",
		"mem-2": "sess-a",
		"mem-3": "sess-b", // outside context window (contextTopK=2)
	}
	retrieved := []string{"mem-1", "mem-2", "mem-3"}
	d := longmemeval.ComputeDiag(retrieved, memoryMap, []string{"sess-a"}, 2)
	// All 3 retrieved: 2 from sess-a (2/3 ≈ 0.667), 1 from sess-b
	want := 2.0 / 3.0
	if d.SessionDominanceRatio < want-0.001 || d.SessionDominanceRatio > want+0.001 {
		t.Errorf("SessionDominanceRatio: got %f, want %f", d.SessionDominanceRatio, want)
	}
}
