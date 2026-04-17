# Sources & Citation Manifest

**Report:** Engram-Go vs. Iusztin GraphRAG — Architectural Comparison
**Date:** 2026-04-17
**QA:** Gordon Ramsay review required before publication

## Evidence Asymmetry — Flag This At The Top Of The Report

- **Engram-Go claims:** sourced from the actual code at `~/projects/engram-go` (~22,000 LOC Go, 11 SQL migrations, file paths cited per claim).
- **Iusztin GraphRAG claims:** sourced from a single Substack note (`https://substack.com/@pauliusztin/note/c-242614337`) — the public description. No source code was inspected.
- **Layton's caveat stands:** "You cannot compare a system you have read against a system you have heard described and claim the comparison is fair." This asymmetry is itself the headline.

## Citation Table

| # | Claim | Source |
|---|---|---|
| C1 | Engram-Go uses PostgreSQL + pgvector across 6+ tables | `internal/db/migrations/001–010_*.sql` |
| C2 | 5-signal composite scoring with weights 0.45/0.30/0.10/0.15 + importance | `internal/search/score.go` |
| C3 | Recency decay `exp(-0.01 * hours)` | `internal/search/score.go` |
| C4 | Ollama-only embeddings, 768-dim, fallback to BM25+recency | `internal/embed/ollama.go`, `internal/search/engine.go:263` |
| C5 | Graph = edges-in-Postgres, recursive CTE traversal | `internal/db/postgres_relationship.go:GetConnected` |
| C6 | 28 MCP tools | `internal/mcp/tools.go`, `server.go` |
| C7 | Tiered ingestion (4 size buckets, synopsis+documents table for >500KB) | `internal/mcp/ingest_document.go:classifyDocumentSize` |
| C8 | Spaced-repetition decay worker × 0.95 every 8h | `internal/search/decay.go:DecayWorker.runOnce` |
| C9 | Sleep cycle: MinHash LSH + LLM contradictions + auto-supersede | `internal/search/sleep.go`, `llm_contradiction.go` |
| C10 | Retrieval feedback loop (times_useful/times_retrieved) | `internal/db/migrations/007_retrieval_events.sql` |
| C11 | Bearer auth required (v3.0); auto-episode per SSE session | `internal/mcp/server.go:OnRegisterSession`, README §v3.0 |
| C12 | Cross-project federation via `memory_adopt` | `internal/mcp/tools.go:memory_adopt` |
| C13 | Bi-temporal versioning (`valid_from`, `valid_to`, `invalidation_reason`) | `internal/db/migrations/006–007` |
| C14 | Tier classification bytes: 500KB / 8MB / 50MB | `internal/mcp/ingest_document.go:43` |
| C15 | Ingest parallelism: 8 concurrent embeds | `internal/search/engine.go:249` |
| C16 | MongoDB unified `knowledge_graph` collection | Iusztin Substack note |
| C17 | Voyage AI embeddings | Iusztin Substack note |
| C18 | Entity + relationship extraction during ingestion | Iusztin Substack note |
| C19 | Entity deduplication step | Iusztin Substack note |
| C20 | 3 MCP tools: query, progressive expansion, ingest | Iusztin Substack note |
| C21 | Claude Code as orchestrator | Iusztin Substack note |
| C22 | Iusztin: no disclosed scoring function, weights, auth, consolidation, feedback | **Absence** in Substack note — tagged `[NOTE-absent]` in Layton's assessment |

## Analyst Deliverables Captured

- Arnold capability-gap matrix (8 axes, HAS/PARTIAL/LACKS + cost) — in synthesis section
- Layton confidence-graded pro/con (5+5, HIGH/MED/LOW) — in synthesis section
- Galland "mutual theft" tactical list (4+4) — in synthesis section
- Yamamoto paradigm-limit analysis (4 sections + 10-year question) — in synthesis section
- Kulik zero-context failure audit (5 anti-patterns × 2 designs) — in synthesis section

## Inferences Explicitly Labeled

- "Voyage currently produces stronger retrieval than Ollama defaults" — **INFERENCE** from public benchmarks, not from the Substack note.
- "28-tool surface increases orchestrator error rate" — **INFERENCE**, not measured.
- "Recursive CTE hits planner cost at 4+ hops" — **INFERENCE** based on PostgreSQL behavior at scale, not measured against engram-go's graph.
- "Iusztin has no fallback if Voyage is down" — **INFERENCE from the absence of disclosure**; the note does not describe a fallback, but that does not prove there isn't one.
