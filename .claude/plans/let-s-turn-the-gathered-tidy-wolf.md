# Plan: Homelab Credential Risk Reduction (via Infisical/ESO)

**Status:** FINAL (amended 2026-06-15 post-prep) — socialized, forks resolved, backup-first gate added. Ready to execute.
**Owner:** Claude (orchestrator) · Founder: Peter Simmons
**Date:** 2026-06-15
**Reframe (adopted):** This is a **blast-radius-reduction project**, not "centralize
everything." Infisical/ESO is a tool used where it reduces risk, sequenced so the
control plane is trustworthy, least-privileged, **and recoverable** *before* crown jewels move.

---

## Context

A credential-sprawl audit (2026-06-15) found ~30+ raw Kubernetes Opaque secrets,
~10 project `.env` files, and ~10 dotfile credential stores holding live secrets
outside Infisical — including high-value keys in plaintext (Cloudflare **global** key,
Stripe keys, Anthropic key) and stale rotated secrets in backup files.

This is **not greenfield**. The homelab already runs **ESO v2.0.1** (26
ClusterSecretStores + 28 ExternalSecrets, 24/26 healthy) syncing from self-hosted
**Infisical** (project `homelab-jz5w`, ID `f49c5b01-4bd1-4883-afbd-51c1fef53a2f`,
env `prod`), with a tiered migration staged at `~/projects/kubernetes/infisical-migration/`.
A break-glass recovery runbook now exists (DRAFT): `docs/infisical-break-glass-recovery.md`.

## Goal / Intended Outcome

Reduce the blast radius of a credential or host compromise: high-value keys are
least-privileged and rotated; the secret-control plane (ESO + Infisical + machine
identities) is stable, scoped, **and provably recoverable**; remaining secrets are
centralized **by trust domain** behind verification gates; stale/exposed secrets are
expunged. Neatness is a side effect, not the objective.

---

## Founder Decisions (locked)
1. **Project shape → Blast-radius reduction.** Stabilize + reduce privilege BEFORE centralizing crown jewels.
2. **Identity scope → Per-domain, path-scoped machine identities.** Retire the single broad Viewer "skeleton key."
3. **Git history → Rotate-only.** Rewrite only if a *still-valid* secret was broadly exposed in a shared repo.
4. **Execution vehicle → Interactive Claude specialists with a founder gate at each phase boundary.** Codex generates the per-item test playbook.
5. **Backup-first (added) → Verifying Infisical's own DB + encryption-key backup and a tested restore is the FIRST blocking gate.** Do not centralize more secrets into a store we cannot prove we can recover. (Concretizes the break-glass GAP-1/GAP-2 below.)

---

## Current-State Reference (proven patterns to REUSE — do not reinvent)

- **k8s ClusterSecretStore template:** `~/projects/kubernetes/infisical/external-secrets/clustersecretstore.yaml`
  - Universal Auth via `infisical-machine-identity-credentials` (ns `external-secrets`, clientId `faaaf48d-...`, Viewer role).
  - `hostAPI: http://infisical.infisical.svc.cluster.local:80/api` (internal DNS only — public host does NOT resolve from ESO pods).
  - `projectSlug: homelab-jz5w`, env `prod`. **One store per Infisical path.**
- **ExternalSecret templates:** single-key `infisical-migration/tier1-cert-manager/external-secret.yaml`; multi-key `infisical-migration/tier1-n8n/external-secret.yaml` (`refreshInterval: 1h`, `creationPolicy: Owner`).
- **Deploy:** manual `kubectl apply` (NO Argo/Flux).
- **Non-k8s reference impls:** boot-export `~/projects/supabase-homelab/fetch-secrets.sh`; file-sync cron `~/bin/engram-sync-key.sh`; machine-identity REST `~/projects/telemetry/deploy/systemd/run-shipper.sh`; wrapper `~/bin/infisical-homelab`. **No `infisical run --` wrapper exists yet.**

---

## Socialization (three-way-plan) — captured, divergence preserved

### Codex (`gpt-5.3-codex-spark`) — implementer's eye. **Accepts goal; hardens execution.**
Folded in: gate before raw-delete (`SecretSynced` + consumer smoke + evidence); double-write window for
high-value rotation; temporary `refreshInterval: 5m` during cutover (restore 1h); atomic file writes +
checksum + restart hook; canary ExternalSecret per store; rollback drill on a pilot; consumer-inventory
verification. Scale traps: store-config drift, Viewer false-success on path scoping, manual-apply drift,
`creationPolicy: Owner` cleanup hazard, format drift, boot-order + offline-Infisical.
*(Per-phase test playbook: requested as a Phase 0 deliverable.)*

### Hermes (contrarian) — **challenges the premise; objection accepted, plan reshaped.**
"Centralization ≠ hardening": one Infisical + one identity + one ESO layer = correlated systemic failure.
"Viewer" is false comfort — read access to prod *is* the privilege. Crown-jewels-first is backwards on an
unstable plane. Reduce privilege before centralizing. Split by trust domain. Git rewrite mostly theater if
you rotate. File-cache pattern is suspect. **Infisical's own DB (the foundation stone) needs extraordinary
treatment** — now the backup-first gate (Decision #5).

### Divergence (named) + break-glass finding
Codex: "do it — safely." Hermes: "do a smaller, better-sequenced thing." Synthesis (founder-ratified):
Hermes on shape/sequencing + Codex on execution rigor. **Break-glass doc surfaced GAP-1 (Infisical DB
backup unverified — no pg_dump CronJob found, ZFS coverage unconfirmed) and GAP-2 (ENCRYPTION_KEY backup
unconfirmed). These are now the first blocking work.**

---

## Converged Design

### Phase 0 — Stabilize & make recoverable (prerequisite for everything)
- **0a (BLOCKING, FIRST): Infisical backup/restore.** Verify or establish: (i) a tested Infisical Postgres
  backup (pg_dump CronJob and/or verified ZFS snapshot of the DB volume), (ii) an offline backup of the
  `infisical-bootstrap-secret` ENCRYPTION_KEY, (iii) a documented + once-tested restore. **No further
  centralization until 0a is GREEN.**
- 0b: fix the 2 broken ESO stores (`infisical-linkedin-ciso-tracker`, invalid path `/default/ciso-tracker-auto`).
- 0c: reconcile `/ai-fleet` vs `/aifleet` + relocate the stray `AIFLEET_API_TOKEN`.
- 0d: delete stale backups (`engram-go/.env.pre-*`).
- 0e: finalize the break-glass runbook (close its gaps), and land the Codex per-item test playbook.

### Phase 1 — Reduce privilege (precedes centralizing crown jewels)
Replace the Cloudflare **global** key with scoped API token(s) + rotate; right-size other overprivileged
keys; **split the single Viewer machine identity into per-trust-domain, path-scoped ESO identities**
(cluster-runtime / billing / tooling / admin-control-plane).

### Phase 2 — Centralize by trust domain, behind gates
Migrate remaining secrets domain-by-domain via ESO (k8s) or the converged non-k8s pattern, each behind
**add → wire → verify → remove-raw**. Double-write for high-value; `refreshInterval: 5m` during cutover then `1h`.

### Phase 3 — Hygiene tail + standing controls
Verify-then-delete duplicate local copies; **rotate-only** for exposure; recurring negative-source scanner + "no new plaintext" control.

### Non-k8s pattern (converge, but not blindly)
Default `infisical run -- <cmd>` / boot-time export (atomic write) for services/compose/systemd. Flat-file
tools: file-cache **only** with atomic write + checksum + restart hook; per-tool broker where unsafe.

### Path taxonomy
`/apps/<app>`, `/databases/<db>/apps/<app>`, `/infra/<tool>`. Reconcile `/ai-fleet` vs `/aifleet` in Phase 0c.

### Bootstrap exceptions (stay raw — extraordinary treatment)
ESO machine-identity creds; **Infisical's own DB/Redis + ENCRYPTION_KEY (foundation stone — see Phase 0a)**;
Claude Code MCP OAuth; AWS SSO. Immutable-tag + human-approval allowlist.

---

## Verification (end-to-end)
- **Phase 0a:** a restore of the Infisical DB into a scratch instance succeeds and secrets decrypt with the backed-up ENCRYPTION_KEY.
- Per k8s secret: canary first; `SecretSynced: True`; consuming pod restarts clean; consumer-inventory matches.
- Per non-k8s secret: consumer starts + authenticates; atomic-write checksum verified; SHA-256 == prior value where lift-and-shift.
- **Rollback drill** on one pilot app before any crown-jewel cutover.
- No regressions: `~/bin/health-check.sh` green; ESO sync SLO tracked during cutover.
- Hygiene: stale backups gone; broken stores `Ready: True`; taxonomy de-duplicated.
- Standing: recurring negative-source scan finds only sanctioned bootstrap exceptions outside Infisical.

## Risks / Failure Modes (register new classes via `~/bin/add-failure-mode.sh`)
- **Unrecoverable foundation stone (GAP-1/2):** Infisical DB/key loss = total secret loss. → Phase 0a, BLOCKING.
- **Correlated failure domain (Hermes):** outage → fleet-wide. → break-glass doc; per-domain identities.
- **Skeleton-key identity (Hermes):** → per-domain path-scoped identities (Phase 1).
- **Raw-delete-too-early (Codex):** → hard gate.
- **Crown-jewels onto unstable plane (Hermes):** → Phase 0 first.
- **Internal-DNS assumption / file-cache races / history exposure:** → per mitigations above.

---

## First Concrete Steps (executing)
1. **Phase 0a (FIRST):** dispatch a specialist to audit Infisical's backup posture — is there a pg_dump CronJob / verified ZFS snapshot of the DB, and an offline ENCRYPTION_KEY backup? If missing, establish them and run one tested restore into a scratch target. **Gate everything else on this.**
2. In parallel (non-blocking): land the Codex per-item test playbook; close the break-glass doc gaps.
3. 0b–0d (after 0a green): fix broken stores; reconcile `/ai-fleet`/`/aifleet` + relocate token; delete stale backups.
4. Founder gate at Phase 0 → Phase 1 boundary.
