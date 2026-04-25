package vram_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/vram"
)

func TestParseNvidiaMB(t *testing.T) {
	cases := []struct {
		input  string
		wantGB float64
		wantOK bool
	}{
		{"8192 MiB\n", 8.0, true},
		{"16384 MiB\n", 16.0, true},
		{"[Not Supported]\n", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		gotGB, gotOK := vram.ParseNvidiaMiB(c.input)
		if gotOK != c.wantOK || (gotOK && gotGB != c.wantGB) {
			t.Errorf("ParseNvidiaMiB(%q) = (%.1f, %v), want (%.1f, %v)",
				c.input, gotGB, gotOK, c.wantGB, c.wantOK)
		}
	}
}

func TestFallback(t *testing.T) {
	info := vram.Fallback()
	if info.GB != 8.0 {
		t.Errorf("want fallback 8.0 GB, got %.1f", info.GB)
	}
	if info.Source != "fallback" {
		t.Errorf("want source=fallback, got %q", info.Source)
	}
}

func TestDetect_ReturnsFallbackWhenNoGPUToolsAvailable(t *testing.T) {
	// On a CI machine without nvidia-smi, rocm-smi, or Apple sysctl,
	// Detect() must return the 8GB fallback rather than panicking.
	info := vram.Detect()
	if info.GB <= 0 {
		t.Errorf("Detect returned non-positive GB: %.1f", info.GB)
	}
	if info.Source == "" {
		t.Errorf("Detect returned empty Source")
	}
	if info.Label == "" {
		t.Errorf("Detect returned empty Label")
	}
}

func TestDetect_FallbackHasExpectedValues(t *testing.T) {
	// Fallback is the guaranteed floor — test its contract explicitly.
	fb := vram.Fallback()
	if fb.Source != "fallback" {
		t.Errorf("want source=fallback, got %q", fb.Source)
	}
	if fb.GB != 8.0 {
		t.Errorf("want 8.0 GB, got %.1f", fb.GB)
	}
}
