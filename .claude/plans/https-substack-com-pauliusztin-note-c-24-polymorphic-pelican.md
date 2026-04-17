# Engram-Go vs. Iusztin GraphRAG — Magazine Spread Comparison

## Context

Paul Iusztin (Substack note c-242614337) published a unified GraphRAG architecture: MongoDB as single memory, Voyage AI embeddings, entity/relationship extraction, three MCP tools, Claude Code as orchestrator. The user wants a rigorous comparison of this design against the actual engram-go **code** (not the README) at `~/projects/engram-go`, produced as a single standalone HTML magazine spread, with a LinkedIn journalist documenting the process.

This is an architectural-critique piece, not a marketing doc. The comparison must be fair, specific, and cite real engram-go file paths / function names.

## Team Assembly

| Role | Operator | Deliverable |
|---|---|---|
| Field Marshal (coordinator) | **Montgomery** | Parallel research plan, integration, schedule discipline |
| Designer | **Mucha** | Single standalone HTML magazine spread, inline SVG charts, editorial elegance |
| QA specialist | **Gordon Ramsay** | Reads every citation back to the code; kills any claim not grounded in a file path |
| Unconventional thinker #1 | **Galland** | What engram-go *should steal* — tactical innovations from the other side |
| Unconventional thinker #2 | **Yamamoto** | Paradigm-level: what each design *cannot* win at |
| Specialist #1 | **Arnold** | R&D / capability-gap analysis of each architecture |
| Specialist #2 | **Layton** | Intelligence synthesis — confidence-graded pro/con matrix |
| Zero-context observer | **Kulik** | Failure-pattern audit on each design, no prior findings |
| Journalist | **Pyle** (via /write skill) | LinkedIn dispatch documenting the whole engagement |

## Established Facts (from Phase 1 exploration)

**Iusztin GraphRAG (from Substack note):**
- MongoDB unified collection (`knowledge_graph`) with text + vector + graph indexes co-located
- Voyage AI embeddings
- Entity + relationship extraction via graph analysis in ingestion pipeline
- 3 MCP tools: query, progressive graph expansion, ingest
- Claude Code as agentic orchestrator via skill-based routing

**Engram-Go (from actual code — `~/projects/engram-go/internal/`):**
- PostgreSQL + pgvector, **separate** tables: `memories`, `chunks`, `relationships`, `documents`, `episodes`, `retrieval_events` (11 migrations, `internal/db/migrations/`)
- **Ollama-only** embeddings, 768-dim, best-effort fallback (`internal/embed/ollama.go`)
- Graph = edges-in-Postgres, recursive CTE multi-hop (`internal/db/postgres_relationship.go:GetConnected()`)
- **5-signal composite scoring**: vector 0.45 + BM25 0.30 + recency 0.10 (exp(-0.01h)) + precision 0.15 + importance (`internal/search/score.go`)
- **28 MCP tools** (not 3): store, recall, connect, correct, consolidate, sleep, feedback, episode_*, timeline, verify, diagnose, adopt, migrate_embedder, etc.
- **Tiered ingestion**: ≤500KB inline, 500KB–8MB synopsis+chunks, 8MB–50MB synopsis+raw-in-documents-table, >50MB reject (`internal/mcp/ingest_document.go`)
- **Spaced repetition**: `dynamic_importance` decay worker, `retrieval_interval_hrs` growth on positive feedback (`internal/search/decay.go`)
- **Consolidation/sleep cycle**: heuristic + MinHash LSH + optional LLM contradiction detection, auto-supersede (`internal/search/sleep.go`, `llm_contradiction.go`)
- **Retrieval feedback loop**: `retrieval_events` table, `times_useful/times_retrieved` → precision signal feeds next recall
- **v3.0 additions**: bearer auth required, auto-episode-start on every SSE session
- **Cross-project federation**: `memory_adopt` creates edges across project boundaries

## Execution Plan

### Phase A — Parallel Analysis (Montgomery dispatches)
Four agents run in parallel. Each receives the fact sheet above and returns ≤500 words.

1. **Arnold** — capability-gap matrix: for each axis (storage, embedding, graph, scoring, ops, extensibility), which design has the capability, which lacks it, and the engineering cost to add.
2. **Layton** — confidence-graded pro/con table. Every claim carries HIGH/MEDIUM/LOW confidence and evidence source (code file vs. Substack note vs. inference).
3. **Galland** — "what engram-go should steal": Voyage embeddings option? Entity extraction? Unified collection? Which are tactically viable vs. architectural dead-ends.
4. **Yamamoto** — "what each design cannot win at": Iusztin's design can't do X at scale; engram-go can't do Y. Paradigm limits, not feature gaps.

### Phase B — Independent Audit (parallel to A)
5. **Kulik** — zero-context failure-pattern audit. Receives only the two design descriptions (no Phase A output). Looks for the five anti-patterns of protected incompetence in each.

### Phase C — Integration + QA
6. **Montgomery** consolidates Phase A + B into a single comparison document with four sections:
   - **Design Diff** (table form, cited)
   - **Pro/Con by Design** (Layton's matrix)
   - **Mutual Theft List** (Galland: tactical, Arnold: capability, Yamamoto: paradigm)
   - **Kulik's Independent Verdict**
7. **Ramsay** reviews the consolidated doc. Every claim must trace to: a file path in engram-go, or a specific line from the Substack note, or be labeled as `inference`. Anything unsourced gets killed.

### Phase D — Magazine Production
8. **Mucha** produces `/home/psimmons/reports/engram-vs-graphrag/index.html` — single standalone HTML, inline SVG charts, organic editorial elegance. Charts required:
   - **Storage topology diagram** — side-by-side (engram-go 6-table schema vs. Iusztin unified collection)
   - **Scoring formula visualization** — engram-go's 5-signal weighted composite (pie or weighted bar) vs. Iusztin's vector+graph
   - **Tool surface comparison** — 28 tools grouped by category vs. 3-tool surface (radial or stacked bar)
   - **Capability coverage radar** — both designs on 8 axes (Arnold's matrix)
   - **"Mutual Theft" flow diagram** — arrows showing what each should adopt from the other
   All charts must pass `visual-output-standards` skill requirements and `bin/render-check.sh`.
9. **Mucha** re-reads `visual-output-standards` skill **before** drawing anything. No generic AI infographic aesthetic.

### Phase E — Journalism
10. **Pyle** (via `/write` skill with writer=pyle) drafts a LinkedIn dispatch documenting the engagement itself: the team, the friction points, the verdicts. Ground-truth voice, not hype. Saved to `/home/psimmons/reports/engram-vs-graphrag/linkedin-dispatch.md`.

## Critical Files / References

**Read (source of truth for engram-go claims):**
- `~/projects/engram-go/internal/search/score.go` — scoring weights
- `~/projects/engram-go/internal/search/engine.go` — RecallWithOpts
- `~/projects/engram-go/internal/search/sleep.go` + `llm_contradiction.go` — consolidation
- `~/projects/engram-go/internal/db/postgres_relationship.go` — graph traversal
- `~/projects/engram-go/internal/db/migrations/*.sql` — schema
- `~/projects/engram-go/internal/mcp/tools.go` + `server.go` — tool registration
- `~/projects/engram-go/internal/mcp/ingest_document.go` — tiered ingestion
- `~/projects/engram-go/internal/embed/ollama.go` — embedding pipeline

**Skills to load before execution:**
- `visual-output-standards` (Mucha, before any chart)
- `write` (Pyle journalist dispatch)

**Output locations:**
- `/home/psimmons/reports/engram-vs-graphrag/index.html` — magazine spread
- `/home/psimmons/reports/engram-vs-graphrag/linkedin-dispatch.md` — Pyle dispatch
- `/home/psimmons/reports/engram-vs-graphrag/sources.md` — full citation manifest (Ramsay's audit trail)

## Verification

1. Open `index.html` in a browser; every chart renders without broken SVG.
2. `bin/render-check.sh /home/psimmons/reports/engram-vs-graphrag/` passes.
3. Ramsay's audit: every claim in the doc has a citation row in `sources.md`.
4. Spot-check: pick 5 random claims; open the cited file; verify the claim holds against current code.
5. Pyle dispatch reads as ground-truth reportage, not marketing — no superlatives without a cited observation.

## Engram Memory Capture (post-work)

Store decision memory after completion:
- `memory_type=decision`, `project=engram`, content: "Comparative architecture study vs. Iusztin GraphRAG completed YYYY-MM-DD. Key divergences: storage model, embedding provider, tool surface breadth, spaced-repetition. See report at reports/engram-vs-graphrag/."
- `memory_type=pattern`, `project=global`: "Magazine-spread comparison format: Montgomery coordinates 5-agent parallel research (Arnold capability / Layton pro-con / Galland tactical theft / Yamamoto paradigm / Kulik independent audit), Ramsay citation-audits, Mucha renders HTML, Pyle dispatches."
