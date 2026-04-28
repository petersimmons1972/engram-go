package mcp

import (
	"os"
	"testing"
)

// TestRecallMaxTopKDefault verifies the default maximum top_k is 500 when
// ENGRAM_RECALL_MAX_TOP_K is not set.
func TestRecallMaxTopKDefault(t *testing.T) {
	os.Unsetenv("ENGRAM_RECALL_MAX_TOP_K")
	if got := recallMaxTopK(); got != 500 {
		t.Errorf("recallMaxTopK() = %d, want 500", got)
	}
}

// TestRecallMaxTopKFromEnv verifies ENGRAM_RECALL_MAX_TOP_K overrides the default.
func TestRecallMaxTopKFromEnv(t *testing.T) {
	t.Setenv("ENGRAM_RECALL_MAX_TOP_K", "200")
	if got := recallMaxTopK(); got != 200 {
		t.Errorf("recallMaxTopK() = %d, want 200", got)
	}
}

// TestRecallMaxTopKEnvInvalid verifies an unparseable env value falls back to
// the default rather than crashing.
func TestRecallMaxTopKEnvInvalid(t *testing.T) {
	t.Setenv("ENGRAM_RECALL_MAX_TOP_K", "not-a-number")
	if got := recallMaxTopK(); got != 500 {
		t.Errorf("recallMaxTopK() = %d, want 500 for invalid env", got)
	}
}

// TestClampTopK verifies the clamping rules:
// - topK < 1 → 10 (default)
// - topK > max → capped at max (not reset to 10)
// - 1 ≤ topK ≤ max → unchanged
func TestClampTopK(t *testing.T) {
	const max = 500
	cases := []struct {
		name  string
		input int
		want  int
	}{
		{"zero resets to default", 0, 10},
		{"negative resets to default", -5, 10},
		{"one is valid", 1, 1},
		{"ten is valid", 10, 10},
		{"max is valid", max, max},
		{"above max is capped at max", max + 1, max},
		{"large value is capped at max", 99999, max},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := clampTopK(tc.input, max)
			if got != tc.want {
				t.Errorf("clampTopK(%d, %d) = %d, want %d", tc.input, max, got, tc.want)
			}
		})
	}
}
