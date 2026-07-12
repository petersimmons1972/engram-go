package main

import (
	"os"
	"strings"
	"testing"
)

// TestPreferenceGround_FlagRegistered verifies that the --preference-ground flag
// is registered in the dispatch function and backed by a Config field.
func TestPreferenceGround_FlagRegistered(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "preference-ground") {
		t.Error("main.go missing 'preference-ground' flag registration — H-PG harness wiring broken")
	}
	if !strings.Contains(text, "PreferenceGround") {
		t.Error("main.go missing PreferenceGround config field — H-PG harness wiring broken")
	}
}

// TestPreferenceGround_RunGoWiresPromptSelection verifies that run.go contains
// the preference-ground prompt selection path.
func TestPreferenceGround_RunGoWiresPromptSelection(t *testing.T) {
	// Prompt selection moved into the shared selectGenerationPrompt helper
	// (generation_prompt.go, #1402); read both files so the check is location-robust.
	var text string
	for _, f := range []string{"run.go", "generation_prompt.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		text += string(src)
	}
	if !strings.Contains(text, "cfg.PreferenceGround") {
		t.Error("prompt selection missing cfg.PreferenceGround check — H-PG flag not applied during prompt selection")
	}
	if !strings.Contains(text, "GenerationPromptForTypePreferenceGround") {
		t.Error("prompt selection missing GenerationPromptForTypePreferenceGround call — H-PG prompt variant not wired")
	}
}
