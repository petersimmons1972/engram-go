# CLAUDE.md + Engram Memory Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate root CLAUDE.md to universal-only rules, push all long-form knowledge into Engram as primary library, and reduce 85 memory files to ~11 permanent reference files.

**Architecture:** Three-pass Engram migration (global → clearwatch → homelab) with ZIP backup before any deletions. Root CLAUDE.md shrinks from 138 → ~58 lines by removing project-specific content. MEMORY.md becomes a recovery map (Engram query index). No homelab project directory exists, so homelab rules go to Engram project="homelab" only.

**Tech Stack:** Engram MCP (`memory_store`, `memory_store_batch`, `memory_recall`), bash zip, git

**Spec:** `/home/psimmons/docs/superpowers/specs/2026-04-01-claude-md-engram-refactor-design.md`

---

### Task 1: Create ZIP backup

**Files:**
- Create: `~/archive/memory-pre-refactor-2026-04-01.zip`

- [ ] **Step 1: Create the backup**

```bash
cd ~ && zip -r ~/archive/memory-pre-refactor-2026-04-01.zip \
  .claude/projects/-home-psimmons/memory/
```

- [ ] **Step 2: Validate backup**

```bash
unzip -l ~/archive/memory-pre-refactor-2026-04-01.zip | tail -1
```

Expected: line showing `85 files` (or close) and total size > 0.

```bash
ls -lh ~/archive/memory-pre-refactor-2026-04-01.zip
```

Expected: file exists, size > 100KB.

- [ ] **Step 3: Commit**

```bash
cd ~ && git add archive/memory-pre-refactor-2026-04-01.zip
git commit -m "chore: backup memory files before Engram migration"
```

---

### Task 2: Engram Pass 1 — Global lessons and feedback

Migrate `lessons-learned.md` domain sections + all feedback files + user profile files into Engram. Do NOT delete files yet — deletion happens in Task 5 after all passes complete.

**Files:**
- Read: `~/.claude/projects/-home-psimmons/memory/lessons-learned.md`
- Read: all `feedback-*.md` and `feedback_*.md` files
- Read: `user-sales-engineer-background.md`, `user-security-experience.md`, `user_wife_ap_english.md`

- [ ] **Step 1: Ingest homelab lessons**

Call `memory_store` with:
- content: full "## Homelab" section from `lessons-learned.md`
- memory_type: "knowledge"
- tags: "lessons homelab k8s cert-manager dns"
- importance: 2
- project: "homelab"

- [ ] **Step 2: Ingest clearwatch chart lessons**

Call `memory_store` with:
- content: full "## Clearwatch — Chart Quality" section from `lessons-learned.md`
- memory_type: "knowledge"
- tags: "lessons charts quality gates evaluation"
- importance: 2
- project: "clearwatch"

- [ ] **Step 3: Ingest clearwatch pipeline lessons**

Call `memory_store` with:
- content: full "## Clearwatch — Pipeline" section from `lessons-learned.md`
- memory_type: "knowledge"
- tags: "lessons pipeline stages gates bugs regen"
- importance: 2
- project: "clearwatch"

- [ ] **Step 4: Ingest general pattern lessons**

Call `memory_store` with:
- content: "## Hardware / 3D Printing" + "## General Patterns" sections combined
- memory_type: "knowledge"
- tags: "lessons general patterns beautifulsoup svg bambu printer"
- importance: 2
- project: "global"

- [ ] **Step 5: Ingest feedback files — homelab domain**

For each of these files, call `memory_store` with the full file content, memory_type="knowledge", importance=2, project="homelab":

Files to ingest with tags shown:
- `feedback-dns-use-cloudflare.md` → tags: "dns cloudflare ubiquiti split-horizon"
- `feedback-chainguard-containers.md` → tags: "k8s chainguard containers fsgroup"
- `feedback-always-pull-xp.md` → tags: "k8s deployment patterns" (if homelab-related) OR project="global"

Read each file first to confirm its domain before storing.

- [ ] **Step 6: Ingest feedback files — clearwatch domain**

For each clearwatch-related feedback file, call `memory_store` with full content, memory_type="knowledge", importance=2, project="clearwatch":

Files to check and ingest:
- `feedback-charts-are-the-gate.md` → tags: "charts gate quality"
- `feedback_chart_readability.md` → tags: "charts readability font"
- `feedback-always-file-issues.md` → tags: "issues tracking bugs"
- `feedback-lenient-test-guards-hide-bugs.md` → tags: "testing guards bugs"
- `feedback-no-whack-a-mole-runs.md` → tags: "pipeline regen runs"
- `feedback-validator-audit-checklist.md` → tags: "validators audit checklist"
- `feedback-blog-scraping-needs-url-and-content.md` → tags: "scraping url content"
- `feedback-google-news-rss-for-events.md` → tags: "research news rss"
- `feedback-fix-classes-not-instances.md` → tags: "python classes instances bugs"

Read each file first to confirm domain.

- [ ] **Step 7: Ingest feedback files — global/generals domain**

For each general-workflow feedback file, call `memory_store` with full content, memory_type="knowledge", importance=2, project="global":

Files to check and ingest:
- `feedback_autofix_bugs.md` → tags: "bugs autofix workflow"
- `feedback_commit_autonomously.md` → tags: "git commits workflow"
- `feedback_dark_backgrounds.md` → tags: "design dark backgrounds"
- `feedback-army-composition-orders.md` → tags: "generals army composition"
- `feedback-author-attribution.md` → tags: "generals attribution service record"
- `feedback-eisenhower-coordinator.md` → tags: "generals eisenhower coordinator"
- `feedback-engram-authority-pointers.md` → tags: "engram memory authority"
- `feedback-gate-tolerances.md` → tags: "gates tolerances quality"
- `feedback-precommit-hook-exit128.md` → tags: "git precommit hooks exit128"
- `feedback-plugin-update-cli-worktree-bug.md` → tags: "plugin worktree cli bug"
- `feedback-skills-organization.md` → tags: "skills organization"
- `feedback-worktree-sandbox-cwd-lock.md` → tags: "worktree sandbox cwd"
- `feedback-dont-modify-upstream-plugin-cache.md` → tags: "plugin cache upstream"

Read each file first; adjust project= if clearly homelab or clearwatch.

- [ ] **Step 8: Ingest user profile files**

Call `memory_store` for each user profile file, project="global":

- `user-sales-engineer-background.md` → tags: "user profile sales engineer background", importance=3
- `user-security-experience.md` → tags: "user profile security experience", importance=3
- `user_wife_ap_english.md` → tags: "user family context ap english", importance=2

- [ ] **Step 9: Ingest remaining global knowledge files**

Call `memory_store` for each, project="global":

- `url-validation-patterns.md` → tags: "url validation user-agent http", importance=2 (also keep file on disk)
- `html-processing-patterns.md` → tags: "html beautifulsoup svg processing", importance=2 (also keep file on disk)
- `generals-accountability-system.md` → tags: "generals accountability xp malus system", importance=2 (also keep file on disk)
- `linkedin-api-lessons.md` → tags: "linkedin api lessons", importance=1
- `cloudflare-workers-web-fetch.md` → tags: "cloudflare workers web fetch", importance=1
- `autoresearch-reference.md` → tags: "research reference auto", importance=1

- [ ] **Step 10: Validate Pass 1**

Call `memory_recall` with these queries and confirm each returns ≥1 relevant result:

```
memory_recall("homelab k8s cert-manager deployment patterns", "homelab")
memory_recall("clearwatch pipeline regen gates", "clearwatch")
memory_recall("user background sales engineer", "global")
```

All three must return relevant content before proceeding to Pass 2.

---

### Task 3: Engram Pass 2 — Clearwatch project knowledge

Migrate clearwatch-specific files that aren't feedback: chart regression, RSAC learnings, issue triage, marketing content, operation notes.

**Files:**
- Read: `chart-regression-2026-02-06.md`, `chart-rebuild-families.md`
- Read: `rsac-integration-learnings.md`, `project-rsac-2026-collector.md`
- Read: `clearwatch-github-issues-work.md`, `clearwatch-marketing-content.md`
- Read: `project-clearwatch-issue-triage-2026-03-29.md`
- Read: `nhi-research-pipeline-architecture-gap.md`

- [ ] **Step 1: Ingest chart regression analysis**

Call `memory_store` with full content of `chart-regression-2026-02-06.md`:
- memory_type: "knowledge"
- tags: "charts regression analysis 2026-02"
- importance: 2
- project: "clearwatch"

- [ ] **Step 2: Ingest chart rebuild families**

Call `memory_store` with full content of `chart-rebuild-families.md`:
- memory_type: "knowledge"
- tags: "charts rebuild families design"
- importance: 2
- project: "clearwatch"

- [ ] **Step 3: Ingest RSAC files**

Call `memory_store` for each:
- `rsac-integration-learnings.md` → tags: "rsac integration learnings pipeline", importance=2, project="clearwatch"
- `project-rsac-2026-collector.md` → tags: "rsac 2026 collector project", importance=1, project="clearwatch"

- [ ] **Step 4: Ingest clearwatch operational files**

Call `memory_store` for each:
- `clearwatch-github-issues-work.md` → tags: "issues github work triage", importance=1, project="clearwatch"
- `clearwatch-marketing-content.md` → tags: "marketing content clearwatch", importance=1, project="clearwatch"
- `project-clearwatch-issue-triage-2026-03-29.md` → tags: "issues triage 2026-03-29", importance=1, project="clearwatch"
- `nhi-research-pipeline-architecture-gap.md` → tags: "nhi research pipeline architecture gap", importance=2, project="clearwatch"

- [ ] **Step 5: Validate Pass 2**

Call `memory_recall` with:

```
memory_recall("chart regression analysis families", "clearwatch")
memory_recall("rsac integration learnings", "clearwatch")
```

Both must return relevant content before proceeding.

---

### Task 4: Engram Pass 3 — Homelab knowledge and historical/archival

Migrate homelab reference files, k8s patterns, and historical session/engram docs.

**Files:**
- Read: `homelab-quick-reference.md`, `homelab-cert-manager.md`, `homelab-k8s-patterns.md`, `homelab-incidents.md`
- Read: `k8s-deployment-lesson.md`
- Read: `engram-phase-3-completion-2026-03-25.md`, `engram-phase-4-completion-2026-03-25.md`, `engram-deployment-handoff-2026-03-25.md`
- Read: `engram-redesign-plan.md`
- Read: `database-protection-incident-2026-03-26.md`
- Read: 3 most recent session handoff files

- [ ] **Step 1: Ingest homelab reference files**

Call `memory_store` for each, project="homelab":
- `homelab-quick-reference.md` → tags: "homelab triage quick-reference patterns", importance=3 (keep file on disk too)
- `homelab-cert-manager.md` → tags: "homelab cert-manager tls dns01", importance=2
- `homelab-k8s-patterns.md` → tags: "homelab k8s patterns deployment", importance=2
- `homelab-incidents.md` → tags: "homelab incidents postmortem history", importance=2
- `k8s-deployment-lesson.md` → tags: "k8s deployment lesson pvc", importance=2

- [ ] **Step 2: Ingest engram phase completion docs**

Call `memory_store` for each, project="global":
- `engram-phase-3-completion-2026-03-25.md` → tags: "engram phase3 completion history", importance=1
- `engram-phase-4-completion-2026-03-25.md` → tags: "engram phase4 completion history", importance=1
- `engram-deployment-handoff-2026-03-25.md` → tags: "engram deployment handoff 2026-03-25", importance=1
- `engram-redesign-plan.md` → tags: "engram redesign plan history", importance=1

- [ ] **Step 3: Ingest database incident**

Call `memory_store` with full content of `database-protection-incident-2026-03-26.md`:
- memory_type: "context"
- tags: "database incident protection 2026-03-26"
- importance: 2
- project: "global"

- [ ] **Step 4: Ingest recent session handoffs (last 3 only)**

Identify the 3 most recent `session-handoff-*.md` files by filename date. Call `memory_store` for each:
- memory_type: "context"
- tags: "session handoff history"
- importance: 1
- project: "global"

Remaining session handoff files are deleted in Task 5 without ingestion (they are stale).

- [ ] **Step 5: Ingest operation and project history files**

Call `memory_store` for each, project="global":
- `operation-triple-crown.md` → tags: "operation triple-crown history", importance=1
- `project-generals-evolution.md` → tags: "generals evolution history project", importance=1
- `session-2026-03-31-final-summary.md` → tags: "session summary 2026-03-31", importance=1
- `session-handoff-2026-03-31-final.md` → tags: "session handoff 2026-03-31", importance=1 (this is the most recent, keep on disk AND ingest)

- [ ] **Step 6: Ingest active priorities**

Call `memory_store` with full content of `ACTIVE-PRIORITIES.md`:
- memory_type: "context"
- tags: "active priorities current focus"
- importance: 3
- project: "global"

(Also keep file on disk — it's updated frequently.)

- [ ] **Step 7: Validate Pass 3**

Call `memory_recall` with:

```
memory_recall("homelab k8s pvc deployment cert-manager", "homelab")
memory_recall("engram phase completion history", "global")
memory_recall("session handoff recent", "global")
```

All three must return relevant content before proceeding.

---

### Task 5: Delete migrated files

Only after all three passes validate. Keep the 11 files on the permanent list.

**Permanent keep list** (DO NOT DELETE):
```
MEMORY.md
ACTIVE-PRIORITIES.md
homelab-quick-reference.md
url-validation-patterns.md
html-processing-patterns.md
generals-accountability-system.md
user-sales-engineer-background.md
user-security-experience.md
user_wife_ap_english.md
lessons-learned.md          ← keep as compressed reference even after Engram ingest
session-handoff-2026-03-31-final.md  ← most recent session
```

- [ ] **Step 1: Identify files to delete**

```bash
ls ~/.claude/projects/-home-psimmons/memory/*.md | \
  grep -v "MEMORY.md\|ACTIVE-PRIORITIES\|homelab-quick-reference\|url-validation\|html-processing\|generals-accountability\|user-sales\|user-security\|user_wife\|lessons-learned\|session-handoff-2026-03-31-final"
```

Review the list. Confirm it matches expectation (~74 files).

- [ ] **Step 2: Delete the files**

```bash
cd ~/.claude/projects/-home-psimmons/memory/ && \
for f in \
  autoresearch-reference.md \
  chart-rebuild-families.md \
  chart-regression-2026-02-06.md \
  clearwatch-github-issues-work.md \
  clearwatch-marketing-content.md \
  cloudflare-workers-web-fetch.md \
  database-protection-incident-2026-03-26.md \
  engram-deployment-handoff-2026-03-25.md \
  engram-phase-3-completion-2026-03-25.md \
  engram-phase-4-completion-2026-03-25.md \
  engram-redesign-plan.md \
  homelab-cert-manager.md \
  homelab-incidents.md \
  homelab-k8s-patterns.md \
  k8s-deployment-lesson.md \
  linkedin-api-lessons.md \
  nhi-research-pipeline-architecture-gap.md \
  operation-triple-crown.md \
  project-clearwatch-issue-triage-2026-03-29.md \
  project-generals-evolution.md \
  project-rsac-2026-collector.md \
  rsac-integration-learnings.md \
  session-2026-03-31-final-summary.md \
  user-bambu-p1s-3d-printer.md; do
  rm -f "$f"
done
```

Then delete all feedback and session files:

```bash
cd ~/.claude/projects/-home-psimmons/memory/ && \
rm -f feedback*.md feedback_*.md && \
rm -f session-handoff-2026-03-25.md session-handoff-2026-03-26.md \
       session-handoff-2026-03-27.md session-handoff-2026-03-28.md
```

- [ ] **Step 3: Validate file count**

```bash
ls ~/.claude/projects/-home-psimmons/memory/*.md | wc -l
```

Expected: ≤ 12 files.

```bash
ls ~/.claude/projects/-home-psimmons/memory/*.md
```

Confirm only the keep-list files remain.

- [ ] **Step 4: Commit**

```bash
cd ~ && git add -A .claude/projects/-home-psimmons/memory/
git commit -m "chore: delete migrated memory files — now in Engram"
```

---

### Task 6: Rewrite MEMORY.md as Engram recovery map

**Files:**
- Modify: `~/.claude/projects/-home-psimmons/memory/MEMORY.md`

- [ ] **Step 1: Verify current line count**

```bash
wc -l ~/.claude/projects/-home-psimmons/memory/MEMORY.md
```

- [ ] **Step 2: Replace with recovery map**

Write the following content to `~/.claude/projects/-home-psimmons/memory/MEMORY.md`:

```markdown
# Memory Index — Engram Recovery Map

**Last Updated**: 2026-04-01
**Format**: memory_recall(query, project) — Engram is the primary store

---

## Global
- Lessons + feedback rules:  `memory_recall("lessons learned feedback rules", "global")`
- User profile:               `memory_recall("user background preferences sales engineer", "global")`
- Generals patterns:          `memory_recall("generals spawn accountability XP malus", "global")`
- URL validation:             `memory_recall("url validation user-agent http", "global")`
- HTML / SVG processing:      `memory_recall("beautifulsoup svg html processing", "global")`
- Session context:            `memory_recall("session handoff recent", "global")`
- Database incident:          `memory_recall("database incident protection", "global")`

## Clearwatch
- Pipeline lessons:           `memory_recall("pipeline stages gates regen bugs", "clearwatch")`
- Chart quality:              `memory_recall("chart gates quality evaluation readability", "clearwatch")`
- Chart regression analysis:  `memory_recall("chart regression analysis families", "clearwatch")`
- Issue workflows:            `memory_recall("issue tracking report run needs-report-run", "clearwatch")`
- RSAC integration:           `memory_recall("rsac integration learnings", "clearwatch")`
- NHI research gap:           `memory_recall("nhi research pipeline architecture gap", "clearwatch")`

## Homelab
- K8s patterns:               `memory_recall("kubernetes deployment pvc strategy", "homelab")`
- DNS rules:                  `memory_recall("dns cloudflare ubiquiti split-horizon", "homelab")`
- Cert-manager:               `memory_recall("cert-manager dns01 challenge tls", "homelab")`
- Incidents:                  `memory_recall("incident postmortem history", "homelab")`
- Quick reference (on disk):  `~/.claude/projects/-home-psimmons/memory/homelab-quick-reference.md`

---

## Permanent Reference Files (on disk)
| File | Purpose |
|------|---------|
| `homelab-quick-reference.md` | Triage reference — usable during outages |
| `url-validation-patterns.md` | Code-adjacent implementation reference |
| `html-processing-patterns.md` | Code-adjacent — stage_5.py pattern |
| `generals-accountability-system.md` | Referenced by `check-general-eligibility.py` |
| `lessons-learned.md` | Compressed lessons index — search Engram for full detail |
| `ACTIVE-PRIORITIES.md` | Current week focus — updated each session |
| `user-*.md` (3 files) | Claude profile context |
| `session-handoff-2026-03-31-final.md` | Most recent session state |
```

- [ ] **Step 3: Validate line count**

```bash
wc -l ~/.claude/projects/-home-psimmons/memory/MEMORY.md
```

Expected: ≤ 50 lines.

- [ ] **Step 4: Commit**

```bash
cd ~ && git add .claude/projects/-home-psimmons/memory/MEMORY.md
git commit -m "docs: rewrite MEMORY.md as Engram recovery map"
```

---

### Task 7: Rewrite root CLAUDE.md

Shorten from 138 → ~58 lines. Remove project lifecycle verbosity. Keep all universal behaviors.

**Files:**
- Modify: `~/CLAUDE.md`

- [ ] **Step 1: Verify current content**

```bash
wc -l ~/CLAUDE.md && cat -n ~/CLAUDE.md
```

Identify any lines that are Clearwatch- or homelab-specific that aren't already in `~/projects/clearwatch/CLAUDE.md`.

- [ ] **Step 2: Write new CLAUDE.md**

Replace the full file with:

```markdown
# Claude Assistant Instructions

## Behavioral Rules
- Never tell the user to do something manually that you can do yourself — just do it.
- **Markdown tables must be human-readable in raw form**: pad columns, use emoji swatches (🔵🟡🟢⚫⚪✅❌⚠️), never leave hex codes or long values unformatted.
- When 'summary' or 'report' is requested, cover ALL items — not just a filtered subset.
- Before starting work, check memory (Engram + AGENTS.md + GitHub issues) for current state.
- **Art direction:** Use the Designated Art Direction Team — see AGENTS.md §9 and `~/projects/art-direction-research/ART-DIRECTION-RULE.md`.
- **Bug & Defect Tracking is non-negotiable** — see section below.

## Parallel Agent Rules
- **Pre-validation (mandatory):** Use ONE agent to confirm the exact problem on 2–3 samples before dispatching the full team.
- Explicitly list which functions each agent will touch. Flag and run full tests if two agents share a function.
- **Always include one zero-context reviewer** in review panels.

## Pre-Flight Protocol — MANDATORY
1. **ENVIRONMENT CHECK** — `git status`, `git branch`, `pwd`. Halt if unexpected.
2. **REQUEST VERIFICATION** — For multi-step tasks, write a one-paragraph summary. Wait for confirmation on ambiguous requests.
3. **BUG ACCOUNTABILITY** — All bugs found must be fixed or filed as GitHub Issues.
4. **BRANCH VERIFICATION ON COMPLETION** — Run `git log --oneline -3` to confirm commits landed.

## Workflow
- **Test first.** Write failing test before implementation. Run tests after every edit.
- Plan mode for non-trivial tasks (3+ steps). Preserve error state if things go sideways.
- **Worktree before implementation — MANDATORY.** Use `superpowers:using-git-worktrees` before any approved plan. No exceptions.
- Use skills for procedural work — authoritative over summaries.
- **Stay in scope.** >15 min tangential issues → GitHub Issue. <15 min quick fixes are fine.
- `superpowers:verification-before-completion` before claiming done.
- **Engram is the primary knowledge store.** Use `memory_recall()` for semantic search. Fall back to `~/.claude/projects/-home-psimmons/memory/` if unavailable. Recovery map: `MEMORY.md`.

## Project Lifecycle
- Every new project needs: README.md (STATUS in first 10 lines), CLAUDE.md if agent sessions will run there.
- Naming: kebab-case, descriptive, no version numbers.
- Archive when: no commits in 90 days (and not a dependency), explicitly superseded, or one-time task completed.
- No orphan files at `~/projects/` root.

## Decisions
- 100% confident → just do it | 80-99% → do + explain | 50-80% → propose first | <50% → ask
- Pre-approved: logs, kubectl get/describe, health-check, diagnostics
- Always ask: delete resources, modify production, data loss risk
- **When blocked:** one focused question with recommended default. Make assumptions explicit.

## Bug & Defect Tracking — NON-NEGOTIABLE
GitHub Issues ARE the work. If not in the issue system, it does not exist.
- Found a bug? File it before continuing. Fixed inline? Still file it as closed.
- Deferred? File it. "Later" without an issue number means never.
- **Continuity test:** Could the next session recover every open defect from GitHub Issues alone?

## Critical Rules
**NEVER:** Commit secrets | Create `.env` with real credentials (use Infisical: `https://infisical.petersimmons.com`) | Restart services before checking logs | Destructive ops without verified backup
**ALWAYS:** `git diff --staged` before every commit | Check logs before restarting | Verify end-to-end output | GitHub = single source of truth

## Self-Learning — NON-NEGOTIABLE
**Never ask permission for:** bug fixes (any severity), feedback integration.
After ANY correction: update `lessons-learned.md` AND store in Engram (`project="global"`, tags="lessons feedback").
Escalate only if: same bug 3+ times, circular loops.

## Project Priority Stack
1. **Clearwatch** — revenue pipeline (reports, cache, charts, grading)
2. **Infrastructure** — K8s cluster stability, cert-manager, DNS, storage
3. **Gmail tracker / job search** — tooling and automation

## Cost Guardrails
- Opus agents: max 3 concurrent unless authorized
- Bulk LLM (>50 calls): require founder approval with cost estimate
- Prefer Sonnet for exploration/routine, Opus for production-quality output

## Wake-the-Founder Triggers
Stop and wait if: >$5 compute | Production namespace deployment | git push to main/master | Data loss detected | Agent stuck >45 min | Same bug/error 3+ times

## Reference
**Generals:** `~/AGENTS.md` | **Eligibility check:** `python ~/projects/generals/bin/check-general-eligibility.py`
**Skills:** Deep debug → `superpowers:systematic-debugging` | Before implementing → `superpowers:brainstorming`
**Web Search:** SearXNG: `https://searxng.petersimmons.com/search?q={query}&format=json` | Fallback: WebSearch tool
**Memory:** Engram recovery map → `~/.claude/projects/-home-psimmons/memory/MEMORY.md`
**Projects catalog:** `~/PROJECTS-CATALOG.md` | **Art direction rule:** `~/projects/art-direction-research/ART-DIRECTION-RULE.md`
```

- [ ] **Step 3: Validate line count**

```bash
wc -l ~/CLAUDE.md
```

Expected: ≤ 65 lines.

- [ ] **Step 4: Verify no Clearwatch-specific pipeline rules remain**

```bash
grep -i "stage\|pipeline\|regen\|chart gate\|mttd\|domain-knowledge" ~/CLAUDE.md
```

Expected: no matches (all clearwatch-specific rules already live in `~/projects/clearwatch/CLAUDE.md`).

- [ ] **Step 5: Commit**

```bash
cd ~ && git add CLAUDE.md
git commit -m "docs: rewrite CLAUDE.md — universal rules only, ~58 lines"
```

---

### Task 8: Trim AGENTS.md art direction section

Replace the 35-line §9 with a 4-line pointer. Content already lives in `~/projects/art-direction-research/`.

**Files:**
- Modify: `~/AGENTS.md`

- [ ] **Step 1: Locate section 9**

```bash
grep -n "## 9\." ~/AGENTS.md
```

Note the start line and find the end of the file.

- [ ] **Step 2: Replace §9 with pointer**

Find the `## 9. Designated Art Direction Team` section and replace everything from that header to end of file with:

```markdown
## 9. Designated Art Direction Team

Six designated artists for all visual work: Mucha, Toulouse-Lautrec, Cassandre, Savignac, Rand, Greiman.

**Full selection guide and enforcement rules:** `~/projects/art-direction-research/ART-DIRECTION-RULE.md`
**Artist profiles (1500-2000 words each):** `~/projects/art-direction-research/profiles/{artist}-profile.md`

❌ Do NOT use generic AI design tools without artist direction. ❌ Do NOT mix artists on a single project.

---

**Detailed docs moved to `~/projects/generals/`:** XP Reference → `PROGRESSION-SYSTEM.md` | Sync Protocol → `SYNC-PROTOCOL.md` | Post-Mortems → `post-mortems/`
```

- [ ] **Step 3: Validate line count**

```bash
wc -l ~/AGENTS.md
```

Expected: ≤ 175 lines (down from 206).

- [ ] **Step 4: Commit**

```bash
cd ~ && git add AGENTS.md
git commit -m "docs: trim AGENTS.md §9 art direction to pointer — full guide in ART-DIRECTION-RULE.md"
```

---

### Task 9: Final validation

Verify all success criteria from the spec.

- [ ] **Step 1: File counts and line counts**

```bash
echo "=== CLAUDE.md ===" && wc -l ~/CLAUDE.md
echo "=== AGENTS.md ===" && wc -l ~/AGENTS.md
echo "=== MEMORY.md ===" && wc -l ~/.claude/projects/-home-psimmons/memory/MEMORY.md
echo "=== Memory file count ===" && ls ~/.claude/projects/-home-psimmons/memory/*.md | wc -l
echo "=== Archive ZIP ===" && ls -lh ~/archive/memory-pre-refactor-2026-04-01.zip
```

Expected:
- CLAUDE.md ≤ 65 lines
- AGENTS.md ≤ 175 lines
- MEMORY.md ≤ 50 lines
- Memory files ≤ 12
- ZIP exists

- [ ] **Step 2: No project-specific rules in root CLAUDE.md**

```bash
grep -iE "stage [0-9]|clearwatch pipeline|mttd|needs-report-run|domain-knowledge|ubiquiti|cloudflare zone|fsgroup|chainguard" ~/CLAUDE.md
```

Expected: no matches.

- [ ] **Step 3: Engram recall — 5 representative queries**

Call each and confirm ≥1 relevant result:
1. `memory_recall("homelab k8s pvc deployment strategy", "homelab")`
2. `memory_recall("clearwatch pipeline regen anti-pattern gates", "clearwatch")`
3. `memory_recall("chart quality evaluation differentiation", "clearwatch")`
4. `memory_recall("user background sales engineer security", "global")`
5. `memory_recall("dns cloudflare ubiquiti split-horizon internal public", "homelab")`

- [ ] **Step 4: clearwatch/CLAUDE.md still complete**

```bash
wc -l ~/projects/clearwatch/CLAUDE.md
grep -c "NON-NEGOTIABLE" ~/projects/clearwatch/CLAUDE.md
```

Expected: > 100 lines, ≥ 3 NON-NEGOTIABLE markers (it was already comprehensive — verify nothing was accidentally removed).

- [ ] **Step 5: Final commit and push check**

```bash
cd ~ && git log --oneline -6
```

Confirm all 6 commits from this plan are present:
1. `chore: backup memory files before Engram migration`
2. `chore: delete migrated memory files — now in Engram`
3. `docs: rewrite MEMORY.md as Engram recovery map`
4. `docs: rewrite CLAUDE.md — universal rules only, ~58 lines`
5. `docs: trim AGENTS.md §9 art direction to pointer`
6. Final validation commit (this step)

```bash
git add -A && git commit -m "chore: complete CLAUDE.md + Engram refactor — all success criteria verified"
```
