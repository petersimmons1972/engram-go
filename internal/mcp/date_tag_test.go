package mcp

import (
	"testing"
	"time"
)

func TestParseDateTag_Valid(t *testing.T) {
	tags := []string{"lme", "sid:abc123", "date:2023-06-15"}
	got := parseDateTag(tags)
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
	got := parseDateTag(tags)
	if got != nil {
		t.Errorf("parseDateTag(%q) = %v, want nil for invalid date", tags, got)
	}
}

func TestParseDateTag_NonePresent(t *testing.T) {
	tags := []string{"lme", "sid:abc", "project:foo"}
	got := parseDateTag(tags)
	if got != nil {
		t.Errorf("parseDateTag with no date: tag = %v, want nil", got)
	}
}

func TestParseDateTag_EmptyTags(t *testing.T) {
	got := parseDateTag(nil)
	if got != nil {
		t.Errorf("parseDateTag(nil) = %v, want nil", got)
	}
}

func TestParseDateTag_FirstWins(t *testing.T) {
	tags := []string{"date:2022-01-01", "date:2024-12-31"}
	got := parseDateTag(tags)
	if got == nil {
		t.Fatal("parseDateTag returned nil")
	}
	want := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseDateTag first tag wins: got %v, want %v", *got, want)
	}
}
