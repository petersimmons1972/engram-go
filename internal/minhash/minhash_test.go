package minhash_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/minhash"
	"github.com/stretchr/testify/require"
)

func TestSignature_IdenticalStrings(t *testing.T) {
	h := minhash.NewHasher(42)
	sig1 := h.Signature("the quick brown fox jumps over the lazy dog")
	sig2 := h.Signature("the quick brown fox jumps over the lazy dog")
	require.Equal(t, sig1, sig2)
	require.InDelta(t, 1.0, minhash.EstimatedJaccard(sig1, sig2), 0.001)
}

func TestSignature_CompletelyDifferent(t *testing.T) {
	h := minhash.NewHasher(42)
	sig1 := h.Signature("aaaaaaaaaa bbbbbbbbbb cccccccccc")
	sig2 := h.Signature("xxxxxxxxxx yyyyyyyyyy zzzzzzzzzz")
	est := minhash.EstimatedJaccard(sig1, sig2)
	require.Less(t, est, 0.15, "completely different strings should have near-zero Jaccard")
}

func TestSignature_Deterministic(t *testing.T) {
	h1 := minhash.NewHasher(42)
	h2 := minhash.NewHasher(42)
	sig1 := h1.Signature("test content here")
	sig2 := h2.Signature("test content here")
	require.Equal(t, sig1, sig2, "same seed + same input must produce same signature")
}

func TestSignature_DifferentSeeds(t *testing.T) {
	h1 := minhash.NewHasher(42)
	h2 := minhash.NewHasher(99)
	sig1 := h1.Signature("test content here")
	sig2 := h2.Signature("test content here")
	require.NotEqual(t, sig1, sig2, "different seeds should produce different signatures")
}

func TestSignature_NearDuplicate(t *testing.T) {
	h := minhash.NewHasher(42)
	base := "kubernetes deployment patterns for production workloads with high availability"
	sig1 := h.Signature(base)
	sig2 := h.Signature(base + " and disaster recovery")
	est := minhash.EstimatedJaccard(sig1, sig2)
	require.Greater(t, est, 0.5, "near-duplicate should have moderate-to-high Jaccard")
}

func TestSignature_EmptyString(t *testing.T) {
	h := minhash.NewHasher(42)
	sig := h.Signature("")
	// Empty string has no bigrams; all signature slots stay at max.
	est := minhash.EstimatedJaccard(sig, sig)
	require.InDelta(t, 1.0, est, 0.001, "empty signature compared to itself is 1.0")
}

func TestSignature_UTF8(t *testing.T) {
	h := minhash.NewHasher(42)
	sig1 := h.Signature("日本語テスト")
	sig2 := h.Signature("日本語テスト")
	require.Equal(t, sig1, sig2, "UTF-8 strings must produce identical signatures")
}
