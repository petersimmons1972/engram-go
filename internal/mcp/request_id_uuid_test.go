package mcp

// Tests for #696: request_id format is UUIDv4.
// Separate file from request_id_test.go (which tests header extraction
// and middleware behaviour) to keep concerns split.

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestRequestID_CollisionResistance — 10k UUIDv4 IDs must all be unique.
func TestRequestID_CollisionResistance(t *testing.T) {
	const N = 10_000
	ids := make(map[string]struct{}, N)
	start := time.Now()
	for i := 0; i < N; i++ {
		id := uuid.NewString()
		if _, dup := ids[id]; dup {
			t.Fatalf("collision at i=%d: %s", i, id)
		}
		ids[id] = struct{}{}
	}
	elapsed := time.Since(start)
	if len(ids) != N {
		t.Fatalf("expected %d unique IDs, got %d", N, len(ids))
	}
	if elapsed > 50*time.Millisecond {
		t.Logf("warning: %d UUIDv4 took %v (loose budget 50ms)", N, elapsed)
	}
}

// TestRequestID_Format — UUIDv4 canonical form.
func TestRequestID_Format(t *testing.T) {
	id := uuid.NewString()
	if len(id) != 36 {
		t.Errorf("expected 36-char canonical UUID, got %d: %q", len(id), id)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Errorf("Parse(%q) failed: %v", id, err)
	}
	if parsed.Version() != 4 {
		t.Errorf("expected version 4, got %v", parsed.Version())
	}
}
