package embedmodel

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

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

// TestCanonicalName_UnknownEmitsWarn verifies that an unrecognised alias
// produces a WARN-level log entry containing the alias, ensuring the
// audit-trail gap is observable.
func TestCanonicalName_UnknownEmitsWarn(t *testing.T) {
	var buf bytes.Buffer
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

	const unknownAlias = "nomic-embed-text"
	got := CanonicalName(unknownAlias)
	if got != "" {
		t.Fatalf("CanonicalName(unknown) = %q, want empty string", got)
	}

	logged := buf.String()
	if !strings.Contains(logged, "WARN") {
		t.Fatalf("expected WARN log for unknown alias, got: %q", logged)
	}
	if !strings.Contains(logged, unknownAlias) {
		t.Fatalf("expected log to contain alias %q, got: %q", unknownAlias, logged)
	}
}
