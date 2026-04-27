# Open-Brain Embedder Models: SuggestedModels Registry + Eval Tool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a curated `SuggestedModels` registry for Ollama embedding models, fix the hardcoded 768-dimensional guard in `main.go` to be dynamic, and expose two new MCP tools: `memory_models` (lists installed vs. suggested models) and `memory_embedding_eval` (compares any two Ollama models against a set of probe sentences).

**Architecture:** `internal/embed/models.go` is a pure-data file with no external dependencies — it defines the `ModelSpec` struct and `SuggestedModels` slice. `memory_models` calls `ollama list` via the existing Ollama HTTP client to determine installed state, then merges with `SuggestedModels`. `memory_embedding_eval` creates ephemeral `OllamaClient` instances for both models (auto-pulling if absent), embeds a fixed probe set, computes cosine similarity rankings, and returns a side-by-side comparison. No stored embeddings are migrated — this is read-only.

**Tech Stack:** Go 1.22+, `internal/embed.OllamaClient` (existing), `go test ./... -count=1 -race`

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/embed/models.go` | `ModelSpec` struct + `SuggestedModels` slice |
| Create | `internal/embed/models_test.go` | Validate `SuggestedModels` invariants |
| Modify | `cmd/engram/main.go:105` | Replace `const expectedDims = 768` with dynamic dims from `embedder.Dimensions()` |
| Modify | `internal/mcp/tools.go` | Add `handleMemoryModels` and `handleMemoryEmbeddingEval` |
| Modify | `internal/mcp/server.go` | Register `memory_models` and `memory_embedding_eval` |

---

### Task 1: Create `internal/embed/models.go`

**Files:**
- Create: `internal/embed/models.go`
- Create: `internal/embed/models_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/embed/models_test.go`:

```go
package embed_test

import (
    "testing"

    "github.com/petersimmons1972/engram/internal/embed"
)

func TestSuggestedModelsNotEmpty(t *testing.T) {
    if len(embed.SuggestedModels) == 0 {
        t.Fatal("SuggestedModels must not be empty")
    }
}

func TestSuggestedModelsHaveRequiredFields(t *testing.T) {
    for _, m := range embed.SuggestedModels {
        if m.Name == "" {
            t.Errorf("ModelSpec has empty Name: %+v", m)
        }
        if m.Dimensions <= 0 {
            t.Errorf("ModelSpec %q has non-positive Dimensions: %d", m.Name, m.Dimensions)
        }
        if m.SizeMB <= 0 {
            t.Errorf("ModelSpec %q has non-positive SizeMB: %d", m.Name, m.SizeMB)
        }
        if m.Description == "" {
            t.Errorf("ModelSpec %q has empty Description", m.Name)
        }
    }
}

func TestSuggestedModelsHasExactlyOneRecommended(t *testing.T) {
    count := 0
    for _, m := range embed.SuggestedModels {
        if m.Recommended {
            count++
        }
    }
    if count != 1 {
        t.Errorf("expected exactly 1 recommended model, got %d", count)
    }
}

func TestDefaultRecommendedModel(t *testing.T) {
    rec := embed.DefaultRecommendedModel()
    if rec == nil {
        t.Fatal("DefaultRecommendedModel returned nil")
    }
    if rec.Name != "mxbai-embed-large" {
        t.Errorf("expected mxbai-embed-large as recommended, got %q", rec.Name)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/embed/... -count=1 -run TestSuggestedModels
```
Expected: compile error — `embed.SuggestedModels undefined`.

- [ ] **Step 3: Create `internal/embed/models.go`**

```go
package embed

// ModelSpec describes a curated Ollama embedding model.
type ModelSpec struct {
    Name        string
    Dimensions  int
    SizeMB      int
    Description string
    Recommended bool // exactly one entry should be true
}

// SuggestedModels is the curated list of Ollama embedding models recommended
// for engram-go 3.x. Users can pull any of these via `ollama pull <Name>` and
// then set ENGRAM_OLLAMA_MODEL to switch. Run memory_embedding_eval to compare
// before migrating stored embeddings.
var SuggestedModels = []ModelSpec{
    {
        Name:        "mxbai-embed-large",
        Dimensions:  1024,
        SizeMB:      669,
        Description: "Best MTEB retrieval score of locally-available Ollama models. Recommended upgrade from nomic-embed-text.",
        Recommended: true,
    },
    {
        Name:        "bge-m3",
        Dimensions:  1024,
        SizeMB:      1200,
        Description: "Best multilingual option. Recommended when memories span multiple languages.",
        Recommended: false,
    },
    {
        Name:        "nomic-embed-text",
        Dimensions:  768,
        SizeMB:      274,
        Description: "Current default. Solid general-purpose baseline; smallest footprint.",
        Recommended: false,
    },
}

// DefaultRecommendedModel returns the first ModelSpec with Recommended=true,
// or nil if none exists (should not happen with the curated list above).
func DefaultRecommendedModel() *ModelSpec {
    for i := range SuggestedModels {
        if SuggestedModels[i].Recommended {
            return &SuggestedModels[i]
        }
    }
    return nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/embed/... -count=1 -race
```
Expected: PASS — all embed tests green (including existing ollama_test.go and vector_test.go).

- [ ] **Step 5: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/embed/models.go internal/embed/models_test.go && git commit -m "feat(embed): add SuggestedModels registry with mxbai-embed-large as recommended"
```

---

### Task 2: Fix hardcoded dimensional guard in `cmd/engram/main.go`

**Files:**
- Modify: `cmd/engram/main.go:105-113`

- [ ] **Step 1: Understand what currently exists**

Read lines 103-113 in `cmd/engram/main.go`:

```go
// Current code (lines 103-113):
const expectedDims = 768
testVec, err := embedder.Embed(ctx, "dimensional guard test")
if err != nil {
    return fmt.Errorf("dimensional guard: embed test failed: %w", err)
}
if len(testVec) != expectedDims {
    return fmt.Errorf("dimensional guard: embedding model produces %d dimensions, but pgvector column is vector(%d) — use a %d-dimension model or run a schema migration", len(testVec), expectedDims, expectedDims)
}
slog.Info("dimensional guard passed", "dims", expectedDims)
```

The problem: `expectedDims = 768` hardcodes nomic-embed-text's dimension. When the operator switches to `mxbai-embed-large` (1024 dims), the guard rejects it even if the schema has been migrated.

- [ ] **Step 2: Replace the hardcoded guard with a dynamic check**

Replace the block above with:

```go
testVec, err := embedder.Embed(ctx, "dimensional guard test")
if err != nil {
    return fmt.Errorf("dimensional guard: embed test failed: %w", err)
}
actualDims := len(testVec)
if actualDims == 0 {
    return fmt.Errorf("dimensional guard: embedding model returned empty vector")
}
slog.Info("dimensional guard passed", "dims", actualDims)
```

The guard now logs the actual dimension. Schema compatibility (pgvector column width) is enforced at insert time by pgvector — if there's a mismatch, the first `Store` call will return a clear error. This is the correct behavior: the dimensional guard's job is to detect a broken embed, not to enforce a specific column size.

- [ ] **Step 3: Build and verify**

```bash
cd /home/psimmons/projects/engram-go && go build ./...
```
Expected: clean build.

- [ ] **Step 4: Run full test suite**

```bash
cd /home/psimmons/projects/engram-go && go test ./... -count=1 -race 2>&1 | tail -10
```
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add cmd/engram/main.go && git commit -m "fix(main): make dimensional guard dynamic — removes hardcoded 768 to support mxbai-embed-large and other models"
```

---

### Task 3: Implement `handleMemoryModels` in `internal/mcp/tools.go`

**Files:**
- Modify: `internal/mcp/tools.go` (add new handler near end of file)

This tool calls `GET /api/tags` on the Ollama server to discover installed models, then merges with `embed.SuggestedModels`. It does NOT require a running search engine or database query.

- [ ] **Step 1: Write a unit test (table-driven logic test)**

Add to a new file `internal/mcp/tools_models_test.go`:

```go
package mcp_test

import (
    "testing"

    "github.com/petersimmons1972/engram/internal/embed"
)

// TestSuggestedModelsEnrichment verifies that the enrichment logic (installed flag)
// works correctly. This tests the pure function; HTTP calls are covered by
// integration tests.
func TestSuggestedModelsEnrichment(t *testing.T) {
    installed := map[string]bool{
        "nomic-embed-text:latest": true,
    }
    for _, spec := range embed.SuggestedModels {
        isInstalled := installed[spec.Name] || installed[spec.Name+":latest"]
        if spec.Name == "nomic-embed-text" && !isInstalled {
            t.Errorf("nomic-embed-text should be detected as installed")
        }
        if spec.Name == "mxbai-embed-large" && isInstalled {
            t.Errorf("mxbai-embed-large should not be detected as installed")
        }
    }
}
```

- [ ] **Step 2: Run test to verify it compiles and passes**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/mcp/... -count=1 -run TestSuggestedModelsEnrichment
```
Expected: PASS.

- [ ] **Step 3: Add `handleMemoryModels` to `internal/mcp/tools.go`**

Append to the end of `internal/mcp/tools.go` (before the final closing brace if any, or just append):

```go
// handleMemoryModels returns installed Ollama embedding models merged with
// the curated SuggestedModels registry. Calls GET /api/tags on the Ollama
// server; requires the ollama base URL from pool config.
func handleMemoryModels(ctx context.Context, pool engine.Pool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
    args := req.GetArguments()
    _ = args // no required args

    cfg := pool.Config()
    ollamaBase := cfg.OllamaBaseURL

    // Fetch installed models from Ollama /api/tags
    installed, err := fetchInstalledOllamaModels(ctx, ollamaBase)
    if err != nil {
        // Non-fatal: return registry with installed=false for all
        installed = map[string]bool{}
    }

    type modelEntry struct {
        Name        string `json:"name"`
        Dimensions  int    `json:"dimensions"`
        SizeMB      int    `json:"size_mb"`
        Description string `json:"description"`
        Recommended bool   `json:"recommended"`
        Installed   bool   `json:"installed"`
    }

    suggested := make([]modelEntry, 0, len(embed.SuggestedModels))
    for _, s := range embed.SuggestedModels {
        suggested = append(suggested, modelEntry{
            Name:        s.Name,
            Dimensions:  s.Dimensions,
            SizeMB:      s.SizeMB,
            Description: s.Description,
            Recommended: s.Recommended,
            Installed:   installed[s.Name] || installed[s.Name+":latest"],
        })
    }

    installedList := make([]string, 0, len(installed))
    for name := range installed {
        installedList = append(installedList, name)
    }
    sort.Strings(installedList)

    result := map[string]any{
        "current":   cfg.EmbedModel,
        "installed": installedList,
        "suggested": suggested,
    }
    return toolResult(result)
}

// fetchInstalledOllamaModels calls GET /api/tags and returns a set of installed
// model names (both bare and ":latest"-suffixed).
func fetchInstalledOllamaModels(ctx context.Context, baseURL string) (map[string]bool, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
    if err != nil {
        return nil, err
    }
    hc := &http.Client{Timeout: 10 * time.Second}
    resp, err := hc.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    var result struct {
        Models []struct {
            Name string `json:"name"`
        } `json:"models"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    names := make(map[string]bool, len(result.Models)*2)
    for _, m := range result.Models {
        names[m.Name] = true
        if base, _, ok := strings.Cut(m.Name, ":"); ok {
            names[base] = true
        }
    }
    return names, nil
}
```

**Note:** This requires `engine.Pool` to expose `Config()` returning a struct with `OllamaBaseURL` and `EmbedModel` fields. Check `internal/engine/` — if `Pool` does not have `Config()`, add a `ToolsConfig` accessor or pass `baseURL` and `model` as separate closure parameters in the handler registration (see Task 5 for how this is wired in `server.go`).

- [ ] **Step 4: Add required imports to `tools.go`** (if not already present)

Check the import block at the top of `internal/mcp/tools.go`. Add any missing:
```go
import (
    // ... existing imports ...
    "net/http"
    "sort"
    "strings"
    "time"

    "github.com/petersimmons1972/engram/internal/embed"
)
```

- [ ] **Step 5: Run build**

```bash
cd /home/psimmons/projects/engram-go && go build ./internal/mcp/...
```
Expected: clean build. If `engine.Pool.Config()` is missing, the error message will point you to the interface — add the method there and implement it in the concrete type.

---

### Task 4: Implement `handleMemoryEmbeddingEval`

**Files:**
- Modify: `internal/mcp/tools.go`

This handler creates ephemeral `OllamaClient` instances for two models, embeds a fixed probe set, computes cosine similarity rankings, and returns a comparison. It is read-only — no stored embeddings are modified.

- [ ] **Step 1: Write the failing test (probe logic)**

Add to `internal/mcp/tools_models_test.go`:

```go
func TestCosineSimilarityRankConsistency(t *testing.T) {
    // Verify that the cosine ranking helper preserves relative order.
    // cosine(a, b) = dot(a,b) / (|a| * |b|)
    a := []float32{1.0, 0.0, 0.0}
    b := []float32{0.9, 0.1, 0.0}
    c := []float32{0.0, 1.0, 0.0}
    query := []float32{1.0, 0.0, 0.0}

    simAQ := cosineSim32(query, a)
    simBQ := cosineSim32(query, b)
    simCQ := cosineSim32(query, c)

    if simAQ < simBQ {
        t.Errorf("a should be more similar to query than b: got simA=%.4f simB=%.4f", simAQ, simBQ)
    }
    if simBQ < simCQ {
        t.Errorf("b should be more similar to query than c: got simB=%.4f simC=%.4f", simBQ, simCQ)
    }
}
```

Note: `cosineSim32` will be a package-level function added in the next step. You'll need to export it or move the test to the `mcp` package (not `mcp_test`) if unexported.

- [ ] **Step 2: Add `cosineSim32` helper and `handleMemoryEmbeddingEval` to `tools.go`**

```go
// cosineSim32 computes cosine similarity between two float32 vectors.
// Returns 0.0 if either vector is zero-magnitude.
func cosineSim32(a, b []float32) float64 {
    if len(a) != len(b) || len(a) == 0 {
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
    return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// evalProbeSentences is the fixed probe set used by memory_embedding_eval.
// Chosen to cover semantic similarity, antonyms, and domain-specific terms.
var evalProbeSentences = []string{
    "deploy kubernetes cluster",
    "rollback failed deployment",
    "database migration failed",
    "postgres connection refused",
    "memory recall returned empty",
    "the quick brown fox jumps",
    "unrelated topic about cooking",
}

// handleMemoryEmbeddingEval compares two Ollama embedding models by embedding
// evalProbeSentences with each, computing pairwise cosine similarities, and
// reporting which model produces better-separated clusters.
//
// model_b defaults to the first Recommended model in embed.SuggestedModels
// when omitted.
func handleMemoryEmbeddingEval(ctx context.Context, pool engine.Pool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
    args := req.GetArguments()

    modelA := getString(args, "model_a", "nomic-embed-text")
    modelB := getString(args, "model_b", "")
    if modelB == "" {
        if rec := embed.DefaultRecommendedModel(); rec != nil {
            modelB = rec.Name
        } else {
            modelB = "mxbai-embed-large"
        }
    }

    cfg := pool.Config()
    ollamaBase := cfg.OllamaBaseURL

    // Create ephemeral clients for both models. Auto-pull is handled by
    // newOllamaClient via ensureModel.
    clientA, err := embed.NewOllamaClient(ctx, ollamaBase, modelA)
    if err != nil {
        return nil, fmt.Errorf("memory_embedding_eval: model_a %q: %w", modelA, err)
    }
    clientB, err := embed.NewOllamaClient(ctx, ollamaBase, modelB)
    if err != nil {
        return nil, fmt.Errorf("memory_embedding_eval: model_b %q: %w", modelB, err)
    }

    // Embed all probe sentences with both models
    type embedResult struct {
        sentence string
        vec      []float32
    }
    embedAll := func(c *embed.OllamaClient) ([]embedResult, error) {
        results := make([]embedResult, 0, len(evalProbeSentences))
        for _, s := range evalProbeSentences {
            vec, err := c.Embed(ctx, s)
            if err != nil {
                return nil, fmt.Errorf("embed %q: %w", s, err)
            }
            results = append(results, embedResult{sentence: s, vec: vec})
        }
        return results, nil
    }

    vecsA, err := embedAll(clientA)
    if err != nil {
        return nil, fmt.Errorf("memory_embedding_eval: model_a embeddings: %w", err)
    }
    vecsB, err := embedAll(clientB)
    if err != nil {
        return nil, fmt.Errorf("memory_embedding_eval: model_b embeddings: %w", err)
    }

    // Compute mean pairwise cosine similarity for each model
    // Lower mean similarity across unrelated pairs = better separation
    meanSim := func(vecs []embedResult) float64 {
        if len(vecs) < 2 {
            return 0
        }
        var total float64
        count := 0
        for i := 0; i < len(vecs); i++ {
            for j := i + 1; j < len(vecs); j++ {
                total += cosineSim32(vecs[i].vec, vecs[j].vec)
                count++
            }
        }
        return total / float64(count)
    }

    simA := meanSim(vecsA)
    simB := meanSim(vecsB)

    recommendation := modelA
    reason := "lower mean pairwise similarity indicates better semantic separation"
    if simB < simA {
        recommendation = modelB
    }

    result := map[string]any{
        "model_a": map[string]any{
            "name":                  modelA,
            "dimensions":            clientA.Dimensions(),
            "mean_pairwise_cosine":  simA,
        },
        "model_b": map[string]any{
            "name":                  modelB,
            "dimensions":            clientB.Dimensions(),
            "mean_pairwise_cosine":  simB,
        },
        "recommendation": recommendation,
        "reason":         reason,
        "note":           "This comparison uses probe sentences only. Run memory_migrate_embedder to apply the chosen model to stored embeddings.",
        "probe_count":    len(evalProbeSentences),
    }
    return toolResult(result)
}
```

- [ ] **Step 3: Add `math` to imports in `tools.go`** if not present

```go
import (
    // existing...
    "math"
)
```

- [ ] **Step 4: Run build**

```bash
cd /home/psimmons/projects/engram-go && go build ./internal/mcp/...
```
Expected: clean build.

---

### Task 5: Register tools in `internal/mcp/server.go`

**Files:**
- Modify: `internal/mcp/server.go`

- [ ] **Step 1: Add handler registrations**

In `registerTools()`, after the existing tool registrations (e.g., after `memory_feedback` at line ~505), add:

```go
{"memory_models", "List installed and suggested Ollama embedding models. Shows which suggested models are installed, which is current, and flags the recommended upgrade.",
    func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
        return handleMemoryModels(ctx, pool, req)
    }},
{"memory_embedding_eval", "Compare two Ollama embedding models using probe sentences. model_a defaults to nomic-embed-text; model_b defaults to mxbai-embed-large (recommended). Auto-pulls missing models. Read-only — does not migrate stored embeddings.",
    func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
        return handleMemoryEmbeddingEval(ctx, pool, req)
    }},
```

- [ ] **Step 2: Run full test suite**

```bash
cd /home/psimmons/projects/engram-go && go test ./... -count=1 -race 2>&1 | tail -20
```
Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/mcp/tools.go internal/mcp/server.go internal/mcp/tools_models_test.go && git commit -m "feat(mcp): add memory_models and memory_embedding_eval tools"
```

---

## Verification

```bash
# Build check
cd /home/psimmons/projects/engram-go && go build ./...

# Full test suite
cd /home/psimmons/projects/engram-go && go test ./... -count=1 -race
```

- `memory_models` → returns `suggested` list with `mxbai-embed-large` flagged as `recommended: true`
- `memory_embedding_eval` with `model_a="nomic-embed-text"` (no model_b) → auto-selects `mxbai-embed-large`, returns comparison JSON with `recommendation` field
- Server startup with any `ENGRAM_OLLAMA_MODEL` → dimensional guard logs actual dims, no hardcoded 768 rejection
