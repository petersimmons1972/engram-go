package consolidate_test

// Feature 3: Sleep Consolidation Daemon
// All tests written BEFORE implementation (TDD).
// They must fail (compile or runtime) until Feature 3 is implemented.

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/consolidate"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/testutil"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDSN(t *testing.T) string  { return testutil.DSN(t) }
func uniqueProject(base string) string { return testutil.UniqueProject(base) }

// fakeEmbedder returns deterministic vectors that encode similarity via a simple hash.
type fakeEmbedder struct{ dims int }

func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, f.dims)
	// Make similar-content texts produce similar vectors by spreading hash bytes.
	h := 0
	for i, c := range text {
		h = h*31 + int(c) + i
	}
	for i := range vec {
		vec[i] = float32((h+i)%100) / 100.0
	}
	return vec, nil
}
func (f *fakeEmbedder) Name() string    { return "fake" }
func (f *fakeEmbedder) Dimensions() int { return f.dims }

var _ embed.Client = (*fakeEmbedder)(nil)


// ── InferRelationships ────────────────────────────────────────────────────────

// TestInferRelationships_CreatesEdges verifies that InferRelationships creates
// relates_to edges between memories whose chunks are nearest neighbors.
func TestInferRelationships_CreatesEdges(t *testing.T) {
	project := uniqueProject("consolidate-infer")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	// Store two closely related memories and manually set their chunks with known embeddings.
	m1 := &types.Memory{
		ID: types.NewMemoryID(), Content: "PostgreSQL uses MVCC for transaction isolation",
		MemoryType: types.MemoryTypeArchitecture, Project: project, Importance: 2, StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID: types.NewMemoryID(), Content: "PostgreSQL MVCC creates row versions for each transaction",
		MemoryType: types.MemoryTypeArchitecture, Project: project, Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))

	// Store chunks with very similar embeddings (nearly identical vectors → should be detected as related).
	vec1 := make([]float32, 1024)
	vec2 := make([]float32, 1024)
	for i := range vec1 {
		vec1[i] = 0.5
		vec2[i] = 0.5 + float32(i)/float32(1024*1000) // nearly identical
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m1.ID, ChunkText: m1.Content, ChunkIndex: 0,
			ChunkHash: "hash1a", ChunkType: "sentence_window", Project: project, Embedding: vec1},
	}))
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m2.ID, ChunkText: m2.Content, ChunkIndex: 0,
			ChunkHash: "hash2a", ChunkType: "sentence_window", Project: project, Embedding: vec2},
	}))

	created, err := runner.InferRelationships(ctx, 0.3, 100) // low threshold to guarantee creation
	require.NoError(t, err)
	assert.Greater(t, created, 0, "InferRelationships must create at least one relates_to edge")

	// Verify the relationship exists.
	count, err := backend.GetConnectionCount(ctx, m1.ID, project)
	require.NoError(t, err)
	assert.Greater(t, count, 0, "memory m1 must have at least one connection after InferRelationships")
}

// TestInferRelationships_SkipsExistingEdges verifies that InferRelationships does
// not create duplicate edges when a relationship already exists between two memories.
func TestInferRelationships_SkipsExistingEdges(t *testing.T) {
	project := uniqueProject("consolidate-skip")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	m1 := &types.Memory{
		ID: types.NewMemoryID(), Content: "Go channels enable safe goroutine communication",
		MemoryType: types.MemoryTypePattern, Project: project, Importance: 2, StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID: types.NewMemoryID(), Content: "Go goroutines communicate via channels",
		MemoryType: types.MemoryTypePattern, Project: project, Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))

	// Pre-create the relationship.
	require.NoError(t, backend.StoreRelationship(ctx, &types.Relationship{
		ID: types.NewMemoryID(), SourceID: m1.ID, TargetID: m2.ID,
		RelType: types.RelTypeRelatesTo, Strength: 0.9, Project: project,
	}))

	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = 0.5
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m1.ID, ChunkText: m1.Content, ChunkIndex: 0,
			ChunkHash: "hash1b", ChunkType: "sentence_window", Project: project, Embedding: vec},
		{ID: types.NewMemoryID(), MemoryID: m2.ID, ChunkText: m2.Content, ChunkIndex: 0,
			ChunkHash: "hash2b", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))

	beforeCount, err := backend.GetConnectionCount(ctx, m1.ID, project)
	require.NoError(t, err)

	_, err = runner.InferRelationships(ctx, 0.0, 100) // threshold=0 → all pairs
	require.NoError(t, err)

	afterCount, err := backend.GetConnectionCount(ctx, m1.ID, project)
	require.NoError(t, err)
	assert.Equal(t, beforeCount, afterCount,
		"InferRelationships must not duplicate existing edges")
}

// ── RunAll ────────────────────────────────────────────────────────────────────

// TestRunAll_ReturnsStats verifies that RunAll returns a non-zero stats map.
func TestRunAll_ReturnsStats(t *testing.T) {
	project := uniqueProject("consolidate-runall")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	m := &types.Memory{
		Content: "Sleep consolidation: infer relationships between related memories",
		MemoryType: types.MemoryTypePattern, Project: project, Importance: 2, StorageMode: "focused",
	}
	m.ID = types.NewMemoryID()
	require.NoError(t, backend.StoreMemory(ctx, m))

	stats, err := runner.RunAll(ctx, consolidate.RunOptions{
		InferRelationshipsMinSimilarity: 0.5,
		InferRelationshipsLimit:         50,
	})
	require.NoError(t, err)
	assert.NotNil(t, stats, "RunAll must return stats")
	// InferRelationships must always run (the others are optional/LLM).
	_, ok := stats["inferred_relationships"]
	assert.True(t, ok, "stats must include inferred_relationships count")
}

// ── DetectContradictions ──────────────────────────────────────────────────────

// TestDetectContradictions_OpposingClaims verifies that two memories where one
// negates the other's core claim produce a "contradicts" edge.
func TestDetectContradictions_OpposingClaims(t *testing.T) {
	project := uniqueProject("consolidate-contra-neg")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	m1 := &types.Memory{
		ID: types.NewMemoryID(), Content: "PostgreSQL uses MVCC for concurrency",
		MemoryType: types.MemoryTypeArchitecture, Project: project, Importance: 2, StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID: types.NewMemoryID(), Content: "PostgreSQL does not use MVCC",
		MemoryType: types.MemoryTypeArchitecture, Project: project, Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))

	// Identical embeddings → cosine similarity = 1.0, guaranteeing the pair is examined.
	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = 0.5
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m1.ID, ChunkText: m1.Content, ChunkIndex: 0,
			ChunkHash: "hash-contra-1a", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m2.ID, ChunkText: m2.Content, ChunkIndex: 0,
			ChunkHash: "hash-contra-1b", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))

	created, err := runner.DetectContradictions(ctx, 0.5, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, created, "DetectContradictions must create exactly one contradicts edge")

	rels, err := backend.GetRelationships(ctx, project, m1.ID)
	require.NoError(t, err)
	found := false
	for _, rel := range rels {
		if rel.RelType == types.RelTypeContradicts {
			found = true
		}
	}
	assert.True(t, found, "a 'contradicts' relationship must exist on m1 after DetectContradictions")
}

// TestDetectContradictions_VersionConflict verifies that two memories referencing
// the same entity with different version numbers produce a "contradicts" edge.
func TestDetectContradictions_VersionConflict(t *testing.T) {
	project := uniqueProject("consolidate-contra-ver")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	m1 := &types.Memory{
		ID: types.NewMemoryID(), Content: "PlanCrux handbook version is v1.2",
		MemoryType: types.MemoryTypeContext, Project: project, Importance: 2, StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID: types.NewMemoryID(), Content: "PlanCrux handbook version is v1.4",
		MemoryType: types.MemoryTypeContext, Project: project, Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))

	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = 0.5
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m1.ID, ChunkText: m1.Content, ChunkIndex: 0,
			ChunkHash: "hash-contra-2a", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m2.ID, ChunkText: m2.Content, ChunkIndex: 0,
			ChunkHash: "hash-contra-2b", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))

	created, err := runner.DetectContradictions(ctx, 0.5, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, created, "DetectContradictions must create exactly one contradicts edge for version conflict")
}

// TestDetectContradictions_SimilarButNotContradicting verifies that two similar
// but mutually consistent memories do NOT produce a contradicts edge.
func TestDetectContradictions_SimilarButNotContradicting(t *testing.T) {
	project := uniqueProject("consolidate-contra-compat")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	m1 := &types.Memory{
		ID: types.NewMemoryID(), Content: "Go uses goroutines for concurrency",
		MemoryType: types.MemoryTypePattern, Project: project, Importance: 2, StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID: types.NewMemoryID(), Content: "Go goroutines are lightweight threads",
		MemoryType: types.MemoryTypePattern, Project: project, Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))

	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = 0.5
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m1.ID, ChunkText: m1.Content, ChunkIndex: 0,
			ChunkHash: "hash-contra-3a", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m2.ID, ChunkText: m2.Content, ChunkIndex: 0,
			ChunkHash: "hash-contra-3b", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))

	created, err := runner.DetectContradictions(ctx, 0.5, 100)
	require.NoError(t, err)
	assert.Equal(t, 0, created, "DetectContradictions must NOT create a contradicts edge for compatible memories")
}

// TestDetectContradictions_SkipsExistingEdges verifies that DetectContradictions
// does not create a duplicate contradicts edge when one already exists.
func TestDetectContradictions_SkipsExistingEdges(t *testing.T) {
	project := uniqueProject("consolidate-contra-dup")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	m1 := &types.Memory{
		ID: types.NewMemoryID(), Content: "PostgreSQL uses MVCC for concurrency",
		MemoryType: types.MemoryTypeArchitecture, Project: project, Importance: 2, StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID: types.NewMemoryID(), Content: "PostgreSQL does not use MVCC",
		MemoryType: types.MemoryTypeArchitecture, Project: project, Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))

	// Pre-create the contradicts edge.
	require.NoError(t, backend.StoreRelationship(ctx, &types.Relationship{
		ID: types.NewMemoryID(), SourceID: m1.ID, TargetID: m2.ID,
		RelType: types.RelTypeContradicts, Strength: 0.95, Project: project,
	}))

	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = 0.5
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m1.ID, ChunkText: m1.Content, ChunkIndex: 0,
			ChunkHash: "hash-contra-4a", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m2.ID, ChunkText: m2.Content, ChunkIndex: 0,
			ChunkHash: "hash-contra-4b", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))

	beforeCount, err := backend.GetConnectionCount(ctx, m1.ID, project)
	require.NoError(t, err)

	created, err := runner.DetectContradictions(ctx, 0.5, 100)
	require.NoError(t, err)
	assert.Equal(t, 0, created, "DetectContradictions must not create a duplicate edge")

	afterCount, err := backend.GetConnectionCount(ctx, m1.ID, project)
	require.NoError(t, err)
	assert.Equal(t, beforeCount, afterCount, "connection count must not change when edge already exists")
}

// ─────────────────────────────────────────────────────────────────────────────
// Unit tests for isContradiction — pure function, no database required.
// These run in every go test invocation.
// ─────────────────────────────────────────────────────────────────────────────

func TestIsContradiction_NegationOpposition(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"clear negation", "PostgreSQL uses MVCC for concurrency control and transaction isolation", "PostgreSQL does not use MVCC for concurrency control and transaction isolation", true},
		{"negation with isn't", "the authentication service is ready for production deployment today", "the authentication service isn't ready for production deployment today", true},
		{"negation with never", "Tailscale client always reconnects after sleep with valid credentials", "Tailscale client never reconnects after sleep with valid credentials", true},
		{"both have negation — not contradictory", "service does not start automatically", "service does not initialize correctly", false},
		{"neither has negation — not contradictory", "Go uses goroutines for concurrency", "Go uses goroutines effectively", false},
		{"too few shared words", "database uses MVCC", "network does not use MVCC", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consolidate.IsContradiction(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsContradiction_VersionConflict(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"different versions same topic", "PlanCrux handbook version is v1.2", "PlanCrux handbook version is v1.4", true},
		{"same version — not contradictory", "PlanCrux handbook version is v1.2", "PlanCrux handbook version is v1.2", false},
		{"different topics with versions", "PlanCrux version is v1.2", "ScoreCrux version is v1.4", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consolidate.IsContradiction(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsContradiction_TemporalSupersession(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"was vs is", "the security team was responsible for auth middleware validation checks", "the platform team is responsible for auth middleware validation checks", true},
		// Cross-service temporal conflict: Redis → Memcached. Shares 5 significant
		// words (authentication, service, session, caching, storage) which meets
		// the >=5 threshold while the service names differ.
		{"previously vs currently", "the authentication service previously used Redis for session caching storage", "the authentication service currently uses Memcached for session caching storage", true},
		{"used to vs now", "the platform team used to deploy services with Ansible automation", "the platform team now deploys services with Terraform automation", true},
		{"both past tense — not contradictory", "the service was built with Java", "the service was tested with JUnit", false},
		{"both present tense — not contradictory", "the service is built with Go", "the service is tested with Go", false},
		{"too few shared words", "was deployed manually", "is deployed automatically now", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consolidate.IsContradiction(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsContradiction_NoFalsePositiveOnUnrelatedContent(t *testing.T) {
	got := consolidate.IsContradiction(
		"Kubernetes uses etcd for cluster state storage",
		"PostgreSQL uses WAL for transaction logging recovery",
	)
	assert.False(t, got, "unrelated content must not trigger contradiction")
}

// ── False-positive regression tests (#156) ────────────────────────────────────
// Design intent: false negatives are preferred over false positives.
// When uncertain, do NOT flag as contradiction.

// TestIsContradiction_FalsePositive_UnrelatedNegation guards against the case
// where two unrelated sentences both contain a negation phrase and happen to
// share enough significant words to cross the shared-word threshold. They are
// NOT contradictions because they describe entirely different subjects.
func TestIsContradiction_FalsePositive_UnrelatedNegation(t *testing.T) {
	got := consolidate.IsContradiction(
		"The API is not rate limited for internal services.",
		"The database is not cached in this environment.",
	)
	assert.False(t, got, "two unrelated negation sentences must not be flagged as contradictions")
}

// TestIsContradiction_FalsePositive_VersionDifferentContext guards against
// version-conflict false positives where the same version pattern appears in
// two sentences about entirely different subjects (JWT vs React).
func TestIsContradiction_FalsePositive_VersionDifferentContext(t *testing.T) {
	got := consolidate.IsContradiction(
		"Authentication uses JWT v2.0 for token signing.",
		"The UI framework requires React v2.1 compatibility mode.",
	)
	assert.False(t, got, "different contexts with incidentally similar version numbers must not be flagged")
}

// TestIsContradiction_FalsePositive_TemporalUnrelated guards against the case
// where one sentence has a past-tense marker and the other a present-tense
// marker, but they describe entirely different subjects. The shared-word
// threshold must be high enough to prevent this from triggering.
func TestIsContradiction_FalsePositive_TemporalUnrelated(t *testing.T) {
	got := consolidate.IsContradiction(
		"The old system was deprecated last year.",
		"The new deployment pipeline is running smoothly.",
	)
	assert.False(t, got, "past/present tense sentences about different subjects must not be flagged")
}

// TestIsContradiction_TruePositive_NegationOpposition preserves detection of
// the canonical negation case: same subject, one affirmative, one negative.
func TestIsContradiction_TruePositive_NegationOpposition(t *testing.T) {
	got := consolidate.IsContradiction(
		"The rate limiter is enabled on all production endpoints.",
		"The rate limiter is not enabled on production endpoints.",
	)
	assert.True(t, got, "direct negation of the same claim must be flagged as contradiction")
}

// TestIsContradiction_TruePositive_VersionConflict preserves detection of the
// canonical version-conflict case: same service, different version numbers.
func TestIsContradiction_TruePositive_VersionConflict(t *testing.T) {
	got := consolidate.IsContradiction(
		"The service requires postgres v14.2 for JSON path queries.",
		"The service requires postgres v15.1 for JSON path queries.",
	)
	assert.True(t, got, "conflicting version numbers for the same component must be flagged")
}

// ── AutoSupersede ─────────────────────────────────────────────────────────────

// storeContradictingPair creates two memories with a contradicts edge between them
// and stores chunks so vector search can return them. olderOffset is subtracted
// from the current time to produce the older memory's CreatedAt.
// Returns (newer, older) — the caller controls which should supersede which.
func storeContradictingPair(t *testing.T, ctx context.Context, backend db.Backend, project string, olderOffset time.Duration) (newer, older *types.Memory) {
	t.Helper()

	// Older memory — created olderOffset in the past.
	mOld := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    "PostgreSQL uses MVCC for concurrency control",
		MemoryType: types.MemoryTypeArchitecture,
		Project:    project,
		Importance: 2,
		StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, mOld))
	// Back-date the older memory so the gap is clear.
	_, err := backend.(*db.PostgresBackend).Pool().Exec(ctx,
		"UPDATE memories SET created_at=$1, updated_at=$1 WHERE id=$2",
		time.Now().UTC().Add(-olderOffset), mOld.ID,
	)
	require.NoError(t, err)

	// Newer memory — created "now" (StoreMemory sets created_at=NOW()).
	mNew := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    "PostgreSQL does not use MVCC for concurrency control",
		MemoryType: types.MemoryTypeArchitecture,
		Project:    project,
		Importance: 2,
		StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, mNew))

	// Identical embeddings so vector search returns this pair.
	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = 0.5
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: mOld.ID, ChunkText: mOld.Content, ChunkIndex: 0,
			ChunkHash: "hash-sup-old-" + mOld.ID, ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: mNew.ID, ChunkText: mNew.Content, ChunkIndex: 0,
			ChunkHash: "hash-sup-new-" + mNew.ID, ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))

	// Manually create the contradicts edge (as DetectContradictions would).
	require.NoError(t, backend.StoreRelationship(ctx, &types.Relationship{
		ID:       types.NewMemoryID(),
		SourceID: mOld.ID,
		TargetID: mNew.ID,
		RelType:  types.RelTypeContradicts,
		Strength: 0.95,
		Project:  project,
	}))

	return mNew, mOld
}

// TestAutoSupersede_NewerSupersedes verifies that when two memories have a
// contradicts edge and the newer is >24 h younger, AutoSupersede:
//  1. Creates a supersedes edge from newer → older.
//  2. Soft-deletes the older memory (ValidTo is set).
//  3. Returns count = 1.
func TestAutoSupersede_NewerSupersedes(t *testing.T) {
	project := uniqueProject("consolidate-supersede-basic")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	mNew, mOld := storeContradictingPair(t, ctx, backend, project, 48*time.Hour)

	count, err := runner.AutoSupersede(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "AutoSupersede must supersede exactly one memory")

	// Verify supersedes edge exists: newer → older.
	rels, err := backend.GetRelationships(ctx, project, mNew.ID)
	require.NoError(t, err)
	foundSupersedes := false
	for _, rel := range rels {
		if rel.RelType == types.RelTypeSupersedes && rel.SourceID == mNew.ID && rel.TargetID == mOld.ID {
			foundSupersedes = true
		}
	}
	assert.True(t, foundSupersedes, "a 'supersedes' edge from newer → older must exist after AutoSupersede")

	// Verify older memory is soft-deleted (ValidTo is set).
	oldMem, err := backend.GetMemory(ctx, mOld.ID)
	require.NoError(t, err)
	// GetMemory filters valid_to IS NULL — so a soft-deleted record returns nil.
	assert.Nil(t, oldMem, "older memory must be soft-deleted after AutoSupersede")
}

// TestAutoSupersede_WithinThreshold_NoAction verifies that when two contradicting
// memories are only 12 h apart, no supersession occurs.
func TestAutoSupersede_WithinThreshold_NoAction(t *testing.T) {
	project := uniqueProject("consolidate-supersede-threshold")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	_, mOld := storeContradictingPair(t, ctx, backend, project, 12*time.Hour)

	count, err := runner.AutoSupersede(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "AutoSupersede must not supersede memories within the 24 h threshold")

	// Older memory must still be active.
	oldMem, err := backend.GetMemory(ctx, mOld.ID)
	require.NoError(t, err)
	assert.NotNil(t, oldMem, "older memory must remain active when within the 24 h threshold")
}

// TestAutoSupersede_SkipsNonContradicts verifies that a relates_to edge between
// two memories is not treated as a supersession candidate.
func TestAutoSupersede_SkipsNonContradicts(t *testing.T) {
	project := uniqueProject("consolidate-supersede-skip")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	// Two memories with only a relates_to edge (no contradicts).
	m1 := &types.Memory{
		ID: types.NewMemoryID(), Content: "Go uses goroutines for concurrency",
		MemoryType: types.MemoryTypePattern, Project: project, Importance: 2, StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID: types.NewMemoryID(), Content: "Go goroutines are lightweight threads managed by the runtime",
		MemoryType: types.MemoryTypePattern, Project: project, Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))
	require.NoError(t, backend.StoreRelationship(ctx, &types.Relationship{
		ID: types.NewMemoryID(), SourceID: m1.ID, TargetID: m2.ID,
		RelType: types.RelTypeRelatesTo, Strength: 0.9, Project: project,
	}))

	count, err := runner.AutoSupersede(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "AutoSupersede must ignore non-contradicts edges")
}

// TestAutoSupersede_AlreadySuperseded verifies that a memory already soft-deleted
// is not superseded a second time.
func TestAutoSupersede_AlreadySuperseded(t *testing.T) {
	project := uniqueProject("consolidate-supersede-already")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 1024})

	mNew, mOld := storeContradictingPair(t, ctx, backend, project, 48*time.Hour)

	// Pre-soft-delete the older memory.
	ok, err := backend.SoftDeleteMemory(ctx, project, mOld.ID, "pre-deleted")
	require.NoError(t, err)
	require.True(t, ok)

	// AutoSupersede should be a no-op — the target is already gone.
	count, err := runner.AutoSupersede(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "AutoSupersede must skip already soft-deleted memories")

	// No new supersedes edge should appear on mNew.
	rels, err := backend.GetRelationships(ctx, project, mNew.ID)
	require.NoError(t, err)
	for _, rel := range rels {
		assert.NotEqual(t, types.RelTypeSupersedes, rel.RelType,
			"no supersedes edge must be created when older memory is already deleted")
	}
}
