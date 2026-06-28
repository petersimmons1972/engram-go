# Socialization Brief: Chonkie Chunking Concepts for LME-S and LME-M Score Lift

**Branch:** `worktree-lme-preference-constraint`
**Date:** 2026-06-28
**Author:** Chester William Nimitz (Claude Code, session d46a7447)
**Audience:** Codex (impl), Hermes (consult), Grok (grok)

---

## 1. Situation

engram-go is a Go memory server benchmarked against LongMemEval (LME). Current best:

| Benchmark | Score | Run |
|-----------|-------|-----|
| LME-M (500Q, 500 sessions) | **78.6%** | camp-22, 378/500Q |
| LME-S KU-78 (knowledge-update slice) | 70.0% strict | KU-B1 (KU-R1 at 67.1% LOW CONFIDENCE NEUTRAL) |

Four LME-S question types: `temporal-reasoning`, `single-session-preference`, `knowledge-update`, `multi-session`.

**Goal:** Lift LME-S and LME-M scores via improved chunking at ingest time.

---

## 2. Source: Chonkie (chonkie-inc/chonkie)

Python chunking library v1.6.8, MIT, ≥Python 3.10.
GitHub: https://github.com/chonkie-ai/chonkie

**Decision: do NOT import as a library.** Reasons:
- Python/Go barrier (subprocess or embedding server required)
- HuggingFace weight downloads on `import`
- RCE surface from `transformers` pickle deserialization

Instead: adopt three algorithmic concepts natively in Go.

---

## 3. Architecture (critical — read before implementing)

### Ingest path

```
cmd/longmemeval/ingest.go :: ingestOne()
  → for each session in item.HaystackSessions:
      content = longmemeval.SessionContent(session)  // concatenates turns
      id, err = restClient.QuickStore(ctx, project, content, tags, expiresAt)
  → server-side: /quick-store endpoint calls chunk.ChunkText(content, maxTokens, overlapTokens)
```

**Key fact:** `ChunkText` is in `internal/chunk/chunker.go:99`, NOT in `engine.go`. The benchmark
client currently sends whole session content; the server chunks it with a fixed overlap. Phase 1
changes this: the client pre-chunks per session and stores each chunk as a separate memory.

### Run path (recall + generate)

```
cmd/longmemeval/run.go :: runOne()
  → recall from Engram
  → truncate each context block at cfg.MaxBlockChars (prompt-assembly layer, NOT chunker)
  → build generation prompt
  → GenerateOAIWithOpts() → Qwen3-32B on oblivion via Olla router
```

`--max-block-chars` lives in the **prompt-assembly** layer. It is NOT a chunking parameter.
Never confuse the two.

### ChunkText signature

```go
// internal/chunk/chunker.go:99
func ChunkText(text string, maxTokens, overlapTokens int) []string
```

- `maxTokens`: 1 token ≈ 4 chars; content ≤ maxTokens*4 chars → returned as single chunk
- `overlapTokens`: tail sentences carried into next chunk; 50 is the current server default
- `LazyChunkThreshold = 8000` chars: content below this is single-chunk on server side
- `sentenceSplitRE = regexp.MustCompile(`(?:[.!?])\s+`)` at chunker.go:30 — known bug:
  splits on abbreviations (Dr., Mr., U.S.)

---

## 4. Three Concepts to Adopt (in order)

### Concept 1: OverlapRefinery (from Chonkie `OverlapRefinery`)

**What:** Slide a configurable char-window of tail sentences into the next chunk at ingest.

**Implementation:**
- Add `BlockOverlapChars int` to `Config` in `cmd/longmemeval/main.go`
- Register flag `--block-overlap-chars` (default 0 = disabled)
- In `ingestOne()` (cmd/longmemeval/ingest.go): when `BlockOverlapChars > 0`, call
  `chunk.ChunkText(content, targetMaxTokens, cfg.BlockOverlapChars/charsPerToken)` and
  loop over returned chunks, storing each separately via `QuickStore`
- `charsPerToken = 4` (same constant as chunker.go)
- `targetMaxTokens`: use a sensible default like `LazyChunkThreshold / charsPerToken = 2000`
  (meaning each chunk ≤ 8000 chars = ~2000 tokens)
- Session boundary: naturally respected — `ingestOne` already processes one session at a time;
  `ChunkText` operates entirely within one session's string. No cross-session overlap is possible.

**Parse-time guard (REQUIRED):**
```go
if cfg.BlockOverlapChars >= chunk.LazyChunkThreshold/2 {
    return fmt.Errorf("--block-overlap-chars must be < %d (LazyChunkThreshold/2)", chunk.LazyChunkThreshold/2)
}
```

**Import:** `"github.com/petersimmons1972/engram/internal/chunk"` from `cmd/longmemeval/ingest.go`

### Concept 2: Turn-boundary-aware splitting (from Chonkie SDPMChunker boundary semantics)

**What:** When splitting session content into chunks, never split mid-turn. Turn delimiters
(`\nUser:`, `\nAssistant:`, `\nhuman:`, `\nassistant:`) are added to the set of valid
sentence-split positions.

**Implementation:**
- Modify `splitSentences()` in `internal/chunk/chunker.go` to detect turn-start patterns and
  treat them as mandatory split points (higher priority than sentence-ending punctuation)
- OR: add a new `splitSentencesWithTurnBoundaries(text string) []string` that wraps
  `splitSentences` and inserts forced splits at turn boundaries
- Gate via `--turn-boundary-chunking` bool flag in Config (default: false to stay conservative)

### Concept 3: Semantic similarity grouping (from Chonkie `SemanticChunker`)

**What:** At split time, compute embedding cosine similarity between adjacent sentences.
When similarity drops below threshold, treat as a strong split point.

**Implementation (gated — only after Phase 2 shows ≥+3pp delta):**
- Add `SemanticChunkThreshold float64` and `SemanticChunkModel string` to Config
- Flags: `--semantic-chunk-threshold=<float>` and `--semantic-chunk-model=<id>` (both required together)
- New file: `internal/chunk/semantic_chunk.go` with:
  - `CosineSimilarity(a, b []float32) float64` (stdlib only)
  - `EmbedSentences(ctx, sentences []string, modelID, embedURL string) ([][]float32, error)`
- Zero new Go dependencies
- Use Olla router embedding endpoint if available:
  `http://192.168.0.138:30411/olla/openai/v1/embeddings`

---

## 5. Four QA Blocker Corrections (must be observed by all implementors)

1. **`--max-block-chars` is prompt-assembly, NOT a chunker param.** The new `--block-overlap-chars`
   flag is a distinct namespace in the ingest/chunking layer.

2. **Semantic flag naming.** `--semantic-chunk` boolean is rejected. Correct form:
   `--semantic-chunk-threshold=<float> --semantic-chunk-model=<id>`. Both required together or
   neither (validate at parse time).

3. **Parse-time guard for overlap.** If `BlockOverlapChars >= LazyChunkThreshold/2 = 4000`:
   return nonzero exit with clear message.

4. **Session boundary is automatically respected** when doing client-side pre-chunking per session
   in `ingestOne()`. No additional flush mechanism required — each session's content is chunked
   independently.

---

## 6. Abbreviation Splitter Fix (pre-Phase-1, SRE finding S4)

`sentenceSplitRE = regexp.MustCompile(`(?:[.!?])\s+`)` at `internal/chunk/chunker.go:30`
splits on abbreviations like "Dr. Smith" or "U.S. dollar". This degrades sentence segmentation.

**Fix:** Replace with a two-pass approach:
1. Protect known abbreviations by replacing their periods with a sentinel (`\x00`)
2. Apply the sentence split regex
3. Restore sentinels

Or use a negative lookbehind workaround (Go's `regexp` package does NOT support lookbehinds —
must use the replacement approach or a tokenizing approach).

Known abbreviations: `Mr`, `Mrs`, `Dr`, `Prof`, `Sr`, `Jr`, `vs`, `etc`, `U.S`, `U.K`, `Inc`, `Ltd`, `Corp`, `Co`, `Ave`, `Blvd`, `Dept`, `Est`

---

## 7. TDD Requirements (CLAUDE.md: test before implementation)

### Phase 1 tests (create BEFORE implementation):

**File:** `internal/chunk/chunker_overlap_test.go`

```go
// Test 1: overlap tokens reach the chunk output
func TestChunkText_OverlapCarriesForward(t *testing.T) {
    // Long text with many sentences; verify second chunk starts with sentences from first chunk's tail
}

// Test 2: zero overlap produces current behavior
func TestChunkText_ZeroOverlapUnchanged(t *testing.T) {
    // Same text with overlapTokens=0; output must match existing behavior exactly
}

// Test 3: ingestOne stores N chunks when content > LazyChunkThreshold
func TestIngestOne_PreChunksWhenBlockOverlapSet(t *testing.T) {
    // cfg.BlockOverlapChars = 400; content > 8000 chars; assert >1 QuickStore call
}

// Test 4: parse-time guard fires when overlap >= LazyChunkThreshold/2
func TestConfig_OverlapGuardRejectsHalfOrMore(t *testing.T) {
    // cfg.BlockOverlapChars = 4000; validate() returns nonzero
}
```

**File:** `internal/chunk/chunker_turnboundary_test.go` (Phase 2)

```go
// Test 1: chunk never straddles a User:/Assistant: boundary
func TestChunkText_TurnBoundaryNeverSplitsMidTurn(t *testing.T)

// Test 2: every chunk starts at a turn boundary or text start
func TestChunkText_TurnBoundaryChunkStartsAtTurnBeginning(t *testing.T)

// Test 3: fallback graceful when no delimiters found
func TestChunkText_TurnBoundaryFallsBackOnNoDelimiters(t *testing.T)
```

---

## 8. Per-Node Asks

### Codex (impl)

Full implementation of Phases 1–3 in Go, TDD-first. Specific questions before starting:
1. What is the correct import path for the `chunk` package from `cmd/longmemeval/ingest.go`?
   (Expected: `github.com/petersimmons1972/engram/internal/chunk`)
2. For `targetMaxTokens` in the pre-chunking call, use `LazyChunkThreshold / charsPerToken = 2000`
   or read from a separate flag?
3. Should `--turn-boundary-chunking` be a toggle flag or should it always be active when
   `--block-overlap-chars > 0`?

### Hermes (consult)

Strategic review. Questions:
1. Produce a gated sequence plan (2A→2E format matching `lme-phase2-hermes-gates.md`):
   which phase gates should we add beyond the ones in the plan?
2. For each of the four question types (temporal, preference, KU, multi-session):
   which chunking change is most likely to lift that type?
3. Are there Chonkie concepts we've missed that could help (e.g., `RecursiveChunker`,
   `TokenChunker`, `WordChunker`)?

Output format: `GATE_ID | GO | rationale` table, plus per-question-type commentary.
Always report deltas BY QUESTION TYPE.

### Grok (grok)

Strategic alternatives. Questions:
1. The plan uses sequential overlap → turn-boundary → semantic. Does the ROI ordering
   look correct to you, or would you reorder?
2. Are there concepts in Chonkie beyond the three we identified that would specifically
   help with temporal-reasoning or multi-session question types in a long-context benchmark?
3. What Go implementation pitfalls should Codex watch for in the sentence-splitter
   abbreviation fix (stdlib regex does not support lookbehinds)?

Output format: `Concept | ADOPT | PARTIAL | REJECT | reason` table, plus ordering commentary.

---

## 9. Constraints

- **No Sonnet for generating** — use Qwen3-32B on oblivion via Olla router for all generation/scoring
- **oblivion/spark for testing** — use these GPU servers for benchmark runs
- **No `--enable-thinking`** — flag code says "do NOT use with Nemotron v3"
- **No `--llm-api-key` needed** — Olla endpoint unauthenticated
- **MI-50 live path** — do NOT reembed for any reason
- **No huge parallelism on LME-S** — use `--workers 2` max for KU-78 runs on oblivion

---

## 10. Files Index

| File | Role |
|------|------|
| `internal/chunk/chunker.go` | ChunkText, splitSentences, LazyChunkThreshold |
| `cmd/longmemeval/main.go` | Config struct, all flag registrations |
| `cmd/longmemeval/ingest.go` | ingestOne(), QuickStore call site |
| `cmd/longmemeval/run.go` | runOne(), prompt assembly, MaxBlockChars usage |
| `specs/chonkie-concepts-lme-lift-plan.html` | Full plan with 5 phases, questionables |
| `results/benchmark-registry.jsonl` | All run results; register new runs here |
