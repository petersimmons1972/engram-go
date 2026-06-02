package markdown_test

import (
	"os"
	"path/filepath"
	"strings"
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

// ---------------------------------------------------------------------------
// ImportClaudeMD
// ---------------------------------------------------------------------------

func TestImportClaudeMD_SingleSection(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "CLAUDE.md")
	body := "## Core Principles\n\nSimplicity first.\n"
	require.NoError(t, os.WriteFile(f, []byte(body), 0o644))

	mems, err := markdown.ImportClaudeMD(f)
	require.NoError(t, err)
	require.Len(t, mems, 1)
	require.Contains(t, mems[0].Content, "Core Principles")
	require.Contains(t, mems[0].Content, "Simplicity first.")
}

func TestImportClaudeMD_MultiSection(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "CLAUDE.md")
	body := `## Section A

Content A.

## Section B

Content B.
`
	require.NoError(t, os.WriteFile(f, []byte(body), 0o644))

	mems, err := markdown.ImportClaudeMD(f)
	require.NoError(t, err)
	require.Len(t, mems, 2)

	found := make(map[string]bool)
	for _, m := range mems {
		if strings.Contains(m.Content, "Content A") {
			found["A"] = true
		}
		if strings.Contains(m.Content, "Content B") {
			found["B"] = true
		}
	}
	require.True(t, found["A"], "expected Section A content")
	require.True(t, found["B"], "expected Section B content")
}

func TestImportClaudeMD_WithFrontMatter(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "CLAUDE.md")
	// ImportClaudeMD calls splitSections which does NOT strip front matter.
	// Content before the first ## heading (including the front matter block) becomes
	// the first section. "## Only Section" becomes the second section.
	body := "---\ntitle: test\n---\n\n## Only Section\n\nBody text.\n"
	require.NoError(t, os.WriteFile(f, []byte(body), 0o644))

	mems, err := markdown.ImportClaudeMD(f)
	require.NoError(t, err)
	// Front matter block (before first ##) produces one section; "Only Section" is the second.
	require.Len(t, mems, 2)
	found := false
	for _, m := range mems {
		if strings.Contains(m.Content, "Only Section") {
			found = true
		}
	}
	require.True(t, found, "expected a section containing 'Only Section'")
}

func TestImportClaudeMD_NoSections(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "CLAUDE.md")
	body := "Just plain text, no headings.\n"
	require.NoError(t, os.WriteFile(f, []byte(body), 0o644))

	mems, err := markdown.ImportClaudeMD(f)
	require.NoError(t, err)
	// No ## headings → one memory containing the whole document.
	require.Len(t, mems, 1)
	require.Contains(t, mems[0].Content, "Just plain text")
}

func TestImportClaudeMD_FileNotFound(t *testing.T) {
	_, err := markdown.ImportClaudeMD("/nonexistent/CLAUDE.md")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ImportClaudeMD")
}

// ---------------------------------------------------------------------------
// Ingest
// ---------------------------------------------------------------------------

func TestIngest_SingleFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "note.md")
	require.NoError(t, os.WriteFile(f, []byte("Hello world.\n"), 0o644))

	mems, err := markdown.Ingest(dir)
	require.NoError(t, err)
	require.Len(t, mems, 1)
	// Ingest calls stripFrontMatter which does not trim plain text; trailing newline preserved.
	require.Contains(t, mems[0].Content, "Hello world.")
	require.Equal(t, types.MemoryTypeContext, mems[0].MemoryType)
	require.Equal(t, 2, mems[0].Importance)
	require.Equal(t, "document", mems[0].StorageMode)
}

func TestIngest_SkipsNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.md"), []byte("MD content.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("Txt content.\n"), 0o644))

	mems, err := markdown.Ingest(dir)
	require.NoError(t, err)
	require.Len(t, mems, 1)
	require.Contains(t, mems[0].Content, "MD content.")
}

func TestIngest_StripsFrontMatter(t *testing.T) {
	dir := t.TempDir()
	content := "---\nid: abc\ntags: [test]\n---\n\nBody after front matter.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(content), 0o644))

	mems, err := markdown.Ingest(dir)
	require.NoError(t, err)
	require.Len(t, mems, 1)
	require.NotContains(t, mems[0].Content, "id: abc")
	require.Contains(t, mems[0].Content, "Body after front matter.")
}

func TestIngest_Recursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "top.md"), []byte("Top.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "inner.md"), []byte("Inner.\n"), 0o644))

	mems, err := markdown.Ingest(dir)
	require.NoError(t, err)
	require.Len(t, mems, 2)
}

func TestIngest_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	mems, err := markdown.Ingest(dir)
	require.NoError(t, err)
	require.Empty(t, mems)
}

// ---------------------------------------------------------------------------
// Dump (delegates to Export)
// ---------------------------------------------------------------------------

func TestDump_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	id := types.NewMemoryID()
	mems := []*types.Memory{
		{
			ID:         id,
			Content:    "Dump test content.",
			MemoryType: types.MemoryTypeContext,
			Importance: 3,
		},
	}

	require.NoError(t, markdown.Dump(mems, dir))

	written, err := os.ReadFile(filepath.Join(dir, id+".md"))
	require.NoError(t, err)
	require.Contains(t, string(written), "Dump test content.")
	require.Contains(t, string(written), "importance: 3")
}

func TestDump_SkipsNilAndEmptyID(t *testing.T) {
	dir := t.TempDir()
	mems := []*types.Memory{
		nil,
		{ID: "", Content: "no id"},
	}
	require.NoError(t, markdown.Dump(mems, dir))

	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	require.NoError(t, err)
	require.Empty(t, files)
}

// ---------------------------------------------------------------------------
// stripFrontMatter (exercised via Ingest)
// ---------------------------------------------------------------------------

func TestStripFrontMatter_NoFrontMatter(t *testing.T) {
	dir := t.TempDir()
	body := "No front matter here.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plain.md"), []byte(body), 0o644))

	mems, err := markdown.Ingest(dir)
	require.NoError(t, err)
	require.Len(t, mems, 1)
	// No front matter → stripFrontMatter returns input unchanged (including trailing newline).
	require.Contains(t, mems[0].Content, "No front matter here.")
}

func TestStripFrontMatter_MalformedFrontMatter(t *testing.T) {
	// Starts with --- but has no closing ---; should return unchanged.
	dir := t.TempDir()
	body := "---\nunterminated: true\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "malformed.md"), []byte(body), 0o644))

	mems, err := markdown.Ingest(dir)
	require.NoError(t, err)
	require.Len(t, mems, 1)
	// Malformed front matter is left as-is.
	require.Contains(t, mems[0].Content, "---")
}
