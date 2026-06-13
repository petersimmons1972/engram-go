# Failure-Mode Standard (Universal)

**Scope:** All projects. Per-repo checklists INHERIT from this catalog and may append repo-specific checks.
**Process:** Every time a bug burns a cycle → append a catalog row + a check entry in the relevant class below.
**Per-repo pattern:** `<repo>/bin/checkin-lint.sh` (automates 🤖 items) + `<repo>/docs/CHECKIN-CHECKLIST.md` (inherits + extends).
**First seeded:** 2026-06-13 from the aifleet embedder-identity and poller-relocation sagas.

---

## Bug Classes

### A. Identity vs. Routing

A routing/serving label (alias, tag, selector, role suffix) leaked into a field that a downstream system persists and compares as a **stable identity** → false mismatch, blocked writes, or spurious reprocessing.

- 🤖 **No routing/role word encoded in a stable identity field.** Flag names containing `-live`, `-reembed`, `-bulk`, `-burst`, `-fast`, `-primary`, host names, ports, or role descriptors wherever they feed a persisted identity comparison.
- **The advertised name == the authoritative identity stored downstream.** If any system persists `<thing>_name` / `<thing>_id`, every producer must advertise the exact same string.
- **Same artifact ⇒ same identity.** Two endpoints/versions serving identical artifacts MUST advertise the identical identity. Different name for the same artifact = a future false mismatch.
- **Different artifact ⇒ deliberately different identity** (never reuse a name across distinct artifacts).
- **Role is expressed in routing, not in the name.** primary/bulk/fast/etc. = which endpoint/URL/queue a consumer targets, or an explicit routing field — never the artifact identity string.
- **No field carries two orthogonal meanings.** If one string is both the identity AND the routing selector, split them before merge.
- **A re-process/re-index operation fires ONLY on a real content-or-schema change or explicit force flag.** An identity rename or label change must short-circuit to no-op rather than triggering a mass re-process. Guard: check whether a genuine underlying change occurred before enqueueing work.

### B. Install / Config Layout Integrity

A unit, manifest, or wrapper references a path that the installer does not actually produce — wrong filename, missing `.sh` extension, binary never vendored, or ordering race between `daemon-reload` and `enable`.

- 🤖 **Every `ExecStart`/`ExecStartPre`/wrapped path resolves to what the installer actually produces** (exact filename, including `.sh`-or-not, and exact location). Render with real substitutions; confirm the target exists post-install. Offline linters (`systemd-analyze verify`) cannot see runtime specifiers — verify against a staged install.
- **Every referenced binary/script is actually vendored AND installed.** Grep each unit's targets; confirm a source file exists in-repo and an install line places it at the expected path.
- **Wrappers that `exit 0` don't mask a failing child.** Any run/notify wrapper must propagate the real exit code.
- **`daemon-reload` before `enable`.** Enabling a unit before the daemon has reloaded the new unit file can self-disable a timer or cause stale-unit behavior. Sequence: write unit → `daemon-reload` → `enable` → `start`.

### C. Portability — No Machine/User Literals

Hardcoded user home paths, version-pinned tool paths, or specifier tokens used outside their valid expansion context break on any other user, machine, or upgrade.

- 🤖 **No `/home/<user>` literal in any unit/drop-in/installer/config.** Use `%h`, `__HOME__`, `__REPO_ROOT__`, or equivalent install-time tokens only. `rg -n '/home/[a-z]' .` must return zero hits.
- **Know where template specifiers expand.** `%h` expands in `ExecStart`, `ReadWritePaths`, etc. — NOT inside `Environment=` strings. Use install-time `sed` substitution tokens in `Environment=`.
- **No version-pinned absolute tool paths** (e.g. `~/.nvm/versions/node/vX.Y.Z/bin/...`). Resolve dynamically (`command -v`) or via install-time template — never pin a path that dies on the next tool upgrade.

### D. Fail-Loud — No Silent Masking

A missing dependency, empty result, failed child process, or degraded fallback is swallowed and reported as success — masking the failure for hours or weeks.

- **Critical/gating paths fail CLOSED, not open.** A missing dependency, empty result, or fallback must surface as a non-zero exit or an explicit error flag, never silently swallowed.
- **A `Result=success` that could mask a failed child is suspect.** Verify the leaf process actually ran and produced output.
- **Fallback/degraded modes must emit a warning-level signal,** not silently score as a normal result.

### E. Thermal / Resource-Role Safety

A change to naming, routing, or configuration removes or bypasses a safety pin that restricts sustained/bulk workloads to hardware that can handle them.

- **Resource-role pinning survives identity/routing changes.** Constrained devices (fanless, low-memory, shared CPU) must never receive sustained/bulk load. If a change alters how a consumer selects a resource, confirm the safety pin still holds by an independent selector (URL, label, affinity) that cannot be removed by a rename.
- **Bulk workloads target the correct resource by a selector that survives a name collapse.**
- **Sequence routing changes resource-safely:** assign the device/resource pin BEFORE collapsing whatever selector currently routes to it.

### F. Repo Hygiene / Canonical Home

Infra or code living in the wrong repo, look-alike stubs installed in place of the real artifact, or dual install paths creating ambiguity about which version is live.

- **Artifacts live in their owning repo.** Do not add infra to a repo whose stated purpose is something else.
- **One canonical artifact — no look-alike stubs.** No stub that can be installed as the real implementation; no dual install paths for the same service.
- 🤖 **`git remote -v` matches the intended repo before editing.** A session's cwd may belong to a different repo or worktree than the one you mean to change — verify before every cross-repo edit.

### G. Deploy / Verify Discipline

Skipping staged verification before enabling a live artifact, or linting offline instead of against the live install output.

- **Prod deploys are staged and reversible.** Pattern: backup → stage with `NO_ENABLE` (or equivalent dry-run) → verify against live artifact → enable → auto-rollback on any verify miss.
- **Verify against the LIVE artifact, not just an offline lint.** Offline tools (`systemd-analyze`, schema linters) can pass an artifact that the live executor rejects. Staged install-layout checks catch what offline lints miss.
- **Prod deploys are founder-gated** (see `~/CLAUDE.md` §Cost Guardrails).

---

## Failure-Mode Catalog

*Append a row every time a bug burns a cycle. Per-repo checklists reference these IDs.*

| ID    | Bug class                        | Concrete instance                                                                                             | Check that catches it                                                                      | Auto? |
|-------|----------------------------------|---------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------|-------|
| FM-01 | Identity vs. routing             | olla `--alias bge-m3-live/reembed` read as Engram identity → `embedder_mismatch`, `memory_store` blocked     | Grep aliases for role/routing words; confirm advertised name == persisted identity         | 🤖    |
| FM-02 | Identity vs. routing             | Role (live/bulk) encoded in model name                                                                        | Same as FM-01                                                                              | 🤖    |
| FM-03 | Identity vs. routing             | Model name doubled as identity AND routing selector                                                           | Split fields; confirm no field carries two orthogonal meanings                             | Judgment |
| FM-04 | Install layout                   | `ExecStart=…codex-poll.sh` vs installed binary named `codex-poll` → 203/EXEC                                 | Render ExecStart with real substitutions; diff against post-install listing                | 🤖    |
| FM-05 | Install layout + silent mask     | `codex-liveness-check` binary never vendored → silent exit-127 for weeks                                     | Grep all unit targets; confirm source + install line exist in-repo                        | 🤖    |
| FM-06 | Portability literal              | `/home/psimmons` hardcoded in systemd units and drop-ins                                                      | `rg -n '/home/[a-z]' .` must return zero hits                                             | 🤖    |
| FM-07 | Portability / specifier scope    | `%h` not expanding in `Environment=` line                                                                     | Document expansion contexts; use sed tokens in `Environment=`                             | Judgment |
| FM-08 | Portability / version pin        | Node-version-pinned `CODEX_BIN` path breaks on nvm upgrade                                                   | Replace pinned paths with `command -v` or install-time template                           | 🤖    |
| FM-09 | Thermal / resource-role safety   | Bulk 259 GB re-embed could land on fanless MI-50 after name collapse                                          | Confirm bulk selector is URL/affinity-based, not name-only, before renaming               | Judgment |
| FM-10 | Repo hygiene / canonical home    | Poller infra in tools repo; 128-line stub installed as real 2798-line poller; dual install paths             | `git remote -v` before edit; one canonical artifact per service                           | 🤖 / Judgment |
| FM-11 | Install layout / ordering        | `daemon-reload`-before-`enable` race self-disabled a timer                                                   | Sequence: write → `daemon-reload` → `enable` → `start`                                    | Judgment |
| FM-12 | Repo hygiene / wrong repo        | Worktree/wrong-repo edits landed in unintended repo                                                           | `git remote -v` + branch check before every cross-repo edit                               | 🤖    |
| FM-13 | Identity vs. routing             | Spurious mass re-process fired by a label/identity rename — no real content/schema change occurred            | Re-process fires ONLY on real content-or-schema change or explicit `--force`; identity rename short-circuits to no-op | Judgment |

---

## Case Studies

### Case Study 1 — Embedder Identity vs. Role Leak (aifleet / Engram, 2026-06-12)

**What happened:** aifleet's olla per-role routing commit encoded each GPU's role into the olla `--alias` (`bge-m3-live`, `bge-m3-reembed`). Engram-go reads that alias as `project_meta.embedder_name` — a persisted identity it compares on every write. Because the alias no longer matched the stored identity `BAAI/bge-m3`, every `memory_store` returned `embedder_mismatch` and was blocked. The model weights were identical on both endpoints; only the advertised name differed. No re-embed was needed.

A secondary hazard: collapsing the names without first giving the bulk worker a URL-based selector would have sent a 259 GB bulk re-embed to the fanless MI-50, which has no active cooling.

**Class:** FM-01, FM-02, FM-03, FM-09.

**The checks that would have caught it:**
- Grep `--alias` values for role words (`-live`, `-reembed`, `-bulk`) before merging the routing commit. (🤖)
- Confirm the advertised alias == the value persisted in the downstream system's identity field. (Judgment, pre-merge)
- Before collapsing names, confirm bulk-workload routing uses a selector other than the name being changed. (Judgment, pre-deploy)

**Fix applied:** Three ordered prod steps — (1) fix primary alias to `BAAI/bge-m3` (stops the bleed), (2) give bulk worker a direct URL pin (thermal safety independent of name), (3) unify bulk alias. Full runbook: `~/.claude/worktrees/advisory-protocol-package/.claude/plans/RUNBOOK-embedder-identity-vs-role.md`.

---

### Case Study 2 — Poller Install-Layout / Repo Relocation (claude-codex, 2026-06-12)

**What happened:** The codex poller service had accumulated three compounding problems: (a) the systemd unit's `ExecStart` referenced `codex-poll.sh` (with `.sh`) but the installer produced a binary named `codex-poll` — every start attempt returned 203/EXEC; (b) `codex-liveness-check` was referenced in the unit but had never been vendored or installed — it had been silently failing with exit-127 every 15 minutes for weeks, with the wrapper masking it as success; (c) the poller infra lived in the `codex` tools repo (Rust CLIs) rather than its owning repo `claude-codex`, and a 128-line stub had been deployed in place of the real 2798-line poller.

**Class:** FM-04, FM-05, FM-06, FM-08, FM-10, FM-11, FM-12.

**The checks that would have caught it:**
- Staged install-layout check: render `ExecStart` with real specifiers, diff against post-install file listing. (🤖 — the offline `systemd-analyze` passed the bug; the staged check caught it)
- Grep all unit targets; confirm source file exists in-repo and an install line places it. (🤖 — would have caught the missing liveness binary before deployment)
- `git remote -v` before editing: verifies you are in the owning repo, not a sibling worktree. (🤖)

**Fix applied:** Six-phase migration — real poller replaces stub, liveness binary vendored, paths de-hardcoded, single portable installer, VM sha symlink repointed, tools repo stripped of poller files. Verified via live fire (`ExecMainStatus=0`). Full record: `~/.claude/worktrees/advisory-protocol-package/.claude/plans/MIGRATION-poller-infra-COMPLETE.md`.
