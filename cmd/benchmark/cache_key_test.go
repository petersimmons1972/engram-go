package main

import (
	"testing"
)

func TestCacheKeyFor_Deterministic(t *testing.T) {
	k1 := cacheKeyFor("base", "v0.1.0", "mistral:7b")
	k2 := cacheKeyFor("base", "v0.1.0", "mistral:7b")
	if k1 != k2 {
		t.Errorf("cacheKeyFor is not deterministic: %q != %q", k1, k2)
	}
}

func TestCacheKeyFor_SensitiveToBase(t *testing.T) {
	k1 := cacheKeyFor("base-a", "v0.1.0", "mistral:7b")
	k2 := cacheKeyFor("base-b", "v0.1.0", "mistral:7b")
	if k1 == k2 {
		t.Errorf("cacheKeyFor should differ when base changes")
	}
}

func TestCacheKeyFor_SensitiveToOllamaVersion(t *testing.T) {
	k1 := cacheKeyFor("base", "v0.1.0", "mistral:7b")
	k2 := cacheKeyFor("base", "v0.2.0", "mistral:7b")
	if k1 == k2 {
		t.Errorf("cacheKeyFor should differ when ollamaVersion changes")
	}
}

func TestCacheKeyFor_SensitiveToModelName(t *testing.T) {
	k1 := cacheKeyFor("base", "v0.1.0", "mistral:7b")
	k2 := cacheKeyFor("base", "v0.1.0", "llama3:8b")
	if k1 == k2 {
		t.Errorf("cacheKeyFor should differ when modelName changes")
	}
}

func TestCacheKeyFor_ReturnsNonEmptyString(t *testing.T) {
	k := cacheKeyFor("", "", "")
	if k == "" {
		t.Errorf("cacheKeyFor must return non-empty string even for empty inputs")
	}
}
