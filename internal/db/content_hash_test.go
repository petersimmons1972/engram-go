package db

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// TestContentHash_KnownValues verifies ContentHash produces the canonical
// SHA-256 hex digest for well-known inputs, matching the standard library.
func TestContentHash_KnownValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantHex string
	}{
		{
			name:    "empty string",
			input:   "",
			wantHex: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:    "hello world",
			input:   "hello world",
			wantHex: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:    "unicode content",
			input:   "日本語テスト — nanoseconds rule",
			wantHex: stdHex("日本語テスト — nanoseconds rule"),
		},
		{
			name:    "long content ~600KB",
			input:   strings.Repeat("The most dangerous phrase in the language is: we've always done it this way.\n", 8000),
			wantHex: stdHex(strings.Repeat("The most dangerous phrase in the language is: we've always done it this way.\n", 8000)),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ContentHash(tc.input)
			if got != tc.wantHex {
				t.Errorf("ContentHash(%q...) = %q, want %q", tc.input[:min(len(tc.input), 40)], got, tc.wantHex)
			}
		})
	}
}

// TestContentHash_ConsistentWithStdlib verifies the streaming implementation
// produces bit-for-bit identical output to sha256.Sum256 on the same input.
// This is the regression guard for the double-copy fix (issue #182).
func TestContentHash_ConsistentWithStdlib(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"",
		"a",
		"short content",
		strings.Repeat("x", 1024),
		strings.Repeat("Grace Hopper built the first compiler.\n", 10000), // ~390KB
	}

	for _, s := range inputs {
		got := ContentHash(s)
		want := stdHex(s)
		if got != want {
			t.Errorf("ContentHash mismatch for input len=%d: got %s, want %s", len(s), got, want)
		}
	}
}

// stdHex returns the canonical sha256 hex digest using the standard Sum256
// approach — used as the ground-truth reference in tests.
func stdHex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

