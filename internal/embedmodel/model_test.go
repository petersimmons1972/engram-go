package embedmodel

import "testing"

func TestCanonicalName_AllAliasesNormalize(t *testing.T) {
	for _, alias := range AcceptedAliases {
		if got := CanonicalName(alias); got != CanonicalBGEM3 {
			t.Fatalf("CanonicalName(%q) = %q, want %q", alias, got, CanonicalBGEM3)
		}
	}
}

func TestCanonicalName_UnknownReturnsEmpty(t *testing.T) {
	if got := CanonicalName("nomic-embed-text"); got != "" {
		t.Fatalf("CanonicalName(unknown) = %q, want empty string", got)
	}
}
