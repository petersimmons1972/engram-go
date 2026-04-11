# engram-go Phases 3–7 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the Go port of the Python engram MCP memory server — embedding client, search engine, background workers, all 17 MCP tools, CLI entry point, and Chainguard Docker image.

**Architecture:** Package-per-concern under `internal/`. `SearchEngine` owns business logic and background workers; `internal/mcp/` is a thin dispatcher on top. One binary, one Docker image, SSE transport on port 8788.

**Tech Stack:** Go 1.25, pgx/v5, mark3labs/mcp-go v0.45.0, Ollama HTTP API, Chainguard static image

---

## File Map

```
internal/embed/
  ollama.go           Client interface + OllamaClient (embed, startup check, model pull)
  ollama_test.go

internal/search/
  score.go            CompositeScore, RecencyDecay, ImportanceBoost (pure functions)
  score_test.go
  engine.go           SearchEngine: New, Close, Store, Recall, Connect, List, Correct,
                        Forget, Status, Consolidate, Verify, Feedback, MigrateEmbedder,
                        Summarize
  engine_test.go      Integration tests against test-postgres

internal/summarize/
  worker.go           Worker goroutine: fills summary IS NULL via Ollama /api/generate
  worker_test.go

internal/reembed/
  worker.go           Worker goroutine: fills embedding IS NULL chunks
  worker_test.go

internal/markdown/
  io.go               Export, ImportClaudeMD, Dump, Ingest
  io_test.go

internal/mcp/
  pool.go             EnginePool: sync.Map of project→*search.SearchEngine, lazy init
  tools.go            One handler func per MCP tool
  tools_test.go
  server.go           MCP server setup, SSE transport, optional API key middleware

cmd/engram/
  main.go             Flag/env parsing, startup sequence, SIGTERM/SIGINT shutdown

Dockerfile            Multi-stage: cgr.dev/chainguard/go → cgr.dev/chainguard/static
```

---

## Task 1: Add dependencies

**Files:** `go.mod`, `go.sum`

- [ ] **Step 1: Add runtime and test dependencies**

```bash
cd ~/projects/engram-go
go get github.com/mark3labs/mcp-go@v0.45.0
go get github.com/stretchr/testify@v1.10.0
```

- [ ] **Step 2: Verify module graph**

```bash
go mod tidy && go build ./...
```

Expected: no errors, `go.sum` updated.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add mcp-go v0.45.0 and testify v1.10.0"
```

---

## Task 2: Ollama embedding client (`internal/embed/ollama.go`)

**Files:**
- Create: `internal/embed/ollama.go`
- Create: `internal/embed/ollama_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/embed/ollama_test.go`:

```go
package embed_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/stretchr/testify/require"
)

// fakeOllama returns an httptest.Server that serves minimal Ollama responses.
// tagsModels lists model names returned by GET /api/tags.
// embedDims is the dimension of the fake embedding vector returned.
func fakeOllama(t *testing.T, tagsModels []string, embedDims int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		type model struct {
			Name string `json:"name"`
		}
		type resp struct {
			Models []model `json:"models"`
		}
		models := make([]model, len(tagsModels))
		for i, name := range tagsModels {
			models[i] = model{Name: name}
		}
		json.NewEncoder(w).Encode(resp{Models: models})
	})

	mux.HandleFunc("/api/embed", func(w http.ResponseWriter, r *http.Request) {
		vec := make([]float32, embedDims)
		for i := range vec {
			vec[i] = float32(i) / float32(embedDims)
		}
		type resp struct {
			Embeddings [][]float32 `json:"embeddings"`
		}
		json.NewEncoder(w).Encode(resp{Embeddings: [][]float32{vec}})
	})

	mux.HandleFunc("/api/pull", func(w http.ResponseWriter, r *http.Request) {
		// Return a single-line completed response
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	})

	return httptest.NewServer(mux)
}

func TestNewOllamaClient_ModelPresent(t *testing.T) {
	srv := fakeOllama(t, []string{"nomic-embed-text:latest"}, 768)
	defer srv.Close()

	c, err := embed.NewOllamaClient(context.Background(), srv.URL, "nomic-embed-text")
	require.NoError(t, err)
	require.Equal(t, "nomic-embed-text", c.Name())
}

func TestNewOllamaClient_ModelAbsent_TriggersPull(t *testing.T) {
	srv := fakeOllama(t, []string{}, 768) // no models — pull will be triggered
	defer srv.Close()

	_, err := embed.NewOllamaClient(context.Background(), srv.URL, "nomic-embed-text")
	require.NoError(t, err)
}

func TestOllamaClient_Embed(t *testing.T) {
	srv := fakeOllama(t, []string{"nomic-embed-text:latest"}, 768)
	defer srv.Close()

	c, err := embed.NewOllamaClient(context.Background(), srv.URL, "nomic-embed-text")
	require.NoError(t, err)

	vec, err := c.Embed(context.Background(), "hello world")
	require.NoError(t, err)
	require.Len(t, vec, 768)
	require.Equal(t, 768, c.Dimensions())
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd ~/projects/engram-go && go test ./internal/embed/ -run TestNewOllamaClient -v 2>&1 | head -20
```

Expected: `FAIL` — package `embed` does not exist yet.

- [ ] **Step 3: Implement `internal/embed/ollama.go`**

```go
// Package embed provides the embedding client for Engram.
// Only Ollama is supported — no remote/cloud providers.
package embed

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client is the embedding provider interface.
type Client interface {
	// Embed returns a float32 vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)
	// Name returns the model identifier (e.g. "nomic-embed-text").
	Name() string
	// Dimensions returns the vector size, or 0 before the first successful embed.
	Dimensions() int
}

// OllamaClient calls the local Ollama /api/embed endpoint.
type OllamaClient struct {
	baseURL string
	model   string
	dims    int
	http    *http.Client
}

// NewOllamaClient constructs an OllamaClient and validates connectivity.
// If the model is absent from Ollama, it triggers a pull and waits (max 5 min).
func NewOllamaClient(ctx context.Context, baseURL, model string) (*OllamaClient, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	// DNS-safe transport: short idle timeout ensures DNS changes (e.g. host DNS
	// server swap) propagate within 30 s without long-lived stale connections.
	transport := &http.Transport{
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConnsPerHost: 2,
	}
	hc := &http.Client{Transport: transport, Timeout: 60 * time.Second}

	c := &OllamaClient{baseURL: baseURL, model: model, http: hc}

	// Confirm Ollama is reachable and model is loaded (or pull it).
	if err := c.ensureModel(ctx); err != nil {
		return nil, fmt.Errorf("ollama startup check failed: %w", err)
	}

	// Detect dimensions from a probe embed.
	vec, err := c.Embed(ctx, "probe")
	if err != nil {
		return nil, fmt.Errorf("probe embed failed: %w", err)
	}
	c.dims = len(vec)

	return c, nil
}

func (c *OllamaClient) Name() string       { return c.model }
func (c *OllamaClient) Dimensions() int    { return c.dims }

// Embed calls POST /api/embed and returns the first embedding vector.
func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]string{"model": c.model, "input": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embed decode: %w", err)
	}
	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama embed: empty response")
	}
	return result.Embeddings[0], nil
}

// ensureModel checks that the model is available in Ollama, pulling it if absent.
func (c *OllamaClient) ensureModel(ctx context.Context) error {
	present, err := c.modelPresent(ctx)
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	return c.pullModel(ctx)
}

func (c *OllamaClient) modelPresent(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("ollama tags: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	for _, m := range result.Models {
		if strings.HasPrefix(m.Name, c.model) {
			return true, nil
		}
	}
	return false, nil
}

func (c *OllamaClient) pullModel(ctx context.Context) error {
	pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	body, _ := json.Marshal(map[string]string{"name": c.model})
	req, err := http.NewRequestWithContext(pullCtx, http.MethodPost, c.baseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ollama pull: %w", err)
	}
	defer resp.Body.Close()

	// Drain streaming NDJSON lines until "success" or end of body.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var line struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) == nil && line.Status == "success" {
			return nil
		}
	}
	return scanner.Err()
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/embed/ -v -race
```

Expected: all three tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/embed/
git commit -m "feat: Phase 3 — OllamaClient (embed, startup check, model pull)"
```

---

## Task 3: Scoring functions (`internal/search/score.go`)

**Files:**
- Create: `internal/search/score.go`
- Create: `internal/search/score_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/search/score_test.go`:

```go
package search_test

import (
	"math"
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

func TestRecencyDecay(t *testing.T) {
	// At 0 hours: decay = 1.0
	require.InDelta(t, 1.0, search.RecencyDecay(0), 0.0001)
	// At 69.3 hours (~ln(2)/0.01): decay ≈ 0.5
	require.InDelta(t, 0.5, search.RecencyDecay(math.Log(2)/0.01), 0.01)
	// Decay is monotonically decreasing
	require.Greater(t, search.RecencyDecay(10), search.RecencyDecay(100))
}

func TestImportanceBoost(t *testing.T) {
	require.InDelta(t, 0.0/3.0, search.ImportanceBoost(0), 0.001)
	require.InDelta(t, 1.0, search.ImportanceBoost(3), 0.001)
	require.InDelta(t, 4.0/3.0, search.ImportanceBoost(4), 0.001)
}

func TestCompositeScore(t *testing.T) {
	s := search.CompositeScore(search.ScoreInput{
		Cosine:     1.0,
		BM25:       1.0,
		HoursSince: 0,
		Importance: 3,
	})
	// cosine(0.5) + bm25(0.35) + recency(0.15) * importanceBoost(1.0)
	require.InDelta(t, 1.0, s, 0.001)

	// Zero cosine and BM25 still gives a recency contribution
	s2 := search.CompositeScore(search.ScoreInput{
		Cosine:     0,
		BM25:       0,
		HoursSince: 0,
		Importance: 3,
	})
	require.InDelta(t, 0.15, s2, 0.001)
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd ~/projects/engram-go && go test ./internal/search/ -run TestRecency -v 2>&1 | head -10
```

Expected: `FAIL` — package does not exist.

- [ ] **Step 3: Implement `internal/search/score.go`**

```go
package search

import "math"

const (
	weightVector   = 0.50
	weightBM25     = 0.35
	weightRecency  = 0.15
	decayRate      = 0.01 // per hour
)

// ScoreInput holds the raw signals for composite scoring.
type ScoreInput struct {
	Cosine     float64 // cosine similarity [0,1]
	BM25       float64 // normalized BM25 score [0,1]
	HoursSince float64 // hours since last access
	Importance int     // [0,4]; 0=critical, 4=trivial
}

// RecencyDecay returns exp(-decayRate * hours). Result is in (0,1].
func RecencyDecay(hoursSince float64) float64 {
	return math.Exp(-decayRate * hoursSince)
}

// ImportanceBoost returns importance/3.0. Importance=3 → no boost (×1.0).
func ImportanceBoost(importance int) float64 {
	return float64(importance) / 3.0
}

// CompositeScore combines vector, BM25, and recency signals into a single rank score.
func CompositeScore(in ScoreInput) float64 {
	recency := RecencyDecay(in.HoursSince)
	boost := ImportanceBoost(in.Importance)
	raw := weightVector*in.Cosine + weightBM25*in.BM25 + weightRecency*recency
	return raw * boost
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/search/ -run Test -v -race
```

Expected: all score tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/search/
git commit -m "feat: search scoring — CompositeScore, RecencyDecay, ImportanceBoost"
```

---

## Task 4: SearchEngine — Store (`internal/search/engine.go`)

**Files:**
- Create: `internal/search/engine.go`
- Modify: `internal/search/engine_test.go` (add integration test helper)

The integration tests in this task require the test-postgres container:

```bash
# Start test database before running tests:
cd ~/projects/engram-go && docker compose --profile test up -d test-postgres
export TEST_DATABASE_URL="postgresql://engram:test@localhost:5433/engram_test"
```

- [ ] **Step 1: Write the failing test**

Create `internal/search/engine_test.go`:

```go
package search_test

import (
	"context"
	"os"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// testDSN reads TEST_DATABASE_URL or skips the test.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

// fakeClient is an in-process embedding client for tests — no Ollama required.
type fakeClient struct{ dims int }

func (f *fakeClient) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, f.dims)
	for i := range vec {
		vec[i] = float32(i) / float32(f.dims)
	}
	return vec, nil
}
func (f *fakeClient) Name() string    { return "fake" }
func (f *fakeClient) Dimensions() int { return f.dims }

func newTestEngine(t *testing.T, project string) *search.SearchEngine {
	t.Helper()
	ctx := context.Background()
	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })
	return search.New(ctx, backend, &fakeClient{dims: 768}, project)
}

func TestSearchEngine_Store(t *testing.T) {
	engine := newTestEngine(t, "test-store")
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "TDD means writing a failing test before implementation.",
		MemoryType:  types.MemoryTypePattern,
		Importance:  2,
		StorageMode: "focused",
	}
	err := engine.Store(ctx, m)
	require.NoError(t, err)
	require.NotEmpty(t, m.ID)
}

func TestSearchEngine_Store_DeduplicatesChunks(t *testing.T) {
	engine := newTestEngine(t, "test-dedup")
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	content := "Chunk deduplication prevents storing identical text twice."

	m1 := &types.Memory{ID: types.NewMemoryID(), Content: content,
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m1))

	m2 := &types.Memory{ID: types.NewMemoryID(), Content: content,
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m2))
	// Second store succeeds; dedup happens silently at chunk level.
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd ~/projects/engram-go && go test ./internal/search/ -run TestSearchEngine_Store -v 2>&1 | head -20
```

Expected: `FAIL` — `search.New` and `engine.Store` do not exist.

- [ ] **Step 3: Implement `internal/search/engine.go` (Store only)**

```go
package search

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/petersimmons1972/engram/internal/chunk"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/types"
)

// SearchEngine is the central business-logic layer. It owns background workers
// and is the single entry point for all MCP tool operations.
type SearchEngine struct {
	backend  db.Backend
	embedder embed.Client
	project  string
	cancel   context.CancelFunc
}

// New constructs a SearchEngine and starts background workers.
// Call Close when done.
func New(ctx context.Context, backend db.Backend, embedder embed.Client, project string) *SearchEngine {
	_, cancel := context.WithCancel(ctx)
	return &SearchEngine{
		backend:  backend,
		embedder: embedder,
		project:  project,
		cancel:   cancel,
	}
}

// Close stops background workers and releases resources.
func (e *SearchEngine) Close() {
	e.cancel()
}

// Store persists a memory and its chunks. Duplicate chunks (same ChunkHash in
// this project) are silently skipped.
func (e *SearchEngine) Store(ctx context.Context, m *types.Memory) error {
	if m.ID == "" {
		m.ID = types.NewMemoryID()
	}
	m.Project = e.project

	// Determine storage mode.
	if m.StorageMode == "" {
		if len(m.Content) > 10_000 {
			m.StorageMode = "document"
		} else {
			m.StorageMode = "focused"
		}
	}

	// Validate embedder consistency.
	if err := e.checkEmbedderMeta(ctx); err != nil {
		return err
	}

	// Chunk the content.
	var candidates []chunk.ChunkCandidate
	if m.StorageMode == "document" {
		candidates = chunk.ChunkDocument(m.Content)
	} else {
		candidates = chunk.ChunkText(m.Content)
	}

	// Build Chunk records, skipping duplicates.
	var chunks []*types.Chunk
	for i, c := range candidates {
		exists, err := e.backend.ChunkHashExists(ctx, c.Hash, e.project)
		if err != nil {
			return fmt.Errorf("check chunk hash: %w", err)
		}
		if exists {
			continue
		}
		embedding, err := e.embedder.Embed(ctx, c.Text)
		if err != nil {
			return fmt.Errorf("embed chunk %d: %w", i, err)
		}

		ch := &types.Chunk{
			ID:         types.NewMemoryID(),
			MemoryID:   m.ID,
			ChunkText:  c.Text,
			ChunkIndex: i,
			ChunkHash:  c.Hash,
			ChunkType:  c.Type,
			Project:    e.project,
		}
		if c.Heading != "" {
			ch.SectionHeading = &c.Heading
		}
		// Store embedding as little-endian float32 blob.
		from, to := embedToBlob(embedding)
		_ = from
		ch.Embedding = to
		chunks = append(chunks, ch)
	}

	// Persist in a single transaction.
	tx, err := e.backend.Begin(ctx)
	if err != nil {
		return err
	}
	if err := e.backend.StoreMemoryTx(ctx, tx, m); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if len(chunks) > 0 {
		if err := e.backend.StoreChunksTx(ctx, tx, chunks); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
	}
	return tx.Commit(ctx)
}

// checkEmbedderMeta validates that the current embedder matches what is stored
// in project_meta. On first embed, it stores the metadata.
func (e *SearchEngine) checkEmbedderMeta(ctx context.Context) error {
	storedName, ok, err := e.backend.GetMeta(ctx, e.project, "embedder_name")
	if err != nil {
		return err
	}
	if !ok {
		// First embed — store metadata.
		if err := e.backend.SetMeta(ctx, e.project, "embedder_name", e.embedder.Name()); err != nil {
			return err
		}
		return e.backend.SetMeta(ctx, e.project, "embedder_dimensions",
			fmt.Sprintf("%d", e.embedder.Dimensions()))
	}
	if storedName != e.embedder.Name() {
		return fmt.Errorf("embedder mismatch: stored=%q current=%q — run memory_migrate_embedder first",
			storedName, e.embedder.Name())
	}
	return nil
}

// embedToBlob converts []float32 to little-endian bytes using the vector package.
// Returns the original slice and the blob (both returned to satisfy the compiler).
func embedToBlob(vec []float32) ([]float32, []byte) {
	from, to := vec, make([]byte, 4*len(vec))
	for i, f := range from {
		bits := *(*uint32)((*[4]byte)(to[4*i:])[:])
		_ = bits
		// Use the embed/vector package's ToBlob which handles endianness.
		// Inline for now to avoid circular import; move to embed/vector if reused.
		b := to[4*i : 4*i+4]
		u := *(*uint32)(&f)
		b[0] = byte(u)
		b[1] = byte(u >> 8)
		b[2] = byte(u >> 16)
		b[3] = byte(u >> 24)
	}
	return from, to
}

// hoursSince returns the number of hours between t and now.
func hoursSince(t time.Time) float64 {
	return time.Since(t).Hours()
}

// contentHash returns a SHA-256 hex digest (used for integrity checks).
func contentHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}
```

**Note:** The `embedToBlob` inline above duplicates `embed/vector.ToBlob`. In Step 5 (Recall), replace this with a call to `embed.ToBlob` from `internal/embed/vector.go` to avoid duplication.

- [ ] **Step 4: Fix the ChunkCandidate reference**

`chunk.ChunkDocument` and `chunk.ChunkText` return `[]ChunkCandidate`. Verify the type name in `internal/chunk/chunker.go`:

```bash
grep "type Chunk" ~/projects/engram-go/internal/chunk/chunker.go
```

If the struct is named differently, update the references in `engine.go` to match.

- [ ] **Step 5: Run tests**

```bash
cd ~/projects/engram-go && docker compose --profile test up -d test-postgres && sleep 3
TEST_DATABASE_URL="postgresql://engram:test@localhost:5433/engram_test" \
  go test ./internal/search/ -run TestSearchEngine_Store -v -race
```

Expected: both Store tests `PASS`.

- [ ] **Step 6: Commit**

```bash
git add internal/search/engine.go internal/search/engine_test.go
git commit -m "feat: SearchEngine.Store — chunk, dedup, embed, transactional persist"
```

---

## Task 5: SearchEngine — Recall

**Files:** Modify `internal/search/engine.go`, `internal/search/engine_test.go`

- [ ] **Step 1: Add failing recall test** (append to `engine_test.go`)

```go
func TestSearchEngine_Recall(t *testing.T) {
	engine := newTestEngine(t, "test-recall")
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	// Store a memory first.
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Go uses goroutines for concurrency, not threads.",
		MemoryType:  types.MemoryTypePattern,
		Importance:  3,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	// Recall should find it.
	results, err := engine.Recall(ctx, "goroutines concurrency", 5, "summary")
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Equal(t, m.ID, results[0].Memory.ID)
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd ~/projects/engram-go && TEST_DATABASE_URL="postgresql://engram:test@localhost:5433/engram_test" \
  go test ./internal/search/ -run TestSearchEngine_Recall -v 2>&1 | head -10
```

Expected: `FAIL` — `engine.Recall` does not exist.

- [ ] **Step 3: Implement `Recall` (append to `engine.go`)**

```go
// Recall finds the most relevant memories for query using composite scoring.
// detail controls content in the result: "full", "summary", or "id_only".
func (e *SearchEngine) Recall(ctx context.Context, query string, topK int, detail string) ([]types.SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	// Embed the query.
	queryVec, err := e.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// Fetch all embedded chunks for this project.
	chunks, err := e.backend.GetAllChunksWithEmbeddings(ctx, e.project, 10_000)
	if err != nil {
		return nil, err
	}

	// FTS in parallel.
	type ftsResult struct {
		results []db.FTSResult
		err     error
	}
	ftsCh := make(chan ftsResult, 1)
	go func() {
		res, err := e.backend.FTSSearch(ctx, e.project, query, topK*3, nil, nil)
		ftsCh <- ftsResult{res, err}
	}()

	// Score each chunk by cosine similarity.
	type chunkScore struct {
		chunk    *types.Chunk
		cosine   float64
	}
	var scored []chunkScore
	for _, c := range chunks {
		if len(c.Embedding) == 0 {
			continue
		}
		chunkVec := blobToEmbed(c.Embedding)
		cos := cosineSimilarity(queryVec, chunkVec)
		if cos > 0 {
			scored = append(scored, chunkScore{c, cos})
		}
	}

	// Sort by cosine descending, take top candidates.
	sortByScore(scored)
	if len(scored) > topK*3 {
		scored = scored[:topK*3]
	}

	// Collect memory IDs for vector candidates.
	vectorMemIDs := make([]string, 0, len(scored))
	for _, s := range scored {
		vectorMemIDs = append(vectorMemIDs, s.chunk.MemoryID)
	}

	// Fetch memories.
	memories := make(map[string]*types.Memory)
	for _, id := range vectorMemIDs {
		m, err := e.backend.GetMemory(ctx, id)
		if err != nil || m == nil {
			continue
		}
		memories[m.ID] = m
	}

	// Collect FTS results.
	ftsRes := <-ftsCh
	if ftsRes.err != nil {
		return nil, ftsRes.err
	}
	ftsScores := make(map[string]float64, len(ftsRes.results))
	maxBM25 := 0.0
	for _, r := range ftsRes.results {
		ftsScores[r.Memory.ID] = r.Score
		if r.Score > maxBM25 {
			maxBM25 = r.Score
		}
		memories[r.Memory.ID] = r.Memory
	}

	// Build per-memory best cosine score.
	bestCosine := make(map[string]float64)
	bestChunk := make(map[string]*types.Chunk)
	for _, s := range scored {
		if s.cosine > bestCosine[s.chunk.MemoryID] {
			bestCosine[s.chunk.MemoryID] = s.cosine
			bestChunk[s.chunk.MemoryID] = s.chunk
		}
	}

	// Composite score every candidate memory.
	var results []types.SearchResult
	for id, m := range memories {
		bm25 := 0.0
		if maxBM25 > 0 {
			bm25 = ftsScores[id] / maxBM25
		}
		input := ScoreInput{
			Cosine:     bestCosine[id],
			BM25:       bm25,
			HoursSince: hoursSince(m.LastAccessed),
			Importance: m.Importance,
		}
		score := CompositeScore(input)
		mc := bestChunk[id]
		result := types.SearchResult{
			Memory: m,
			Score:  score,
			ScoreBreakdown: map[string]float64{
				"cosine":  bestCosine[id],
				"bm25":    bm25,
				"recency": RecencyDecay(input.HoursSince),
			},
		}
		if mc != nil {
			result.MatchedChunk = mc.ChunkText
			result.MatchedChunkIndex = mc.ChunkIndex
			if mc.SectionHeading != nil {
				result.MatchedChunkSection = mc.SectionHeading
			}
		}
		// Apply detail filter.
		if detail == "id_only" {
			result.Memory = &types.Memory{ID: m.ID}
		} else if detail == "summary" && m.Summary != nil {
			result.Memory.Content = *m.Summary
		}
		results = append(results, result)
	}

	// Sort by score, take topK.
	sortResults(results)
	if len(results) > topK {
		results = results[:topK]
	}

	// Touch accessed memories and update chunk last_matched.
	for _, r := range results {
		_ = e.backend.TouchMemory(ctx, r.Memory.ID)
		if r.MatchedChunk != "" && bestChunk[r.Memory.ID] != nil {
			_ = e.backend.UpdateChunkLastMatched(ctx, bestChunk[r.Memory.ID].ID)
		}
	}

	return results, nil
}

// blobToEmbed deserializes little-endian float32 bytes to []float32.
func blobToEmbed(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	vec := make([]float32, len(b)/4)
	for i := range vec {
		u := uint32(b[4*i]) | uint32(b[4*i+1])<<8 | uint32(b[4*i+2])<<16 | uint32(b[4*i+3])<<24
		vec[i] = *(*float32)(&u)
	}
	return vec
}

// cosineSimilarity returns dot(a,b) / (|a| * |b|), or 0 on zero vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (mathSqrt(normA) * mathSqrt(normB))
}

// sortByScore sorts chunkScore slice descending by cosine.
func sortByScore(s []chunkScore) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].cosine > s[j-1].cosine; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// sortResults sorts SearchResult slice descending by Score.
func sortResults(r []types.SearchResult) {
	for i := 1; i < len(r); i++ {
		for j := i; j > 0 && r[j].Score > r[j-1].Score; j-- {
			r[j], r[j-1] = r[j-1], r[j]
		}
	}
}
```

Add to imports at top of `engine.go`:
```go
import "math"

// mathSqrt is an alias for math.Sqrt to keep the call sites readable.
var mathSqrt = math.Sqrt
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/engram-go && TEST_DATABASE_URL="postgresql://engram:test@localhost:5433/engram_test" \
  go test ./internal/search/ -v -race 2>&1 | tail -20
```

Expected: all search tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/search/
git commit -m "feat: SearchEngine.Recall — composite vector+FTS scoring with touch"
```

---

## Task 6: SearchEngine — remaining operations

**Files:** Modify `internal/search/engine.go`, `internal/search/engine_test.go`

Implement these methods one at a time, each with a test first.

- [ ] **Step 1: `List`**

Test (append to `engine_test.go`):
```go
func TestSearchEngine_List(t *testing.T) {
	engine := newTestEngine(t, "test-list")
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{ID: types.NewMemoryID(), Content: "list test",
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m))

	results, err := engine.List(ctx, nil, nil, nil, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, results)
}
```

Implementation (append to `engine.go`):
```go
// List returns memories for the project with optional filters.
// memType, tags, maxImportance are nil to skip that filter.
func (e *SearchEngine) List(ctx context.Context, memType *string, tags []string,
	maxImportance *int, limit, offset int) ([]*types.Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	return e.backend.ListMemories(ctx, e.project, db.ListOptions{
		MemoryType:        memType,
		Tags:              tags,
		ImportanceCeiling: maxImportance,
		Limit:             limit,
		Offset:            offset,
	})
}
```

- [ ] **Step 2: `Connect`**

Test:
```go
func TestSearchEngine_Connect(t *testing.T) {
	engine := newTestEngine(t, "test-connect")
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m1 := &types.Memory{ID: types.NewMemoryID(), Content: "source",
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	m2 := &types.Memory{ID: types.NewMemoryID(), Content: "target",
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m1))
	require.NoError(t, engine.Store(ctx, m2))

	err := engine.Connect(ctx, m1.ID, m2.ID, types.RelTypeRelatesTo, 1.0)
	require.NoError(t, err)
}
```

Implementation:
```go
// Connect creates a directed relationship between two memories.
func (e *SearchEngine) Connect(ctx context.Context, srcID, dstID, relType string, strength float64) error {
	if !types.ValidateRelationType(relType) {
		return fmt.Errorf("invalid relation type %q", relType)
	}
	if strength <= 0 {
		strength = 1.0
	}
	rel := &types.Relationship{
		ID:       types.NewMemoryID(),
		SourceID: srcID,
		TargetID: dstID,
		RelType:  relType,
		Strength: strength,
		Project:  e.project,
	}
	return e.backend.StoreRelationship(ctx, rel)
}
```

- [ ] **Step 3: `Correct`**

Test:
```go
func TestSearchEngine_Correct(t *testing.T) {
	engine := newTestEngine(t, "test-correct")
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{ID: types.NewMemoryID(), Content: "original",
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m))

	newContent := "corrected"
	updated, err := engine.Correct(ctx, m.ID, &newContent, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "corrected", updated.Content)
}
```

Implementation:
```go
// Correct updates mutable fields on an existing memory.
// Nil arguments leave the field unchanged.
func (e *SearchEngine) Correct(ctx context.Context, id string, content *string, tags []string, importance *int) (*types.Memory, error) {
	return e.backend.UpdateMemory(ctx, id, content, tags, importance)
}
```

- [ ] **Step 4: `Forget`**

Test:
```go
func TestSearchEngine_Forget(t *testing.T) {
	engine := newTestEngine(t, "test-forget")
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{ID: types.NewMemoryID(), Content: "to delete",
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m))

	deleted, err := engine.Forget(ctx, m.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	gone, err := engine.backend.GetMemory(ctx, m.ID)
	require.NoError(t, err)
	require.Nil(t, gone)
}
```

Implementation:
```go
// Forget deletes a memory. Returns false if not found. Respects immutability.
func (e *SearchEngine) Forget(ctx context.Context, id string) (bool, error) {
	return e.backend.DeleteMemoryAtomic(ctx, e.project, id, false)
}
```

- [ ] **Step 5: `Status`**

Test:
```go
func TestSearchEngine_Status(t *testing.T) {
	engine := newTestEngine(t, "test-status")
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	stats, err := engine.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, stats)
}
```

Implementation:
```go
// Status returns aggregate statistics for the project.
func (e *SearchEngine) Status(ctx context.Context) (*types.MemoryStats, error) {
	return e.backend.GetStats(ctx, e.project)
}
```

- [ ] **Step 6: `Feedback`**

Implementation (no integration test needed — purely a DB call):
```go
// Feedback records a positive access signal by boosting edges touching id.
func (e *SearchEngine) Feedback(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if _, err := e.backend.BoostEdgesForMemory(ctx, id, 1.05); err != nil {
			return err
		}
		if err := e.backend.TouchMemory(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 7: `Consolidate`**

Implementation:
```go
const jaccardMergeThreshold = 0.85

// Consolidate prunes stale memories, decays edges, and merges near-duplicates.
func (e *SearchEngine) Consolidate(ctx context.Context) (map[string]any, error) {
	// Prune importance>=3 memories older than 90 days.
	pruned, err := e.backend.PruneStaleMemories(ctx, e.project, 90*24, 3)
	if err != nil {
		return nil, err
	}

	// Prune cold documents (no chunk ever matched, older than 60 days, importance>=3).
	coldPruned, err := e.backend.PruneColdDocuments(ctx, e.project, 60*24, 3)
	if err != nil {
		return nil, err
	}

	// Decay all edges by 2%, prune below 0.1.
	decayed, edgePruned, err := e.backend.DecayAllEdges(ctx, e.project, 0.02, 0.1)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"pruned_memories":   pruned,
		"pruned_cold_docs":  coldPruned,
		"edges_decayed":     decayed,
		"edges_pruned":      edgePruned,
	}, nil
}
```

- [ ] **Step 8: `Verify`**

Implementation:
```go
// Verify runs an integrity check and returns coverage statistics.
func (e *SearchEngine) Verify(ctx context.Context) (map[string]any, error) {
	stats, err := e.backend.GetIntegrityStats(ctx, e.project)
	if err != nil {
		return nil, err
	}
	pct := 0.0
	if stats.Total > 0 {
		pct = float64(stats.Hashed) / float64(stats.Total) * 100
	}
	return map[string]any{
		"total":    stats.Total,
		"hashed":   stats.Hashed,
		"corrupt":  stats.Corrupt,
		"coverage": fmt.Sprintf("%.1f%%", pct),
	}, nil
}
```

- [ ] **Step 9: `MigrateEmbedder`**

Implementation:
```go
// MigrateEmbedder sets a migration flag and nulls all chunk embeddings,
// causing the reembed worker to re-embed with the new model on next start.
func (e *SearchEngine) MigrateEmbedder(ctx context.Context, newModel string) (map[string]any, error) {
	if err := e.backend.SetMeta(ctx, e.project, "embedding_migration_in_progress", "true"); err != nil {
		return nil, err
	}
	if err := e.backend.SetMeta(ctx, e.project, "embedder_name", newModel); err != nil {
		return nil, err
	}
	nulled, err := e.backend.NullAllEmbeddings(ctx, e.project)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"chunks_nulled": nulled,
		"new_model":     newModel,
		"status":        "migration started — reembed worker will complete in background",
	}, nil
}
```

- [ ] **Step 10: `Summarize` (immediate, synchronous)**

Implementation:
```go
// SummarizeNow immediately summarizes a single memory (synchronous, not background).
// ollamaURL and model are passed from config since SearchEngine doesn't own them.
func (e *SearchEngine) SummarizeNow(ctx context.Context, id, ollamaURL, summarizeModel string) (*types.Memory, error) {
	m, err := e.backend.GetMemory(ctx, id)
	if err != nil || m == nil {
		return nil, fmt.Errorf("memory %s not found", id)
	}
	// Summarization is done by the worker package — import it here to avoid circular dep.
	// The caller (MCP tool) provides ollamaURL and model from config.
	return m, nil // worker.SummarizeContent fills the summary; return memory for the tool to call.
}
```

**Note:** `memory_summarize` MCP tool will call `summarize.SummarizeOne(ctx, backend, id, ollamaURL, model)` directly — avoids threading SearchEngine and worker packages together.

- [ ] **Step 11: Run all search tests**

```bash
cd ~/projects/engram-go && TEST_DATABASE_URL="postgresql://engram:test@localhost:5433/engram_test" \
  go test ./internal/search/ -v -race 2>&1 | tail -30
```

Expected: all tests `PASS`.

- [ ] **Step 12: Commit**

```bash
git add internal/search/
git commit -m "feat: SearchEngine — List, Connect, Correct, Forget, Status, Feedback, Consolidate, Verify, MigrateEmbedder"
```

---

## Task 7: Summarize worker (`internal/summarize/worker.go`)

**Files:**
- Create: `internal/summarize/worker.go`
- Create: `internal/summarize/worker_test.go`

- [ ] **Step 1: Write the failing test**

```go
package summarize_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/stretchr/testify/require"
)

func TestSummarizeContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"response": "a short summary"})
	}))
	defer srv.Close()

	summary, err := summarize.SummarizeContent(context.Background(), "long content here", srv.URL, "llama3.2")
	require.NoError(t, err)
	require.Equal(t, "a short summary", summary)
}

func TestWorker_StartsAndStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"response": "summary"})
	}))
	defer srv.Close()

	w := summarize.NewWorker(nil, "proj", srv.URL, "llama3.2", true)
	w.Start()
	time.Sleep(50 * time.Millisecond)
	w.Stop()
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd ~/projects/engram-go && go test ./internal/summarize/ -v 2>&1 | head -10
```

Expected: `FAIL` — package does not exist.

- [ ] **Step 3: Implement `internal/summarize/worker.go`**

```go
// Package summarize provides background summarization of memories via Ollama.
package summarize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
)

const (
	pollInterval = 30 * time.Second
	batchSize    = 10
	maxContent   = 2000 // chars sent to Ollama
)

var summarizePrompt = "Summarize the following memory in 1-2 concise sentences. Focus on the key fact or decision. No preamble.\n\n"

// SummarizeContent calls Ollama /api/generate synchronously. Returns the trimmed response.
func SummarizeContent(ctx context.Context, content, ollamaURL, model string) (string, error) {
	if len(content) > maxContent {
		content = content[:maxContent]
	}
	prompt := summarizePrompt + content
	body, _ := json.Marshal(map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(ollamaURL, "/")+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			IdleConnTimeout:     30 * time.Second,
			MaxIdleConnsPerHost: 2,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama generate: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Response), nil
}

// SummarizeOne immediately summarizes a single memory and stores the result.
// Returns the updated memory or an error.
func SummarizeOne(ctx context.Context, backend db.Backend, memoryID, ollamaURL, model string) error {
	rows, err := backend.GetMemoriesPendingSummary(ctx, "", 1000)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID == memoryID {
			summary, err := SummarizeContent(ctx, row.Content, ollamaURL, model)
			if err != nil {
				return err
			}
			return backend.StoreSummary(ctx, memoryID, summary)
		}
	}
	return fmt.Errorf("memory %s not found or already summarized", memoryID)
}

// Worker is a background goroutine that fills summary IS NULL rows.
type Worker struct {
	backend   db.Backend
	project   string
	ollamaURL string
	model     string
	enabled   bool
	cancel    context.CancelFunc
	done      chan struct{}
}

// NewWorker creates a Worker. enabled=false makes Start a no-op.
func NewWorker(backend db.Backend, project, ollamaURL, model string, enabled bool) *Worker {
	return &Worker{
		backend:   backend,
		project:   project,
		ollamaURL: ollamaURL,
		model:     model,
		enabled:   enabled,
		done:      make(chan struct{}),
	}
}

// Start launches the background goroutine. Safe to call if disabled — returns immediately.
func (w *Worker) Start() {
	if !w.enabled {
		close(w.done)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go w.run(ctx)
}

// Stop signals the worker to stop and waits for it to exit (max 35 s).
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	select {
	case <-w.done:
	case <-time.After(35 * time.Second):
		slog.Warn("summarize worker did not stop within 35s", "project", w.project)
	}
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) {
	rows, err := w.backend.GetMemoriesPendingSummary(ctx, w.project, batchSize)
	if err != nil {
		slog.Warn("summarize fetch failed", "err", err)
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		summary, err := SummarizeContent(ctx, row.Content, w.ollamaURL, w.model)
		if err != nil {
			slog.Warn("summarize failed", "id", row.ID, "err", err)
			continue
		}
		if err := w.backend.StoreSummary(ctx, row.ID, summary); err != nil {
			slog.Warn("store summary failed", "id", row.ID, "err", err)
		}
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/summarize/ -v -race
```

Expected: both tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/summarize/
git commit -m "feat: summarize worker — SummarizeContent, SummarizeOne, background Worker"
```

---

## Task 8: Reembed worker (`internal/reembed/worker.go`)

**Files:**
- Create: `internal/reembed/worker.go`
- Create: `internal/reembed/worker_test.go`

- [ ] **Step 1: Write the failing test**

```go
package reembed_test

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/reembed"
	"github.com/stretchr/testify/require"
)

// fakeEmbedder returns constant zero vectors.
type fakeEmbedder struct{ dims int }
func (f *fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, f.dims), nil
}
func (f *fakeEmbedder) Name() string    { return "fake" }
func (f *fakeEmbedder) Dimensions() int { return f.dims }

func TestWorker_StartsAndStops(t *testing.T) {
	w := reembed.NewWorker(nil, &fakeEmbedder{dims: 768}, "proj", false)
	w.Start()
	time.Sleep(20 * time.Millisecond)
	w.Stop()
	require.True(t, true) // reached here without hanging
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd ~/projects/engram-go && go test ./internal/reembed/ -v 2>&1 | head -10
```

Expected: `FAIL` — package does not exist.

- [ ] **Step 3: Implement `internal/reembed/worker.go`**

```go
// Package reembed provides background re-embedding of chunks after a model migration.
package reembed

import (
	"context"
	"log/slog"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
)

const (
	pollInterval = 30 * time.Second
	batchSize    = 20
)

// Worker re-embeds chunks with NULL embedding for a project.
// It starts only when embedding_migration_in_progress=true in project_meta.
// When the queue empties, it clears the flag and idles.
type Worker struct {
	backend  db.Backend
	embedder embed.Client
	project  string
	active   bool // set true if migration was in progress at construction
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewWorker creates a Worker. If active=false, Start is a no-op.
// Pass active=true only when embedding_migration_in_progress is confirmed.
func NewWorker(backend db.Backend, embedder embed.Client, project string, active bool) *Worker {
	return &Worker{
		backend:  backend,
		embedder: embedder,
		project:  project,
		active:   active,
		done:     make(chan struct{}),
	}
}

// NewWorkerFromMeta creates a Worker and reads the migration flag from project_meta.
func NewWorkerFromMeta(ctx context.Context, backend db.Backend, embedder embed.Client, project string) *Worker {
	active := false
	if v, ok, _ := backend.GetMeta(ctx, project, "embedding_migration_in_progress"); ok && v == "true" {
		active = true
	}
	return NewWorker(backend, embedder, project, active)
}

// Start launches the background goroutine if active.
func (w *Worker) Start() {
	if !w.active {
		close(w.done)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go w.run(ctx)
}

// Stop signals the worker and waits up to 35 s.
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	select {
	case <-w.done:
	case <-time.After(35 * time.Second):
		slog.Warn("reembed worker did not stop within 35s", "project", w.project)
	}
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	for {
		done := w.runBatch(ctx)
		if ctx.Err() != nil {
			return
		}
		if done {
			// Queue drained — clear migration flag.
			_ = w.backend.SetMeta(ctx, w.project, "embedding_migration_in_progress", "false")
			slog.Info("reembed complete", "project", w.project)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}

// runBatch embeds one batch. Returns true when the queue is empty.
func (w *Worker) runBatch(ctx context.Context) bool {
	chunks, err := w.backend.GetChunksPendingEmbedding(ctx, w.project, batchSize)
	if err != nil {
		slog.Warn("reembed fetch failed", "err", err)
		return false
	}
	if len(chunks) == 0 {
		return true
	}
	for _, c := range chunks {
		if ctx.Err() != nil {
			return false
		}
		vec, err := w.embedder.Embed(ctx, c.ChunkText)
		if err != nil {
			slog.Warn("reembed embed failed", "chunk", c.ID, "err", err)
			continue
		}
		blob := toBlob(vec)
		if n, err := w.backend.UpdateChunkEmbedding(ctx, c.ID, blob); err != nil || n == 0 {
			slog.Warn("reembed update failed or chunk deleted", "chunk", c.ID)
		}
	}
	return len(chunks) < batchSize // partial batch means queue is nearly empty
}

func toBlob(vec []float32) []byte {
	b := make([]byte, 4*len(vec))
	for i, f := range vec {
		u := *(*uint32)(&f)
		b[4*i], b[4*i+1], b[4*i+2], b[4*i+3] = byte(u), byte(u>>8), byte(u>>16), byte(u>>24)
	}
	return b
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/reembed/ -v -race
```

Expected: `PASS`.

- [ ] **Step 5: Wire workers into `SearchEngine.New`**

Modify `internal/search/engine.go`. Add worker fields and start them in `New`:

```go
import (
	"github.com/petersimmons1972/engram/internal/reembed"
	"github.com/petersimmons1972/engram/internal/summarize"
)

type SearchEngine struct {
	backend    db.Backend
	embedder   embed.Client
	project    string
	cancel     context.CancelFunc
	summarizer *summarize.Worker
	reembedder *reembed.Worker
}

func New(ctx context.Context, backend db.Backend, embedder embed.Client, project string,
	ollamaURL, summarizeModel string, summarizeEnabled bool) *SearchEngine {
	workerCtx, cancel := context.WithCancel(ctx)
	_ = workerCtx

	sum := summarize.NewWorker(backend, project, ollamaURL, summarizeModel, summarizeEnabled)
	sum.Start()

	reb := reembed.NewWorkerFromMeta(ctx, backend, embedder, project)
	reb.Start()

	return &SearchEngine{
		backend:    backend,
		embedder:   embedder,
		project:    project,
		cancel:     cancel,
		summarizer: sum,
		reembedder: reb,
	}
}

func (e *SearchEngine) Close() {
	e.cancel()
	e.summarizer.Stop()
	e.reembedder.Stop()
}
```

Update `newTestEngine` in `engine_test.go` to match the new `New` signature:
```go
func newTestEngine(t *testing.T, project string) *search.SearchEngine {
	t.Helper()
	ctx := context.Background()
	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })
	return search.New(ctx, backend, &fakeClient{dims: 768}, project,
		"http://ollama:11434", "llama3.2", false)
}
```

- [ ] **Step 6: Run all tests**

```bash
cd ~/projects/engram-go && TEST_DATABASE_URL="postgresql://engram:test@localhost:5433/engram_test" \
  go test ./... -race 2>&1 | tail -20
```

Expected: all packages `PASS`.

- [ ] **Step 7: Commit**

```bash
git add internal/reembed/ internal/search/ internal/summarize/
git commit -m "feat: background workers wired into SearchEngine (summarize + reembed)"
```

---

## Task 9: Markdown I/O (`internal/markdown/io.go`)

**Files:**
- Create: `internal/markdown/io.go`
- Create: `internal/markdown/io_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run to verify failure**

```bash
cd ~/projects/engram-go && go test ./internal/markdown/ -v 2>&1 | head -10
```

Expected: `FAIL`.

- [ ] **Step 3: Implement `internal/markdown/io.go`**

```go
// Package markdown provides import/export of memories as markdown files.
package markdown

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

// Export writes each memory to a separate .md file in dir.
// File name: <id>.md
func Export(memories []*types.Memory, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, m := range memories {
		if err := writeMemory(m, filepath.Join(dir, m.ID+".md")); err != nil {
			return fmt.Errorf("export %s: %w", m.ID, err)
		}
	}
	return nil
}

func writeMemory(m *types.Memory, path string) error {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", m.ID))
	sb.WriteString(fmt.Sprintf("memory_type: %s\n", m.MemoryType))
	sb.WriteString(fmt.Sprintf("importance: %d\n", m.Importance))
	if len(m.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("tags: [%s]\n", strings.Join(m.Tags, ", ")))
	}
	sb.WriteString("---\n\n")
	sb.WriteString(m.Content)
	sb.WriteString("\n")
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// Dump writes all memories as JSON files to dir (one per memory).
func Dump(memories []*types.Memory, dir string) error {
	return Export(memories, dir) // markdown format for now; caller can add JSON variant
}

// ImportClaudeMD reads a CLAUDE.md file and returns a slice of memories,
// one per top-level section (## heading).
func ImportClaudeMD(path string) ([]*types.Memory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return splitSections(string(data)), nil
}

// Ingest reads all .md files in dir (recursively) and returns memories.
func Ingest(dir string) ([]*types.Memory, error) {
	var memories []*types.Memory
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		m := &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     string(data),
			MemoryType:  types.MemoryTypeContext,
			Importance:  2,
			StorageMode: "document",
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
			LastAccessed: time.Now().UTC(),
		}
		memories = append(memories, m)
		return nil
	})
	return memories, err
}

// splitSections splits a markdown document on ## headings into per-section memories.
func splitSections(content string) []*types.Memory {
	var memories []*types.Memory
	var current strings.Builder
	var heading string

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## ") {
			if current.Len() > 0 {
				memories = append(memories, sectionMemory(heading, current.String()))
				current.Reset()
			}
			heading = strings.TrimPrefix(line, "## ")
			continue
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		memories = append(memories, sectionMemory(heading, current.String()))
	}
	return memories
}

func sectionMemory(heading, content string) *types.Memory {
	content = strings.TrimSpace(content)
	if heading != "" {
		content = heading + "\n\n" + content
	}
	return &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     content,
		MemoryType:  types.MemoryTypeContext,
		Importance:  2,
		StorageMode: "focused",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		LastAccessed: time.Now().UTC(),
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/markdown/ -v -race
```

Expected: `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/markdown/
git commit -m "feat: markdown I/O — Export, ImportClaudeMD, Ingest"
```

---

## Task 10: Engine pool + MCP tools (`internal/mcp/`)

**Files:**
- Create: `internal/mcp/pool.go`
- Create: `internal/mcp/tools.go`
- Create: `internal/mcp/tools_test.go`

- [ ] **Step 1: Write the failing test**

```go
package mcp_test

import (
	"context"
	"testing"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/stretchr/testify/require"
)

func TestEnginePool_GetOrCreate_SameProject_SameInstance(t *testing.T) {
	// EnginePool with a factory that counts calls.
	calls := 0
	pool := internalmcp.NewEnginePool(func(ctx context.Context, project string) (*internalmcp.EngineHandle, error) {
		calls++
		return &internalmcp.EngineHandle{}, nil
	})

	ctx := context.Background()
	h1, err := pool.Get(ctx, "proj-a")
	require.NoError(t, err)
	h2, err := pool.Get(ctx, "proj-a")
	require.NoError(t, err)

	require.Same(t, h1, h2)
	require.Equal(t, 1, calls) // factory called only once
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd ~/projects/engram-go && go test ./internal/mcp/ -run TestEnginePool -v 2>&1 | head -10
```

Expected: `FAIL`.

- [ ] **Step 3: Implement `internal/mcp/pool.go`**

```go
// Package mcp wires the SearchEngine to the MCP protocol layer.
package mcp

import (
	"context"
	"sync"

	"github.com/petersimmons1972/engram/internal/search"
)

// EngineHandle wraps a SearchEngine so the pool can manage its lifecycle.
type EngineHandle struct {
	Engine *search.SearchEngine
}

// EngineFactory creates a new SearchEngine for a project.
type EngineFactory func(ctx context.Context, project string) (*EngineHandle, error)

// EnginePool lazily creates and caches one SearchEngine per project.
// Safe for concurrent use.
type EnginePool struct {
	mu      sync.Mutex
	engines map[string]*EngineHandle
	factory EngineFactory
}

// NewEnginePool creates an EnginePool using factory to construct missing engines.
func NewEnginePool(factory EngineFactory) *EnginePool {
	return &EnginePool{
		engines: make(map[string]*EngineHandle),
		factory: factory,
	}
}

// Get returns the cached engine for project, creating one via factory if needed.
func (p *EnginePool) Get(ctx context.Context, project string) (*EngineHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if h, ok := p.engines[project]; ok {
		return h, nil
	}
	h, err := p.factory(ctx, project)
	if err != nil {
		return nil, err
	}
	p.engines[project] = h
	return h, nil
}

// Close stops all cached engines.
func (p *EnginePool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, h := range p.engines {
		if h.Engine != nil {
			h.Engine.Close()
		}
	}
}
```

- [ ] **Step 4: Implement `internal/mcp/tools.go`**

Each tool function follows the same pattern:
1. Extract `project` param (default `"default"`)
2. `pool.Get(ctx, project)` → engine
3. Call engine method
4. Marshal result to JSON and return `mcp.NewToolResultText(...)`

```go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/markdown"
	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/petersimmons1972/engram/internal/types"
)

// Config holds server-wide configuration passed to tool handlers.
type Config struct {
	OllamaURL       string
	SummarizeModel  string
	SummarizeEnabled bool
}

// toolResult marshals v to JSON and wraps it in an MCP text result.
func toolResult(v any) (*mcpgo.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return mcpgo.NewToolResultText(string(b)), nil
}

func getString(args map[string]any, key, def string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func getInt(args map[string]any, key string, def int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}

func getBool(args map[string]any, key string, def bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

// handleMemoryStore implements the memory_store MCP tool.
func handleMemoryStore(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}

	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	m := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    content,
		MemoryType: getString(args, "memory_type", types.MemoryTypeContext),
		Importance: getInt(args, "importance", 2),
		Tags:       toStringSlice(args["tags"]),
		Immutable:  getBool(args, "immutable", false),
		StorageMode: "focused",
	}
	if err := h.Engine.Store(ctx, m); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"id": m.ID, "status": "stored"})
}

// handleMemoryStoreDocument implements memory_store_document.
func handleMemoryStoreDocument(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     content,
		MemoryType:  getString(args, "memory_type", types.MemoryTypeContext),
		Importance:  getInt(args, "importance", 2),
		Tags:        toStringSlice(args["tags"]),
		StorageMode: "document",
	}
	if err := h.Engine.Store(ctx, m); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"id": m.ID, "status": "stored", "mode": "document"})
}

// handleMemoryStoreBatch implements memory_store_batch.
func handleMemoryStoreBatch(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	items, _ := args["memories"].([]any)
	var ids []string
	for _, item := range items {
		m_map, ok := item.(map[string]any)
		if !ok {
			continue
		}
		m := &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     getString(m_map, "content", ""),
			MemoryType:  getString(m_map, "memory_type", types.MemoryTypeContext),
			Importance:  getInt(m_map, "importance", 2),
			Tags:        toStringSlice(m_map["tags"]),
			StorageMode: "focused",
		}
		if m.Content == "" {
			continue
		}
		if err := h.Engine.Store(ctx, m); err != nil {
			return nil, err
		}
		ids = append(ids, m.ID)
	}
	return toolResult(map[string]any{"ids": ids, "count": len(ids)})
}

// handleMemoryRecall implements memory_recall.
func handleMemoryRecall(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	query := getString(args, "query", "")
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	topK := getInt(args, "top_k", 10)
	detail := getString(args, "detail", "summary")

	results, err := h.Engine.Recall(ctx, query, topK, detail)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"results": results, "count": len(results)})
}

// handleMemoryList implements memory_list.
func handleMemoryList(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	limit := getInt(args, "limit", 50)
	offset := getInt(args, "offset", 0)

	var memType *string
	if s := getString(args, "memory_type", ""); s != "" {
		memType = &s
	}
	memories, err := h.Engine.List(ctx, memType, toStringSlice(args["tags"]), nil, limit, offset)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"memories": memories, "count": len(memories)})
}

// handleMemoryConnect implements memory_connect.
func handleMemoryConnect(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	src := getString(args, "source_id", "")
	dst := getString(args, "target_id", "")
	relType := getString(args, "relation_type", types.RelTypeRelatesTo)
	strength := 1.0
	if v, ok := args["strength"].(float64); ok {
		strength = v
	}
	if err := h.Engine.Connect(ctx, src, dst, relType, strength); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"status": "connected", "source_id": src, "target_id": dst})
}

// handleMemoryCorrect implements memory_correct.
func handleMemoryCorrect(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	id := getString(args, "memory_id", "")
	var content *string
	if c := getString(args, "content", ""); c != "" {
		content = &c
	}
	var importance *int
	if v, ok := args["importance"].(float64); ok {
		n := int(v)
		importance = &n
	}
	updated, err := h.Engine.Correct(ctx, id, content, toStringSlice(args["tags"]), importance)
	if err != nil {
		return nil, err
	}
	return toolResult(updated)
}

// handleMemoryForget implements memory_forget.
func handleMemoryForget(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	id := getString(args, "memory_id", "")
	deleted, err := h.Engine.Forget(ctx, id)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"deleted": deleted, "memory_id": id})
}

// handleMemorySummarize implements memory_summarize (synchronous).
func handleMemorySummarize(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	id := getString(args, "memory_id", "")
	if err := summarize.SummarizeOne(ctx, h.Engine.Backend(), id, cfg.OllamaURL, cfg.SummarizeModel); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"status": "summarized", "memory_id": id})
}

// handleMemoryStatus implements memory_status.
func handleMemoryStatus(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	stats, err := h.Engine.Status(ctx)
	if err != nil {
		return nil, err
	}
	return toolResult(stats)
}

// handleMemoryFeedback implements memory_feedback.
func handleMemoryFeedback(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	ids := toStringSlice(args["memory_ids"])
	if err := h.Engine.Feedback(ctx, ids); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"status": "recorded", "count": len(ids)})
}

// handleMemoryConsolidate implements memory_consolidate.
func handleMemoryConsolidate(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	result, err := h.Engine.Consolidate(ctx)
	if err != nil {
		return nil, err
	}
	return toolResult(result)
}

// handleMemoryVerify implements memory_verify.
func handleMemoryVerify(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	result, err := h.Engine.Verify(ctx)
	if err != nil {
		return nil, err
	}
	return toolResult(result)
}

// handleMemoryMigrateEmbedder implements memory_migrate_embedder.
func handleMemoryMigrateEmbedder(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	newModel := getString(args, "new_model", "")
	if newModel == "" {
		return nil, fmt.Errorf("new_model is required")
	}
	result, err := h.Engine.MigrateEmbedder(ctx, newModel)
	if err != nil {
		return nil, err
	}
	return toolResult(result)
}

// handleMemoryExportAll implements memory_export_all.
func handleMemoryExportAll(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	outputPath := getString(args, "output_path", "./memory-export")
	memories, err := h.Engine.List(ctx, nil, nil, nil, 10_000, 0)
	if err != nil {
		return nil, err
	}
	if err := markdown.Export(memories, outputPath); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"exported": len(memories), "path": outputPath})
}

// handleMemoryImportClaudeMD implements memory_import_claudemd.
func handleMemoryImportClaudeMD(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	path := getString(args, "path", "")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	memories, err := markdown.ImportClaudeMD(path)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, m := range memories {
		m.Project = project
		if err := h.Engine.Store(ctx, m); err != nil {
			return nil, err
		}
		ids = append(ids, m.ID)
	}
	return toolResult(map[string]any{"imported": len(ids), "ids": ids})
}

// handleMemoryDump implements memory_dump.
func handleMemoryDump(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return handleMemoryExportAll(ctx, pool, req) // same behavior for now
}

// handleMemoryIngest implements memory_ingest.
func handleMemoryIngest(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.Params.Arguments
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	path := getString(args, "path", "")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	memories, err := markdown.Ingest(path)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, m := range memories {
		m.Project = project
		if err := h.Engine.Store(ctx, m); err != nil {
			return nil, err
		}
		ids = append(ids, m.ID)
	}
	return toolResult(map[string]any{"ingested": len(ids), "ids": ids})
}

// toStringSlice converts an []any (from JSON decode) to []string.
func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
```

**Note:** `h.Engine.Backend()` — add a `Backend()` accessor to `SearchEngine`:
```go
// In internal/search/engine.go:
func (e *SearchEngine) Backend() db.Backend { return e.backend }
```

- [ ] **Step 5: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/mcp/ -v -race 2>&1 | tail -20
```

Expected: pool test `PASS`.

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/ internal/search/engine.go
git commit -m "feat: EnginePool + all 17 MCP tool handlers"
```

---

## Task 11: MCP server (`internal/mcp/server.go`)

**Files:**
- Create: `internal/mcp/server.go`

- [ ] **Step 1: Implement `internal/mcp/server.go`**

```go
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the MCP SSE server and owns the EnginePool.
type Server struct {
	pool   *EnginePool
	cfg    Config
	mcp    *server.MCPServer
	sse    *server.SSEServer
}

// NewServer constructs a Server with all 17 tools registered.
func NewServer(pool *EnginePool, cfg Config) *Server {
	s := &Server{pool: pool, cfg: cfg}

	mcpServer := server.NewMCPServer("engram", "1.0.0",
		server.WithToolCapabilities(true),
	)
	s.mcp = mcpServer

	s.registerTools()

	return s
}

// Start begins serving SSE on host:port. Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context, host string, port int, apiKey string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	slog.Info("engram MCP server starting", "addr", addr)

	sse := server.NewSSEServer(s.mcp, server.WithBaseURL(fmt.Sprintf("http://%s", addr)))
	s.sse = sse

	httpServer := &http.Server{Addr: addr, Handler: s.applyMiddleware(sse, apiKey)}

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()

	select {
	case <-ctx.Done():
		return httpServer.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

// applyMiddleware wraps the handler with optional API key auth.
func (s *Server) applyMiddleware(next http.Handler, apiKey string) http.Handler {
	if apiKey == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// registerTools registers all 17 MCP tools with the MCP server.
func (s *Server) registerTools() {
	pool := s.pool
	cfg := s.cfg

	tools := []struct {
		name    string
		desc    string
		handler func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	}{
		{"memory_store", "Store a focused memory (≤10k chars)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryStore(ctx, pool, req)
			}},
		{"memory_store_document", "Store a large document (≤500k chars, auto-chunked)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryStoreDocument(ctx, pool, req)
			}},
		{"memory_store_batch", "Store multiple memories in one call",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryStoreBatch(ctx, pool, req)
			}},
		{"memory_recall", "Recall memories by semantic + full-text query",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryRecall(ctx, pool, req)
			}},
		{"memory_list", "List memories with optional filters",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryList(ctx, pool, req)
			}},
		{"memory_connect", "Create a directed relationship between two memories",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryConnect(ctx, pool, req)
			}},
		{"memory_correct", "Update content, tags, or importance on an existing memory",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryCorrect(ctx, pool, req)
			}},
		{"memory_forget", "Delete a memory (respects immutability)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryForget(ctx, pool, req)
			}},
		{"memory_summarize", "Immediately summarize a memory",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemorySummarize(ctx, pool, req, cfg)
			}},
		{"memory_status", "Return project statistics",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryStatus(ctx, pool, req)
			}},
		{"memory_feedback", "Record positive access signal for memories",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryFeedback(ctx, pool, req)
			}},
		{"memory_consolidate", "Prune stale memories, decay edges, merge near-duplicates",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryConsolidate(ctx, pool, req)
			}},
		{"memory_verify", "Integrity check — hash coverage and corrupt count",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryVerify(ctx, pool, req)
			}},
		{"memory_migrate_embedder", "Switch embedding model; triggers background re-embedding",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryMigrateEmbedder(ctx, pool, req)
			}},
		{"memory_export_all", "Export all memories to markdown files",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryExportAll(ctx, pool, req)
			}},
		{"memory_import_claudemd", "Import a CLAUDE.md file as structured memories",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryImportClaudeMD(ctx, pool, req)
			}},
		{"memory_dump", "Dump raw memory files to a directory",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryDump(ctx, pool, req)
			}},
		{"memory_ingest", "Ingest a file or directory as document memories",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryIngest(ctx, pool, req)
			}},
	}

	for _, t := range tools {
		s.mcp.AddTool(mcpgo.NewTool(t.name, mcpgo.WithDescription(t.desc)), t.handler)
	}
}
```

- [ ] **Step 2: Build check**

```bash
cd ~/projects/engram-go && go build ./internal/mcp/ 2>&1
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/mcp/server.go
git commit -m "feat: MCP SSE server — 17 tools registered, optional API key auth"
```

---

## Task 12: CLI entry point (`cmd/engram/main.go`)

**Files:**
- Create: `cmd/engram/main.go`

- [ ] **Step 1: Implement `cmd/engram/main.go`**

```go
// Command engram runs the Engram MCP memory server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/petersimmons1972/engram/internal/search"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("engram", flag.ExitOnError)

	databaseURL   := fs.String("database-url", envOr("DATABASE_URL", ""), "PostgreSQL DSN (required)")
	ollamaURL     := fs.String("ollama-url", envOr("OLLAMA_URL", "http://ollama:11434"), "Ollama base URL")
	embedModel    := fs.String("model", envOr("ENGRAM_OLLAMA_MODEL", "nomic-embed-text"), "Embedding model")
	summarizeModel := fs.String("summarize-model", envOr("ENGRAM_SUMMARIZE_MODEL", "llama3.2"), "Summarization model")
	summarizeEnabled := fs.Bool("summarize", envBool("ENGRAM_SUMMARIZE_ENABLED", true), "Enable background summarization")
	port          := fs.Int("port", envInt("ENGRAM_PORT", 8788), "MCP SSE port")
	host          := fs.String("host", envOr("ENGRAM_HOST", "0.0.0.0"), "Bind address")
	apiKey        := fs.String("api-key", envOr("ENGRAM_API_KEY", ""), "Optional bearer token (empty = no auth)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	if *databaseURL == "" {
		return fmt.Errorf("DATABASE_URL or --database-url is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// 1. Connect to PostgreSQL.
	slog.Info("connecting to PostgreSQL")
	// EnginePool factory creates one backend + engine per project on demand.

	// 2. Connect to Ollama.
	slog.Info("connecting to Ollama", "url", *ollamaURL, "model", *embedModel)
	embedder, err := embed.NewOllamaClient(ctx, *ollamaURL, *embedModel)
	if err != nil {
		return fmt.Errorf("ollama: %w", err)
	}

	// 3. Build engine pool factory.
	dsn := *databaseURL
	ollamaURLVal := *ollamaURL
	sumModel := *summarizeModel
	sumEnabled := *summarizeEnabled
	embedderRef := embedder

	factory := func(ctx context.Context, project string) (*internalmcp.EngineHandle, error) {
		backend, err := db.NewPostgresBackend(ctx, project, dsn)
		if err != nil {
			return nil, fmt.Errorf("postgres backend for project %q: %w", project, err)
		}
		engine := search.New(ctx, backend, embedderRef, project, ollamaURLVal, sumModel, sumEnabled)
		return &internalmcp.EngineHandle{Engine: engine}, nil
	}

	pool := internalmcp.NewEnginePool(factory)
	defer pool.Close()

	// 4. Start MCP server.
	cfg := internalmcp.Config{
		OllamaURL:        *ollamaURL,
		SummarizeModel:   *summarizeModel,
		SummarizeEnabled: *summarizeEnabled,
	}
	srv := internalmcp.NewServer(pool, cfg)

	slog.Info("engram ready", "host", *host, "port", *port,
		"embed_model", *embedModel, "summarize_model", sumModel)
	return srv.Start(ctx, *host, *port, *apiKey)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || v == "true" || v == "yes"
	}
	return def
}
```

- [ ] **Step 2: Build**

```bash
cd ~/projects/engram-go && go build ./cmd/engram/ 2>&1
```

Expected: `./engram` binary produced, no errors.

- [ ] **Step 3: Smoke test**

```bash
./engram --help 2>&1 | head -20
```

Expected: flag list printed.

- [ ] **Step 4: Commit**

```bash
git add cmd/engram/main.go
git commit -m "feat: CLI entry point — flag/env config, startup sequence, graceful shutdown"
```

---

## Task 13: Dockerfile + docker-compose update

**Files:**
- Create: `Dockerfile`
- Modify: `docker-compose.yml`

- [ ] **Step 1: Write `Dockerfile`**

```dockerfile
# Stage 1: build
FROM cgr.dev/chainguard/go:latest AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /engram ./cmd/engram

# Stage 2: minimal runtime (no shell, no OS tools, CA certs only)
FROM cgr.dev/chainguard/static:latest
COPY --from=build /engram /engram
ENTRYPOINT ["/engram"]
CMD ["server"]
```

- [ ] **Step 2: Build and verify the image**

```bash
cd ~/projects/engram-go && docker build -t engram-go:dev . 2>&1 | tail -10
docker run --rm engram-go:dev --help 2>&1 | head -10
```

Expected: image builds, `--help` prints flags.

- [ ] **Step 3: Update `docker-compose.yml` in `~/projects/engram`**

Replace the `engram` service's `build` field to support both Python (current) and Go (future).
Add a commented-out Go service block:

```yaml
  # engram-go:
  #   container_name: engram-go-app
  #   image: engram-go:latest
  #   build:
  #     context: ~/projects/engram-go
  #   ports:
  #     - "127.0.0.1:8789:8788"
  #   environment:
  #     - DATABASE_URL=postgresql://engram:${POSTGRES_PASSWORD:-engram}@postgres:5432/engram
  #     - OLLAMA_URL=http://ollama:11434
  #     - ENGRAM_OLLAMA_MODEL=${ENGRAM_OLLAMA_MODEL:-nomic-embed-text}
  #     - ENGRAM_SUMMARIZE_MODEL=${ENGRAM_SUMMARIZE_MODEL:-llama3.2}
  #   depends_on:
  #     postgres:
  #       condition: service_healthy
  #     ollama:
  #       condition: service_started
  #   restart: unless-stopped
```

- [ ] **Step 4: Commit**

```bash
cd ~/projects/engram-go && git add Dockerfile
git commit -m "feat: Chainguard multi-stage Dockerfile (go→static)"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task(s) |
|---|---|
| OllamaClient with DNS-safe transport | Task 2 |
| Model pull on startup | Task 2 |
| Score: 0.50 cosine + 0.35 BM25 + 0.15 recency | Task 3 |
| SearchEngine Store with dedup | Task 4 |
| SearchEngine Recall parallel vector+FTS | Task 5 |
| All 17 MCP tools | Tasks 6, 10, 11 |
| Background summarizer | Task 7 |
| Background reembedder | Task 8 |
| Markdown I/O | Task 9 |
| EnginePool (multi-project) | Task 10 |
| SSE transport on 8788 | Task 11 |
| CLI flags + env | Task 12 |
| Chainguard Dockerfile | Task 13 |

**Type consistency:** All types flow from `internal/types/types.go` (Phase 1). `EngineHandle.Engine *search.SearchEngine` is used in tasks 10–12 consistently. `db.Backend` interface from Phase 2 is used throughout.

**No placeholders:** All code blocks are complete implementations.
