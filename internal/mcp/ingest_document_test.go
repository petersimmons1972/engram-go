package mcp

// Internal tests for the Tier-1 / Tier-2 document-ingest core. They exercise
// execStoreDocument with stub collaborators so the routing and synopsis
// behaviour is verified without a PostgreSQL or Ollama dependency.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// stubEngine captures the last StoreWithRawBody call so tests can assert the
// split between synopsis (Memory.Content) and raw body (chunk source).
type stubEngine struct {
	calls []struct {
		mem     types.Memory
		rawBody string
	}
	err error
}

func (s *stubEngine) StoreWithRawBody(_ context.Context, m *types.Memory, rawBody string) error {
	if s.err != nil {
		return s.err
	}
	s.calls = append(s.calls, struct {
		mem     types.Memory
		rawBody string
	}{mem: *m, rawBody: rawBody})
	return nil
}

type stubDocBackend struct {
	stored   map[string]string // docID -> content
	linked   map[string]string // memID -> docID
	storeErr error
	linkErr  error
	nextID   int
}

func newStubDocBackend() *stubDocBackend {
	return &stubDocBackend{stored: map[string]string{}, linked: map[string]string{}}
}

func (s *stubDocBackend) StoreDocument(_ context.Context, _ string, content string) (string, error) {
	if s.storeErr != nil {
		return "", s.storeErr
	}
	s.nextID++
	id := fmt.Sprintf("doc-%d", s.nextID)
	s.stored[id] = content
	return id, nil
}

func (s *stubDocBackend) SetMemoryDocumentID(_ context.Context, memID, docID string) error {
	if s.linkErr != nil {
		return s.linkErr
	}
	s.linked[memID] = docID
	return nil
}

// ── classifyDocumentSize ─────────────────────────────────────────────────────

func TestClassifyDocumentSize(t *testing.T) {
	const (
		maxDoc = 8 * 1024 * 1024
		rawMax = 50 * 1024 * 1024
	)
	cases := []struct {
		size int
		want Tier
	}{
		{0, TierSmall},
		{types.MaxContentLength, TierSmall},
		{types.MaxContentLength + 1, TierStreamSynopsis},
		{maxDoc, TierStreamSynopsis},
		{maxDoc + 1, TierRawDocument},
		{rawMax, TierRawDocument},
		{rawMax + 1, TierReject},
	}
	for _, tc := range cases {
		got := classifyDocumentSize(tc.size, maxDoc, rawMax)
		require.Equalf(t, tc.want, got, "size=%d", tc.size)
	}
}

// ── buildSynopsis ────────────────────────────────────────────────────────────

func TestBuildSynopsis_SmallContent_Passthrough(t *testing.T) {
	in := "# Title\n\nShort content, well under 8 KiB."
	require.Equal(t, in, buildSynopsis(in))
}

func TestBuildSynopsis_LargeContent_TruncatesAndExtractsHeadings(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Intro\n")
	b.WriteString(strings.Repeat("x", synopsisPrefixBytes))
	// Headings beyond the 8 KiB prefix — must still appear in the outline.
	b.WriteString("\n## Deep section\n")
	b.WriteString("## Another\n")
	syn := buildSynopsis(b.String())

	require.LessOrEqual(t, len(syn), synopsisPrefixBytes+len("\n\n--- Outline ---\n")+synopsisHeadingBytes+8)
	require.Contains(t, syn, "--- Outline ---")
	require.Contains(t, syn, "## Deep section")
	require.Contains(t, syn, "## Another")
}

func TestBuildSynopsis_HeadingBudgetCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString(strings.Repeat("x", synopsisPrefixBytes))
	// Many headings: should stop once the outline crosses 2 KiB.
	for i := 0; i < 1000; i++ {
		b.WriteString(fmt.Sprintf("\n## Heading number %d with some padding text", i))
	}
	syn := buildSynopsis(b.String())
	// Outline section must be ≤ synopsisHeadingBytes.
	marker := "--- Outline ---\n"
	idx := strings.Index(syn, marker)
	require.Greater(t, idx, 0)
	outline := syn[idx+len(marker):]
	require.LessOrEqual(t, len(outline), synopsisHeadingBytes)
}

// ── execStoreDocument routing ────────────────────────────────────────────────

func makeContent(n int) string { return strings.Repeat("a", n) }

func TestHandleMemoryStoreDocument_SmallContent(t *testing.T) {
	const maxDoc = 8 * 1024 * 1024
	const rawMax = 50 * 1024 * 1024

	eng := &stubEngine{}
	back := newStubDocBackend()
	m := &types.Memory{ID: "m-small", Project: "p", MemoryType: types.MemoryTypeContext}
	content := makeContent(100_000) // < 500 KB

	out, err := execStoreDocument(context.Background(), storeDocumentDeps{engine: eng, backend: back}, m, content, maxDoc, rawMax)
	require.NoError(t, err)
	require.Equal(t, "m-small", out["id"])
	require.Equal(t, "document", out["mode"])
	require.Equal(t, "stored", out["status"])

	require.Len(t, eng.calls, 1)
	require.Equal(t, content, eng.calls[0].mem.Content, "small content stored verbatim as Memory.Content")
	require.Empty(t, eng.calls[0].rawBody, "small content: rawBody must be empty so existing pipeline runs")
	require.Empty(t, back.stored, "small content must not touch documents table")
}

func TestHandleMemoryStoreDocument_Tier1_LargeContent(t *testing.T) {
	const maxDoc = 8 * 1024 * 1024
	const rawMax = 50 * 1024 * 1024

	eng := &stubEngine{}
	back := newStubDocBackend()
	m := &types.Memory{ID: "m-tier1", Project: "p", MemoryType: types.MemoryTypeContext}
	content := "# Top\n" + makeContent(600_000) // > 500 KB, < 8 MB

	out, err := execStoreDocument(context.Background(), storeDocumentDeps{engine: eng, backend: back}, m, content, maxDoc, rawMax)
	require.NoError(t, err)
	require.Equal(t, "document_synopsis", out["mode"])
	require.Equal(t, len(content), out["size_bytes"])

	require.Len(t, eng.calls, 1)
	stored := eng.calls[0].mem
	require.Less(t, len(stored.Content), len(content), "Tier-1: synopsis must be smaller than raw body")
	require.LessOrEqual(t, len(stored.Content), synopsisPrefixBytes+len("\n\n--- Outline ---\n")+synopsisHeadingBytes+8)
	require.Equal(t, content, eng.calls[0].rawBody, "Tier-1: rawBody must be the full content so chunks stay grounded")
	require.Empty(t, back.stored, "Tier-1 must not park raw content in documents table")
}

func TestHandleMemoryStoreDocument_Tier2_HugeContent(t *testing.T) {
	const maxDoc = 8 * 1024 * 1024
	const rawMax = 50 * 1024 * 1024

	eng := &stubEngine{}
	back := newStubDocBackend()
	m := &types.Memory{ID: "m-tier2", Project: "p", MemoryType: types.MemoryTypeContext}
	content := "# Huge doc\n" + makeContent(9*1024*1024) // > 8 MB, < 50 MB

	out, err := execStoreDocument(context.Background(), storeDocumentDeps{engine: eng, backend: back}, m, content, maxDoc, rawMax)
	require.NoError(t, err)
	require.Equal(t, "raw_document", out["mode"])
	require.NotEmpty(t, out["document_id"])
	require.Equal(t, len(content), out["size_bytes"])

	require.Len(t, back.stored, 1, "Tier-2: raw body parked in documents table")
	var docID string
	for k := range back.stored {
		docID = k
	}
	require.Equal(t, content, back.stored[docID])
	require.Equal(t, docID, back.linked["m-tier2"], "Tier-2: memory must be linked to the document")

	require.Len(t, eng.calls, 1)
	stored := eng.calls[0].mem
	require.Less(t, len(stored.Content), len(content))
	require.Empty(t, eng.calls[0].rawBody, "Tier-2: raw documents are not chunked inline — rawBody must be empty")
	require.Equal(t, docID, stored.DocumentID)
}

func TestHandleMemoryStoreDocument_TooLarge(t *testing.T) {
	const maxDoc = 8 * 1024 * 1024
	const rawMax = 50 * 1024 * 1024

	eng := &stubEngine{}
	back := newStubDocBackend()
	m := &types.Memory{ID: "m-huge", Project: "p", MemoryType: types.MemoryTypeContext}
	content := makeContent(rawMax + 1)

	_, err := execStoreDocument(context.Background(), storeDocumentDeps{engine: eng, backend: back}, m, content, maxDoc, rawMax)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds maximum size")
	require.Empty(t, eng.calls)
	require.Empty(t, back.stored)
}

func TestConfigOrDefaults_ZeroFallback(t *testing.T) {
	maxDoc, rawMax := configOrDefaults(Config{})
	require.Equal(t, 8*1024*1024, maxDoc)
	require.Equal(t, 50*1024*1024, rawMax)
}

func TestConfigOrDefaults_RespectsSetValues(t *testing.T) {
	maxDoc, rawMax := configOrDefaults(Config{MaxDocumentBytes: 1, RawDocumentMaxBytes: 2})
	require.Equal(t, 1, maxDoc)
	require.Equal(t, 2, rawMax)
}
