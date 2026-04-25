package cache_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/cache"
	"github.com/petersimmons1972/engram/internal/types"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	c := cache.New(path)

	result := types.ModelResult{
		Model:  "mistral:7b",
		VRAMGB: 4.5,
		Tier:   "4-6GB",
		Vendor: "Mistral AI",
		Score: types.Score{
			Verdict:   types.VerdictRecommended,
			Composite: 7.44,
		},
	}
	key := "sha256abc"
	if err := c.Write("mistral:7b", key, result); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, ok, err := c.Read("mistral:7b", key, 24*time.Hour)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Score.Verdict != types.VerdictRecommended {
		t.Errorf("want Recommended, got %s", got.Score.Verdict)
	}
}

func TestRead_Miss_WrongKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	c := cache.New(path)
	result := types.ModelResult{Model: "mistral:7b"}
	if err := c.Write("mistral:7b", "key-a", result); err != nil {
		t.Fatalf("Write: %v", err)
	}

	_, ok, err := c.Read("mistral:7b", "key-b", 24*time.Hour)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if ok {
		t.Error("expected cache miss for wrong key")
	}
}

func TestRead_Miss_Expired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	c := cache.New(path)
	result := types.ModelResult{Model: "mistral:7b"}
	if err := c.Write("mistral:7b", "key-a", result); err != nil {
		t.Fatalf("Write: %v", err)
	}

	_, ok, err := c.Read("mistral:7b", "key-a", -1*time.Second) // expired immediately
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if ok {
		t.Error("expected cache miss for expired entry")
	}
}
