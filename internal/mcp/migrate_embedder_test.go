package mcp

// Tests for handleMemoryMigrateEmbedder dimension pre-flight (#251).
// Uses a mock embedder that returns a configurable dimension count, and a
// noopBackend stub with GetMeta wired to return a stored dimension.

import (
	"context"
	"fmt"
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

// TestHandleMemoryMigrateEmbedder_ProbeError verifies that when the Ollama
// probe fails (model unreachable), the handler returns an error and does NOT
// proceed to null existing embeddings. Regression test for the fix in c09a552.
func TestHandleMemoryMigrateEmbedder_ProbeError(t *testing.T) {
	// Stored dims present — the pre-flight block must execute.
	pool := newDimPool(t, "384", 384)
	req := makeMigrateRequest("proj", "unreachable-model")
	cfg := Config{
		OllamaURL: "http://ollama-test:11434",
		testEmbedProbe: func(_ context.Context, _, _ string) (embed.Client, error) {
			return nil, fmt.Errorf("dial tcp: connect: connection refused")
		},
	}

	_, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot verify new embedder model dimensions",
		"probe failure must surface as a dimension-verification error, not a silent skip")
	require.Contains(t, err.Error(), "connection refused")
}

// TestHandleMemoryMigrateEmbedder_DimensionMismatch verifies that when the new
// model reports a different vector dimension than what is stored, migration is
// refused before any embeddings are nulled.
func TestHandleMemoryMigrateEmbedder_DimensionMismatch(t *testing.T) {
	pool := newDimPool(t, "384", 384) // stored: 384-dim
	req := makeMigrateRequest("proj", "wide-model")
	cfg := Config{
		OllamaURL: "http://ollama-test:11434",
		testEmbedProbe: func(_ context.Context, _, _ string) (embed.Client, error) {
			return dimEmbedder{dims: 1024}, nil // new model: 1024-dim — mismatch
		},
	}

	_, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dimension mismatch")
	require.Contains(t, err.Error(), "384")
	require.Contains(t, err.Error(), "1024")
}

// migrateReadyBackend extends dimBackend with a Begin that returns a noopTx so
// that MigrateEmbedder can proceed past its transaction without panicking.
type migrateReadyBackend struct{ dimBackend }

func (b migrateReadyBackend) Begin(_ context.Context) (db.Tx, error) { return noopTx{}, nil }

var _ db.Backend = migrateReadyBackend{}

// newMigratePool creates an EnginePool backed by migrateReadyBackend.
func newMigratePool(t *testing.T, storedDims string, clientDims int) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		be := migrateReadyBackend{dimBackend: dimBackend{storedDims: storedDims}}
		emb := dimEmbedder{dims: clientDims}
		engine := search.New(ctx, be, emb, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// TestHandleMemoryMigrateEmbedder_MigrateError verifies that when MigrateEmbedder
// itself returns an error, the handler surfaces that error and does not fire
// testOnPostMigrate.
func TestHandleMemoryMigrateEmbedder_MigrateError(t *testing.T) {
	pool := newDimPool(t, "", 384) // no stored dims → pre-flight skipped
	req := makeMigrateRequest("proj", "new-model")

	var postMigrateFired bool
	cfg := Config{
		OllamaURL: "http://ollama-test:11434",
		testMigrateFunc: func(_ context.Context, _ string) (map[string]any, error) {
			return nil, fmt.Errorf("db unavailable")
		},
		testOnPostMigrate: func(_ context.Context, _ string) {
			postMigrateFired = true
		},
	}

	_, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "db unavailable")
	require.False(t, postMigrateFired, "testOnPostMigrate must not fire when MigrateEmbedder fails")
}

// TestHandleMemoryMigrateEmbedder_HappyPath verifies the complete success path:
// probe matches stored dims, MigrateEmbedder succeeds via stub, testOnPostMigrate fires.
// Uses testMigrateFunc to bypass embed.NewOllamaClient inside MigrateEmbedder (no Ollama
// available in test environment).
func TestHandleMemoryMigrateEmbedder_HappyPath(t *testing.T) {
	pool := newDimPool(t, "384", 384) // stored: 384-dim; pre-flight will run
	req := makeMigrateRequest("proj", "same-dim-model")

	var postMigrateFired bool
	cfg := Config{
		OllamaURL: "http://ollama-test:11434",
		testEmbedProbe: func(_ context.Context, _, _ string) (embed.Client, error) {
			return dimEmbedder{dims: 384}, nil // pre-flight passes
		},
		testMigrateFunc: func(_ context.Context, _ string) (map[string]any, error) {
			return map[string]any{"nulled": 0, "model": "same-dim-model"}, nil
		},
		testOnPostMigrate: func(_ context.Context, _ string) {
			postMigrateFired = true
		},
	}

	_, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	require.True(t, postMigrateFired, "testOnPostMigrate must fire on success path")
}

// TestHandleMemoryMigrateEmbedder_DimsMatch_ProceedToMigrate verifies that when
// probe succeeds and dimensions match, the pre-flight passes and execution reaches
// MigrateEmbedder. The noop backend panics there — we recover and assert we got
// past the pre-flight without a dimension or probe error.
func TestHandleMemoryMigrateEmbedder_DimsMatch_ProceedToMigrate(t *testing.T) {
	pool := newDimPool(t, "384", 384) // stored: 384-dim
	req := makeMigrateRequest("proj", "same-dim-model")
	cfg := Config{
		OllamaURL: "http://ollama-test:11434",
		testEmbedProbe: func(_ context.Context, _, _ string) (embed.Client, error) {
			return dimEmbedder{dims: 384}, nil // same dim: pre-flight passes
		},
	}

	var reachedMigrate bool
	var preflightErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				reachedMigrate = true // noop backend panics in MigrateEmbedder — expected
			}
		}()
		_, preflightErr = handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	}()
	if preflightErr != nil {
		require.NotContains(t, preflightErr.Error(), "dimension mismatch",
			"matching dims must not produce a pre-flight error")
	}
	require.True(t, reachedMigrate, "pre-flight passed, execution should reach MigrateEmbedder")
}
