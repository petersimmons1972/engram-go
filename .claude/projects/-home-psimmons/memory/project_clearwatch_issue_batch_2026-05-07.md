---
name: Clearwatch issue batch 2026-05-07 — closed and deferred
description: Snapshot of which Clearwatch GitHub issues closed in PR #4706, which W1/W2 follow-ups remain, and where the strategy plan lives
type: project
originSessionId: d78f1ef0-76f8-4af6-bca5-fb99755d3d8d
---
**PR #4706** (merge commit `59070dd0`, merged 2026-05-07) closed 5 Clearwatch issues:

- #4624 — git hooks unified into `hooks/`, single `bin/setup-hooks.sh` installer with `cmp -s` diff guard
- #4650 — blocking-gate count reconciled across `CRITICAL-REQUIREMENTS.md` + 2 READMEs (14 blocking + 1 advisory)
- #4605 — canonical `pipeline/pipeline_stages/STAGES.md` (10 stages mapped); inline counts removed from CLI + orchestrator
- #4661 — 8 missing devops_secrets vendor aliases added to seed
- #4606 — W4 trust-boundary tags + `--disallowedTools` hardening on the Claude subprocess; 20 unit tests

**Same PR landed but did NOT close** the Stage-7 W2 epic — gates implemented but not wired:

- #4578 (epic) + 8 children (#4569, #4568, #4567, #4566, #4565, #4564, #4563, #4559) all have wiring-pending comments referencing `59070dd0`.
- F1/F2/F3 gate functions live at `pipeline/compiler/validator.py`: `gate_f1_executive_verdict`, `gate_f2_claims_opacity`, `gate_f3_pricing_unit_comparability`. 18 unit tests pass.
- **Why open:** the gates are standalone importable functions; calling them on assembled HTML from the `compile()` pass is a separate architectural touch. Wire them in to close the epic.

**Why:** the strategy plan (S0) at `~/.claude/plans/using-https-github-com-petersimmons1972-cosmic-taco.md` consolidated 23 open issues into 5 workstreams. This PR closed W4 + W5 + S1 (seed) + parts of W1 (#4605 docs) and landed W2 functions. Remaining workstreams are larger and need their own follow-on Opus sessions (S1–S7 in the plan map).

**How to apply:**

1. Before touching the W2 epic, read `~/.claude/plans/using-https-github-com-petersimmons1972-cosmic-taco.md` for the F1/F2/F3 framing — don't redesign.
2. Wiring is "where in the compile() pass do we call these on assembled HTML." Likely the right hook is post-compile, pre-render — a final-output gate, not a per-section IR gate.
3. W1 (segment-plugin architecture, #4669/#4668/#4667/#4628/#4346) needs a fresh Opus session per the S0 plan — don't start it without re-reading S0 first; the reframe (treat devops_secrets as the second segment to force the plugin abstraction) is load-bearing.

**Don't repeat:**

- The 8 Stage-7 children are NOT 8 separate prompt edits. They map to 3 generic structural failures (F1 verdict-first, F2 claims-trail, F3 unit-normalization). The existing gates already encode that — wiring is the only remaining step.
- W1 implementation issues are tagged "Go port: <X> for devops_secrets" but the right move is segment-agnostic abstraction, not a literal Go port. Don't take those issues at face value.
