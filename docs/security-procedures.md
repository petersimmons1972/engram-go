> **Status: DRAFT — for founder review. Do NOT treat as policy until signed off.**
> Last updated: 2026-05-30 · Author: Claude (Sonnet 4.6) · Review state: Pending founder sign-off

---

## Open Questions for Founder

1. **DB password rotation timeline.** VERIFIED 2026-05-30: The Engram DB credential (PGPASSWORD exposed during PR #1 QA) was already rotated on 2026-05-19 (both TruNAS and source-DB instances). Tracking issue petersimmons1972/engram-go#743 is CLOSED with full rotation confirmation. No open security/rotation-deferred issue exists. The "Outstanding as of 2026-05-30" banners in §2.2 and §5.4 are stale and are removed in this revision.
2. **Infisical project structure.** VERIFIED 2026-05-30: Both Infisical MCP tools were queried read-only; infisical-personal returned HTTP 422, infisical-agentgateway returned an empty project list (machine identity has no project access). Assumed names (engram, homelab, clearwatch) could not be confirmed. FOUNDER ACTION: confirm via the Infisical web UI at https://infisical.petersimmons.com or fix agentgateway identity scopes.
3. **Branch-protection authority.** VERIFIED 2026-05-30: (i) engram-go (public): protection configured but enforce_admins=false (admins bypass all rules — the §3.1 incident failure mode); required check "Test & Coverage"; no required PR review. (ii) olla (public): main completely unprotected. (iii) aifleet & claude-codex (private): protection API needs GitHub Pro — verify in UI. (iv) Session token scopes show no admin:org; agent/Codex PAT scopes need a founder audit.
4. **Incident log location.** FOUNDER QUESTION: confirm GitHub Issues + security/incident label is the canonical incident record (vs separate private repo / Notion).
5. **Rotation cadence.** FOUNDER QUESTION: confirm the assumed cadences (90-day DB/PATs, 180-day API keys/Infisical tokens, 365-day SSH keys) or adjust.

---

# Security Procedures

This document covers secrets handling, credential rotation, branch protection, agent execution safety, and incident logging for the homelab + Clearwatch + agent-fleet environment.

**Scope:** `clearwatch`, `homelab`, `engram`, and supporting repos under `petersimmons1972`. Not applicable to the deprecated SIB repo.

---

## Table of Contents

1. [Secrets Handling](#1-secrets-handling)
2. [Credential Rotation](#2-credential-rotation)
3. [Branch Protection](#3-branch-protection)
4. [Agent Execution Safety](#4-agent-execution-safety)
5. [Incident Logging](#5-incident-logging)
6. [Quick-Reference Checklists](#6-quick-reference-checklists)

---

## 1. Secrets Handling

### 1.1 Infisical Is the Source of Truth

All secrets — database passwords, API keys, service tokens, GitHub PATs — MUST live in Infisical at `https://infisical.petersimmons.com`. No other secrets store is authoritative.

| Rule | What it means |
|------|---------------|
| No secret in a command line | `psql postgresql://user:PASSWORD@host` is forbidden in any context (shell history, pod logs, CI logs, GitHub issue bodies, commit messages). |
| No secret in code | Secrets MUST NOT appear as string literals, even in test files. |
| No secret in a CI log | Any step that touches a secret must suppress output (`--quiet`, log masking, or env-var injection — never echo the value). |
| No secret in an issue or PR | GitHub Issues and PRs are semi-public by default. Reference the Infisical key path, never the value. |

**Motivation (incident 1):** Diagnostic agents ran inline `psql postgresql://engram:<password>@host` commands inside ephemeral k8s pods. The password was captured verbatim in k8s pod logs and was readable by any principal with `kubectl logs` access to that namespace.

### 1.2 Connecting to PostgreSQL Without Inline Credentials

Never pass a password in a connection string or as a CLI argument. Use one of:

**Option A — `~/.pgpass` (interactive / developer workstation)**

```
# ~/.pgpass — chmod 600
hostname:port:database:username:password
```

`psql -h hostname -U username database` — libpq reads `.pgpass` automatically; password never appears in process arguments or shell history.

**Option B — Environment variable (agent pods, CI)**

```yaml
# In the pod spec / k8s Secret
env:
  - name: PGPASSWORD
    valueFrom:
      secretKeyRef:
        name: engram-db-credentials
        key: password
```

Then invoke: `psql -h hostname -U username database` — `PGPASSWORD` is consumed by libpq; it does not appear in `kubectl logs`.

**Option C — Infisical CLI injection (preferred for agent pods)**

```bash
infisical run --projectId <id> --env prod -- psql -h hostname -U username database
```

The Infisical sidecar or init-container injects secrets as environment variables. The pod spec never stores the value; rotation in Infisical propagates on the next pod restart.

ASSUMPTION: The cluster has the Infisical operator or a compatible secrets injection mechanism installed. If not, Option B (k8s Secret) is the minimum viable alternative until the operator is deployed.

### 1.3 Agents Fetching Secrets at Runtime

- Agents MUST use the `mcp__infisical-agentgateway__get-secret` or `mcp__infisical-personal__get-secret` MCP tools to retrieve secrets at task start.
- Agents MUST NOT store retrieved secret values in memory files, Engram, GitHub Issues, or any persistent store.
- Agents MUST NOT log secret values. When a secret must be verified (e.g., "is this token valid?"), use an API call that returns a non-secret response (e.g., a `whoami` endpoint) rather than printing the token.
- ASSUMPTION: Both `infisical-agentgateway` and `infisical-personal` MCP servers are available. The gateway variant is preferred for agent dispatches; the personal variant is for interactive coordinator sessions only.

### 1.4 SAST Enforcement

Per QC.1, the PostToolUse Semgrep hook and pre-commit SAST stack fail closed on any finding that matches secret-detection rules. This is the last line of defense — the rules above are the first. Do not rely on SAST to catch what discipline should prevent.

---

## 2. Credential Rotation

### 2.1 Rotation Procedure

When a credential must be rotated (scheduled, post-incident, or post-exposure):

1. **Generate new credential** in the target system (DB, API provider, etc.).
2. **Update in Infisical** (`mcp__infisical-personal__update-secret` or web UI) before deploying the new value anywhere.
3. **Roll pods / services** that consume the secret (restart deployments, not just pods, so k8s re-fetches the Secret).
4. **Verify connectivity** with the new credential before revoking the old one.
5. **Revoke the old credential** in the target system.
6. **Close the rotation GitHub Issue** (see §2.2) with a comment confirming steps 1–5 are complete and the old credential is revoked.
7. **Store a rotation event in Engram** (`memory_store`, type: `decision`, project: `homelab`, content: "Rotated <credential-name> on <date>. Reason: <reason>.").

### 2.2 Tracked-Deferral Mechanism

If a rotation cannot happen immediately, it MUST be tracked so it is not forgotten.

**When deferral is declared:**

```bash
gh issue create \
  --repo petersimmons1972/<repo> \
  --title "SECURITY: Rotate <credential-name> — deferred" \
  --label "security/rotation-deferred,priority/high" \
  --body "Credential: <name>
Infisical path: <project>/<env>/<key>
Exposed in: <incident description>
Deferred because: <reason>
Target rotation date: <date>
Rotation procedure: See docs/security-procedures.md §2.1"
```

**Rules:**
- A deferred rotation issue blocks the next scheduled security review.
- ASSUMPTION: Security reviews happen quarterly. The review checklist (§6) includes "no open `security/rotation-deferred` issues."
- Deferred rotations MUST have a target date in the issue body. An undated deferral is not a deferral — it is a forgotten credential.

### 2.3 Rotation Schedule (Default Cadence)

ASSUMPTION: These cadences reflect a reasonable homelab risk posture. Adjust to match actual risk tolerance.

| Credential type                        | Rotation cadence | Trigger for immediate rotation                              |
|----------------------------------------|-----------------|-------------------------------------------------------------|
| Database passwords                     | 90 days         | Exposure in logs, agent logs, issues, or commits            |
| GitHub PATs (agent service accounts)   | 90 days         | Any unauthorized use or unexpected scope                    |
| API keys (Anthropic, Infisical, etc.)  | 180 days        | Exposure or suspected compromise                            |
| Infisical machine identity tokens      | 180 days        | Pod or host compromise                                      |
| SSH keys (homelab nodes)               | 365 days        | Node decommission or compromise                             |

---

## 3. Branch Protection

### 3.1 Required Configuration for Shared Repos

Every repo under `petersimmons1972` that receives agent commits or human PRs MUST have branch protection on `main` configured as follows:

| Setting                                              | Required value            | Why                                                              |
|------------------------------------------------------|--------------------------|------------------------------------------------------------------|
| Require a pull request before merging                | ✅ Enabled               | Prevents direct pushes to main                                   |
| Required approving reviews                           | ≥ 1 (founder)            | Coordinator/founder reviews all agent work                       |
| Require status checks to pass before merging         | ✅ Enabled               | Core of the check-blocking requirement                           |
| **Require branches to be up to date before merging** | ✅ Enabled               | Prevents race-condition merges that bypass CI                    |
| Require status checks — specific checks              | See §3.2                 | The check must be listed by exact name                           |
| Do not allow bypassing the above settings            | ✅ Enabled               | Admins MUST NOT be exempt                                        |
| Restrict who can push to matching branches           | ✅ Enabled — founder only | No agent service account may push to main                        |

**Motivation (incident 2):** An engram-go push landed past a required "Test & Coverage" status check. Investigation showed the check was listed as required but the merge was not blocked. This is the "configured but not enforced" failure mode.

### 3.2 What "Required Check" Must Mean

A status check is only meaningfully required if ALL of the following are true:

1. **The check name in branch protection matches the exact name reported by CI.** If CI reports `test-and-coverage` but the protection rule lists `Test & Coverage`, the rule does not match and does not block.
2. **The check is produced by the branch's CI run, not inherited from a prior run.** GitHub can show a green check from a previous commit if the branch was not required to be up to date. Rule: "Require branches to be up to date" (§3.1) MUST be enabled.
3. **The check is not bypassed by the merge actor.** Admin bypass must be disabled (§3.1). Agent service accounts must not have admin role.
4. **The check actually runs on PRs targeting main.** Verify by opening a test PR and confirming the named check appears in the "Checks" tab as pending, then passes or fails.

### 3.3 Verifying Protection Is Actually Enforced

After configuring or modifying branch protection:

```bash
# Step 1: Confirm protection settings via API
gh api repos/petersimmons1972/<repo>/branches/main/protection \
  --jq '{
    required_pr: .required_pull_request_reviews.required_approving_review_count,
    required_checks: [.required_status_checks.contexts[]],
    enforce_admins: .enforce_admins.enabled,
    up_to_date: .required_status_checks.strict
  }'

# Step 2: Confirm check names match CI output
# Open a test PR, let CI run, then:
gh pr checks <pr-number>
# Compare the "name" column output against the names in required_checks above.
# If any required check name does not appear in `gh pr checks` output, the rule is inert.

# Step 3: Attempt a direct push to main — it must be rejected
git push origin HEAD:main
# Expected: "remote: error: GH006: Protected branch update failed"
# If it succeeds, admin bypass is not disabled — fix immediately.
```

Run this verification after every branch-protection change.

### 3.4 Publish-Boundary Rule (AP.11)

This rule governs who may push and when:

| Actor                                     | May do                                   | May NOT do                                              |
|-------------------------------------------|------------------------------------------|--------------------------------------------------------|
| Implementer agent (Codex, worktree agent) | `git add`, `git commit`                  | `git push` to any remote                               |
| Coordinator (Claude, session)             | `git push` to feature branches           | `git push` to `main` without explicit founder approval |
| Founder                                   | `git push` to `main` after review        | Push without reviewing `git diff --staged`             |

**Rationale:** Agents operate in ephemeral context. A coordinator push to main before founder review removes the last human checkpoint. The publish boundary is the only guaranteed human gate.

**Enforcement:** No agent brief may include `git push`. Briefs may include `git add` and `git commit` only. The coordinator surfaces the diff and waits for founder confirmation ("go", "yes", "merge it") before pushing.

---

## 4. Agent Execution Safety

### 4.1 Read-Only Validator Guard

When dispatching a validator or reviewer agent (Spruance, Rickover-validator, or any agent briefed as read-only):

```bash
# Before dispatching:
touch ~/.claude/.validator-bash-guard

# After the validation session ends:
rm ~/.claude/.validator-bash-guard
```

The guard file enables the read-only Bash enforcement hook. Without it, a validator agent can accidentally mutate state. This is not optional.

### 4.2 No Secrets in Ephemeral Pod Logs

- Agent pods MUST NOT run commands that include secrets in positional arguments. See §1.2.
- Agent pods MUST NOT `echo`, `print`, or `cat` secret values for debugging. Use a redacted format: `echo "PGPASSWORD is set: $([ -n "$PGPASSWORD" ] && echo YES || echo NO)"`.
- ASSUMPTION: The cluster has log aggregation (e.g., Loki or similar). Any secret that appears in a log line is potentially captured by log aggregation and retained beyond the pod's lifetime. Treat pod logs as persistent.
- After any incident where a secret appeared in logs: rotate the credential (§2.1), then audit the log aggregation system to confirm the captured value is purged or access-controlled.

### 4.3 Least-Privilege Agent Permissions

| Principle                                                                     | Implementation                                                                       |
|-------------------------------------------------------------------------------|--------------------------------------------------------------------------------------|
| Agents receive only the secrets they need for their specific task             | Use per-task Infisical machine identity tokens scoped to the minimum set of secrets  |
| Agent k8s service accounts have no cluster-admin                              | ASSUMPTION: Agent pods use a dedicated ServiceAccount with RBAC limited to their target namespace |
| Agent GitHub tokens are scoped to required repos and permissions only         | No agent token should have `repo:admin` or `delete_repo` scope                       |
| Validators and read-only agents receive no write credentials                  | Brief must explicitly state which MCP tools are permitted; read-only agents receive no Infisical write-capable tokens |

### 4.4 Worktree Isolation for Parallel Agents

Per AP.1: when dispatching 2+ implementer agents against the same git repository, each MUST be spawned with `isolation: "worktree"`. Without this, agents share the same checkout and can observe each other's uncommitted secrets (e.g., a `.env` file written by one agent becoming visible to another before cleanup).

---

## 5. Incident Logging

### 5.1 What Counts as a Security Incident

Any of the following trigger the incident procedure:

- A secret value appears in a log, commit, issue body, PR comment, or any output visible beyond the intended audience.
- A branch protection rule was bypassed or failed to block a merge.
- An agent acquired permissions beyond its stated brief.
- A credential is suspected compromised (unauthorized use, unexpected API calls, etc.).
- A pod or host is suspected compromised.

### 5.2 Incident Procedure

1. **Contain immediately.** Revoke or rotate the affected credential before any other step (see §2.1). If a host is compromised, isolate it from the cluster (NetworkPolicy or node tainting) before investigating.
2. **File a GitHub Issue within 1 hour** using the template below.
3. **Do not discuss credential values in the issue.** Reference Infisical paths only.
4. **Perform root-cause analysis** and document findings in the issue before closing.
5. **Close the issue** only after: (a) the affected credential is rotated, (b) the log/output containing the secret is purged or access-controlled, and (c) the procedure that caused the incident is updated to prevent recurrence.
6. **Store an incident summary in Engram** (`memory_store`, type: `error`, project: `homelab`, content: summary of what happened and what changed).

### 5.3 Incident Issue Template

```markdown
## Security Incident: <short title>

**Date/time detected:** YYYY-MM-DD HH:MM UTC
**Detected by:** <human / agent / automated check>
**Severity:** blocker | high | medium

## What happened
<Factual description. No secret values. Reference Infisical paths if relevant.>

## Affected credentials
- Infisical path: <project>/<env>/<key> — status: rotated / pending rotation

## Containment actions taken
- [ ] Credential revoked or rotated
- [ ] Affected logs purged or access-restricted
- [ ] Affected hosts/pods isolated (if applicable)

## Root cause
<To be filled in after investigation>

## Recurrence prevention
<Procedure change, doc update, or automation that prevents this class of incident>

## References
- Related commits:
- Related issues:
```

Label the issue: `security/incident` + appropriate `severity/*` label.

### 5.4 Known Open Incidents (as of 2026-05-30)

| Incident                                                              | Status               | Action needed                                          |
|-----------------------------------------------------------------------|----------------------|-------------------------------------------------------|
| Engram DB password captured in k8s pod logs (PR #1 QA)               | VERIFIED 2026-05-30: Rotated 2026-05-19, issue #743 closed | None — rotation complete. |
| ENGRAM_API_KEY Bearer token appeared in two local (never-pushed) commits; history soft-reset before push; .claude/mcp_servers.json now gitignored (issue #65) | Contained — not pushed. | Rotate ENGRAM_API_KEY as precaution; add gitleaks pre-commit hook. |

---

## 6. Quick-Reference Checklists

### 6.1 Pre-Commit Security Checklist

- [ ] `git diff --staged` reviewed — no secret values visible
- [ ] No connection strings with inline passwords
- [ ] No `.env` files staged
- [ ] Semgrep hook passed (PostToolUse and pre-commit)

### 6.2 Agent Dispatch Security Checklist

- [ ] Agent brief does not include `git push`
- [ ] Agent brief does not include any secret values — only Infisical paths
- [ ] Parallel implementer agents use `isolation: "worktree"`
- [ ] Validator agents: `~/.claude/.validator-bash-guard` created before dispatch
- [ ] After validation: `~/.claude/.validator-bash-guard` removed

### 6.3 Quarterly Security Review Checklist

- [ ] No open `security/rotation-deferred` issues with past target dates
- [ ] All credentials rotated within their cadence window (§2.3)
- [ ] Branch protection verified on all active repos (§3.3)
- [ ] Semgrep rules updated within the last 90 days
- [ ] Agent service account tokens reviewed for scope creep
- [ ] Log aggregation system audited for retained secret values
- [ ] Infisical machine identity tokens audited — unused tokens revoked

### 6.4 Post-Incident Closure Checklist

- [ ] Affected credential rotated and old value revoked
- [ ] Affected log entries purged or access-controlled
- [ ] Root cause documented in the GitHub Issue
- [ ] Recurrence-prevention action taken (code/procedure/automation)
- [ ] Engram memory updated with incident summary
- [ ] Issue closed with all checklist items confirmed in a comment

---

## Appendix A — Infisical MCP Tool Reference

| Task                                  | Tool                                          |
|---------------------------------------|-----------------------------------------------|
| Retrieve a secret (interactive session) | `mcp__infisical-personal__get-secret`       |
| Retrieve a secret (agent dispatch)    | `mcp__infisical-agentgateway__get-secret`     |
| List secrets in a project             | `mcp__infisical-personal__list-secrets`       |
| Update / rotate a secret value        | `mcp__infisical-personal__update-secret`      |
| Create a new secret                   | `mcp__infisical-personal__create-secret`      |
| Delete a secret                       | `mcp__infisical-personal__delete-secret`      |

Never call `get-secret` and then store or log the returned value beyond the immediate use.

---

## Appendix B — Cross-References

| Topic                                                     | Authoritative source                                      |
|-----------------------------------------------------------|-----------------------------------------------------------|
| Container hardening (Chainguard, nonroot UID, fsGroup)    | `~/docs/container-images.md`, `~/docs/container-hardening.md` |
| K8s network policy (egress firewall)                      | `~/docs/k8s-firewall.md`                                  |
| SAST enforcement details                                  | `~/docs/quality-contract.md` §QC.1                        |
| Memory persistence (Engram)                               | `~/docs/engram-memory-rules.md`                           |
| Agent dispatch protocols                                  | `~/CLAUDE.md` §Parallel Agent Rules                       |
| Publish boundary (AP.11)                                  | `~/CLAUDE.md` §Parallel Agent Rules                       |
| Advisory protocol (ADV.1–ADV.5)                           | `~/docs/advisory-protocol.md`                             |

---

*DRAFT — not policy until founder sign-off. All ASSUMPTION tags require founder confirmation before this document is finalized.*
