package search

// Tests for MigrateEmbedder safety guards (#spurious-reembed).
//
// Guards implemented here (all TDD: test written before implementation):
//   G1: Same-canonical-identity → no-op (NULLs 0 chunks)
//   G2: Same-dimension without force → refused
//   G3: dry_run counts without nulling; large volume without confirm → refused
//   G4: Stamp canonical, not raw, in project meta

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// newMigrateGuardEngine builds a SearchEngine backed by cacheMetaBackend with
// the embed-gateway path enabled (avoids a live Ollama call in MigrateEmbedder).
// storedName/storedDims pre-populate the project meta so the engine believes a
// prior embedder has been registered.
func newMigrateGuardEngine(t *testing.T, storedName string, storedDims int, chunkCount int) (*SearchEngine, *cacheMetaBackend) {
	t.Helper()
	backend := newCacheMetaBackend()
	emb := &cacheTestEmbedder{name: storedName, dims: storedDims}

	ctx := context.Background()

	// Pre-populate meta so MigrateEmbedder's guard reads the right stored model.
	_ = backend.SetMeta(ctx, "test-project", "embedder_name", canonicalEmbedderName(storedName))
	_ = backend.SetMeta(ctx, "test-project", "embedder_dimensions", fmt.Sprintf("%d", storedDims))

	// Wire chunk count so the volume guard can read it.
	backend.setChunkCount("test-project", chunkCount)

	eng := New(ctx, backend, emb, "test-project", "http://localhost:11434", "", false, nil, 0)
	eng.SetEmbedGateway(&noopGateway{})
	t.Cleanup(eng.Close)
	return eng, backend
}

// ── G1: Same-canonical-identity → no-op ──────────────────────────────────────

// TestMigrateSameCanonicalIdentityIsNoop: migrating to an alias of the current
// model must return chunks_nulled=0 and status="identity unchanged" without
// touching any chunk rows.
func TestMigrateSameCanonicalIdentityIsNoop(t *testing.T) {
	// Stored model is "BAAI/bge-m3". We migrate to alias "bge-m3" — same canonical.
	eng, backend := newMigrateGuardEngine(t, "BAAI/bge-m3", 1024, 500)
	ctx := context.Background()

	result, err := eng.MigrateEmbedder(ctx, MigrateParams{NewModel: "bge-m3"})
	require.NoError(t, err)
	require.Equal(t, 0, result["chunks_nulled"], "alias migrate must null 0 chunks")
	require.Equal(t, "identity unchanged", result["status"])
	require.Equal(t, int32(0), backend.nullAllCalls.Load(), "NullAllEmbeddingsTx must not be called on identity no-op")
}

// TestMigrateSameCanonicalIdentityIsNoop_OtherAliases checks further aliases.
func TestMigrateSameCanonicalIdentityIsNoop_OtherAliases(t *testing.T) {
	aliases := []string{"bge-m3:latest", "BAAI/bge-m3:latest", "bge-m3-Q8_0.gguf", "bge-m3-Q4_K_M.gguf"}
	for _, alias := range aliases {
		alias := alias
		t.Run(alias, func(t *testing.T) {
			eng, backend := newMigrateGuardEngine(t, "BAAI/bge-m3", 1024, 100)
			ctx := context.Background()

			result, err := eng.MigrateEmbedder(ctx, MigrateParams{NewModel: alias})
			require.NoError(t, err)
			require.Equal(t, 0, result["chunks_nulled"])
			require.Equal(t, "identity unchanged", result["status"])
			require.Equal(t, int32(0), backend.nullAllCalls.Load())
		})
	}
}

// ── G2: Same-dimension without force → refused ────────────────────────────────

// TestMigrateSameDimRequiresForce: migrating to a genuinely different model
// (different canonical name) but same dimension count must be refused unless
// force=true is passed.
func TestMigrateSameDimRequiresForce(t *testing.T) {
	eng, backend := newMigrateGuardEngine(t, "BAAI/bge-m3", 1024, 50)
	ctx := context.Background()

	// "other-1024-model" is a different model family also producing 1024-dim.
	// The guard should refuse it without force.
	result, err := eng.MigrateEmbedder(ctx, MigrateParams{
		NewModel: "other-1024-model",
		NewDims:  1024,
		Force:    false,
	})
	require.NoError(t, err, "same-dim refusal is a soft refusal, not a Go error")
	require.NotNil(t, result)
	errVal, hasErr := result["error"]
	require.True(t, hasErr, "result must contain 'error' key on same-dim refusal")
	errStr, _ := errVal.(string)
	require.Contains(t, errStr, "force", "refusal message must mention 'force'")
	require.Equal(t, int32(0), backend.nullAllCalls.Load(), "NullAllEmbeddingsTx must not be called on refused migrate")
}

// TestMigrateSameDimWithForceProceeds: same-dim migrate WITH force=true proceeds
// to null chunks.
func TestMigrateSameDimWithForceProceeds(t *testing.T) {
	eng, backend := newMigrateGuardEngine(t, "BAAI/bge-m3", 1024, 50)
	ctx := context.Background()

	result, err := eng.MigrateEmbedder(ctx, MigrateParams{
		NewModel: "other-1024-model",
		NewDims:  1024,
		Force:    true,
		Confirm:  true,
	})
	require.NoError(t, err)
	_, hasErr := result["error"]
	require.False(t, hasErr, "force=true must allow same-dim migration")
	require.Equal(t, int32(1), backend.nullAllCalls.Load(), "NullAllEmbeddingsTx must be called once")
}

// ── G3: Volume guard + dry_run ────────────────────────────────────────────────

// TestMigrateDryRunCountsWithoutNulling: dry_run=true returns chunks_would_null
// without touching any chunk rows.
func TestMigrateDryRunCountsWithoutNulling(t *testing.T) {
	const chunkCount = 42
	eng, backend := newMigrateGuardEngine(t, "BAAI/bge-m3", 1024, chunkCount)
	ctx := context.Background()

	result, err := eng.MigrateEmbedder(ctx, MigrateParams{
		NewModel: "totally-different-model",
		NewDims:  768,
		DryRun:   true,
	})
	require.NoError(t, err)
	require.Equal(t, chunkCount, result["chunks_would_null"], "dry_run must report exact pending chunk count")
	_, hasNulled := result["chunks_nulled"]
	require.False(t, hasNulled, "dry_run must NOT have chunks_nulled key")
	require.Equal(t, int32(0), backend.nullAllCalls.Load(), "NullAllEmbeddingsTx must not be called in dry_run mode")
}

// TestMigrateLargeVolumeRequiresConfirm: when affected chunks exceed the
// threshold (migrateConfirmThreshold) and confirm=false, migration is refused
// with the count in the error message.
func TestMigrateLargeVolumeRequiresConfirm(t *testing.T) {
	const overThreshold = migrateConfirmThreshold + 1
	eng, backend := newMigrateGuardEngine(t, "BAAI/bge-m3", 1024, overThreshold)
	ctx := context.Background()

	result, err := eng.MigrateEmbedder(ctx, MigrateParams{
		NewModel: "totally-different-model",
		NewDims:  768,
		Confirm:  false,
	})
	require.NoError(t, err, "volume refusal is a soft refusal, not a Go error")
	errVal, hasErr := result["error"]
	require.True(t, hasErr, "must return error key when confirm missing for large corpus")
	errStr, _ := errVal.(string)
	require.Contains(t, errStr, "confirm", "error must mention confirm=true requirement")
	require.Equal(t, int32(0), backend.nullAllCalls.Load(), "NullAllEmbeddingsTx must not be called on unconfirmed large migrate")
}

// TestMigrateSmallVolumeNoConfirmRequired: chunk count below threshold proceeds
// without confirm.
func TestMigrateSmallVolumeNoConfirmRequired(t *testing.T) {
	const underThreshold = migrateConfirmThreshold - 1
	eng, _ := newMigrateGuardEngine(t, "BAAI/bge-m3", 1024, underThreshold)
	ctx := context.Background()

	result, err := eng.MigrateEmbedder(ctx, MigrateParams{
		NewModel: "totally-different-model",
		NewDims:  768,
		Confirm:  false,
	})
	require.NoError(t, err)
	_, hasErr := result["error"]
	require.False(t, hasErr, "small corpus must not require confirm")
}

// TestMigrateLargeVolumeWithConfirmProceeds: confirm=true on large corpus proceeds.
func TestMigrateLargeVolumeWithConfirmProceeds(t *testing.T) {
	const overThreshold = migrateConfirmThreshold + 1
	eng, backend := newMigrateGuardEngine(t, "BAAI/bge-m3", 1024, overThreshold)
	ctx := context.Background()

	result, err := eng.MigrateEmbedder(ctx, MigrateParams{
		NewModel: "totally-different-model",
		NewDims:  768,
		Confirm:  true,
	})
	require.NoError(t, err)
	_, hasErr := result["error"]
	require.False(t, hasErr, "confirm=true on large corpus must proceed")
	require.Equal(t, int32(1), backend.nullAllCalls.Load())
}

// ── G4: Stamp canonical, not raw ──────────────────────────────────────────────

// TestMigrateStampsCanonicalEmbedderName: after a successful migration, the
// embedder_name stored in project meta must be the canonical form, not the
// raw alias passed in.
func TestMigrateStampsCanonicalEmbedderName(t *testing.T) {
	eng, backend := newMigrateGuardEngine(t, "BAAI/bge-m3", 1024, 10)
	ctx := context.Background()

	// Migrate to a model with no alias — canonical == raw. Dims differ so no G2.
	result, err := eng.MigrateEmbedder(ctx, MigrateParams{
		NewModel: "new-model-raw",
		NewDims:  768,
		Confirm:  true,
	})
	require.NoError(t, err)
	_, hasErr := result["error"]
	require.False(t, hasErr)

	storedName, ok, _ := backend.GetMeta(ctx, "test-project", "embedder_name")
	require.True(t, ok)
	require.Equal(t, canonicalEmbedderName("new-model-raw"), storedName,
		"migrate must stamp canonical name, not raw arg")
}

// TestMigrateStampsCanonicalEmbedderName_AliasInput: when a raw alias IS the
// input, the stored name must still be the canonical form.
func TestMigrateStampsCanonicalEmbedderName_AliasInput(t *testing.T) {
	backend := newCacheMetaBackend()
	ctx := context.Background()

	// Pre-populate with a different model so migrating to bge-m3-Q8_0.gguf is real.
	_ = backend.SetMeta(ctx, "test-project", "embedder_name", "old-model-512")
	_ = backend.SetMeta(ctx, "test-project", "embedder_dimensions", "512")
	backend.setChunkCount("test-project", 5)

	emb := &cacheTestEmbedder{name: "old-model-512", dims: 512}
	eng := New(ctx, backend, emb, "test-project", "http://localhost:11434", "", false, nil, 0)
	eng.SetEmbedGateway(&noopGateway{})
	defer eng.Close()

	// Migrate using alias "bge-m3-Q8_0.gguf" (canonical: "BAAI/bge-m3").
	// Dims differ (512→1024), so no G2 trigger.
	result, err := eng.MigrateEmbedder(ctx, MigrateParams{
		NewModel: "bge-m3-Q8_0.gguf",
		NewDims:  1024,
		Confirm:  true,
	})
	require.NoError(t, err)
	_, hasErr := result["error"]
	require.False(t, hasErr)

	storedName, ok, _ := backend.GetMeta(ctx, "test-project", "embedder_name")
	require.True(t, ok)
	require.Equal(t, "BAAI/bge-m3", storedName,
		"bge-m3-Q8_0.gguf alias must be stored as canonical BAAI/bge-m3")
}
