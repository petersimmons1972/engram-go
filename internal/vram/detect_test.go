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
