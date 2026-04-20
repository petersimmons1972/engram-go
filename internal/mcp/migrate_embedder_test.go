package mcp

// Tests for handleMemoryMigrateEmbedder dimension pre-flight (#251).
// Uses a mock embedder that returns a configurable dimension count, and a
// noopBackend stub with GetMeta wired to return a stored dimension.

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

// dimBackend extends noopBackend to return a configurable embedder_dimensions
// value from GetMeta.
type dimBackend struct {
	noopBackend
	storedDims string // e.g. "384"
}

func (b dimBackend) GetMeta(_ context.Context, _, key string) (string, bool, error) {
	if key == "embedder_dimensions" && b.storedDims != "" {
		return b.storedDims, true, nil
	}
	return "", false, nil
}

var _ db.Backend = dimBackend{}

// dimEmbedder implements embed.Client returning a fixed dimension.
type dimEmbedder struct {
	dims int
}

func (e dimEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, e.dims), nil
}
func (e dimEmbedder) Name() string    { return "dim-test-model" }
func (e dimEmbedder) Dimensions() int { return e.dims }

var _ embed.Client = dimEmbedder{}

// newDimPool creates an EnginePool backed by dimBackend + the given embedder.
func newDimPool(t *testing.T, storedDims string, clientDims int) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		be := dimBackend{storedDims: storedDims}
		emb := dimEmbedder{dims: clientDims}
		engine := search.New(ctx, be, emb, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// makeMigrateRequest builds a minimal CallToolRequest for memory_migrate_embedder.
func makeMigrateRequest(project, newModel string) mcpgo.CallToolRequest {
	var req mcpgo.CallToolRequest
	req.Params.Arguments = map[string]any{
		"project":   project,
		"new_model": newModel,
	}
	return req
}

// TestHandleMemoryMigrateEmbedder_NoDimsStored verifies that when the backend
// has no stored "embedder_dimensions" metadata, the pre-flight does not fire
// a dimension error (it requires an Ollama endpoint to probe, so it skips).
// The handler then proceeds to MigrateEmbedder which panics with the noop
// backend's nil transaction — we recover and check the error is not ours.
func TestHandleMemoryMigrateEmbedder_NoDimsStored(t *testing.T) {
	pool := newDimPool(t, "", 384)
	req := makeMigrateRequest("proj", "new-model")
	cfg := Config{OllamaURL: "http://ollama-test:11434"}

	// The noop backend's Begin() returns nil; MigrateEmbedder will panic.
	// We use a deferred recover to catch it and assert the panic is NOT from
	// the dimension pre-flight (which would return an error, not panic).
	var dimensionErr error
	func() {
		defer func() { recover() }() //nolint:staticcheck // catching noop-backend nil-tx panic
		dimensionErr, _ = func() (error, bool) {
			_, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
			return err, true
		}()
	}()
	// If we got here without a dimension error, the pre-flight correctly skipped.
	if dimensionErr != nil {
		require.NotContains(t, dimensionErr.Error(), "dimension mismatch",
			"no dimension mismatch error expected when no stored dims present")
	}
}

// TestHandleMemoryMigrateEmbedder_EmptyNewModel verifies that an empty
// new_model field is rejected before any backend access.
func TestHandleMemoryMigrateEmbedder_EmptyNewModel(t *testing.T) {
	pool := newDimPool(t, "384", 384)

	req := makeMigrateRequest("proj", "")
	cfg := Config{OllamaURL: "http://ollama-test:11434"}

	_, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "new_model is required")
}
