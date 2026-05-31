package search

import "testing"

func TestCanonicalEmbedderName_InfinityVariants(t *testing.T) {
	t.Helper()
	for input, want := range map[string]string{
		"bge-m3:latest":      "BAAI/bge-m3",
		"BAAI/bge-m3:latest": "BAAI/bge-m3",
	} {
		t.Run(input, func(t *testing.T) {
			if got := canonicalEmbedderName(input); got != want {
				t.Fatalf("expected %s, got %s", want, got)
			}
		})
	}
}
