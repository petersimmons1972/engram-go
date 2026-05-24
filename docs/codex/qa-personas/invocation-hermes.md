# QA Personas — Hermes Invocation Guide

Hermes runs on `hermes.petersimmons.com` as a Docker container
(`hermes-agent`), using Qwen3-32B via Olla. It has a kanban task board and a
`hermes send` command for programmatic dispatch.

## Parallel Dispatch via Kanban Swarm

Hermes' `hermes kanban swarm` command creates parallel workers that converge
on a single result — this is the native parallel dispatch mechanism.

Replace `{TARGET}` with the artifact description.

### Option A: Kanban Swarm (Preferred — True Parallel)

```bash
# Inside the hermes-agent container (or via docker exec):
hermes kanban swarm \
  --title "QA personas sweep: {TARGET}" \
  --body "$(cat <<'EOF'
Run the six-persona fault-finder sweep against: {TARGET}

Dispatch six parallel reviewer personas. Each persona uses only its own lens.
No persona sees another's findings. Collect all six outputs and produce an
aggregate report.

PERSONA 1 — Skeptical Staff Engineer:
You are a 10-year production veteran. Review {TARGET} for hidden side effects,
implicit dependencies, unexpected coupling, failure modes the happy path hides,
trust assumptions, and convenience features that are security holes. Do not
suggest fixes. Report observations and concerns.
Output: Trust Breakpoints list (Location / Observation / Concern / Severity:
blocker|serious|nitpick), What Earned Trust, Questions Before I Sign Off.

PERSONA 2 — Security Reviewer:
You are an adversarial security auditor. Review {TARGET} for secret material
(hardcoded credentials, API keys, tokens in any encoding), permission boundary
violations, remote execution surface (eval/exec/shell-out with interpolated
strings/deserialization), trust-boundary crossings, status-vs-mutation
mismatches, logging that exposes secrets, and default-permissive configs.
Do not write fixes.
Output: Findings list (Location / Class / Observation / Risk / Severity:
critical|high|medium|low), What Was Done Right, Questions for the Author.

PERSONA 3 — New Maintainer:
You just inherited this project. No prior context. Two hours to orient. Review
{TARGET} for first-impression clarity, first-command documentation, orientation
gaps, implicit setup, stale/contradictory docs, institutional-context naming,
missing "why", and onboarding traps. Every place you had to guess is a finding.
Output: Trust Breakpoints (Location / What I tried / What broke / Severity:
blocker|serious|friction), What Worked, Questions I Couldn't Answer.

PERSONA 4 — Heavy CLI User:
You pipe, script, and parallelize everything. Review {TARGET} for subcommand
consistency (flag name matching), output discipline (stdout/stderr separation),
exit code discipline, idempotency, composability (--json, ISO 8601 timestamps),
POSIX/GNU flag conventions, non-TTY confirmation handling, help completeness,
and output reproducibility.
Output: Trust Breakpoints (Location / Class / Observation / Impact / Severity:
blocker|serious|friction), What Composes Cleanly, Questions Before I Build.

PERSONA 5 — Operator / SRE:
You will be paged when this breaks at 3 AM. Review {TARGET} for logging quality
(enough context to reconstruct timeline?), structured logs, health/liveness
signals (including downstream checks), failure recovery posture, notification
pipeline integrity, metrics with cardinality, graceful degradation, operational
levers (kill switches), and runbook surface.
Output: Trust Breakpoints (Location / Class / Observation / 3-AM Impact /
Severity: blocker|serious|friction), What I Could Operate, Runbook Gaps.

PERSONA 6 — Docs-First Newcomer:
CRITICAL RULE: You do not read source code. If you would need source to answer
a question, that question is a finding. Review {TARGET} documentation for
README completeness (what/why/who/install/run/example), example accuracy
(does copy-paste work?), missing prerequisites, stale links, section drift,
first-failure guidance, versioning honesty, and "go read the source" punts.
Output: Trust Breakpoints (Location / What I tried / What happened / Where I
would have had to read source / Severity: blocker|serious|friction), What the
Docs Got Right, Questions the Docs Don't Answer.

AGGREGATION (after all six complete):
1. List every Trust Breakpoint from every persona, noting reporter.
2. Tag any finding reported by 2+ personas as HIGH-CONFIDENCE.
3. Rank by severity: blocker/critical → serious/high → friction/medium → nitpick/low.
4. Ship gate: ANY blocker or critical = NO-GO.
5. Output consolidated report with ship gate decision at the top.
EOF
)"
```

### Option B: Sequential Runs (Fallback — if swarm unavailable)

If the swarm command fails or is unavailable, run each persona sequentially
using `hermes send`. Collect output to files, then aggregate.

```bash
# Run from within the hermes-agent container
for persona in \
  "skeptical-staff-engineer" \
  "security-reviewer" \
  "new-maintainer" \
  "heavy-cli-user" \
  "operator-sre" \
  "docs-first-newcomer"
do
  echo "Running persona: $persona against {TARGET}"
  hermes send \
    "Adopt the $persona persona from /home/hermes/.hermes/skills/qa-personas/persona-${persona}.md \
     and review the target: {TARGET}. \
     Output the Trust Breakpoints format defined in your profile. \
     Be specific: cite exact file paths, line numbers, command invocations, doc sections. \
     You are not seeing the other personas' findings." \
    > /tmp/qa-${persona}.txt 2>&1
  echo "Done: $persona"
done

# Aggregate
cat /tmp/qa-skeptical-staff-engineer.txt \
    /tmp/qa-security-reviewer.txt \
    /tmp/qa-new-maintainer.txt \
    /tmp/qa-heavy-cli-user.txt \
    /tmp/qa-operator-sre.txt \
    /tmp/qa-docs-first-newcomer.txt \
> /tmp/qa-aggregate.txt
echo "Aggregate at /tmp/qa-aggregate.txt"
```

## Via SSH from Coordinator (Codex or Claude)

The coordinator can inject a kanban task from the workstation:

```bash
ssh hermes.petersimmons.com \
  'sudo docker exec hermes-agent hermes kanban create \
    --title "QA sweep: {TARGET}" \
    --body "Run the six-persona fault-finder sweep per /home/hermes/.hermes/skills/qa-personas/runbook.md against: {TARGET}. Return aggregate findings with ship gate (GO/NO-GO)."'
```

Then check the result:

```bash
ssh hermes.petersimmons.com \
  'sudo docker exec hermes-agent hermes kanban list | head -5'
```

## Aggregate Report Format

After all six personas complete (swarm or sequential), produce:

1. **Ship gate**: GO / NO-GO + reason (any blocker/critical = NO-GO)
2. **HIGH-CONFIDENCE findings**: raised by 2+ personas independently
3. **Per-severity sections**: blockers → serious → friction → nitpick
4. **Questions**: deduplicated from all personas

## Round 2

After fixing all blockers from Round 1, re-run the same dispatch against the
post-fix artifact. Verify Round 1 blockers are resolved in Round 2 output.
Do not mark work done until Round 2 returns zero blockers/critical.
