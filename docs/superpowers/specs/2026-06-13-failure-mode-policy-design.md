# Failure-Mode Policy — Global Rollout Design

**Date:** 2026-06-13  
**Status:** Approved — ready for implementation  
**Scope:** clearwatch, infrastructure, job-search-system, aifleet, engram-go  
**Related:** `~/docs/failure-modes-standard.md`, `~/CLAUDE.md` §Pre-Flight Protocol, §BUG ACCOUNTABILITY

---

## Problem

The global failure-mode catalog (`failure-modes-standard.md`) exists and is well-built — 22 failure modes across 7 bug classes, seeded from real painful bug cycles. The enforcement layer does not exist. Only `aifleet` has a checklist and lint script; no project has a CI gate; the catalog has no capture loop connecting new bugs back into checks. The result: the same class of bug can (and does) recur across projects, and the catalog drifts stale because there is no trigger to update it.

---

## Goals

1. Every major project has a deterministic pre-check-in gate that catches known failure mode classes automatically.
2. Global checks live in one place and propagate to all projects via a single command.
3. Painful bug cycles automatically feed back into the catalog and into the checks.
4. The gate cannot be bypassed by an agent or a developer without an explicit founder action.

---

## Architecture

Three layers:

```
~/docs/failure-modes-standard.md     ← human-readable catalog (existing)
~/bin/checkin-lint-core.sh           ← automatable universal checks (NEW)
~/bin/sync-failure-modes.sh          ← propagates core to all projects (NEW)
~/bin/add-failure-mode.sh            ← capture loop: bug → catalog → sync (NEW)

Per project:
  bin/checkin-lint-core.sh           ← vendored copy (updated by sync)
  bin/checkin-lint.sh                ← sources core + project-specific checks
  bin/checkin-lint.baseline          ← accepted/known findings
  bin/install-hooks.sh               ← wires pre-commit hook
  docs/CHECKIN-CHECKLIST.md          ← judgment items + quick-start
  .github/workflows/checkin-lint.yml ← CI gate (hard gate, cannot be skipped)
```

---

## Section 1 — Global Layer

### `~/bin/checkin-lint-core.sh`

Single source of truth for all automatable universal checks. Projects **source** this file; they do not copy logic from it.

**Universal checks (every project):**

| Check ID | Failure modes | What it scans |
|----------|---------------|---------------|
| `C.home-literal` | FM-06 | `rg '/home/[a-z][a-z0-9_-]*'` across yaml/sh/conf/service/toml — no hardcoded user paths |
| `C.version-pinned-path` | FM-08 | `.nvm/versions/node/vX` and equivalent pinned tool paths |
| `F.remote-guard` | FM-12 | `git remote -v` against `EXPECTED_REMOTE` env var — refuses to run if remote doesn't match |
| `D.exit-zero-wrapper` | FM-18 | Scan `.sh` files for bare unconditional `exit 0` — flags wrappers that mask child exit codes |

**Opt-in K8s checks (`CHECKIN_K8S=1`):**

| Check ID | Failure modes | What it scans |
|----------|---------------|---------------|
| `G.latest-image` | FM-15 | YAML files for `image: *:latest` in container specs |
| `G.hardcoded-ip` | FM-16 | NetworkPolicy YAML for IP literals in egress rules |
| `G.missing-namespace` | FM-17 | Every `namespace:` reference in a manifest bundle has a corresponding `00-namespace.yaml` |

The core exports `section`, `finding`, `pass_rule`, and `hint` helpers. It exposes one public entry point: `run_core_checks "$@"`. Project scripts call this function, then append their own checks using the same helpers.

### `~/bin/sync-failure-modes.sh`

Propagates the current `checkin-lint-core.sh` to all registered projects.

- Reads project list from `~/bin/failure-modes-projects.conf` — one `<project-path>::<expected-remote>` per line
- Copies `~/bin/checkin-lint-core.sh` to `<project>/bin/checkin-lint-core.sh`
- Prints diff summary: "updated" vs "already current" per project
- Does **not** commit — the developer reviews and commits each project in the natural flow

Running `sync-failure-modes.sh` is the only mechanism by which a new global check reaches all five projects. It is explicit: you see what changed and commit it with context.

---

## Section 2 — Per-Project Layer

### Common file structure

```
<project>/
  bin/
    checkin-lint-core.sh     # vendored copy — updated by sync-failure-modes.sh
    checkin-lint.sh          # sources core + project-specific checks
    checkin-lint.baseline    # accepted/known findings (blank to start)
    install-hooks.sh         # wires .git/hooks/pre-commit
  docs/
    CHECKIN-CHECKLIST.md     # judgment items + quick-start
  .github/workflows/
    checkin-lint.yml         # CI gate
```

### The wrapper pattern

Every `bin/checkin-lint.sh` follows this shape. Only the three config lines and the project-specific block vary:

```bash
#!/usr/bin/env bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

EXPECTED_REMOTE="petersimmons1972/<project>"   # F.remote-guard
CHECKIN_K8S=0                                  # 1 for K8s projects
export EXPECTED_REMOTE CHECKIN_K8S

source "${SCRIPT_DIR}/checkin-lint-core.sh"
run_core_checks "$@"

# ── Project-specific checks below ──
```

### Per-project extensions

| Project | `CHECKIN_K8S` | Project-specific automatable checks | Judgment items |
|---------|:---:|---|---|
| **aifleet** | `0`¹ | A.alias-identity (absorbed from existing script — alias values must not contain routing/role tokens) | E1–E3 thermal/GPU-role safety; A2–A6 identity vs. routing review |
| **engram-go** | `1` | `rg 'postgres://[^$]'` — no hardcoded DB connection strings; scan `*_health.go` for probes that don't query a real dependency (FM-14) | FM-14 stateless-vs-stateful health probe; FM-21 destructive method self-guard |
| **clearwatch** | `0` | `rg 'sk-ant-\|ANTHROPIC_API_KEY\s*='` in `.py` — no hardcoded API keys; Python `exit(0)` unconditional in wrapper scripts (FM-18 Python variant) | FM-19 doc drift: after any fix, grep docs/runbooks for old pattern in same PR; FM-20 dry-run shares validation gates with real path |
| **infrastructure** | `1` | B.execstart-path: render `ExecStart` with substitutions, confirm target exists in install script; B.daemon-reload-order: flag `systemctl enable` without prior `daemon-reload` in same script | B1–B3 install layout; G1–G2 deploy/verify discipline |
| **job-search-system** | TBD² | TBD after stack audit | TBD |

¹ aifleet has K8s CRDs but no namespace/NetworkPolicy manifests — `CHECKIN_K8S=0` with alias-identity covering its actual risk surface.  
² Run `ls ~/projects/job-search-system/` and review stack before implementation to determine applicable checks.

### CHECKIN-CHECKLIST.md structure

Each project's checklist owns **only** project-specific judgment items. It does not repeat the global standard:

```markdown
# <Project> Pre-Check-In Checklist

Quick start:
  bin/checkin-lint.sh           # automated checks; exit non-zero on findings
  bin/checkin-lint.sh --fix-hints

Automated checks (🤖) run via bin/checkin-lint.sh.
Universal checks inherited from ~/docs/failure-modes-standard.md.

## Judgment items (👁 — not automatable)

### [Class name]
| # | Check | How |
|---|-------|-----|
| P1 | ... | Code review |
```

### aifleet migration

aifleet's existing `bin/checkin-lint.sh` (~200 lines) implements C and F checks inline. Migration:

1. C and F logic moves into `checkin-lint-core.sh` (aifleet is the reference implementation)
2. aifleet's script is rewritten to source the core + keep only the A.alias-identity block (~40 lines)
3. `bin/checkin-lint.baseline` stays untouched
4. No behavior change — same checks, same exit codes, same baseline suppression

---

## Section 3 — Gate Wiring

### Pre-commit hook

`bin/install-hooks.sh` — identical across all projects:

```bash
#!/usr/bin/env bash
set -euo pipefail
REPO_ROOT="$(git rev-parse --show-toplevel)"
HOOK="$REPO_ROOT/.git/hooks/pre-commit"

cat > "$HOOK" <<'EOF'
#!/usr/bin/env bash
exec "$(git rev-parse --show-toplevel)/bin/checkin-lint.sh"
EOF

chmod +x "$HOOK"
echo "✓ pre-commit hook installed → $HOOK"
```

Developer onboarding (added to each project's README or CONTRIBUTING.md):
```
bin/install-hooks.sh    # one-time; re-run after pulling hook changes
```

### GitHub Actions workflow

`.github/workflows/checkin-lint.yml` — identical across projects except the `name` field:

```yaml
name: checkin-lint

on:
  pull_request:
  push:
    branches: [main, master]

jobs:
  checkin-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install ripgrep
        run: |
          sudo apt-get update -q
          sudo apt-get install -y ripgrep
      - name: Run checkin-lint
        run: bin/checkin-lint.sh
```

Notes:
- `ripgrep` installed explicitly — removes the `grep` fallback path, keeps CI output clean
- No `--fix-hints` in CI — hints are for local developer use only
- Triggers on PR (primary enforcement) and push to main/master (belt-and-suspenders)

### Branch protection

Each project repo requires a one-time GitHub settings change: require the `checkin-lint` CI check to pass before merge. This is the hard gate that closes the `--no-verify` bypass.

### Bypass matrix

| Bypass attempt | What happens |
|---|---|
| `git commit --no-verify` | Hook skipped; CI blocks merge on PR |
| Merge without PR | Branch protection blocks direct push to main |
| Agent commit without hook installed | No hook in agent worktrees; CI is the hard gate |
| Disable CI check in branch protection | Founder action only |

---

## Section 4 — Capture Loop

### The gap

BUG ACCOUNTABILITY (CLAUDE.md) and the failure-mode catalog are disconnected. A painful bug cycle produces a GitHub Issue and stops. The catalog update — which prevents the same class of bug across all projects — requires someone to remember. That memory degrades immediately.

### `~/bin/add-failure-mode.sh`

A Python script (bash is fragile for markdown editing) that runs interactively immediately after a painful bug is fixed, while details are fresh.

**Interaction flow:**

```
$ ~/bin/add-failure-mode.sh

Describe the bug (concrete instance, 1–2 sentences):
> <user input>

Bug class:
  A) Identity vs. Routing      E) Thermal / Resource-Role Safety
  B) Install / Config Layout   F) Repo Hygiene / Canonical Home
  C) Portability               G) Deploy / Verify Discipline
  D) Fail-Loud                 N) New class
> <user input>

What check catches it? (one sentence):
> <user input>

Automatable? (y/n): > <user input>

Is this a NEW check item (not just a new instance of an existing check)? (y/n): > <user input>

GitHub Issue # (leave blank to skip cross-reference): > <user input>
```

**What the script writes:**

1. A new row in the catalog table: next FM-XX ID, bug class, concrete instance, check, auto/judgment flag
2. If new class: a new `### H.` (or next letter) section with check item
3. If existing class + new check item: appends check item to the existing class section
4. If automatable: prints the exact `rg`/`grep` pattern to add to `checkin-lint-core.sh` — does **not** auto-edit the script (a bad regex in the core is worse than a manual paste step)

**After writing:**

Runs `sync-failure-modes.sh` automatically. Prints a commit message template covering `failure-modes-standard.md` + all updated `checkin-lint-core.sh` files.

### CLAUDE.md amendment — BUG ACCOUNTABILITY

Add step (c) to the existing Pre-Flight Protocol row:

> **(c) If the bug burned a multi-step cycle (>30 min, repeated pattern, or production incident): run `~/bin/add-failure-mode.sh` before continuing other work.**

**Threshold for "catalog-worthy":** Would a checklist item have caught this before it burned time? A typo fix: no. An hour chasing a routing label that leaked into a persisted identity field: yes.

### Full capture loop

```
Bug burns a cycle (>30 min / repeated / production)
        ↓
BUG ACCOUNTABILITY → GitHub Issue filed
        ↓
~/bin/add-failure-mode.sh  (run while details are fresh)
        ↓
FM-XX appended to failure-modes-standard.md
        ↓
If automatable → check pattern added to checkin-lint-core.sh
        ↓
sync-failure-modes.sh → all 5 projects pick up the new check
        ↓
Next commit on any project → new check fires in pre-commit hook + CI
```

---

## Rollout Sequence

1. **Build global layer** — create `checkin-lint-core.sh`, `sync-failure-modes.sh`, `add-failure-mode.sh`, `failure-modes-projects.conf`
2. **Migrate aifleet** — refactor existing script to source core; verify no behavior change; add CI workflow; install hook; wire branch protection
3. **Wire engram-go** — stack audit, create all files, CI, hook, branch protection
4. **Wire clearwatch** — stack audit, create all files, CI, hook, branch protection
5. **Wire infrastructure** — create all files, CI, hook, branch protection
6. **Wire job-search-system** — stack audit (determine `CHECKIN_K8S`, project-specific checks), create all files, CI, hook, branch protection
7. **Amend CLAUDE.md** — add step (c) to BUG ACCOUNTABILITY

Each project is independently shippable. Aifleet first because it has the reference implementation that seeds the core.

---

## Open Questions

- **job-search-system stack** — needs a 5-minute audit before implementation (step 6 above)
- **`add-failure-mode.sh` auto/manual boundary** — the script prints the `rg` pattern rather than auto-editing `checkin-lint-core.sh`; revisit if the manual paste step proves friction
- **New failure-mode classes (H and beyond)** — the current catalog covers classes A–G; the capture loop will generate new classes organically; no pre-seeding needed
