package search_test

// Feature 6: Episodic Memory / Session Context Binding
// All tests written BEFORE implementation (TDD).
// They must fail (compile or runtime) until Feature 6 is implemented.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEpisode_StartEndPersists verifies that StartEpisode creates an episode
// record and EndEpisode sets ended_at and optional summary.
func TestEpisode_StartEndPersists(t *testing.T) {
	project := uniqueProject("ep-start-end")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	ep, err := backend.StartEpisode(ctx, project, "Auth migration session")
	require.NoError(t, err)
	require.NotEmpty(t, ep.ID, "episode must have an ID")
	assert.Equal(t, project, ep.Project)
	assert.Equal(t, "Auth migration session", ep.Description)
	assert.False(t, ep.StartedAt.IsZero(), "StartedAt must be set")
	assert.True(t, ep.EndedAt.IsZero(), "EndedAt must be zero before end")

	require.NoError(t, backend.EndEpisode(ctx, ep.ID, "Migrated JWT to session tokens"))

	eps, err := backend.ListEpisodes(ctx, project, 10)
	require.NoError(t, err)
	require.Len(t, eps, 1)
	assert.Equal(t, ep.ID, eps[0].ID)
	assert.False(t, eps[0].EndedAt.IsZero(), "EndedAt must be set after EndEpisode")
	assert.Equal(t, "Migrated JWT to session tokens", eps[0].Summary)
}

// TestEpisode_MemoriesAttach verifies that memories stored with an episode_id
// are returned by RecallEpisode in chronological order.
func TestEpisode_MemoriesAttach(t *testing.T) {
	project := uniqueProject("ep-recall")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	ep, err := backend.StartEpisode(ctx, project, "Feature planning session")
	require.NoError(t, err)

	// Store three memories attached to this episode, with deliberate time gaps.
	for i, content := range []string{
		"First: identified the problem",
		"Second: designed the solution",
		"Third: implementation started",
	} {
		m := &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     content,
			MemoryType:  types.MemoryTypeContext,
			Project:     project,
			Importance:  2,
			StorageMode: "focused",
			EpisodeID:   ep.ID,
		}
		require.NoError(t, backend.StoreMemory(ctx, m))
		if i < 2 {
			time.Sleep(2 * time.Millisecond) // ensure distinct created_at ordering
		}
	}

	// Store one memory WITHOUT episode — must not appear.
	other := &types.Memory{
		ID: types.NewMemoryID(), Content: "Unrelated memory",
		MemoryType: types.MemoryTypeContext, Project: project, Importance: 1, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, other))

	memories, err := backend.RecallEpisode(ctx, ep.ID)
	require.NoError(t, err)
	assert.Len(t, memories, 3, "only memories with this episode_id must be returned")
	for _, m := range memories {
		assert.Equal(t, ep.ID, m.EpisodeID, "all returned memories must have the episode_id")
	}
	// Verify chronological order (created_at ascending).
	for i := 1; i < len(memories); i++ {
		assert.True(t, !memories[i].CreatedAt.Before(memories[i-1].CreatedAt),
			"memories must be in chronological order")
	}
}

// TestEpisode_ListMultiple verifies that ListEpisodes returns all episodes for
// a project and respects the limit parameter.
func TestEpisode_ListMultiple(t *testing.T) {
	project := uniqueProject("ep-list")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	for i := range 5 {
		_, err := backend.StartEpisode(ctx, project, fmt.Sprintf("Session %d", i))
		require.NoError(t, err)
	}

	all, err := backend.ListEpisodes(ctx, project, 10)
	require.NoError(t, err)
	assert.Len(t, all, 5, "must return all 5 episodes")

	limited, err := backend.ListEpisodes(ctx, project, 3)
	require.NoError(t, err)
	assert.Len(t, limited, 3, "limit must be respected")
}
