package mcp

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/embed"
)

// TestSuggestedModelsEnrichment verifies the installed-flag detection logic.
func TestSuggestedModelsEnrichment(t *testing.T) {
	installed := map[string]bool{
		"nomic-embed-text:latest": true,
	}
	for _, spec := range embed.SuggestedModels {
		isInstalled := installed[spec.Name] || installed[spec.Name+":latest"]
		if spec.Name == "nomic-embed-text" && !isInstalled {
			t.Errorf("nomic-embed-text should be detected as installed")
		}
		if spec.Name == "mxbai-embed-large" && isInstalled {
			t.Errorf("mxbai-embed-large should not be detected as installed")
		}
	}
}

func TestCosineSim32(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.9, 0.1, 0.0}
	c := []float32{0.0, 1.0, 0.0}
	query := []float32{1.0, 0.0, 0.0}

	simAQ := cosineSim32(query, a)
	simBQ := cosineSim32(query, b)
	simCQ := cosineSim32(query, c)

	if simAQ < simBQ {
		t.Errorf("a should be more similar to query than b: simA=%.4f simB=%.4f", simAQ, simBQ)
	}
	if simBQ < simCQ {
		t.Errorf("b should be more similar to query than c: simB=%.4f simC=%.4f", simBQ, simCQ)
	}
}

func TestCosineSim32ZeroMagnitude(t *testing.T) {
	zero := []float32{0.0, 0.0, 0.0}
	a := []float32{1.0, 0.0, 0.0}
	if got := cosineSim32(zero, a); got != 0 {
		t.Errorf("cosineSim32(zero, a) = %v, want 0", got)
	}
	if got := cosineSim32(a, zero); got != 0 {
		t.Errorf("cosineSim32(a, zero) = %v, want 0", got)
	}
}

func TestCosineSim32LengthMismatch(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	if got := cosineSim32(a, b); got != 0 {
		t.Errorf("cosineSim32 length mismatch = %v, want 0", got)
	}
}
