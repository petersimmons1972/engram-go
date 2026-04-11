package mcp_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/stretchr/testify/require"
)

func TestSafePath_AllowsInsideBase(t *testing.T) {
	base := t.TempDir()
	got, err := internalmcp.SafePath(base, base+"/subdir/file.md")
	require.NoError(t, err)
	require.Contains(t, got, "subdir")
}

func TestSafePath_BlocksTraversal(t *testing.T) {
	base := t.TempDir()
	_, err := internalmcp.SafePath(base, base+"/../../etc/passwd")
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes allowed directory")
}

func TestSafePath_BlocksDotDotRelative(t *testing.T) {
	base := t.TempDir()
	// A relative traversal that resolves outside base.
	_, err := internalmcp.SafePath(base, "../../../../etc/passwd")
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes allowed directory")
}

func TestSafePath_AllowsBaseItself(t *testing.T) {
	base := t.TempDir()
	got, err := internalmcp.SafePath(base, base)
	require.NoError(t, err)
	require.Equal(t, base, got)
}

func TestSafePath_EmptyBaseReturnsError(t *testing.T) {
	// When no DataDir is configured, SafePath must refuse the path entirely.
	_, err := internalmcp.SafePath("", "/tmp/anything")
	require.Error(t, err)
	require.Contains(t, err.Error(), "base directory is not configured")
}

func TestSafePath_BlocksSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()

	// Create a symlink inside base that points outside base.
	link := base + "/evil"
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("cannot create symlink:", err)
	}

	_, err := internalmcp.SafePath(base, link+"/secret.txt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes allowed directory")
}

func TestEnginePool_LRU_EvictsAtCap(t *testing.T) {
	const cap = 50
	created := make(map[string]int)
	pool := internalmcp.NewEnginePool(func(ctx context.Context, project string) (*internalmcp.EngineHandle, error) {
		created[project]++
		return &internalmcp.EngineHandle{}, nil
	})
	ctx := context.Background()

	// Fill pool to capacity.
	for i := range cap {
		project := fmt.Sprintf("proj-%d", i)
		_, err := pool.Get(ctx, project)
		require.NoError(t, err)
	}
	// One more project must succeed (evicts LRU).
	_, err := pool.Get(ctx, "proj-overflow")
	require.NoError(t, err)
}
