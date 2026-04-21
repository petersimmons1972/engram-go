package embed_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/embed"
)

func TestSuggestedModelsNotEmpty(t *testing.T) {
	if len(embed.SuggestedModels) == 0 {
		t.Fatal("SuggestedModels must not be empty")
	}
}

func TestSuggestedModelsHaveRequiredFields(t *testing.T) {
	for _, m := range embed.SuggestedModels {
		if m.Name == "" {
			t.Errorf("ModelSpec has empty Name: %+v", m)
		}
		if m.Dimensions <= 0 {
			t.Errorf("ModelSpec %q has non-positive Dimensions: %d", m.Name, m.Dimensions)
		}
		if m.SizeMB <= 0 {
			t.Errorf("ModelSpec %q has non-positive SizeMB: %d", m.Name, m.SizeMB)
		}
		if m.Description == "" {
			t.Errorf("ModelSpec %q has empty Description", m.Name)
		}
	}
}

func TestSuggestedModelsHasExactlyOneRecommended(t *testing.T) {
	count := 0
	for _, m := range embed.SuggestedModels {
		if m.Recommended {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 recommended model, got %d", count)
	}
}

func TestDefaultRecommendedModel(t *testing.T) {
	rec := embed.DefaultRecommendedModel()
	if rec == nil {
		t.Fatal("DefaultRecommendedModel returned nil")
	}
	if rec.Name != "mxbai-embed-large" {
		t.Errorf("expected mxbai-embed-large as recommended, got %q", rec.Name)
	}
}
