package embedmodel

import "testing"

func TestLiveAndReembedAliasesDistinct(t *testing.T) {
	if !IsLiveAlias("bge-m3-live") {
		t.Fatal("expected bge-m3-live to be classified as the live alias")
	}
	if IsLiveAlias("bge-m3-reembed") {
		t.Fatal("bge-m3-reembed must not be classified as the live alias")
	}
	if !IsReembedAlias("bge-m3-reembed") {
		t.Fatal("expected bge-m3-reembed to be classified as the reembed alias")
	}
	if IsReembedAlias("bge-m3-live") {
		t.Fatal("bge-m3-live must not be classified as the reembed alias")
	}
	if got := CanonicalName("bge-m3-live"); got != CanonicalBGEM3 {
		t.Fatalf("CanonicalName(bge-m3-live) = %q, want %q", got, CanonicalBGEM3)
	}
	if got := CanonicalName("bge-m3-reembed"); got != CanonicalBGEM3 {
		t.Fatalf("CanonicalName(bge-m3-reembed) = %q, want %q", got, CanonicalBGEM3)
	}
}
