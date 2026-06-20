# Runbook: fleet-dispatch ATTACH_TOKENS Rotation

**Cross-references:** FM-86 (ESO race / env-var no-hot-reload) · FM-78 (direct kubectl edit reverted by ESO) · FM-85 (trailing-newline token mismatch)
**Last verified:** 2026-06-19

---

## When to use this runbook

- A bearer token in `ATTACH_TOKENS` is suspected or confirmed leaked.
- Rotating as part of a scheduled credential cycle.
- Capability scope changes (adding/removing `produces`/`consumes` fields on any token entry).
- Any 401 or 403 from fleet-dispatch consumers that is not explained by a misconfigured capability.

**Do not use this runbook** to add new consumers — that requires updating the `ATTACH_TOKENS` JSON structure itself and re-validating all capability fields before rotation.

---

## The three pitfalls that turned the last rotation into a 2-hour ordeal

### Pitfall 1 — ESO's 1-hour default sync interval
External Secrets Operator polls Infisical on a 1-hour cycle by default. If you update Infisical and then wait for ESO to notice on its own, you will wait up to 60 minutes before the k8s `fleet-dispatch-tokens` Secret reflects the new value. **Always force-sync** (step 3 below).

### Pitfall 2 — No hot-reload: ATTACH_TOKENS is read only at pod start
`fleet-dispatch` reads `ATTACH_TOKENS` from an environment variable at startup, not via a volume watch. Updating the Secret is not enough — the running pods still hold the old value in memory. **Always rollout-restart** the deployment (step 4) after confirming the Secret has been updated, never before.

### Pitfall 3 — The lockstep gap: consumers hold independent copies
This is the one that kills you. When `ATTACH_TOKENS` is rotated, **both** consumers must be cycled in the same rotation window or they will authenticate with stale tokens and return 401:

- **Codex consumer** (`codex-attach.service`) reads its token from `~/.config/fleet-dispatch/token` on the codex node. This file is NOT managed by ESO — it must be updated manually and the service restarted.
- **Hermes consumer** (`hermes-attach` container) reads its token from an env var or Secret mount. It picks up the new value only after its container restarts.

**The race:** if you restart fleet-dispatch (step 4) before cycling both consumers, those consumers' stale tokens immediately start returning 401. Work stalls silently — items are claimed but the claim calls fail auth. Always cycle all three components (fleet-dispatch + both consumers) before the old tokens expire.

### Race warning: never restart before the Secret resourceVersion bumps
ESO may still be writing when you annotate the force-sync. **Watch the Secret's `resourceVersion`** — it increments only once ESO has committed the new value. Restarting fleet-dispatch before this confirmation loads the old `ATTACH_TOKENS` into the new pods.

---

## Ordered rotation sequence

### Step 1 — Update Infisical (source of truth)

Update `ATTACH_TOKENS` at the Infisical path:
- **Project:** `homelab-jz5w`
- **Environment:** `prod`
- **App path:** `apps/ai-fleet`
- **Key:** `ATTACH_TOKENS`

Use `mcp__infisical-personal__update-secret` with the full new JSON value. **Never put the token values in a command line, log, or issue.** Confirm the update via `mcp__infisical-personal__get-secret` (verify only field names and structure, never print values).

### Step 2 — Force-sync the ExternalSecret

```bash
codex-guard kubectl annotate externalsecret fleet-dispatch-tokens \
  -n ai-fleet \
  force-sync=$(date +%s) \
  --overwrite
```

### Step 3 — WAIT for Secret resourceVersion to change

Do not proceed until the Secret shows the new data. Poll until `resourceVersion` increments from its current value:

```bash
# Record current resourceVersion
OLD_RV=$(codex-guard kubectl get secret fleet-dispatch-tokens -n ai-fleet \
  -o jsonpath='{.metadata.resourceVersion}')
echo "Waiting for resourceVersion > $OLD_RV ..."

# Poll (runs until resourceVersion changes — expect < 30s after force-sync)
until [ "$(codex-guard kubectl get secret fleet-dispatch-tokens -n ai-fleet \
  -o jsonpath='{.metadata.resourceVersion}')" != "$OLD_RV" ]; do
  sleep 3
done
echo "Secret updated. Proceeding."
```

If `resourceVersion` does not change within 2 minutes, check ESO logs:

```bash
codex-guard kubectl logs -n ai-fleet \
  -l app.kubernetes.io/name=external-secrets \
  --tail=50
```

### Step 4 — Rollout-restart fleet-dispatch

Only after step 3 confirms the Secret is updated:

```bash
codex-guard kubectl rollout restart deployment/fleet-dispatch -n ai-fleet
codex-guard kubectl rollout status deployment/fleet-dispatch -n ai-fleet
```

### Step 5 — Cycle the Hermes consumer (lockstep)

Restart the hermes-attach container so it re-reads its mounted credentials:

```bash
codex-guard kubectl rollout restart deployment/hermes-attach -n ai-fleet
codex-guard kubectl rollout status deployment/hermes-attach -n ai-fleet
```

(If hermes-attach is a sidecar in another deployment, restart that deployment instead.)

### Step 6 — Cycle the Codex consumer (lockstep)

SSH into the codex node as a **diagnostic** (this is the one approved ssh use during rotation):

```bash
# On codex.petersimmons.com:
# 1. Write the new token (retrieve value from Infisical — never store or echo it)
#    The token file must have NO trailing newline (FM-85):
#    printf '%s' "<new-token-value>" > ~/.config/fleet-dispatch/token
#    chmod 600 ~/.config/fleet-dispatch/token
# 2. Restart the consumer service:
systemctl --user restart codex-attach.service
systemctl --user status codex-attach.service
```

Token file path: `~/.config/fleet-dispatch/token` (and `token-consult`, `token-review` if those are separate files — rotate all that changed).

**Token file format:** no trailing newline. Verify: `wc -c ~/.config/fleet-dispatch/token` should equal the expected token length exactly (e.g. 64 hex chars = 64 bytes, not 65).

### Step 7 — Prove it works

Run the round-trip canary from a host that can reach `fleet-dispatch.petersimmons.com`:

```bash
scripts/fleet-canary.sh
```

Or exercise a minimal consult round-trip:

```bash
TOKEN_FILE=~/.config/fleet-dispatch/token-consult \
  scripts/fleet-consult.sh "rotation smoke test $(date +%s)" \
  --repo petersimmons1972/claude-codex --issue 1
```

A successful terminal `done` with a non-empty result body confirms: Infisical → ESO → Secret → fleet-dispatch → consumer → claim → report all work end-to-end (per FM-87: verify the deliverable, not just the status).

---

## Full copy-paste command block

Run this block in order. Fill in `$OLD_RV` and substitute the correct deployment names for your environment.

```bash
# [1] Update Infisical via MCP tool (mcp__infisical-personal__update-secret)
#     project=homelab-jz5w env=prod path=apps/ai-fleet key=ATTACH_TOKENS

# [2] Force-sync ESO
codex-guard kubectl annotate externalsecret fleet-dispatch-tokens \
  -n ai-fleet force-sync=$(date +%s) --overwrite

# [3] Wait for Secret resourceVersion to change
OLD_RV=$(codex-guard kubectl get secret fleet-dispatch-tokens \
  -n ai-fleet -o jsonpath='{.metadata.resourceVersion}')
until [ "$(codex-guard kubectl get secret fleet-dispatch-tokens \
  -n ai-fleet -o jsonpath='{.metadata.resourceVersion}')" != "$OLD_RV" ]; do
  sleep 3; echo -n ".";
done && echo "Secret updated."

# [4] Restart fleet-dispatch
codex-guard kubectl rollout restart deployment/fleet-dispatch -n ai-fleet
codex-guard kubectl rollout status deployment/fleet-dispatch -n ai-fleet

# [5] Restart hermes consumer
codex-guard kubectl rollout restart deployment/hermes-attach -n ai-fleet
codex-guard kubectl rollout status deployment/hermes-attach -n ai-fleet

# [6] On codex node — update token file + restart service
#     (ssh codex.petersimmons.com for this step — diagnostics-only ssh is approved here)
#     printf '%s' "<token>" > ~/.config/fleet-dispatch/token
#     systemctl --user restart codex-attach.service

# [7] Canary
scripts/fleet-canary.sh
```

---

## Failure modes

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| Consumers returning 401 immediately after rotation | Codex or Hermes not cycled in lockstep (FM-96) | Complete step 5 and/or step 6; do not skip |
| `resourceVersion` not changing after force-sync | ESO unreachable or ExternalSecret in error state | Check ESO logs; verify Infisical reachability |
| Fleet-dispatch pods loading old `ATTACH_TOKENS` | Rollout-restart ran before Secret updated (FM-86 race) | Roll back; wait for Secret update; re-restart |
| Token length 65 instead of 64 | Trailing newline in token file (FM-85) | Regenerate without trailing newline; `printf '%s'` not `echo` |
| 403 instead of 401 from claim endpoint | Token exists but missing required `consumes` capability | Verify JSON structure in Infisical; check `produces`/`consumes` fields |
