# Decision Memo — Harness-Engineering Adoption Review

**Date:** 2026-06-26 · **Coordinator:** Eisenhower · **Disposition:** reject-unless-proven
**Deliverable type:** Decision memo (implementation deferred to a later pass)
**Socialized with:** Codex (gpt-5.3-codex-spark), Hermes (gpt-5.4), Grok — each ruled
independently on all 40 not-yet-ported primitives before this synthesis was drafted.

---

## 1. Executive summary

`harness-port` is our fork of upstream `rocklambros/harness-engineering` (50 primitives,
U1–U50). The fork ported only the **first slice** (~10 primitives: QC/AP vocabulary docs,
commit-msg validator, CLAUDE.md budget linter, install/backfill, JOURNEY.md). Verified
against the committed tree: Project A complete and internally consistent (42 bats cases,
all matching plan); **Project B — the entire deterministic security-hook layer — has zero
committed code**; Project C is a doc-only scope-close (which carried a self-contradiction we
fixed → PR #9).

The real adoption question is the **40 not-yet-ported primitives**. The three-harness panel
converged on a clear answer: **adopt the unported Project-B security-hook layer** (it closes
concrete automatic gaps our current stack genuinely misses), **adopt a small set of cheap
integrity/process primitives**, and **reject the harness-construction scaffolding and the
memory/permission primitives we already cover by other means.**

**7 unanimous ADOPTs** form the spine. **One architectural decision (Wave 0) gates all of
them** and must be made first.

---

## 2. The gating decision — fail-open vs fail-closed (Wave 0)

This is the sharpest thing the panel surfaced (Grok, echoed by Codex). **It must be resolved
before any Project-B hook lands.**

- Our `codex-guard` PreToolUse hook is **fail-OPEN with logging** (if the guard is
  unavailable, the command proceeds and a miss is logged).
- Every upstream Project-B hook is **fail-CLOSED** (block/ask on error).

Adopting fail-closed hooks alongside a fail-open guard, without an explicit **precedence
policy**, creates a class of incidents where one layer logs-but-allows while another blocks —
and operators cannot tell which layer actually stopped (or failed to stop) an action.

**Recommended policy (for your ratification — see Questionables Q1):** security-class hooks
(U24/U25/U27/U28/U30) are **fail-closed**; codex-guard remains the fail-open destructive-op
backstop *beneath* them; precedence is documented as a new AP (e.g. AP.13 "enforcement-layer
precedence: pre-trust > write-scope > supply-chain > destructive-op backstop").

---

## 3. Decision matrix (all 40)

Votes: H=Hermes, C=Codex, G=Grok. Verdict = coordinator call. Wave = adoption sequence.

| U | Primitive | H | C | G | Verdict | Wave / reason |
|---|-----------|---|---|---|---------|---------------|
| U30 | Config hash-audit (pre-trust-init defense) | A | A | A | **ADOPT** | W1 — closes CVE-class pre-trust gap; we have nothing |
| U38 | MCP server pre-trust audit | A | A | A | **ADOPT** | W1 — we add MCP servers with zero gate; code-exec+egress surface |
| U25 | Supply-chain bash checks | A | A | A | **ADOPT** | W1 — curl\|sh, unpinned installs untouched by codex-guard |
| U27 | External-write gate | A | A | A | **ADOPT** | W1 — writes to ~/.ssh, system paths currently ungated |
| U28 | Cached-prefix write gate | A | A | A | **ADOPT** | W1 — CLAUDE.md/foundation poisoning persists across sessions |
| U24 | PostToolUse Semgrep hardening | A | A | A | **ADOPT** | W1 — vulns introduced mid-session sit unsurfaced; biggest auto gap |
| U16 | ID reference-integrity verifier | A | A | A | **ADOPT** | W2 — dangling QC/T/R cites reach agents as real rules; we have the namespace |
| U26 | Bash subcommand hard cap | P | P | A | **ADOPT (cap only)** | W1 — keep 49 hard-cap (Adversa deny-stops-firing>50); drop chatty ask |
| U43 | Deployed-hook parity check | P | P | A | **ADOPT** | W1 — MUST ship with every hook or you verify parity on uninstalled hooks |
| U7 | Four-field commit + validator | P | P | P | **ADOPT (content)** | W2 — fork ALREADY built the warn-not-block validator; port it |
| U12 | Security-review output contract | P | P | P | **ADOPT (contract)** | W2 — fold severity+CWE+file:line+FP+READY into our security-reviewer persona |
| U17 | Pinned pre-commit SAST stack | P | P | A | **ADOPT (scoped)** | W2 — rule: pre-commit=blocking floor, U24=in-session layer (resolves culture clash) |
| U39 | Seed-evaluation skill (anti-rubric) | R | A | A | **ADOPT (pilot)** | W3 — sandbox-exercise > matrix scoring; pilot on THIS exercise |
| U40 | Seed-eval methodology doc | R | P | P | **PARTIAL** | W3 — companion to U39; worked examples only |
| U34 | PreCompact state preservation | P | P | A | **ADOPT (scoped)** | W3 — adopt if compaction drops live plan state; Engram partially covers |
| U31 | Autonomous forensic-log + hard cap | R | P | P | **PARTIAL** | W3 — add forensic TSV + cost hard-cap to night-protocols for AP.11 evidence |
| U44 | SBOM + exact pinning | P | P | P | **PARTIAL** | W3 — SBOM only where we ship external containers/binaries |
| U14 | Same-family subagent cache lineage | R | P | A | **DEFER** | hidden cost: pays off only if dispatch briefs model parent/child cache |
| U3 | Threat-model IDs T.1–T.7 | R | P | R | REJECT | redundant w/ failure-modes catalog; no enforcement unless cited |
| U4 | Reference IDs R.1–R.4 | R | P | R | REJECT | value only with U16 resolver; inline cites already work |
| U5 | Six-phase build sequence | R | R | R | **REJECT** | cargo-cult — builds a harness we already have (panel's top reject) |
| U6 | Per-phase artifact filenames | R | R | R | REJECT | naming ceremony; planf3 phases cover it |
| U8 | Numbered operations docs | R | R | R | REJECT | redundant w/ skills + runbooks |
| U9 | Cross-pollination procedure | R | R | R | REJECT | N/A single-machine |
| U11 | Inventory subagent | R | R | R | REJECT | redundant w/ Explore + roster |
| U13 | Writer/reviewer pair | R | R | R | REJECT | redundant w/ write/groves skills |
| U15 | Effort in agent frontmatter | R | R | P | REJECT | redundant w/ per-dispatch M2 |
| U20 | MemPalace protocol | R | R | R | REJECT | redundant w/ Engram |
| U21 | AAAK diary Stop-hook | R | P | R | REJECT | redundant w/ Engram + lessons-learned |
| U23 | 90-day session-log prune | R | P | R | REJECT | redundant w/ session-start janitor |
| U29 | git push --force ask | R | P | R | REJECT | redundant w/ no-push mandate |
| U32 | Deny-first ML permission model | R | P | R | REJECT | redundant w/ deny-list + codex-guard; adds nondeterminism |
| U33 | Deny rules as separate files | R | P | R | REJECT | maintainability polish, not coverage |
| U36 | Three-layer security stack (model) | R | R | R | **REJECT** | Grok's top reject — false-posture; wire the layers, don't diagram them |
| U37 | Lazy-loaded security-review skill | R | R | P | REJECT | re-platforming unless our /security-review bulk-loads |
| U41 | Cross-platform parity mandate | R | R | R | REJECT | N/A single-machine |
| U42 | Version pinning + re-eval triggers | R | R | R | REJECT | redundant w/ AP.9/QC.5 |
| U45 | audit-claude-config.sh CLI | R | R | R | REJECT* | *adopt only as plumbing IF U30 lands (it does → include in W1) |
| U46 | Permission mode per phase | R | R | R | REJECT | not our dispatch model |
| U48 | Writing rules | R | P | R | REJECT | style preference, not a harness/security failure |

**Tally:** 7 unanimous ADOPT · 8 further ADOPT/PARTIAL (scoped) · 1 defer · 24 reject.
(U45 flips to include-in-W1 because U30 — its prerequisite — is adopted.)

---

## 4. Adoption waves (sequenced — sequencing is load-bearing, per all three harnesses)

**Wave 0 — Ratify the fail-open/closed precedence policy (§2).** Blocks everything below.

**Wave 1 — Pre-trust + boundary security hooks (the Project-B spine).** Order matters:
1. **U38** — audit existing MCP servers + config FIRST (Hermes: audit before you freeze, or
   you hash-lock unaudited junk).
2. **U30 + U45** — then hash-lock settings.json/.mcp.json at SessionStart.
3. **U25, U27, U28, U24, U26(cap)** — the write-scope / supply-chain / commit-time gates.
4. **U43** — ship parity check *with* the hooks so we verify what's actually installed.

**Wave 2 — Cheap, high-leverage integrity & process.** U16 (ID integrity — we already hold
the QC/AP namespace), U7 (port the validator the fork already wrote, warn-not-block), U12
(security-review output contract), U17 (pinned pre-commit as the blocking floor).

**Wave 3 — Scoped partials.** U34 (PreCompact preservation), U31 (forensic TSV + cost
hard-cap into night-protocols for AP.11 evidence), U39+U40 (seed-evaluation, piloted), U44
(SBOM where external artifacts ship).

---

## 5. Cross-cutting risks the panel caught (carry into implementation)

1. **Sequencing (all three):** U38 before U30; U43 with every hook; U45 dead without U30;
   U16 needs the stable QC/AP namespace (we have it — safe).
2. **Fail-open/closed precedence (Grok):** the Wave 0 decision. Non-negotiable prerequisite.
3. **U36 false-posture trap (Grok):** do NOT adopt the three-layer *model doc* — it makes the
   harness look complete while U24/U17/U43 stay unwired and tempts people to skip manual SAST.
   Wire the layers; skip the diagram.
4. **U17 culture clash (Codex+Grok):** without an explicit rule, pinned pre-commit SAST will
   be undermined as "redundant with AP.2 semgrep-first-step." Rule: pre-commit = blocking
   floor; in-session U24 = second layer; manual semgrep-first-step is retired as the primary.
5. **U39 meta-irony (Grok):** this very review socialized 40 primitives with no formal
   seed-eval. Honor it: each ADOPT lands as a **sandbox pilot first**, not a direct merge —
   which *is* the U39 methodology, applied to its own adoption.
6. **U31 AP.11 gap (Grok):** `bypassPermissions` + night-protocols currently leave no forensic
   cost-trail; a runaway unattended run has no evidence for the wake-the-founder trigger.

---

## 6. Questionables (open decisions for the founder)

- **Q1 — Fail-open/closed precedence (Wave 0):** ratify the recommended policy (security hooks
  fail-closed above a fail-open codex-guard backstop, documented as a new AP)? Or keep
  everything fail-open for operational smoothness?
- **Q2 — Friction budget:** U27/U28/U30 add attended prompts. Adopt with narrow allowlists +
  worktree/tmp/managed-store exemptions (upstream's exact design), or run a 1-week soak in
  warn-only mode first?
- **Q3 — Scope of Wave 1:** all 9 Wave-1 hooks at once, or land U30+U38 (pre-trust) first as a
  minimal high-value beachhead and evaluate before the write/bash gates?
- **Q4 — Where does this live:** port hooks into `harness-port` (continue the fork as the
  methodology home, Projects B/C), or implement directly into `~/.claude/hooks` + CLAUDE.md?

---

## 7. What we reject, and why it's the right call

The 24 rejects cluster into three honest categories: **harness-construction scaffolding**
(U5/U6/U8/U9/U11/U13/U46 — we already have a built, mature harness; these solve a bootstrap
problem we don't have), **already-covered-by-our-machinery** (U20/U21/U23 memory → Engram;
U32/U33/U29 permissions → codex-guard+deny; U42 → AP.9/QC.5; U36/U37 security → QC.1/AP.2),
and **ceremony without enforcement** (U3/U4/U15/U48). None of these named a concrete failure
our current setup misses. That is the reject-unless-proven bar doing its job.

---

## 8. Next step

This memo is the decision artifact. Implementation is deferred per scope. When ready, Wave 0
ratification (Q1) unblocks a write-codex-plan / planf3 implementation plan for Wave 1.
Recommended first build target: **U30 + U38** (highest-value, lowest-blast-radius, pre-trust).

---

## Amendments

**2026-06-26 — Post-decision provenance.** (1) QC.4a contradictory-Enforcement-line bug fixed → harness-port PR #9. (2) During the three-harness socialization, a recurring fleet-auth failure surfaced: Codex/Grok rolling single-use OAuth tokens shared across one ChatGPT account (leviathan + codex host + codex-agent container, divergent auth.json stores) cause mutual revocation; `codex exec --ephemeral` additionally suppresses token write-back. Root-caused and filed as FM-125. Fix tracked in `~/.claude/plans/fleet-auth-decouple-plan.md` (decouple fleet consumers onto API-key auth; leviathan keeps OAuth). Grok restored to service via container restart (model pinned to grok-build, not Composer). Not part of the adoption decision; recorded for provenance.
