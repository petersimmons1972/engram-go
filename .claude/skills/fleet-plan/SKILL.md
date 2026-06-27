---
name: fleet-plan
description: >
  Fleet-dispatch-integrated three-way planning loop. Claude orchestrates,
  dispatching the draft to every registered review worker in parallel, surfacing
  divergence, converging, and recording the result in fleet-dispatch for SSE mirror
  to GitHub. Use when the user says "fleet plan", "run fleet-plan", "dispatch a
  plan to the fleet", or any three-way plan that needs a queue audit record.
  NOT a solo planning session (use three-way-plan for lighter-weight socialization).
  Extensible: adding a new fleet worker requires only one row in the registry below.
---

# fleet-plan

Claude leads a planning session tracked in fleet-dispatch. The draft is socialized
with every registered review worker in parallel. Disagreements are surfaced, not
papered over. The converged plan is reported done and mirrored to GitHub via
fleet-sync SSE.

You are the orchestrator — in charge, not a peer. The workers review; you decide.

---

## 1. Worker registry

**This table is the only thing that changes when a worker joins the fleet.**
Add one row per new worker. The rest of the skill adapts automatically.

| Worker | Capabilities it reviews | Contact method | Default model |
|--------|------------------------|----------------|---------------|
| codex  | impl · review · design | `ssh codex.petersimmons.com 'codex exec --skip-git-repo-check --cd <repo-or-/tmp> --sandbox read-only' <<'PROMPT'\n<prompt>\nPROMPT` | gpt-5.3-codex-spark |
| hermes | review · design        | `PB=$(printf '%s' "<prompt>" \| base64 -w0); ssh hermes.petersimmons.com "docker exec -e PB='$PB' hermes-agent sh -lc 'echo \"\$PB\" \| base64 -d \| /app/venv/bin/hermes chat -Q --yolo -q \"\$(cat)\""'` | gpt-5.4 |
| grok   | review · design        | `GB=$(printf '%s' "<prompt>" \| base64 -w0); ssh grok.petersimmons.com "docker exec -e PB='$GB' grok-agent sh -lc 'grok --single \"\$(echo \$PB\|base64 -d)\" --output-format plain'"` | grok (grok-agent) |

**Golden rule — ASK, don't infer.** Before the first round, check capabilities if
unsure: `mcp__codex-tools__codex_capabilities` for Codex; a capabilities prompt for
Hermes. Never infer a model or feature from observation (FM: feedback-ask-dont-infer).

**Adding a new worker:** add a row to this table. Specify which capabilities it
reviews. The dispatch loop in §4 hits every worker whose capability column
intersects the plan topic. No other skill changes required.

---

## 2. Fleet-dispatch record (optional but strongly recommended)

A fleet record gives you SSE mirror to GitHub, audit trail, and a lease clock
for long sessions. Skip only for throwaway experiments.

> **ESO ownership note.** `fleet-dispatch-tokens` is managed by ExternalSecrets
> Operator (ClusterSecretStore `infisical-ai-fleet-tokens`, project `homelab-jz5w`,
> env `prod`, path `/apps/ai-fleet`). Direct `kubectl patch` is silently reverted.
> ATTACH_TOKENS is injected as an **env var** (not a volume mount), so a pod restart
> is required after every Infisical update. To modify the token set, use:
> 1. `mcp__infisical-personal__update-secret` (source of truth)
> 2. `codex-guard kubectl annotate externalsecret fleet-dispatch-tokens -n ai-fleet "force-sync=$(date +%s)" --overwrite`
> 3. Poll `kubectl get secret fleet-dispatch-tokens -n ai-fleet -o jsonpath='{.metadata.resourceVersion}'` until it changes
> 4. `codex-guard kubectl rollout restart deployment/fleet-dispatch -n ai-fleet && kubectl rollout status ...`
>
> Coordinator tokens (`principal=claude-coordinator`) need **both** `"plan"` in
> `produces` (to enqueue) and `"plan"` in `consumes` (to claim). Missing either
> returns HTTP 403 or `{"empty":true}` respectively.

### Enqueue (using existing coordinator token — requires `plan` in both produces AND consumes)

```bash
TOKEN=$(codex-guard kubectl get secret fleet-dispatch-tokens -n ai-fleet \
  -o json | python3 -c "
import sys,json,base64
s=json.load(sys.stdin)
tokens=json.loads(base64.b64decode(s['data']['ATTACH_TOKENS']))
# pick any coordinator token that produces AND consumes 'plan'
for k,v in tokens.items():
    if v.get('principal')=='claude-coordinator' \
       and 'plan' in v.get('produces',[]) \
       and 'plan' in v.get('consumes',[]):
        print(k); break
")

ITEM=$(xh POST https://fleet-dispatch.petersimmons.com/enqueue \
  Authorization:"Bearer $TOKEN" \
  capability=plan \
  model_tier=sonnet \
  correlation_id="plan:$(date +%Y%m%d-%H%M%S)" \
  expected_delivery=comment_only \
  --body | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['id'])")

echo "Fleet item: $ITEM"
```

> If enqueue returns 409 (active-window dedup hit), an identical plan item is
> already in flight — use that item's ID instead.

### Claim (Claude takes the lease — human class = 15 min TTL)

```bash
LEASE=$(xh POST https://fleet-dispatch.petersimmons.com/claim \
  Authorization:"Bearer $TOKEN" \
  node=claude-coordinator \
  lease_class=human \
  capabilities:='["plan"]' \
  --body)
LEASE_ID=$(echo $LEASE | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
FENCE=$(echo $LEASE | python3 -c "import sys,json; print(json.load(sys.stdin)['fence_gen'])")
```

### Renewal loop (run in background while the session continues)

Human-class lease: 15 min TTL / 5 min grace. Renew every **5 min** (TTL÷3).

```bash
# Paste into a second terminal or run as a background job
while true; do
  sleep 300
  xh POST https://fleet-dispatch.petersimmons.com/renew \
    Authorization:"Bearer $TOKEN" \
    lease_id=$LEASE_ID \
    fence_gen=$FENCE > /dev/null && echo "$(date) renewed" || echo "$(date) renew FAILED"
done &
RENEW_PID=$!
```

Kill with `kill $RENEW_PID` after reporting done.

---

## 3. Draft the plan

Write the plan/design with:
- **Problem** — one paragraph, concrete
- **Scope** — what is and isn't included
- **Approach** — the specific decisions under review
- **Open questions** — what you want each worker to evaluate

Be concrete. Vague drafts get vague reviews.

---

## 4. Dispatch loop (parallel)

Send to every worker in the registry whose capability intersects the plan topic.
Run both calls concurrently — do not wait for one before sending the other.

**Codex** (implementer's eye: test cases, edge cases, failure modes, "what breaks this"):

```bash
ssh codex.petersimmons.com 'codex exec --skip-git-repo-check \
  --cd /tmp --sandbox read-only' <<'PROMPT'
<paste plan draft>

Review this plan as an implementer. Provide:
1. Concrete test cases (name the test, the input, the expected outcome)
2. Edge cases you would trip over implementing this
3. Failure modes — what breaks silently, what blows up loudly
4. Any implementation gotcha not addressed in the plan
5. The single strongest objection to this approach

Be specific. Vague concerns are not useful.
PROMPT
```

**Hermes** (contrarian + optional 6-persona QA sweep):

```bash
PB=$(printf '%s\n\nYou are the contrarian. Argue AGAINST this plan. Find the strongest objection. If the surface is user-facing or security-relevant, run your 6-persona QA sweep. Do NOT validate — interrogate.\n\nSpecifically:\n1. The single strongest argument against this approach\n2. What assumption is most likely wrong\n3. What the plan does not account for\n4. If you had to break this in production, how would you do it' \
  | base64 -w0)
ssh hermes.petersimmons.com \
  "docker exec -e PB='$PB' hermes-agent sh -lc \
  'echo \"\$PB\" | base64 -d | /app/venv/bin/hermes chat -Q --yolo -q \"\$(cat)\"'"
```

**Per-call model override:** `codex --model <tier>`, hermes `-m <model>` / `HERMES_MODEL=<model>`.
Default tiers unless the plan warrants escalation — ask before escalating.

---

## 5. Surface disagreements — do NOT agree too early

Collect both responses. Where workers disagree with you or with each other:
- **Name the disagreement explicitly** in the plan
- Do not resolve it by averaging or ignoring the minority view
- The founder wants divergence visible, not smoothed over

Record the provenance of every critique: which worker raised it, in what round.

---

## 6. Iterate to convergence

Re-socialize revised sections until no open criticals remain (a GO).

For high-stakes designs: run multiple rounds. Single-voice review misses what
engineered dissent catches. There is no maximum round count — stop when the
remaining objections are acknowledged, not suppressed.

---

## 7. Synthesize and decide

You are in charge. Integrate the input, make the call, record the rationale.
The plan is yours to own — not a committee average.

Capture:
- Every accepted test case and failure mode (fold into the plan body)
- New failure-mode classes → `~/bin/add-failure-mode.sh`
- Where you diverged from a worker's recommendation, and why

---

## 8. Report done to fleet-dispatch

```bash
kill $RENEW_PID 2>/dev/null  # stop the renewal loop

xh POST https://fleet-dispatch.petersimmons.com/report \
  Authorization:"Bearer $TOKEN" \
  lease_id=$LEASE_ID \
  fence_gen=$FENCE \
  disposition=done \
  summary="<one-sentence outcome>" \
  artifact_text="<full plan text or GitHub comment URL>"
```

Fleet-sync mirrors this to the GitHub issue that created the fleet item within
seconds via SSE. If the mirror doesn't appear within 60 s, poll:

```bash
xh GET "https://fleet-dispatch.petersimmons.com/items/$ITEM" \
  Authorization:"Bearer $TOKEN"
```

---

## 9. Hand-off

If the GO'd plan is destined for Codex implementation → **write-codex-plan** skill
(7-section format → queue-agent). The fleet-plan produces the converged design;
write-codex-plan packages it for the durable work loop.

---

## Constraints

- **No `claude -p`** — never spawn an autonomous Claude daemon to plan; it burns
  API tokens. The orchestrating Claude is the human-attended session you are in.
- **Clean up ephemeral resources** (FM-16 / Class H). Any Docker/k8s spun up
  during review must be torn down before the round closes.
- **Codex and Hermes are shared endpoints.** Avoid double-asking the same question
  in overlapping parallel sessions; each exec is a separate process (safe) but
  not isolated.
- **codex-guard for all git/gh/kubectl calls** — prefix any shell ops with
  `codex-guard` per CLAUDE.md §Agent Dispatch Mandates M9.

---

## Cross-references

- `three-way-plan` — lighter-weight socialization (no fleet-dispatch tracking)
- `write-codex-plan` — Codex implementation handoff (downstream of a GO'd plan)
- `fleet-dispatch#52` — Phase B RFC: fleet-plan as requestable fleet service
- fleet-dispatch queue: `https://fleet-dispatch.petersimmons.com`
- Lease classes: autonomous (30s/60s grace) · human (15m/5m grace) — `r3.go DefaultClassConfig()`
- Memory: `feedback-ask-dont-infer`, `feedback-engineer-dissent-review`
