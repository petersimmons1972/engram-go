---
name: engram-redesign-plan
description: Architecture redesign plan for Engram based on 30-issue adversarial review and 7-agent research/debate team
type: project
---

Engram architecture redesign plan written 2026-03-25 after adversarial code review (30 issues, GitHub #45-#75) and research operation with 7 agents (3 research streams + 4 generals).

**Key decisions:**
- Replace weighted linear combination scoring with FTS-first + vector re-rank + recency tiebreaker (Phase 1)
- Defer RRF and pgvector to Phase 2 (when data exceeds ~1,000 memories)
- Graph becomes enrichment only, not a scoring signal
- Importance as WHERE filter, not multiplicative boost
- Relationships table stays (JSONB rejected — breaks supersedes detection)
- Add project column to relationships table (Issue #75: cross-project edge pollution)
- Lazy chunking: skip for memories under 2000 chars
- Test-first for every change

**Why:** Research found Engram's scoring formula is used by zero production systems. Mem0, Zep, Weaviate, Qdrant all use RRF or vector-authoritative ranking. But at 68 memories with one user, full RRF and pgvector are premature.

**How to apply:** See /home/psimmons/projects/engram/docs/architecture-redesign-plan.md for full phased plan with issue resolution map.

**Phase 1 scope:** ~200-300 lines of changes, fixes ~20 of 30 issues.
