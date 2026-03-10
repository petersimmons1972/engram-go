---
name: memory-cleanup
description: Use when session-start janitor reports memory over budget or stale files, or periodically every 5-10 sessions. Triggers on "memory cleanup", "clean memory", janitor warnings, or when memory files feel bloated.
---

# Memory Cleanup

## Overview

Actionable procedure for when the memory janitor flags issues. The janitor (`~/bin/memory-janitor.py`) does the assessment — this skill tells you what to do with its output. Do NOT invent your own assessment; always start with the janitor.

## When to Use

- Session-start janitor reports over-budget or stale files
- Every 5-10 sessions as maintenance
- When MEMORY.md feels bloated or topic files have stale content

## Procedure

### 1. Assess — ALWAYS Start Here

```bash
python3 ~/bin/memory-janitor.py
```

If under budget and no flags → **done, no action needed. Stop.**

Do NOT manually scan files or invent your own cleanup criteria. The janitor defines what needs attention.

### 2. Act on Flagged Files

**If the janitor flags a file, act on it** — the janitor's judgment overrides the age/size defaults in the table below. Don't second-guess it because a file "seems too recent."

For each flagged file, check its front matter `Category:` tag to determine the action:

| Category | Default Trigger | Action |
|----------|----------------|--------|
| `ephemeral` | Status: COMPLETE | Archive (not delete) with `--apply` |
| `active-work` | All items done | Extract lessons → archive |
| `reference` | >90 days old | Compress to root-cause + lesson only |
| `permanent` | Oversized | Prune `(1x)` entries older than 90 days |

**If a file lacks a `Category:` tag**, add one before processing. Every topic file needs this tag.

### 3. Archive Process

Archive destination: `memory/archive/` (timestamped filenames, e.g., `2026-03-10-original-name.md`)

**ALWAYS extract lessons before archiving:**
- One-liner lessons → add to MEMORY.md Key Lessons section (increment counter or add new)
- Detailed patterns → add to relevant topic file
- Update cross-references in MEMORY.md Topic Files section

Use `python3 ~/bin/memory-janitor.py --apply` when available to automate moves.

### 4. Verify

```bash
python3 ~/bin/memory-janitor.py
```

Confirm janitor reports under budget. Target ~90% of budget (720/800) to leave headroom.

## Protected Files

- **NEVER** delete or restructure `MEMORY.md` — only update specific sections (Key Lessons, Topic Files, Recent Activity)
- **NEVER** delete `MEMORY-TEMPLATE.md`
- Do NOT manually trim MEMORY.md based on your own judgment — the janitor and session-start hook manage its structure

## Common Mistakes

- Inventing your own cleanup rules instead of following janitor output
- Deleting files instead of archiving to `memory/archive/`
- Archiving without extracting lessons first — knowledge lost permanently
- Manually restructuring MEMORY.md — the session-start hook manages this file's structure
