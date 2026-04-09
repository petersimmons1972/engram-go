// Package embed provides embedding providers and vector math utilities.
// This file implements low-level vector operations that must match the Python
// reference implementation byte-for-byte so that embeddings stored by one
// runtime can be read by the other during the migration period.
package embed

import (
	"encoding/binary"
	"math"
)

// CosineSimilarity returns the cosine similarity between two float32 vectors.
// Returns 0 when either vector has zero magnitude.
// Mirrors Python numpy cosine_similarity() used in search.py.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dot, magA, magB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		magA += ai * ai
		magB += bi * bi
	}

	magA = math.Sqrt(magA)
	magB = math.Sqrt(magB)
	if magA == 0 || magB == 0 {
		return 0.0
	}
	return dot / (magA * magB)
}

// ToBlob serializes a float32 slice to a little-endian byte slice.
// Must be byte-for-byte identical to Python:
//
//	import struct; struct.pack(f"{len(v)}f", *v)
//
// Python's struct.pack uses native float (C float = IEEE 754 single = float32)
// in little-endian order on x86/x64 (the default native byte order).
// We use binary.LittleEndian explicitly for portability.
func ToBlob(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		bits := math.Float32bits(f)
		binary.LittleEndian.PutUint32(buf[i*4:], bits)
	}
	return buf
}

// FromBlob deserializes a little-endian byte slice to a float32 slice.
// Inverse of ToBlob. Returns nil for an empty or malformed input.
func FromBlob(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		v[i] = math.Float32frombits(bits)
	}
	return v
}
