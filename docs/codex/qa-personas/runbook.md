# QA Personas — Runbook

## When to Run

**Mandatory pre-ship gate.** Run this before closing any GitHub Issue that
touches user-facing code, CLI, API, or documentation. Do not mark work done
until Round 2 returns clean.

## Step-by-Step Procedure

### Step 1 — Identify the Target

Define the artifact to review. It should be as specific as possible:

- A Git branch name or PR URL
- A directory path or file list
- A CLI surface description
- A docs section or README file

Example: `"PR #42 — new embed command in aifleet CLI (watcher/cmd/embed.go, docs/embed.md)"`

### Step 2 — Round 1: Dispatch All Six Personas in Parallel

Spawn all six observer personas simultaneously. They do not see each other's
findings. Give each the same target description.

Prompt template for each persona:

```
You are the {display_name}. Review the target: {TARGET}.
Use only your persona's lens. Output the Trust Breakpoints format defined in
your profile. You will not see the other personas' findings. Be specific:
cite exact file paths, line numbers, command invocations, doc sections.
```

See `invocation-codex.md` or `invocation-hermes.md` for the platform-specific
dispatch commands.

### Step 3 — Collect All Six Outputs

Wait for all six to complete. Do not proceed until all six are returned.

### Step 4 — Aggregate and Classify

1. **List every Trust Breakpoint** from every persona, noting who reported it.
2. **Deduplicate**: any finding raised independently by two or more personas
   is tagged **HIGH-CONFIDENCE** (multi-lens convergence). Prioritize these.
3. **Rank by severity** (highest first):
   - `blocker` / `critical`
   - `serious` / `high`
   - `friction` / `medium`
   - `nitpick` / `low`
4. **Determine ship gate**:
   - Any `blocker` or `critical` from any persona → **NO-GO**
   - Zero `blocker` or `critical` → **GO** (pending round completion)

### Step 5 — Report Round 1 Findings

Write a consolidated report with:
- Ship gate: GO / NO-GO + reason
- HIGH-CONFIDENCE findings (reported by 2+ personas)
- Per-severity findings, each tagged with reporting persona(s)
- Per-persona "What Worked" sections (condensed)
- Deduplicated "Questions" from all personas

### Step 6 — Fix Blockers (if any)

Fix every `blocker` and `critical` from Round 1. Do not skip to Round 2 with
unresolved blockers.

File non-blocking findings (`serious` and below) as GitHub Issues with
appropriate severity labels before proceeding.

### Step 7 — Round 2: Re-run All Six Personas

After fixes are complete, re-run the same six personas against the post-fix
artifact.

Use the same prompt template. Mention in the target description that this is
a Round 2 re-check: `"Round 2 re-check of PR #42 after blocker fixes. ..."`.

### Step 8 — Verify Round 2 Results

Read Round 2 output carefully. Do not assume Round 1 blockers are resolved —
verify by reading the Round 2 findings.

**Pass criteria (ready to ship):**
- Zero `blocker` and zero `critical` in Round 2 output
- All Round 1 blockers confirmed resolved in Round 2
- Remaining findings are `friction`, `medium`, `nitpick`, or `low`

**Fail criteria (do not ship):**
- Any `blocker` or `critical` in Round 2 → fixes were incomplete or
  introduced regressions. Return to Step 6.

### Step 9 — File Remaining Findings

File all remaining `serious` / `friction` / `nitpick` findings as GitHub
Issues with labels:
- `severity/nice-to-have` for friction/nitpick
- `severity/high` or `severity/medium` for serious/high

### Step 10 — Close the Work

Once Round 2 is clean:
- Mark the original Issue as done
- Merge the PR or complete the task
- Note in the closing comment: "QA sweep completed — Round 2 clean"

## Worktree Note

Pattern 6 is **observer-only**. All six personas have `disallowedTools: [Write, Edit, NotebookEdit]`. No worktree isolation is required for the personas themselves. They read; they report.

If the *fixes* from Step 6 require a worktree (for parallel implementation work), create one per normal rules. The QA sweep itself does not need one.
