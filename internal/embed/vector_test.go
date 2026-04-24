package embed_test

import (
	"math"
	"testing"

	"github.com/petersimmons1972/engram/internal/embed"
)

func TestCosineSimilarityIdentical(t *testing.T) {
	v := []float32{1.0, 2.0, 3.0}
	got := embed.CosineSimilarity(v, v)
	if math.Abs(got-1.0) > 1e-6 {
		t.Errorf("CosineSimilarity(v,v) = %f, want 1.0", got)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	got := embed.CosineSimilarity(a, b)
	if math.Abs(got) > 1e-6 {
		t.Errorf("CosineSimilarity(orthogonal) = %f, want 0.0", got)
	}
}

func TestCosineSimilarityOpposite(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{-1.0, 0.0}
	got := embed.CosineSimilarity(a, b)
	if math.Abs(got+1.0) > 1e-6 {
		t.Errorf("CosineSimilarity(opposite) = %f, want -1.0", got)
	}
}

func TestCosineSimilarityZeroVector(t *testing.T) {
	a := []float32{0.0, 0.0}
	b := []float32{1.0, 2.0}
	if got := embed.CosineSimilarity(a, b); got != 0.0 {
		t.Errorf("zero vector cosine = %f, want 0.0", got)
	}
	if got := embed.CosineSimilarity(b, a); got != 0.0 {
		t.Errorf("zero vector cosine (reversed) = %f, want 0.0", got)
	}
}

func TestCosineSimilarityLengthMismatch(t *testing.T) {
	a := []float32{1.0, 2.0}
	b := []float32{1.0}
	if got := embed.CosineSimilarity(a, b); got != 0.0 {
		t.Errorf("mismatched lengths cosine = %f, want 0.0", got)
	}
}

func TestCosineSimilarityKnownValue(t *testing.T) {
	// [1,1] vs [1,0]: cos(45°) = 1/√2 ≈ 0.7071
	a := []float32{1.0, 1.0}
	b := []float32{1.0, 0.0}
	want := 1.0 / math.Sqrt2
	got := embed.CosineSimilarity(a, b)
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("CosineSimilarity = %f, want %f", got, want)
	}
}

// TestToBlobFromBlobRoundTrip verifies that ToBlob→FromBlob is lossless.
func TestToBlobFromBlobRoundTrip(t *testing.T) {
	cases := [][]float32{
		{1.0, 2.0, 3.0},
		{0.0},
		{-1.5, 0.0, 1.5, math.MaxFloat32},
		{},
	}
	for _, input := range cases {
		blob := embed.ToBlob(input)
		got := embed.FromBlob(blob)

		if len(input) == 0 {
			// Empty input → nil blob → nil output
			if got != nil {
				t.Errorf("ToBlob/FromBlob([]) = %v, want nil", got)
			}
			continue
		}

		if len(got) != len(input) {
			t.Errorf("FromBlob length %d, want %d", len(got), len(input))
			continue
		}
		for i := range input {
			if got[i] != input[i] {
				t.Errorf("FromBlob[%d] = %v, want %v", i, got[i], input[i])
			}
		}
	}
}

// TestToBlobByteOrder verifies little-endian encoding.
// 1.0 in IEEE 754 single precision = 0x3F800000 = bytes [0x00, 0x00, 0x80, 0x3F] (LE).
func TestToBlobByteOrder(t *testing.T) {
	blob := embed.ToBlob([]float32{1.0})
	want := []byte{0x00, 0x00, 0x80, 0x3F}
	if len(blob) != 4 {
		t.Fatalf("ToBlob([1.0]) length = %d, want 4", len(blob))
	}
	for i, b := range want {
		if blob[i] != b {
			t.Errorf("byte[%d] = 0x%02X, want 0x%02X", i, blob[i], b)
		}
	}
}

// TestFromBlobMalformed verifies nil return for malformed input.
func TestFromBlobMalformed(t *testing.T) {
	if got := embed.FromBlob([]byte{0x01, 0x02, 0x03}); got != nil {
		t.Errorf("FromBlob(3-byte input) = %v, want nil", got)
	}
	if got := embed.FromBlob(nil); got != nil {
		t.Errorf("FromBlob(nil) = %v, want nil", got)
	}
}

// TestPythonCompatibility verifies byte-level compatibility with Python struct.pack.
// Python: struct.pack("3f", 1.0, -0.5, 0.25)
// Result: b'\x00\x00\x80?\x00\x00\x00\xbf\x00\x00\x80>'.
func TestPythonCompatibility(t *testing.T) {
	input := []float32{1.0, -0.5, 0.25}
	blob := embed.ToBlob(input)

	// Python-verified bytes for [1.0, -0.5, 0.25] in little-endian float32
	want := []byte{
		0x00, 0x00, 0x80, 0x3F, // 1.0
		0x00, 0x00, 0x00, 0xBF, // -0.5
		0x00, 0x00, 0x80, 0x3E, // 0.25
	}

	if len(blob) != len(want) {
		t.Fatalf("blob length = %d, want %d", len(blob), len(want))
	}
	for i, b := range want {
		if blob[i] != b {
			t.Errorf("byte[%d] = 0x%02X, want 0x%02X", i, blob[i], b)
		}
	}
}
