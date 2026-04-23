package longmemeval_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestRecallAny(t *testing.T) {
	retrieved := []string{"sid-a", "sid-b", "sid-c"}
	relevant := map[string]bool{"sid-b": true, "sid-z": true}

	if got := longmemeval.RecallAny(retrieved, relevant, 3); got != 1.0 {
		t.Errorf("RecallAny@3 = %.2f, want 1.0", got)
	}
	if got := longmemeval.RecallAny(retrieved, relevant, 2); got != 1.0 {
		t.Errorf("RecallAny@2 = %.2f, want 1.0", got)
	}
	if got := longmemeval.RecallAny(retrieved, relevant, 1); got != 0.0 {
		t.Errorf("RecallAny@1 = %.2f, want 0.0", got)
	}
}

func TestRecallAll(t *testing.T) {
	retrieved := []string{"sid-a", "sid-b", "sid-c", "sid-d"}
	relevant := map[string]bool{"sid-b": true, "sid-c": true}

	if got := longmemeval.RecallAll(retrieved, relevant, 4); got != 1.0 {
		t.Errorf("RecallAll@4 = %.2f, want 1.0", got)
	}
	if got := longmemeval.RecallAll(retrieved, relevant, 2); got != 0.0 {
		t.Errorf("RecallAll@2 = %.2f, want 0.0", got)
	}
}

func TestRecallAny_Empty(t *testing.T) {
	if got := longmemeval.RecallAny(nil, map[string]bool{"x": true}, 5); got != 0.0 {
		t.Errorf("RecallAny with nil retrieved = %.2f, want 0.0", got)
	}
	if got := longmemeval.RecallAny([]string{"a"}, nil, 5); got != 0.0 {
		t.Errorf("RecallAny with nil relevant = %.2f, want 0.0", got)
	}
}

func TestSessionIDs(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sid-a",
		"mem-2": "sid-b",
		"mem-3": "sid-c",
	}
	retrieved := []string{"mem-2", "mem-3", "mem-1"}
	want := []string{"sid-b", "sid-c", "sid-a"}
	got := longmemeval.SessionIDs(retrieved, memoryMap)
	for i, g := range got {
		if g != want[i] {
			t.Errorf("SessionIDs[%d] = %q, want %q", i, g, want[i])
		}
	}
}
