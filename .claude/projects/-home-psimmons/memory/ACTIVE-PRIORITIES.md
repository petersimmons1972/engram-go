---
name: active-priorities
description: Current work focus and pending action items
type: project
Category: active-work
originSessionId: f13be8e1-9cae-4933-afaa-71fe701071a8
---
# Active Priorities

**Last Updated**: 2026-04-11
**Current Focus**: Memory housekeeping → Visual QA cache invalidation → Go migration P1

---

## Just Completed (2026-04-11)
- Closed #4217 (S1 vs PA Gate 14 CRIT-012) — v092 ships A-
- Closed #4237 (PA vs CS Stage 7 B+ TCO precision) — v132 ships A- after range-based TCO edit (commit 86e0189)
- Memory compression: lessons-learned.md 128L→55L, audio-pipewire-zoom.md 66L→28L, project_engram_go_cutover.md 50L→22L
- Added `Category:` front matter to 20 memory files

## Pending (priority order)
- [ ] **Visual QA cache invalidation** — spec at `docs/superpowers/specs/2026-04-10-visual-qa-cache-invalidation-design.md`. Five files to create (migration 003, store.py additions, loop.py hook, unit tests, integration tests). Unblocks #4234, #4235.
- [ ] **Go migration P2 — Stage 6 gates** — plan `~/.claude/plans/snoopy-seeking-dragonfly.md`. Worktree: `mig/p2-stage-6-gates`. **Task 5 batch 1 DONE** (commit `6931a73` — 19 gates, 68 tests). **7 code fixes dispatched, not yet confirmed** (Gate 29 viewBox, Gate 20 substring, Gate 36 regex loop, dead code, unsafe rune, stale comment, dead import). **Batch 2 still stubbed** (11 gates: 9, 10, 14-19, 25, 26, 28). Tasks 6-9 pending (CLI wiring, adapter, parity tests, checkpoint).

## Blocked/Deferred
| Task                       | Status                                      |
|----------------------------|---------------------------------------------|
| SCALE bugs (#2340-#2364)   | Deferred — 25 P2 concurrency bugs, non-blocking |
| NHI vendor data quality    | Open: #2904, #2923 — monitoring after RSAC  |
