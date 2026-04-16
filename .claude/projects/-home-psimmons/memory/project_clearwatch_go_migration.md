---
name: Clearwatch Go Migration Master Plan
description: Python-to-Go migration strategy for Clearwatch — 6 phases, atomic checkpoints, team of 6 generals, approved 2026-04-10
type: project
category: clearwatch
originSessionId: f13be8e1-9cae-4933-afaa-71fe701071a8
---
Migration master plan approved 2026-04-10. Drivers: Performance (goroutines) + Maintainability (types).

Plan file: `/home/psimmons/.claude/plans/snoopy-seeking-dragonfly.md`

**Why:** ~150K LOC Python, GIL caps chart parallelism at 4 threads, untyped dict contracts between stages are the biggest maintainability liability.

**6 phases + pre-migration:**
- PM-1: Schema archaeology script (capture inter-stage dict payloads as JSON Schema)
- PM-2: Delete legacy `pipeline.py` (853 LOC)
- PM-3: Regex equivalence audit (~50+ patterns, Go is RE2)
- PM-4: WeasyPrint/PDF decision — **RESOLVED 2026-04-10: playwright-go** (`playwright-community/playwright-go`). Same API as Python Playwright. `page.PDF()` for generation. Single Chromium binary. No chromedp.
- P0: Foundation (types, interfaces, CI, svgdiff tool) — no Python replacement
- P1: Thin pipeline (Stages 0, 0.5, 1-2, 8) — validates strangler fig pattern
- P2: Validators (all 17, 17.5K LOC) — on critical path before P5
- P3: Research (30 collectors) — **off critical path, parallel to P1+P2**
- P4: Charts (SVG library + 88 generators) — primary perf win
- P5: Generation (Stages 3, 5, 6)
- P6: Orchestrator (Stage 7, CLI, delete adapter) — migration complete

**Critical path:** P0 → P1 → P2 → P4 → P5 → P6

**Key decisions:**
- Bottom-up sequencing (types first, not stage-by-stage)
- Strangler fig at JSON boundary with feature flags per stage + 30-day stability window
- PDF: chromedp (Chromium headless) — founder must decide before P4
- SVG: port `svg_primitives.py` directly (no Go equivalent has geometry-aware BBox)
- Playwright collectors: permanent Python sidecar (no Go Playwright client)
- Stage 6: stays sequential (Gate 14 short-circuit is load-bearing)

**5 red lines** (migration stops if any broken):
1. Forbidden tier leakage
2. Silent data fabrication (MISSING sentinel)
3. Gate 14 short-circuit integrity
4. The $500 test (would_pay_500)
5. MTTD metric isolation

**How to apply:** Each new session working on this migration should start by reading the master plan and the relevant phase mini-plan. Phase mini-plans live at `/home/psimmons/.claude/plans/cw-migo-p{N}-{slug}.md` and must be written before implementation begins.
