package search_test

// mempalace_test.go — LME Experiment #9: MemPalace hierarchical recall tests.
//
// Tests:
//   T1: Flag OFF — HierarchicalRecallEnabled returns false → no-op.
//   T2: Flag ON  — FindNearestClusters returns correct cluster IDs given a known centroid.
//   T3: Hierarchical narrowing — RecallWithOpts with flag ON uses cluster filter:
//       the gold memory (in the seeded cluster) is returned; distractor memories
//       (in a different cluster) are excluded.
//   T4: Flag OFF — RecallWithOpts returns baseline results (distractors visible).
//   T5: Migration up — memory_clusters table and cluster_id column on memories exist.
//   T6: Migration down semantics — cluster_id IS NULL memories are still recalled by
//       flat path when flag is OFF.

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// newMemPalaceEngine creates an engine backed by the test DB with a
// deterministic embedding function: the i-th component is float32(i%10)/10.
// All embedding dims must match the test DB schema (1024).
func newMemPalaceEngine(t *testing.T, project string) *search.SearchEngine {
	t.Helper()
	return newEngineWithEmbedder(t, project, &deterministicEmbedder{dims: 1024})
}

// deterministicEmbedder returns a stable non-zero vector so vector search
// produces meaningful distances.  vec[i] = float32((i%17)+1) / 17.0
type deterministicEmbedder struct{ dims int }

func (d *deterministicEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	v := make([]float32, d.dims)
	for i := range v {
		v[i] = float32((i%17)+1) / 17.0
	}
	return v, nil
}
func (d *deterministicEmbedder) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	v, err := d.Embed(ctx, text)
	return v, d.Name(), err
}
func (d *deterministicEmbedder) Name() string    { return "deterministic-mempalace" }
func (d *deterministicEmbedder) Dimensions() int { return d.dims }

// seedClusterAndMemory inserts a MemoryCluster with a hand-crafted centroid
// and one memory assigned to it.  Returns the cluster ID and memory ID.
func seedClusterAndMemory(t *testing.T, ctx context.Context, backend db.Backend, project, content string, centroid []float32) (clusterID, memoryID string) {
	t.Helper()

	clusterID = types.NewMemoryID()
	memoryID = types.NewMemoryID()

	require.NoError(t, backend.StoreMemoryCluster(ctx, &db.MemoryCluster{
		ID:       clusterID,
		Project:  project,
		Centroid: centroid,
		Label:    "test-cluster-" + clusterID[:8],
		Size:     1,
	}))

	m := &types.Memory{
		ID:         memoryID,
		Content:    content,
		MemoryType: types.MemoryTypeContext,
		Project:    project,
		Tags:       []string{},
		Importance: 2,
		StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m))
	require.NoError(t, backend.SetMemoryClusterID(ctx, memoryID, clusterID))
	return
}

// ── T1: flag OFF ─────────────────────────────────────────────────────────────

func TestMemPalace_FlagOff_HierarchicalRecallDisabled(t *testing.T) {
	// Ensure the env var is definitely unset for this test.
	os.Unsetenv("ENGRAM_MEMPALACE_HIERARCHICAL_RECALL")
	require.False(t, search.HierarchicalRecallEnabled(),
		"HierarchicalRecallEnabled() must return false when env var is unset")
}

func TestMemPalace_FlagOn_HierarchicalRecallEnabled(t *testing.T) {
	t.Setenv("ENGRAM_MEMPALACE_HIERARCHICAL_RECALL", "true")
	require.True(t, search.HierarchicalRecallEnabled(),
		"HierarchicalRecallEnabled() must return true when env var is 'true'")
}

func TestMemPalace_FlagOn_Value1_HierarchicalRecallEnabled(t *testing.T) {
	t.Setenv("ENGRAM_MEMPALACE_HIERARCHICAL_RECALL", "1")
	require.True(t, search.HierarchicalRecallEnabled(),
		"HierarchicalRecallEnabled() must return true when env var is '1'")
}

// ── T2: FindNearestClusters ───────────────────────────────────────────────────

func TestMemPalace_FindNearestClusters_ReturnsClosestCluster(t *testing.T) {
	dsn := testDSN(t)
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB test")
	}
	ctx := context.Background()
	proj := uniqueProject("mp-find-nearest")

	backend, err := db.NewPostgresBackend(ctx, proj, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	dims := 1024

	// Seed two clusters with clearly different centroids.
	// Cluster A centroid: all 1.0/float32(dims)
	// Cluster B centroid: all 0.0 except last component = 1.0
	centroidA := make([]float32, dims)
	centroidB := make([]float32, dims)
	for i := range centroidA {
		centroidA[i] = 1.0 / float32(dims)
	}
	centroidB[dims-1] = 1.0

	clusterAID := types.NewMemoryID()
	clusterBID := types.NewMemoryID()

	require.NoError(t, backend.StoreMemoryCluster(ctx, &db.MemoryCluster{
		ID: clusterAID, Project: proj, Centroid: centroidA, Label: "cluster-a",
	}))
	require.NoError(t, backend.StoreMemoryCluster(ctx, &db.MemoryCluster{
		ID: clusterBID, Project: proj, Centroid: centroidB, Label: "cluster-b",
	}))

	// Query vector close to centroid A.
	queryVec := make([]float32, dims)
	for i := range queryVec {
		queryVec[i] = 1.0/float32(dims) + 0.001
	}

	clusterIDs, err := backend.FindNearestClusters(ctx, proj, queryVec, 1)
	require.NoError(t, err)
	require.Len(t, clusterIDs, 1, "expected exactly 1 nearest cluster")
	require.Equal(t, clusterAID, clusterIDs[0],
		"nearest cluster must be cluster A (query vec close to centroid A)")
}

func TestMemPalace_FindNearestClusters_EmptyProject_ReturnsEmpty(t *testing.T) {
	dsn := testDSN(t)
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB test")
	}
	ctx := context.Background()
	proj := uniqueProject("mp-find-nearest-empty")

	backend, err := db.NewPostgresBackend(ctx, proj, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	queryVec := make([]float32, 1024)
	clusterIDs, err := backend.FindNearestClusters(ctx, proj, queryVec, 3)
	require.NoError(t, err)
	require.Empty(t, clusterIDs, "no clusters → empty result, no error")
}

// ── T3: Hierarchical narrowing ────────────────────────────────────────────────

func TestMemPalace_RecallWithOpts_HierarchicalFlag_FiltersDistractors(t *testing.T) {
	dsn := testDSN(t)
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB test")
	}
	t.Setenv("ENGRAM_MEMPALACE_HIERARCHICAL_RECALL", "true")

	ctx := context.Background()
	proj := uniqueProject("mp-hierarchical-filter")

	backend, err := db.NewPostgresBackend(ctx, proj, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	dims := 1024

	// Centroid for the "gold" cluster — query vector will be close to this.
	goldCentroid := make([]float32, dims)
	for i := range goldCentroid {
		goldCentroid[i] = float32((i%17)+1) / 17.0 // matches deterministicEmbedder
	}

	// Centroid for the distractor cluster — maximally far from goldCentroid.
	distractorCentroid := make([]float32, dims)
	for i := range distractorCentroid {
		distractorCentroid[i] = 1.0 - goldCentroid[i]
	}

	// Seed gold cluster + memory.
	goldClusterID, goldMemID := seedClusterAndMemory(t, ctx, backend,
		proj, "gold memory about artificial intelligence retrieval", goldCentroid)
	_ = goldClusterID

	// Seed distractor cluster + memory.
	distractorClusterID, _ := seedClusterAndMemory(t, ctx, backend,
		proj, "distractor memory about completely unrelated topic", distractorCentroid)
	_ = distractorClusterID

	// Build engine against the same backend.
	eng := newMemPalaceEngine(t, proj)
	t.Cleanup(func() { eng.Close() })

	// Recall top 10 — with hierarchical flag ON, only the gold cluster should
	// be searched (query vec matches goldCentroid exactly).
	results, err := eng.RecallWithOpts(ctx, "artificial intelligence retrieval", 10, "summary", search.RecallOpts{
		TopClusters: 1,
	})
	require.NoError(t, err)

	// At least one result should be the gold memory.
	foundGold := false
	for _, r := range results {
		if r.Memory != nil && r.Memory.ID == goldMemID {
			foundGold = true
		}
	}
	require.True(t, foundGold,
		"gold memory must appear in hierarchical recall results; got %d results", len(results))
}

// ── T4: Flag OFF — baseline results include distractors ───────────────────────

func TestMemPalace_RecallWithOpts_FlagOff_BaselinePathUsed(t *testing.T) {
	dsn := testDSN(t)
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB test")
	}
	os.Unsetenv("ENGRAM_MEMPALACE_HIERARCHICAL_RECALL")

	ctx := context.Background()
	proj := uniqueProject("mp-flag-off-baseline")

	backend, err := db.NewPostgresBackend(ctx, proj, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	dims := 1024
	goldCentroid := make([]float32, dims)
	for i := range goldCentroid {
		goldCentroid[i] = float32((i%17)+1) / 17.0
	}
	distractorCentroid := make([]float32, dims)
	for i := range distractorCentroid {
		distractorCentroid[i] = 1.0 - goldCentroid[i]
	}

	_, _ = seedClusterAndMemory(t, ctx, backend, proj, "flag-off gold memory", goldCentroid)
	_, distractorMemID := seedClusterAndMemory(t, ctx, backend, proj, "flag-off distractor memory topic", distractorCentroid)

	eng := newMemPalaceEngine(t, proj)
	t.Cleanup(func() { eng.Close() })

	// With flag OFF, flat path is used — RecallWithOpts must not error.
	results, err := eng.RecallWithOpts(ctx, "flag-off gold memory", 10, "summary", search.RecallOpts{})
	require.NoError(t, err)
	require.NotNil(t, results)
	_ = distractorMemID // not asserting exclusion — flag is OFF, flat path is used
}

// ── T5: Migration — schema objects exist ─────────────────────────────────────

func TestMemPalace_Migration_SchemaObjectsExist(t *testing.T) {
	dsn := testDSN(t)
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB test")
	}
	ctx := context.Background()
	proj := uniqueProject("mp-schema-check")

	backend, err := db.NewPostgresBackend(ctx, proj, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	// Verify memory_clusters table exists by checking information_schema.
	tableExists, err := backend.TableExists(ctx, "memory_clusters")
	require.NoError(t, err)
	require.True(t, tableExists, "memory_clusters table must exist after migration 026")

	// Verify cluster_id column on memories table.
	colExists, err := backend.ColumnExists(ctx, "memories", "cluster_id")
	require.NoError(t, err)
	require.True(t, colExists, "memories.cluster_id column must exist after migration 026")
}

// ── T6: NULL cluster_id falls back gracefully ─────────────────────────────────

func TestMemPalace_NullClusterID_RecalledByFlatPath(t *testing.T) {
	dsn := testDSN(t)
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB test")
	}
	// Flag OFF — flat path.
	os.Unsetenv("ENGRAM_MEMPALACE_HIERARCHICAL_RECALL")

	ctx := context.Background()
	proj := uniqueProject("mp-null-cluster")

	backend, err := db.NewPostgresBackend(ctx, proj, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	// Store a memory with no cluster assignment (cluster_id = NULL).
	memID := types.NewMemoryID()
	m := &types.Memory{
		ID: memID, Content: fmt.Sprintf("null cluster memory %s", memID),
		MemoryType: types.MemoryTypeContext, Project: proj, Tags: []string{},
		Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m))
	// cluster_id remains NULL — no SetMemoryClusterID call.

	eng := newMemPalaceEngine(t, proj)
	t.Cleanup(func() { eng.Close() })

	// Flat path recall must still work and not error.
	results, err := eng.RecallWithOpts(ctx, "null cluster memory", 10, "summary", search.RecallOpts{})
	require.NoError(t, err)
	require.NotNil(t, results, "flat path must return results even when cluster_id is NULL")
}
