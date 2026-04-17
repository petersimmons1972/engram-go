package mcp

// Internal tests for the Tier-1 / Tier-2 document-ingest core. They exercise
// execStoreDocument with stub collaborators so the routing and synopsis
// behaviour is verified without a PostgreSQL or Ollama dependency.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
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

// ── handleMemoryIngestDocumentStream: registry + action dispatch ──────────────
// These tests exercise start/append validation and the uploadRegistry's TTL,
// cap, and mutex without needing a live EnginePool. The finish action routes
// into runStreamIngest which requires a real SearchEngine, covered by the
// integration tests in e2e.

// resetUploadRegistry wipes the process-global registry between tests.
func resetUploadRegistry(t *testing.T) {
	t.Helper()
	uploadRegistryMu.Lock()
	defer uploadRegistryMu.Unlock()
	for k := range uploadRegistry {
		delete(uploadRegistry, k)
	}
}

func streamReq(args map[string]any) mcpgo.CallToolRequest {
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// resultMap parses the JSON body of a tool result.
func resultMap(t *testing.T, r *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	require.NotNil(t, r)
	require.NotEmpty(t, r.Content)
	tc, ok := r.Content[0].(mcpgo.TextContent)
	require.Truef(t, ok, "expected TextContent, got %T", r.Content[0])
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &out))
	return out
}

// Test A: chunked upload happy path up through append. finish requires a live
// engine and is covered by e2e tests — this test stops after the second append
// and verifies the registry state is exactly what finish would consume.
func TestHandleStream_ChunkedUpload_AppendHappyPath(t *testing.T) {
	resetUploadRegistry(t)
	ctx := context.Background()

	// start
	out, err := handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "start", "upload_id": "uA", "project": "p"}),
		Config{})
	require.NoError(t, err)
	require.Equal(t, "started", resultMap(t, out)["status"])

	// append part 0
	part0 := base64.StdEncoding.EncodeToString([]byte("hello "))
	out, err = handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "append", "upload_id": "uA", "part": float64(0), "data": part0}),
		Config{})
	require.NoError(t, err)
	m := resultMap(t, out)
	require.Equal(t, float64(1), m["parts_received"])
	require.Equal(t, float64(6), m["bytes_received"])

	// append part 1
	part1 := base64.StdEncoding.EncodeToString([]byte("world"))
	out, err = handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "append", "upload_id": "uA", "part": float64(1), "data": part1}),
		Config{})
	require.NoError(t, err)
	m = resultMap(t, out)
	require.Equal(t, float64(2), m["parts_received"])
	require.Equal(t, float64(11), m["bytes_received"])

	// Session should hold the combined buffer, ready for finish.
	sess, err := lookupUploadSession("uA")
	require.NoError(t, err)
	require.Equal(t, "hello world", string(sess.buf))
}

// Test B: out-of-order part is rejected.
func TestHandleStream_OutOfOrderPart(t *testing.T) {
	resetUploadRegistry(t)
	ctx := context.Background()

	_, err := handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "start", "upload_id": "uB", "project": "p"}),
		Config{})
	require.NoError(t, err)

	data := base64.StdEncoding.EncodeToString([]byte("x"))
	_, err = handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "append", "upload_id": "uB", "part": float64(1), "data": data}),
		Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 0")
}

// Test C: size overflow rejects append once accumulated bytes cross rawMax.
func TestHandleStream_SizeOverflow(t *testing.T) {
	resetUploadRegistry(t)
	ctx := context.Background()

	// Tight cap so the test finishes fast.
	cfg := Config{MaxDocumentBytes: 1024, RawDocumentMaxBytes: 16}

	_, err := handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "start", "upload_id": "uC", "project": "p"}),
		cfg)
	require.NoError(t, err)

	big := base64.StdEncoding.EncodeToString(make([]byte, 32)) // 32 bytes > 16 cap
	_, err = handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "append", "upload_id": "uC", "part": float64(0), "data": big}),
		cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds maximum size")
}

// Test D: upload_id length validation on start.
func TestHandleStream_UploadIDValidation(t *testing.T) {
	resetUploadRegistry(t)
	ctx := context.Background()

	// empty
	_, err := handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "start", "upload_id": "", "project": "p"}),
		Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "upload_id")

	// 129 chars (one over the cap)
	tooLong := strings.Repeat("a", maxUploadIDLen+1)
	_, err = handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "start", "upload_id": tooLong, "project": "p"}),
		Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "upload_id")

	// valid
	valid := strings.Repeat("a", maxUploadIDLen)
	_, err = handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "start", "upload_id": valid, "project": "p"}),
		Config{})
	require.NoError(t, err)
}

// Test E: session cap.
func TestHandleStream_TooManySessions(t *testing.T) {
	resetUploadRegistry(t)
	ctx := context.Background()

	// Fill the registry directly to avoid making maxUploadSessions handler calls.
	uploadRegistryMu.Lock()
	now := time.Now()
	for i := 0; i < maxUploadSessions; i++ {
		id := fmt.Sprintf("sess-%d", i)
		uploadRegistry[id] = &uploadSession{project: "p", createdAt: now}
	}
	uploadRegistryMu.Unlock()

	_, err := handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "start", "upload_id": "overflow", "project": "p"}),
		Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "too many in-progress uploads")
}

// TTL eviction: stale sessions are dropped before the cap check, freeing slots.
func TestHandleStream_TTLEviction(t *testing.T) {
	resetUploadRegistry(t)
	ctx := context.Background()

	uploadRegistryMu.Lock()
	stale := time.Now().Add(-(uploadSessionTTL + time.Minute))
	for i := 0; i < maxUploadSessions; i++ {
		id := fmt.Sprintf("stale-%d", i)
		uploadRegistry[id] = &uploadSession{project: "p", createdAt: stale}
	}
	uploadRegistryMu.Unlock()

	// All stale — a fresh start should succeed because eviction frees the slots.
	_, err := handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "start", "upload_id": "fresh", "project": "p"}),
		Config{})
	require.NoError(t, err)

	uploadRegistryMu.Lock()
	_, ok := uploadRegistry["fresh"]
	count := len(uploadRegistry)
	uploadRegistryMu.Unlock()
	require.True(t, ok, "fresh session should be registered")
	require.Equal(t, 1, count, "all stale sessions should have been evicted")
}

// Unknown action is rejected.
func TestHandleStream_UnknownAction(t *testing.T) {
	resetUploadRegistry(t)
	_, err := handleMemoryIngestDocumentStream(context.Background(), nil,
		streamReq(map[string]any{"action": "upload", "upload_id": "x"}),
		Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown action")
}

// Append against an unknown upload_id (e.g. expired) is rejected.
func TestHandleStream_AppendUnknownUpload(t *testing.T) {
	resetUploadRegistry(t)
	data := base64.StdEncoding.EncodeToString([]byte("x"))
	_, err := handleMemoryIngestDocumentStream(context.Background(), nil,
		streamReq(map[string]any{"action": "append", "upload_id": "ghost", "part": float64(0), "data": data}),
		Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown upload_id")
}

// TestEngineStorerAdapter covers the tiny trampoline used to plug a
// *search.SearchEngine into the memoryStorer interface. No SearchEngine is
// needed — the adapter just forwards to the supplied func.
func TestEngineStorerAdapter(t *testing.T) {
	var gotRaw string
	var gotID string
	a := engineStorerAdapter{store: func(_ context.Context, m *types.Memory, rawBody string) error {
		gotID = m.ID
		gotRaw = rawBody
		return nil
	}}
	err := a.StoreWithRawBody(context.Background(), &types.Memory{ID: "m1"}, "raw")
	require.NoError(t, err)
	require.Equal(t, "m1", gotID)
	require.Equal(t, "raw", gotRaw)
}

// TestBackendDocumentAdapter covers the db.Backend → documentStorer adapter.
// backendStub satisfies enough of db.Backend for StoreDocument and
// SetMemoryDocumentID to be exercised.
type backendStubForAdapter struct {
	db.Backend
	storeCalls  int
	linkCalls   int
	lastProject string
	lastMem     string
	lastDoc     string
}

func (b *backendStubForAdapter) StoreDocument(_ context.Context, project, _ string) (string, error) {
	b.storeCalls++
	b.lastProject = project
	return "doc-id", nil
}

func (b *backendStubForAdapter) SetMemoryDocumentID(_ context.Context, memID, docID string) error {
	b.linkCalls++
	b.lastMem = memID
	b.lastDoc = docID
	return nil
}

func TestBackendDocumentAdapter(t *testing.T) {
	bs := &backendStubForAdapter{}
	a := backendDocumentAdapter{b: bs}
	id, err := a.StoreDocument(context.Background(), "proj", "body")
	require.NoError(t, err)
	require.Equal(t, "doc-id", id)
	require.Equal(t, 1, bs.storeCalls)
	require.Equal(t, "proj", bs.lastProject)

	err = a.SetMemoryDocumentID(context.Background(), "mem-1", "doc-1")
	require.NoError(t, err)
	require.Equal(t, 1, bs.linkCalls)
	require.Equal(t, "mem-1", bs.lastMem)
	require.Equal(t, "doc-1", bs.lastDoc)
}

// TestRunStreamIngest_PoolError exercises runStreamIngest's error path when
// the engine pool cannot produce a handle. This drives the function's
// opening statements (pool.Get + error return) so the coverage profile
// no longer shows 0% for runStreamIngest. The happy path requires a live
// SearchEngine and is covered by e2e tests.
func TestRunStreamIngest_PoolError(t *testing.T) {
	pool := NewEnginePool(func(_ context.Context, _ string) (*EngineHandle, error) {
		return nil, fmt.Errorf("factory refused")
	})
	_, err := runStreamIngest(context.Background(), pool, "p", "body", Config{}, 8*1024*1024, 50*1024*1024)
	require.Error(t, err)
	require.Contains(t, err.Error(), "factory refused")
}

// TestHandleStream_FinishProjectMismatch verifies the A4 project-isolation
// guard: finish called under a different project than start must be
// rejected so a caller cannot silently park a body in the wrong project.
func TestHandleStream_FinishProjectMismatch(t *testing.T) {
	resetUploadRegistry(t)
	ctx := context.Background()

	_, err := handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "start", "upload_id": "mismatch", "project": "A"}),
		Config{})
	require.NoError(t, err)

	_, err = handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "finish", "upload_id": "mismatch", "project": "B"}),
		Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "project mismatch")
}

// TestHandleStream_AppendProjectMismatch: same guard on the append path.
func TestHandleStream_AppendProjectMismatch(t *testing.T) {
	resetUploadRegistry(t)
	ctx := context.Background()

	_, err := handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "start", "upload_id": "mm-app", "project": "A"}),
		Config{})
	require.NoError(t, err)

	data := base64.StdEncoding.EncodeToString([]byte("x"))
	_, err = handleMemoryIngestDocumentStream(ctx, nil,
		streamReq(map[string]any{"action": "append", "upload_id": "mm-app", "part": float64(0), "data": data, "project": "B"}),
		Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "project mismatch")
}

// TestBuildSynopsis_UTF8BoundarySafe verifies we never return a synopsis
// that ends mid-rune. A single 3-byte rune placed straddling the cut point
// must be dropped (or the full rune preserved) — never split.
func TestBuildSynopsis_UTF8BoundarySafe(t *testing.T) {
	// Build content where the byte at synopsisPrefixBytes lands inside a
	// multi-byte rune. "€" is 3 bytes (0xE2 0x82 0xAC). Pad ASCII so the
	// euro sign starts at byte (synopsisPrefixBytes - 1) — the naive byte
	// cut would slice between bytes 1 and 2 of the rune.
	pad := strings.Repeat("a", synopsisPrefixBytes-1)
	content := pad + "€" + strings.Repeat("b", 1000)

	syn := buildSynopsis(content)
	require.True(t, len(syn) < len(content))
	// The returned prefix must be valid UTF-8 end-to-end. strings.ToValidUTF8
	// is a no-op on valid input; if any byte is a lone continuation, the
	// result shrinks. We compare lengths to detect that case.
	require.Equal(t, len([]rune(syn)), len([]rune(strings.ToValidUTF8(syn, ""))),
		"synopsis must not end mid-rune")
}

// Fix 4: Tier-1 response carries "summary" (string) not "summary_bytes" (int).
func TestExecStoreDocument_Tier1_ReturnsSummaryText(t *testing.T) {
	const maxDoc = 8 * 1024 * 1024
	const rawMax = 50 * 1024 * 1024

	eng := &stubEngine{}
	back := newStubDocBackend()
	m := &types.Memory{ID: "m-sum", Project: "p", MemoryType: types.MemoryTypeContext}
	content := "# Top\n" + makeContent(600_000)

	out, err := execStoreDocument(context.Background(), storeDocumentDeps{engine: eng, backend: back}, m, content, maxDoc, rawMax)
	require.NoError(t, err)

	summary, ok := out["summary"].(string)
	require.True(t, ok, "Tier-1 response must carry 'summary' as a string, not a byte count")
	require.NotEmpty(t, summary)
	require.Less(t, len(summary), len(content))
	_, hadBytes := out["summary_bytes"]
	require.False(t, hadBytes, "legacy 'summary_bytes' field should be gone")
}
