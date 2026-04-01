# Design: CLAUDE.md + Memory Refactor — Engram as Primary Library

**Date:** 2026-04-01
**Status:** Approved
**Goal:** Move root CLAUDE.md to universal-only rules, push project-specific rules to project CLAUDE.mds, and migrate all long-form memory to Engram with MEMORY.md as a recovery map.

---

## Problem

Root `CLAUDE.md` carries project-specific rules (Clearwatch pipeline, DNS, art direction) that don't belong at the global level. The memory system has grown to 85 files / 3,010 lines against an 800-line budget — because files were the only durable store. Engram is now stable and trusted. The architecture should reflect that.

---

## Architecture

```
Root CLAUDE.md          ~55 lines — universal behaviors only
├── ~/projects/clearwatch/CLAUDE.md   — clearwatch pipeline, issue tracking, chart gates
└── ~/projects/homelab/CLAUDE.md      — k8s, cert-manager, DNS rules

AGENTS.md               ~150 lines — trim art direction to a pointer

MEMORY.md               ~40 lines — Engram recovery map (tags + queries, no content)

Engram (primary store)
├── project="global"     — lessons, feedback rules, user prefs, generals patterns
├── project="clearwatch" — pipeline lessons, chart rules, issue workflows
└── project="homelab"    — k8s patterns, cert-manager, DNS, incidents
```

---

## What Moves Where

| Source | Destination | Action |
|--------|-------------|--------|
| Clearwatch-specific rules in root CLAUDE.md | `clearwatch/CLAUDE.md` | Move |
| DNS / homelab rules in root CLAUDE.md | `homelab/CLAUDE.md` | Move |
| Art direction §9 in AGENTS.md | One-line pointer to `ART-DIRECTION-RULE.md` | Trim |
| `lessons-learned.md` (all domains) | Engram split by project | Migrate → delete |
| 51 session + feedback files | Engram `project="global"` or project-specific | Migrate → delete |
| 3 engram phase completion docs (573L) | Engram `project="global"` | Migrate → delete |
| `global-todo-list.md` (125L) | GitHub Issues; pointer in MEMORY.md | File issues → delete |
| Homelab quick-ref, k8s patterns, cert-manager, incidents | Engram `project="homelab"` | Migrate → delete |
| `chart-regression`, `clearwatch-*`, `rsac-*` files | Engram `project="clearwatch"` | Migrate → delete |
| `html-processing-patterns.md`, `url-validation-patterns.md` | Engram `project="global"` | Migrate → keep as file too (code-adjacent) |

---

## Files That Stay on Disk

These remain as files — needed during outages, referenced by scripts, or too structural to store as memories:

| File | Why Keep |
|------|----------|
| `CLAUDE.md` (root) | Universal rules — loaded every session |
| `AGENTS.md` | Roster — spawn decisions need fast local access |
| `MEMORY.md` | Recovery map — index to Engram |
| `homelab-quick-reference.md` | Used during outages when Engram may be unavailable |
| `url-validation-patterns.md` | Code-adjacent, referenced in implementation |
| `html-processing-patterns.md` | Code-adjacent — stage_5.py implements this pattern |
| `generals-accountability-system.md` | Referenced by `check-general-eligibility.py` |
| `ACTIVE-PRIORITIES.md` | Current week focus — updated frequently |
| `user-sales-engineer-background.md` | Claude profile context |
| `user-security-experience.md` | Claude profile context |
| `user_wife_ap_english.md` | Claude profile context |

All other files (~75) are deleted after migration to Engram.

---

## Root CLAUDE.md — Content Rules

**Remove:**
- Any Clearwatch-specific rules (pipeline, issue tracking, chart gates, domain-knowledge mirror rule)
- DNS split-horizon rule
- Art direction rule (reduce to one pointer line)
- Any lesson that is domain knowledge, not universal behavior

**Keep:**
- Pre-flight protocol
- Workflow (TDD, plan mode, worktrees, skills, verification)
- Decision thresholds (100% / 80% / 50% / <50%)
- Bug tracking mandate
- Critical rules (secrets, .env, logs before restart)
- Self-learning rule
- Project priority stack
- Cost guardrails
- Wake-the-Founder triggers
- Reference section (updated to point to Engram queries)

**Target:** ~55 lines

---

## MEMORY.md — New Format

Replace current content-heavy index with a recovery map:

```markdown
# Memory Index — Engram Recovery Map

## How to Recall
memory_recall(query, project) — use project="global" for cross-cutting knowledge

## Global
- Lessons + feedback rules:  memory_recall("lessons learned feedback", "global")
- User profile:               memory_recall("user background preferences", "global")
- Generals patterns:          memory_recall("generals spawn accountability XP", "global")
- URL validation:             memory_recall("url validation user-agent", "global")
- HTML processing:            memory_recall("beautifulsoup svg html processing", "global")

## Clearwatch
- Pipeline lessons:           memory_recall("pipeline stages gates bugs", "clearwatch")
- Chart quality:              memory_recall("chart gates quality evaluation", "clearwatch")
- Issue workflows:            memory_recall("issue tracking report run", "clearwatch")
- RSAC integration:           memory_recall("rsac integration learnings", "clearwatch")

## Homelab
- K8s patterns:               memory_recall("kubernetes deployment pvc", "homelab")
- DNS rules:                  memory_recall("dns cloudflare ubiquiti", "homelab")
- Cert-manager:               memory_recall("cert-manager dns01 challenge", "homelab")
- Incidents:                  memory_recall("incident postmortem", "homelab")

## Reference Files (on disk)
- homelab-quick-reference.md — triage during outages
- generals-accountability-system.md — referenced by eligibility script
- ACTIVE-PRIORITIES.md — current week focus
```

---

## Project CLAUDE.mds — Content

### `~/projects/clearwatch/CLAUDE.md` additions:
- Clearwatch-specific pre-flight steps
- Pipeline run workflow (smoke test → validate-existing → regen)
- Issue lifecycle (needs-report-run label, close superseded issues)
- CLAUDE.md rules are invisible to Stage 3 LLM → mirror in domain-knowledge/
- Chart gate rules and evaluation criteria
- Regen cycle anti-pattern rule

### `~/projects/homelab/CLAUDE.md` (create if not exists):
- DNS split-horizon rule (internal = Ubiquiti, public = Cloudflare)
- K8s deployment patterns (RWO PVCs → Recreate, Chainguard fsGroup)
- cert-manager patterns (dnsPolicy: None)
- Homepage network policy label rule
- Backup before modifying critical files

---

## Engram Migration Strategy

Migrate in three passes to avoid data loss:

**Pass 1 — Global lessons and feedback** (highest value, lowest risk)
- Batch-ingest `lessons-learned.md` sections by domain
- Ingest each feedback file as a separate memory with appropriate tags
- Ingest user profile files

**Pass 2 — Project-specific knowledge**
- Clearwatch: chart regression, pipeline lessons, RSAC learnings, issue workflows
- Homelab: k8s patterns, cert-manager, DNS, incident summaries

**Pass 3 — Historical / archival**
- Session handoff files (last 3 kept as files, rest to Engram)
- Engram phase completion docs
- Delete source files after confirming Engram recall works

**Pre-deletion backup:** Before any files are deleted, create `~/archive/memory-pre-refactor-2026-04-01.zip` containing the full `~/.claude/projects/-home-psimmons/memory/` directory snapshot.

**Validation after each pass:** Run memory_recall() for 3 representative queries and confirm results before deleting source files.

---

## Success Criteria

- [ ] `~/archive/memory-pre-refactor-2026-04-01.zip` exists before any deletions
- [ ] Root CLAUDE.md ≤ 60 lines, zero project-specific rules
- [ ] MEMORY.md ≤ 50 lines, recovery-map format only
- [ ] Memory file count ≤ 12 (only the keep-on-disk list above)
- [ ] memory_recall() returns useful results for 5 representative queries across global/clearwatch/homelab
- [ ] clearwatch/CLAUDE.md exists and contains all moved rules
- [ ] Homelab rules ingested in Engram project="homelab" (no ~/projects/homelab/ directory exists — Engram is the store)
- [ ] No rule lost — every removed rule either lives in a project CLAUDE.md or Engram
