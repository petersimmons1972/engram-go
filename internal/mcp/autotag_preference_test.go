package mcp

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
)

// TestAutoTagPreference verifies that memory_type is set to "preference" when
// the stored content contains preference signals and no explicit type is given.
func TestAutoTagPreference(t *testing.T) {
	preferenceContent := "I really prefer dark roast coffee over light roast."

	// Simulate args without explicit memory_type.
	args := map[string]any{
		"content": preferenceContent,
	}
	memType := getString(args, "memory_type", "context")
	if _, hasExplicitType := args["memory_type"]; !hasExplicitType {
		if search.IsPreferenceContent(preferenceContent) {
			memType = "preference"
		}
	}
	if memType != "preference" {
		t.Errorf("auto-tag: got memory_type=%q, want %q", memType, "preference")
	}
}

// TestAutoTagPreferenceNotAppliedWhenExplicit verifies that an explicit
// memory_type is never overridden by auto-tagging.
func TestAutoTagPreferenceNotAppliedWhenExplicit(t *testing.T) {
	preferenceContent := "I really prefer dark roast coffee."
	args := map[string]any{
		"content":     preferenceContent,
		"memory_type": "decision", // explicitly provided
	}
	memType := getString(args, "memory_type", "context")
	if _, hasExplicitType := args["memory_type"]; !hasExplicitType {
		if search.IsPreferenceContent(preferenceContent) {
			memType = "preference"
		}
	}
	if memType != "decision" {
		t.Errorf("explicit memory_type overridden by auto-tag: got %q, want %q", memType, "decision")
	}
}

// TestAutoTagPreferenceNotAppliedToNeutralContent verifies that neutral
// content is not auto-tagged as preference.
func TestAutoTagPreferenceNotAppliedToNeutralContent(t *testing.T) {
	neutralContent := "The deployment succeeded. All pods are running."
	args := map[string]any{"content": neutralContent}
	memType := getString(args, "memory_type", "context")
	if _, hasExplicitType := args["memory_type"]; !hasExplicitType {
		if search.IsPreferenceContent(neutralContent) {
			memType = "preference"
		}
	}
	if memType != "context" {
		t.Errorf("neutral content auto-tagged: got %q, want %q", memType, "context")
	}
}
