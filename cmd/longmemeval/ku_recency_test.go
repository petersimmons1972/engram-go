package main

import (
	"os"
	"strings"
	"testing"
)

// TestKURecencyPrompt_FlagRegistered verifies that the --ku-recency-prompt
// flag is registered in the dispatch function and backed by a Config field.
// H-KUR, issue #1178.
func TestKURecencyPrompt_FlagRegistered(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "ku-recency-prompt") {
		t.Error("main.go missing 'ku-recency-prompt' flag registration — H-KUR harness wiring broken")
	}
	if !strings.Contains(text, "KURecencyPrompt") {
		t.Error("main.go missing KURecencyPrompt config field — H-KUR harness wiring broken")
	}
}

// TestKURecencyPrompt_RunGoWiresPromptSelection verifies that run.go contains
// the ku-recency-prompt prompt selection path.
func TestKURecencyPrompt_RunGoWiresPromptSelection(t *testing.T) {
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "cfg.KURecencyPrompt") {
		t.Error("run.go missing cfg.KURecencyPrompt check — H-KUR flag not applied during prompt selection")
	}
	if !strings.Contains(text, "GenerationPromptForTypeKURecency") {
		t.Error("run.go missing GenerationPromptForTypeKURecency call — H-KUR prompt variant not wired")
	}
}
