# Infisical Secret Control Plane — Break-Glass / Outage Recovery Runbook

> **STATUS: DRAFT** — Reviewed by Hermes red-team; not yet field-validated.  
> Owner: Peter Simmons · Last revised: 2026-06-15  
> Related plan: `docs/plans/` (credential-centralization)

---

## Purpose & Threat Model

Centralizing homelab secrets into Infisical + ESO (External Secrets Operator) creates an **availability coupling**: if the secret plane is degraded, running pods keep working (k8s Secrets already exist in etcd) but pod restarts and new deploys during the outage will fail to inject credentials. This runbook covers triage, keep-running tactics, and step-by-step recovery for each failure mode.

**No secret values appear in this document.**  
All verification commands are read-only. Mutations are marked ⚠️ MUTATING.

---

## Architecture Snapshot (reference)

| Component                        | Location / endpoint                                                      |
|----------------------------------|--------------------------------------------------------------------------|
| Infisical app (2 replicas)       | `ns:infisical` · pods on worker138/worker139                             |
| Infisical Redis (1 replica)      | `ns:infisical` · pod on worker135                                        |
| Infisical PostgreSQL             | ExternalName SVC → `trunas.petersimmons.com:5432` (TrueNAS)             |
| Infisical internal API           | `http://infisical.infisical.svc.cluster.local:80/api`                    |
| Infisical UI                     | `https://infisical.petersimmons.com`                                     |
| ESO controller (v2.0.1)          | `ns:external-secrets` · 1 controller + cert-controller + webhook pod     |
| ESO auth                         | k8s Secret `infisical-machine-identity-credentials` in `ns:external-secrets` (keys: `clientId`, `clientSecret`) |
| ClusterSecretStores              | 28 stores total; all project-slug `homelab-jz5w`, env `prod`            |
| ExternalSecrets (live)           | 27 ESes across 15 namespaces; `refreshInterval: 1h`; `creationPolicy: Owner` |
| Bootstrap "foundation stone"     | k8s Secret `infisical-bootstrap-secret` in `ns:infisical` (keys: `ENCRYPTION_KEY`, `JWT_*_SECRET` ×4, `POSTGRES_PASSWORD`, `POSTGRES_USER`, `REDIS_PASSWORD`) |

---

## Section 1 — Failure Scenarios & Blast Radius

### 1a. Infisical App / API Down (pods crash-looping or terminated)

**What still works:**  
All existing k8s Secrets remain in etcd and are mounted normally. Running pods continue operating without interruption. The last-synced secret values are valid until ESO's next refresh attempt.

**What breaks:**  
- ESO cannot sync new/changed secret values (next poll at T+1h fails)
- New `ExternalSecret` resources will not materialise a k8s Secret
- Any deploy that creates a new pod mounting an ESO-owned Secret will hang (Secret present from last sync — pod starts; but if the ES has never synced, the Secret does not exist yet)
- Infisical UI inaccessible

**How to detect:**
```bash
# Check Infisical pod health
kubectl get pods -n infisical

# Test the internal API from a debug pod (or any pod in the cluster)
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -s http://infisical.infisical.svc.cluster.local:80/api/status

# Check ESO sync errors appearing in ExternalSecret status
kubectl get externalsecret -A | grep -v SecretSynced

# Check ESO controller logs for HTTP errors
kubectl logs -n external-secrets deployment/external-secrets --tail=50 | grep -iE 'error|failed|503|502'
```

**Estimated safe window:** ~1 hour before ESO first retries and logs errors. Running workloads safe indefinitely (see Section 3).

---

### 1b. Infisical PostgreSQL Down (TrueNAS unreachable or DB crashed)

Infisical's DB is an ExternalName Service pointing to `trunas.petersimmons.com:5432`.

**What still works:** Same as 1a. In-flight Infisical pods may continue serving cached state briefly, then begin failing API calls that require DB reads.

**What breaks:** Infisical app itself starts returning 5xx errors; all ESO syncs fail; UI unusable. If Infisical pods restart during DB outage they will fail to start.

**How to detect:**
```bash
# Check Infisical pod logs for DB connection errors
kubectl logs -n infisical -l app=infisical --tail=50 | grep -iE 'postgres|connection|error|ECONNREFUSED'

# Test TCP connectivity to TrueNAS postgres from inside cluster
kubectl run -it --rm pg-check --image=postgres:15-alpine --restart=Never -- \
  pg_isready -h trunas.petersimmons.com -p 5432 -U postgres
```

**Note:** If TrueNAS itself is down this impacts all postgres-backed services (Supabase, clearwatch-research, etc.) — the blast radius is wider than Infisical alone.

---

### 1c. ESO Controller Down

**What still works:** All existing k8s Secrets are unaffected. Running pods see no change. The k8s Secrets owned by ESO remain in etcd.

**What breaks:** No secret rotation or sync. New ExternalSecrets do not create k8s Secrets. `kubectl get externalsecret -A` shows stale `lastSyncTime` but status messages stop updating.

**How to detect:**
```bash
# ESO controller pod status
kubectl get pods -n external-secrets

# Webhook pod — if down, ExternalSecret admission will fail
kubectl get pods -n external-secrets | grep webhook

# Age of last successful sync (compare against current time)
kubectl get externalsecret -A -o wide | head -5
```

---

### 1d. Machine-Identity Auth Failure / Credential Expiry

The ESO ClusterSecretStores authenticate via Universal Auth using `infisical-machine-identity-credentials` in `ns:external-secrets`. If the clientSecret rotates in Infisical without updating the k8s Secret, or if the machine identity is revoked, all ClusterSecretStores fail simultaneously.

**What still works:** Existing k8s Secrets remain. Currently-running pods are unaffected.

**What breaks:** All 28 ClusterSecretStores go `Invalid`; all ExternalSecret syncs fail with 401/403 errors.

**How to detect:**
```bash
# Check ClusterSecretStore status (all should show Valid/True)
kubectl get clustersecretstore -A

# Describe a failing store for the error message
kubectl describe clustersecretstore infisical | grep -A5 'Conditions'

# ESO logs will show auth failures
kubectl logs -n external-secrets deployment/external-secrets --tail=100 | grep -iE 'auth|401|403|unauthorized|credential'
```

**Distinguishing signal:** All stores fail at once → auth credential issue. Single store fails → that store's specific machine identity or path scope.

---

### 1e. Internal DNS Failure

ESO reaches Infisical via `http://infisical.infisical.svc.cluster.local:80/api`. If CoreDNS fails, this name does not resolve.

**What still works:** Running pods are unaffected. The ClusterIP (`10.43.165.233`) remains valid if the SVC object exists.

**What breaks:** ESO sync fails with DNS resolution errors. Any pod that resolves Infisical by hostname at startup also fails.

**How to detect:**
```bash
# Test CoreDNS from a debug pod
kubectl run -it --rm dns-check --image=busybox --restart=Never -- \
  nslookup infisical.infisical.svc.cluster.local

# CoreDNS pod health
kubectl get pods -n kube-system -l k8s-app=kube-dns

# Direct IP fallback (use the ClusterIP — verify it hasn't changed)
kubectl get svc -n infisical infisical -o jsonpath='{.spec.clusterIP}'
```

**Workaround:** If CoreDNS is intermittently broken, ESO can be patched temporarily to use the ClusterIP directly in `hostAPI` — but this requires a ⚠️ MUTATING patch to each ClusterSecretStore and should only be done as a last resort with founder confirmation.

---

### 1f. Bad Secret Version Pushed

An operator pushes incorrect values to Infisical. After the next ESO sync (within 1h), all pods mounting that secret may misbehave or crash.

**What still works:** Pods running before the sync continue with the previously-mounted values (environment variable injection is at pod start; ConfigMap/Secret volume mounts update within ~1 minute on a live pod).

**What breaks:** Pods that restart after the bad sync pick up wrong credentials. Mounts refresh automatically for volume-mounted Secrets (within `kubelet` sync period, typically 1–2 min).

**How to detect:**
```bash
# Find recently-synced ExternalSecrets
kubectl get externalsecret -A -o wide | sort -k7

# Check the specific ESO-owned secret's lastSyncTime
kubectl describe externalsecret <name> -n <ns> | grep -E 'Last Sync|Reason|Message'

# For volume-mounted secrets: check if pods have entered CrashLoopBackOff since last sync
kubectl get pods -A | grep -v Running | grep -v Completed
```

**Recovery:** Correct the value in Infisical UI immediately, then force a resync (Section 4 — "Resync ExternalSecrets").

---

## Section 2 — Immediate Triage Sequence

Run these **read-only** commands in order to localize any secret-plane fault. Takes < 5 minutes.

```bash
# Step 1: ESO controller health
kubectl get pods -n external-secrets
kubectl logs -n external-secrets deployment/external-secrets --tail=100 | grep -iE 'error|failed|warn'

# Step 2: ClusterSecretStore health (all should be Valid/True)
kubectl get clustersecretstore -A
# Any showing 'False' or 'InvalidProviderConfig':
kubectl describe clustersecretstore <name> | grep -A10 'Conditions'

# Step 3: ExternalSecret sync status across the fleet
kubectl get externalsecret -A
# Note any NOT showing 'SecretSynced'
kubectl describe externalsecret <name> -n <ns> | grep -A10 'Conditions'

# Step 4: Infisical app health
kubectl get pods -n infisical
curl -s http://infisical.infisical.svc.cluster.local:80/api/status   # from inside cluster
# Or via ingress:
curl -s https://infisical.petersimmons.com/api/status

# Step 5: Infisical DB reachability
kubectl run -it --rm pg-check --image=postgres:15-alpine --restart=Never -- \
  pg_isready -h trunas.petersimmons.com -p 5432

# Step 6: Machine identity credential secret is present (name only, not value)
kubectl get secret -n external-secrets infisical-machine-identity-credentials
```

**Triage decision tree:**
- ESO pods down → Section 4 "Restart ESO"
- ClusterSecretStores all Invalid → Section 4 "Restore Machine Identity Secret"
- Infisical pods crash-looping → check bootstrap secret / DB reachability
- Infisical DB unreachable → TrueNAS recovery path (Section 4 "Restore Infisical")
- Everything healthy but single ES failing → likely a path/key misconfiguration in that specific ES

---

## Section 3 — Keep-Running Tactics (Safety Window)

### The core safety guarantee

`creationPolicy: Owner` means ESO **creates and owns** the k8s Secret — it is stored in etcd. When ESO or Infisical is down, **those Secrets do not disappear**. Running pods continue mounting them normally.

```
Infisical outage starts
        │
        ▼
 ESO poll at T+1h fails → logs error → does NOT delete the Secret
        │
        ▼
 Running pods: NO IMPACT
 Pod restarts: OK (Secret still exists in etcd, pod mounts it)
 New ExternalSecret resources: NO Secret created ← DANGER
 ExternalSecret deleted during outage: Secret deleted ← DANGER (see below)
```

### Danger window: do NOT do these during an outage

1. **Do not delete an ExternalSecret** during an outage. Because `creationPolicy: Owner`, deleting the ES will GC the owned k8s Secret. The secret value is still in Infisical; creating a new ES will re-sync it when Infisical recovers — but in the meantime any pod restarting will fail.

2. **Do not deploy new workloads that reference ExternalSecret-owned Secrets that have not yet synced.** The Secret may not exist yet if the ES was created after the outage started.

3. **Do not rotate the machine identity credentials in Infisical** without simultaneously updating the k8s Secret — this is a self-inflicted auth failure (scenario 1d).

### RefreshInterval behavior during outage

ESO polls each ExternalSecret on its `refreshInterval` (currently `1h` for all ESes in this cluster). If a poll fails:
- ESO logs the error and retries on the next interval
- The existing k8s Secret value is **not modified** on failure
- `lastSyncTime` in the ES status will stop advancing

You can observe drift: `kubectl get externalsecret -A -o wide` shows `LAST SYNC` timestamps that fall increasingly behind wall clock.

### Estimating risk during extended outage

| Time since last sync | Risk                                              |
|----------------------|---------------------------------------------------|
| 0–1h                 | No visible impact                                 |
| 1–24h                | ESO logs errors; existing Secrets valid; safe     |
| 24h+                 | Watch for any certs or tokens with short TTL that rotate via Infisical |
| Any time             | New pod deploys requiring a never-synced Secret fail immediately |

---

## Section 4 — Recovery Procedures

### 4a. Restart ESO Controller

If ESO pods are crash-looping or unresponsive:

```bash
# ⚠️ MUTATING — restart ESO
kubectl rollout restart deployment/external-secrets -n external-secrets
kubectl rollout restart deployment/external-secrets-webhook -n external-secrets
kubectl rollout restart deployment/external-secrets-cert-controller -n external-secrets

# Verify recovery
kubectl rollout status deployment/external-secrets -n external-secrets
kubectl get pods -n external-secrets
```

If ESO was in a bad state for an extended period, force a resync after recovery (Section 4d).

---

### 4b. Restore the Machine-Identity Secret

If `infisical-machine-identity-credentials` is missing or corrupted:

The `clientId` and `clientSecret` values live in Infisical (project `homelab-jz5w`, env `prod`) AND must be retrieved from there (or from a secure offline backup). They are NOT in this document.

**To restore the secret — never print the values to a terminal:**

```bash
# Pattern: fetch via infisical CLI + inject directly into kubectl stdin
# DO NOT: echo $SECRET or print to log

# Retrieve and apply without touching the value in any shell variable visible to history:
infisical secrets get CLIENT_ID \
  --projectId f49c5b01-4bd1-4883-afbd-51c1fef53a2f \
  --env prod \
  --path /eso \
  --plain 2>/dev/null \
  | kubectl create secret generic infisical-machine-identity-credentials \
      --namespace external-secrets \
      --from-literal=clientId="$(cat -)" \
      --dry-run=client -o yaml | kubectl apply -f -
```

> **Note:** The exact Infisical path for the ESO machine identity credentials must be confirmed from the Infisical UI (`https://infisical.petersimmons.com`) before running the above. The `/eso` path is an example — verify the actual path. ⚠️ TODO: Document the exact Infisical path for this secret.

After restoring the secret:
```bash
# ⚠️ MUTATING — restart ESO to pick up the new secret
kubectl rollout restart deployment/external-secrets -n external-secrets

# Verify ClusterSecretStores recover
kubectl get clustersecretstore -A
```

---

### 4c. Restore Infisical (App or DB Recovery)

#### App crash-loop (bootstrap-secret intact, DB reachable)

```bash
# ⚠️ MUTATING
kubectl rollout restart deployment/infisical -n infisical

# Watch recovery
kubectl rollout status deployment/infisical -n infisical
kubectl logs -n infisical -l app=infisical --tail=50
```

#### DB recovery from backup

Infisical's PostgreSQL lives on TrueNAS (`trunas.petersimmons.com`) as an ExternalName service. The DB is named `infisical`.

**Backup location:** ⚠️ **UNVERIFIED — see Section 7 (Gaps/TODO).** TrueNAS ZFS snapshots may cover the postgres dataset. No automated logical (pg_dump) backup job for the Infisical DB has been confirmed in this cluster. This is the single highest-risk gap in this runbook.

**Assumed recovery path (verify against actual TrueNAS config before relying on this):**

1. SSH to TrueNAS: `ssh trunas.petersimmons.com`
2. Identify the postgres dataset: `zfs list | grep postgres`
3. List recent snapshots: `zfs list -t snapshot | grep infisical`
4. If a snapshot exists and data corruption is suspected, roll back: `zfs rollback <snapshot>` (⚠️ MUTATING — data loss for changes after snapshot)
5. Verify postgres is responding after rollback: `pg_isready -h localhost -p 5432`
6. Restart Infisical app pods to reconnect: `kubectl rollout restart deployment/infisical -n infisical`

**Critical note:** The `infisical-bootstrap-secret` k8s Secret (in `ns:infisical`) holds `ENCRYPTION_KEY` and `JWT_*_SECRET` values that were set when Infisical was first deployed. These keys encrypt all secrets stored in the Infisical database. **If this k8s Secret is lost and the DB is restored from backup, Infisical cannot decrypt its own data.** This secret must be backed up separately from the cluster. ⚠️ See Section 7.

---

### 4d. Resync ExternalSecrets After Recovery

After Infisical and ESO are healthy, ESO will resync automatically within 1h. To force immediate resync:

```bash
# Annotate an individual ExternalSecret to trigger immediate resync
kubectl annotate externalsecret <name> -n <ns> \
  force-sync=$(date +%s) --overwrite

# To resync ALL ExternalSecrets (careful — this is a broad operation):
kubectl get externalsecret -A -o json | \
  jq -r '.items[] | "\(.metadata.namespace) \(.metadata.name)"' | \
  while read ns name; do
    kubectl annotate externalsecret "$name" -n "$ns" \
      force-sync=$(date +%s) --overwrite
  done

# Verify sync recovers
kubectl get externalsecret -A
```

---

## Section 5 — Break-Glass Secret Access

When automation is down and you need a specific secret value directly:

### Via Infisical UI

Navigate to `https://infisical.petersimmons.com` → project `homelab-jz5w` → env `prod`. Copy the value using the UI's masked field — it is never written to logs.

### Via Infisical CLI on leviathan

The `infisical` CLI v0.43.69 is installed at `/home/psimmons/bin/infisical` and is authenticated on leviathan.

**Command-substitution pattern (never prints the value):**

```bash
# Retrieve and pass directly into the consuming command via process substitution.
# The value never touches a shell variable, command history, or stdout.

# Example: inject a DB password into psql without it appearing in ps or history
PGPASSWORD=$(infisical secrets get POSTGRES_PASSWORD \
  --projectId f49c5b01-4bd1-4883-afbd-51c1fef53a2f \
  --env prod \
  --path /clearwatch \
  --plain 2>/dev/null) \
  psql -h <host> -U <user> -d <db>

# NEVER do: echo $(infisical secrets get ...)
# NEVER do: export MY_SECRET=$(infisical secrets get ...) then kubectl create secret ... $MY_SECRET
```

Reference implementations of this pattern:
- `~/projects/supabase-homelab/fetch-secrets.sh` (TrueNAS pre-init pattern)
- `~/bin/engram-sync-key.sh` (engram API key rotation pattern)

### Via Infisical DB directly (last resort)

If both the Infisical app and CLI are unavailable, secrets can be read from the PostgreSQL DB on TrueNAS. All secret values are **encrypted at rest** using `ENCRYPTION_KEY` from `infisical-bootstrap-secret`. Direct DB access yields ciphertext, not plaintext. This path is only useful for metadata inspection, not for retrieving usable credentials.

```bash
# Read-only DB inspection (never print decrypted values)
psql -h trunas.petersimmons.com -U <postgres-user> -d infisical \
  -c "SELECT workspace_id, environment, secret_key FROM secrets LIMIT 10;"
# Returns key names only (values are ciphertext blobs)
```

---

## Section 6 — Bootstrap-Exception Inventory

These secrets MUST NOT depend on Infisical for injection. If they did, Infisical itself could not start (circular dependency), or the recovery path would be blocked.

| Secret | Location | Reason |
|--------|----------|--------|
| `infisical-bootstrap-secret` (ns: `infisical`) | Raw k8s Secret, manually applied at install time | Holds Infisical's own `ENCRYPTION_KEY`, `JWT_*`, `POSTGRES_PASSWORD`, `REDIS_PASSWORD` — Infisical cannot start without these, so they cannot come from Infisical |
| `infisical-machine-identity-credentials` (ns: `external-secrets`) | Raw k8s Secret | ESO uses this to authenticate TO Infisical — it cannot be sourced FROM Infisical (circular) |
| Claude Code MCP OAuth / engram API key | `~/.claude.json` on leviathan (managed by `~/bin/engram-sync-key.sh`) | Non-k8s context; the sync script itself depends on leviathan-local auth, not ESO |
| AWS SSO / OIDC credentials (if applicable) | Out-of-band (AWS SSO device flow or local config) | Must survive k8s cluster loss; cannot be stored in a cluster secret |

**Implication for DR:** When rebuilding a cluster from scratch, `infisical-bootstrap-secret` and `infisical-machine-identity-credentials` must be applied manually (from a secure offline source) BEFORE any other workload can receive secrets via ESO. All other secrets will self-populate via ESO once Infisical is healthy.

---

## Section 7 — Gaps / TODO (Unverified Items)

These items could not be confirmed from the cluster state at the time of writing. Each is a real risk that must be addressed before this runbook can be considered field-validated.

### GAP-1 (HIGH): Infisical DB backup — existence and test status UNKNOWN

**Risk:** TrueNAS hosts the Infisical PostgreSQL database. No automated `pg_dump` or logical backup job for the `infisical` database was found in this cluster (only `clearwatch-research` has a confirmed backup CronJob). TrueNAS ZFS snapshots may cover the underlying dataset, but this has NOT been verified. If the DB is lost and no verified backup exists, all secrets stored in Infisical are unrecoverable (they are encrypted with `ENCRYPTION_KEY` which is in `infisical-bootstrap-secret` — but without the DB, there are no ciphertexts to decrypt).

**Action required:**
- [ ] Confirm whether TrueNAS has ZFS snapshots on the postgres dataset containing the `infisical` DB
- [ ] If snapshots exist: verify the snapshot schedule, retention, and test a restore
- [ ] Add a logical `pg_dump` backup CronJob for `infisical` DB (similar to `clearwatch-research` pattern at `clearwatch-research/research-postgres-backup`)
- [ ] Test: drop and restore the `infisical` DB from backup, verify Infisical starts cleanly

### GAP-2 (HIGH): `infisical-bootstrap-secret` offline backup — unconfirmed

**Risk:** If the k8s etcd is lost (cluster rebuild scenario), `infisical-bootstrap-secret` is gone. Without `ENCRYPTION_KEY`, the Infisical DB backup contains only unusable ciphertext. No offline backup location for this secret was confirmed.

**Action required:**
- [ ] Confirm whether `infisical-bootstrap-secret` values are stored in a secure offline location (password manager, Bitwarden, physical safe)
- [ ] If not: export the key names (not values) and establish a process to back them up securely
- [ ] Document the exact offline backup location in a physical/out-of-band document (NOT in this runbook)

### GAP-3 (MEDIUM): Exact Infisical path for ESO machine-identity credentials

The path within Infisical where `clientId`/`clientSecret` for the ESO machine identity are stored was not confirmed during runbook authoring. Section 4b uses a placeholder path `/eso`.

**Action required:**
- [ ] Confirm the Infisical project/env/path for the ESO machine identity credentials
- [ ] Update Section 4b with the exact path

### GAP-4 (MEDIUM): Machine identity token expiry policy unknown

It is unknown whether the Universal Auth machine identity has an expiry configured or if token rotation is manual/automatic.

**Action required:**
- [ ] Check Infisical UI → Machine Identities → review expiry and rotation policy
- [ ] If there is an expiry: create an alert (Alertmanager rule or cron) before expiry to rotate proactively

### GAP-5 (LOW): No GitOps / deploy automation

Deploy is manual `kubectl apply`. During an outage, no automated re-deploy mechanism restores services. This means human operator action is required for every recovery step. Red-team note (Hermes): the absence of GitOps is an exacerbating factor — recovery depends entirely on the operator having leviathan access, cluster access, and this runbook.

**Action required:**
- [ ] Evaluate ArgoCD or Flux for at least the Infisical and ESO namespaces
- [ ] At minimum: ensure all manifests are committed to git so `kubectl apply -f <dir>` is a known-good recovery path

### GAP-6 (LOW): `infisical-linkedin-ciso-tracker` ClusterSecretStore in InvalidProviderConfig

One store (`infisical-linkedin-ciso-tracker`) currently shows `InvalidProviderConfig / Ready: False`. The corresponding ExternalSecret `ciso-tracker-linkedin-token` is in `SecretSyncedError`. This is a pre-existing fault unrelated to an outage but should be resolved to reduce noise in triage output (makes it harder to spot new failures).

**Action required:**
- [ ] Investigate and fix the `infisical-linkedin-ciso-tracker` store configuration
- [ ] Or formally decommission this store if the workload is retired

---

## Quick-Reference Card

```
DETECT:   kubectl get pods -n external-secrets -n infisical
          kubectl get clustersecretstore -A
          kubectl get externalsecret -A | grep -v SecretSynced
          curl -s https://infisical.petersimmons.com/api/status

REMEMBER: Running pods are SAFE. Danger is: new deploys + new ExternalSecrets.
          DO NOT delete ExternalSecrets during outage (deletes owned k8s Secret).

RECOVER:  ESO down      → kubectl rollout restart deployment/external-secrets -n external-secrets
          Auth failure  → restore infisical-machine-identity-credentials + restart ESO
          Infisical down → kubectl rollout restart deployment/infisical -n infisical
          DB down       → TrueNAS recovery (see Section 4c) — VERIFY BACKUP EXISTS FIRST
          Resync all    → annotate force-sync on ExternalSecrets (Section 4d)

BREAK-GLASS: infisical secrets get <KEY> --plain ... | <consuming command>
             Never: echo / print / export the value
```
