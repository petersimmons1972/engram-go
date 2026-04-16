---
name: Engram Store Big Retrieve Small
description: Architecture decisions and implementation details for Engram's chunk-level retrieval feature — shipped 2026-04-09
type: project
Category: project
originSessionId: 98b53916-59ae-42ac-b0a2-e5b0d51c3537
---
## Engram "Store Big, Retrieve Small" — Shipped 2026-04-09

Three phases committed to main and pushed to GitHub (petersimmons1972/engram):

**Phase 1** (7d5a224): MAX_CONTENT_LENGTH 50k->500k, LAZY_CHUNK_THRESHOLD 2k->8k, removed 200-char matched_chunk truncation, added chunk_score/matched_chunk_index to SearchResult. Also fixed pre-existing chunk_hash_exists KeyError bug.

**Phase 2** (80165d4): Semantic chunker (chunk_document() -- headings->paragraphs->sentence-window fallback with overlap), schema v9 (storage_mode on memories, section_heading/chunk_type/last_matched on chunks), memory_store_document MCP tool, storage_mode="auto" on memory_store, passive last_matched tracking in recall().

**Phase 3** (347cad2): Cold document pruning in consolidate() -- documents with zero matched chunks after 60 days get pruned. Also fixed pre-existing conftest teardown bug (eliminated 92 spurious test errors).

## Architecture Decisions

- Two storage modes coexist: "focused" (current, <=10k, agent-curated) and "document" (new, <=500k, system-chunked). Auto-detection at 10k chars.
- Semantic chunking splits on markdown headings first, then paragraphs, then sentence-window fallback. Each chunk carries section_heading metadata.
- Scoring formula unchanged -- already correct at chunk level.
- Knowledge graph stays memory-level. Chunk-to-chunk relationships deferred.
- last_matched set passively in recall() (same pattern as touch_memory).
- Cold document pruning is usage-based: zero chunks matched + 60 days + importance>=3 = prune.

## Deferred (GitHub Issues)

- Chunk-level importance_delta
- Chunk-level FTS (BM25 against chunk text)
- Chunk-to-memory graph relationships
- memory_ingest markdown awareness
- Supervised chunk feedback table

## Process Notes

- Yamamoto designed the strategic architecture, Grace Hopper designed the ship-fast implementation. Plan synthesized both: Hopper's pragmatism for Phase 1, Yamamoto's semantic chunking for documents, compromise on scoring.
- Container needs proper image rebuild: `cd ~/projects/engram && docker compose up -d --build engram` (Chainguard registry was slow during session)
- Schema v9 migration auto-runs on first MCP connection after restart.
