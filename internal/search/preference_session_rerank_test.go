package search

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
)

func preferenceSessionResult(id, sid, memType, content string, score float64) types.SearchResult {
	tags := []string{}
	if sid != "" {
		tags = append(tags, "sid:"+sid)
	}
	return types.SearchResult{
		Memory: &types.Memory{
			ID:         id,
			Tags:       tags,
			MemoryType: memType,
			Content:    content,
		},
		Score: score,
	}
}

func TestPreferenceSessionRerank_FlagOffNoop(t *testing.T) {
	results := []types.SearchResult{
		preferenceSessionResult("generic-pref", "s1", "preference", "I like hiking.", 0.95),
		preferenceSessionResult("gold-pref", "s2", "preference", "I prefer espresso drinks.", 0.40),
	}

	got := preferenceSessionRerank(results, []string{"espresso"}, false)

	if got[0].Memory.ID != "generic-pref" || got[1].Memory.ID != "gold-pref" {
		t.Fatalf("flag-off rerank changed order: got %q, %q", got[0].Memory.ID, got[1].Memory.ID)
	}
}

func TestPreferenceSessionRerank_OnTopicPreferenceSessionBeatsGenericPreferenceNoise(t *testing.T) {
	results := []types.SearchResult{
		preferenceSessionResult("generic-pref-a", "s-generic-a", "preference", "I love hiking on weekends.", 0.98),
		preferenceSessionResult("generic-pref-b", "s-generic-b", "preference", "I usually prefer window seats.", 0.95),
		preferenceSessionResult("gold-pref", "s-gold", "preference", "I really prefer espresso and dark roast coffee.", 0.42),
		preferenceSessionResult("gold-context", "s-gold", "context", "Coffee chat follow-up notes.", 0.32),
	}

	got := preferenceSessionRerank(results, []string{"coffee", "espresso"}, true)

	if len(got) != len(results) {
		t.Fatalf("rerank length = %d, want %d", len(got), len(results))
	}
	if got[0].Memory.ID != "gold-pref" {
		t.Fatalf("on-topic preference session should rank first, got %q", got[0].Memory.ID)
	}
	if got[1].Memory.ID != "gold-context" {
		t.Fatalf("rerank should keep the winning session packed together, got second %q", got[1].Memory.ID)
	}
}

func TestPreferenceSessionRerank_EmptyAnchorTokensNoop(t *testing.T) {
	results := []types.SearchResult{
		preferenceSessionResult("a", "s1", "preference", "I like tea.", 0.7),
		preferenceSessionResult("b", "s2", "preference", "I like coffee.", 0.6),
	}

	got := preferenceSessionRerank(results, nil, true)

	if got[0].Memory.ID != "a" || got[1].Memory.ID != "b" {
		t.Fatalf("empty-anchor rerank changed order: got %q, %q", got[0].Memory.ID, got[1].Memory.ID)
	}
}

func TestPreferenceSessionRerank_DoesNotPackOffTopicSessions(t *testing.T) {
	results := []types.SearchResult{
		preferenceSessionResult("generic-a-high", "s-generic", "preference", "I like hiking.", 0.98),
		preferenceSessionResult("generic-b", "s-other", "preference", "I like window seats.", 0.95),
		preferenceSessionResult("gold-pref", "s-gold", "preference", "I prefer espresso.", 0.40),
		preferenceSessionResult("generic-a-low", "s-generic", "context", "Hiking follow-up.", 0.10),
	}

	got := preferenceSessionRerank(results, []string{"espresso"}, true)

	want := []string{"gold-pref", "generic-a-high", "generic-b", "generic-a-low"}
	if len(got) != len(want) {
		t.Fatalf("rerank length = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].Memory.ID != id {
			t.Fatalf("rank %d: got %q, want %q", i, got[i].Memory.ID, id)
		}
	}
}
