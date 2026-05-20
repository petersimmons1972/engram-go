package mcp

import (
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

func TestParseDateTag_Valid(t *testing.T) {
	tags := []string{"lme", "sid:abc123", "date:2023-06-15"}
	got := types.ParseDateTag(tags)
	if got == nil {
		t.Fatal("parseDateTag returned nil, want 2023-06-15")
	}
	want := time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseDateTag = %v, want %v", *got, want)
	}
}

func TestParseDateTag_Invalid(t *testing.T) {
	tags := []string{"lme", "date:not-a-date"}
	got := types.ParseDateTag(tags)
	if got != nil {
		t.Errorf("types.ParseDateTag(%q) = %v, want nil for invalid date", tags, got)
	}
}

func TestParseDateTag_NonePresent(t *testing.T) {
	tags := []string{"lme", "sid:abc", "project:foo"}
	got := types.ParseDateTag(tags)
	if got != nil {
		t.Errorf("parseDateTag with no date: tag = %v, want nil", got)
	}
}

func TestParseDateTag_EmptyTags(t *testing.T) {
	got := types.ParseDateTag(nil)
	if got != nil {
		t.Errorf("types.ParseDateTag(nil) = %v, want nil", got)
	}
}

func TestParseDateTag_FirstWins(t *testing.T) {
	tags := []string{"date:2022-01-01", "date:2024-12-31"}
	got := types.ParseDateTag(tags)
	if got == nil {
		t.Fatal("parseDateTag returned nil")
	}
	want := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseDateTag first tag wins: got %v, want %v", *got, want)
	}
}

// TestParseDateTag_LongMemEvalFormat verifies the LongMemEval haystack date
// format "2006/01/02 (Mon) 15:04" is parsed correctly. This is the format
// written by cmd/longmemeval/ingest.go from item.HaystackDates.
func TestParseDateTag_LongMemEvalFormat(t *testing.T) {
	cases := []struct {
		tag  string
		want time.Time
	}{
		{"date:2023/05/20 (Sat) 00:04", time.Date(2023, 5, 20, 0, 0, 0, 0, time.UTC)},
		{"date:2023/05/30 (Tue) 22:37", time.Date(2023, 5, 30, 0, 0, 0, 0, time.UTC)},
		{"date:2024/01/01 (Mon) 09:00", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		got := types.ParseDateTag([]string{tc.tag})
		if got == nil {
			t.Errorf("types.ParseDateTag(%q) = nil, want %v", tc.tag, tc.want)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("types.ParseDateTag(%q) = %v, want %v", tc.tag, *got, tc.want)
		}
	}
}
