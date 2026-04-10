package markdown_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/petersimmons1972/engram/internal/markdown"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func TestExportImportRoundtrip(t *testing.T) {
	dir := t.TempDir()
	memories := []*types.Memory{
		{
			ID:         types.NewMemoryID(),
			Content:    "TDD means test first.",
			MemoryType: types.MemoryTypePattern,
			Tags:       []string{"tdd", "testing"},
			Importance: 2,
		},
	}

	err := markdown.Export(memories, dir)
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	content, err := os.ReadFile(files[0])
	require.NoError(t, err)
	require.Contains(t, string(content), "TDD means test first.")
}
