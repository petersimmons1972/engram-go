// Package minhash provides MinHash signature computation for near-duplicate
// detection via Jaccard similarity estimation.
package minhash

import (
	"hash/fnv"
	"math"
	"math/rand"
)

// NumHashes is the number of hash functions in a MinHash signature.
const NumHashes = 128

// Signature is a MinHash signature — the minimum hash value per hash function.
type Signature [NumHashes]uint64

// prime is a large Mersenne prime used as the hash modulus (2^61 - 1).
const prime = (1 << 61) - 1

// Hasher computes MinHash signatures from string content using character bigrams.
type Hasher struct {
	a [NumHashes]uint64
	b [NumHashes]uint64
}

// NewHasher creates a Hasher with deterministic coefficients from seed.
func NewHasher(seed int64) *Hasher {
	rng := rand.New(rand.NewSource(seed))
	var h Hasher
	for i := range NumHashes {
		h.a[i] = rng.Uint64()%prime + 1 // a must be non-zero
		h.b[i] = rng.Uint64() % prime
	}
	return &h
}

// Signature computes the MinHash signature for content using rune-based
// character bigrams. An empty string returns a signature with all slots
// set to math.MaxUint64.
func (h *Hasher) Signature(content string) Signature {
	var sig Signature
	for i := range sig {
		sig[i] = math.MaxUint64
	}

	runes := []rune(content)
	if len(runes) < 2 {
		return sig
	}

	for i := 0; i+1 < len(runes); i++ {
		// Hash the bigram to a uint64.
		bg := bigramHash(runes[i], runes[i+1])

		// For each hash function, compute h_i(bg) = (a_i * bg + b_i) mod p
		// and keep the minimum.
		for j := range NumHashes {
			val := (h.a[j]*bg + h.b[j]) % prime
			if val < sig[j] {
				sig[j] = val
			}
		}
	}
	return sig
}

// bigramHash hashes a rune pair to a uint64 using FNV-1a.
func bigramHash(a, b rune) uint64 {
	h := fnv.New64a()
	buf := [8]byte{
		byte(a), byte(a >> 8), byte(a >> 16), byte(a >> 24),
		byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24),
	}
	h.Write(buf[:])
	return h.Sum64()
}

// EstimatedJaccard returns the estimated Jaccard similarity from two signatures.
// The estimate is the fraction of hash slots where both signatures agree.
func EstimatedJaccard(a, b Signature) float64 {
	matches := 0
	for i := range NumHashes {
		if a[i] == b[i] {
			matches++
		}
	}
	return float64(matches) / float64(NumHashes)
}
