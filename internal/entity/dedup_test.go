package entity_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/petersimmons1972/engram/internal/entity"
)

func TestDedup_ExactMatch(t *testing.T) {
	existing := []entity.Entity{{ID: "e1", Name: "New York City", Aliases: []string{"NYC"}}}
	candidates := []entity.Entity{{Name: "NYC"}}
	merged, fresh := entity.Deduplicate(existing, candidates)
	assert.Len(t, fresh, 0)
	assert.Len(t, merged, 1)
	assert.Contains(t, merged[0].Aliases, "NYC")
	assert.Equal(t, "e1", merged[0].ID)
}

func TestDedup_NewEntity(t *testing.T) {
	existing := []entity.Entity{}
	candidates := []entity.Entity{{Name: "Amazon"}}
	merged, fresh := entity.Deduplicate(existing, candidates)
	assert.Len(t, fresh, 1)
	assert.Equal(t, "Amazon", fresh[0].Name)
	assert.Len(t, merged, 0)
}

func TestDedup_NormalizationMatch(t *testing.T) {
	existing := []entity.Entity{{ID: "e2", Name: "openai", Aliases: []string{}}}
	candidates := []entity.Entity{{Name: "OpenAI"}}
	merged, fresh := entity.Deduplicate(existing, candidates)
	assert.Len(t, fresh, 0)
	assert.Len(t, merged, 1)
	assert.Equal(t, "e2", merged[0].ID)
}

func TestDedup_AliasMatch(t *testing.T) {
	existing := []entity.Entity{{ID: "e3", Name: "Kubernetes", Aliases: []string{"k8s", "kube"}}}
	candidates := []entity.Entity{{Name: "kube"}}
	merged, fresh := entity.Deduplicate(existing, candidates)
	assert.Len(t, fresh, 0)
	assert.Equal(t, "e3", merged[0].ID)
}

func TestDedup_DuplicateCandidatesBatch(t *testing.T) {
	// Two candidates with same name, both new — must not panic, fresh has 1 entry.
	existing := []entity.Entity{}
	candidates := []entity.Entity{{Name: "Amazon"}, {Name: "amazon"}}
	merged, fresh := entity.Deduplicate(existing, candidates)
	assert.Len(t, merged, 0)
	assert.Len(t, fresh, 1, "duplicate new candidates should collapse to one fresh entry")
}

func TestDedup_EmptyInputs(t *testing.T) {
	merged, fresh := entity.Deduplicate(nil, nil)
	assert.Nil(t, merged)
	assert.Nil(t, fresh)
}

func TestDedup_SliceAliasIsolation(t *testing.T) {
	// Caller's existing aliases slice must not be mutated.
	original := []string{"NYC"}
	existing := []entity.Entity{{ID: "e1", Name: "New York City", Aliases: original}}
	candidates := []entity.Entity{{Name: "Big Apple"}}
	entity.Deduplicate(existing, candidates)
	assert.Equal(t, []string{"NYC"}, original, "caller aliases slice must not be mutated")
}

func TestDedup_MultiCandidateSameExisting(t *testing.T) {
	// Two candidates both matching the same existing entity → merged has exactly 1 entry.
	existing := []entity.Entity{{ID: "e1", Name: "Kubernetes", Aliases: []string{"k8s", "kube"}}}
	candidates := []entity.Entity{{Name: "k8s"}, {Name: "kube"}}
	merged, fresh := entity.Deduplicate(existing, candidates)
	assert.Len(t, fresh, 0)
	assert.Len(t, merged, 1, "same existing entity matched by two candidates must appear once in merged")
}
