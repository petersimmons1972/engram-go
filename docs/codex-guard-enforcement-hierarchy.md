# codex-guard Enforcement Hierarchy

**Status:** Live (Layers 1 + 4 active; Layers 2–3 planned)
**Last updated:** 2026-06-20
**Plan ref:** `~/.claude/plans/codex-guard-remote-fix.md` (Phase 3)

---

## Why this document exists

Three separate incidents had agents run `ssh host 'codex-guard git…'` — the binary
does not exist on remote hosts, so the call fails with `command not found`. This was
treated as a codex-guard enforcement failure. It was not. It was a documentation
failure: the mandate "prefix git with codex-guard" was silently misread as "prefix
the remote command," when the correct form is to prefix the local ssh call.

The deeper problem: the mandate was also being treated as the **primary** enforcement
gate. It is not. It is audit/best-effort (Layer 4). The real hard gates are below.
This document retires that fiction and establishes the correct model.

---

## Enforcement layers (strongest → audit)

### Layer 1 — Claude Code settings deny-list (HARD mechanical gate)

**File:** `~/.claude/settings.json` `permissions.deny`

This is the only **hard mechanical gate** — Claude Code will refuse to run a matched
command regardless of what any agent brief says.

Current deny patterns cover: `git push --force`, `git reset --hard`, `git clean`,
`kubectl delete` on production namespaces, destructive `docker` operations.

**Status: ACTIVE. Must be tested.**
Open work (Phase 1.5): build a red-team matrix confirming each deny pattern fires
across all invocation paths (local git · `codex-guard ssh` · raw ssh · gh · kubectl).
Verify the deny-list sees through quoting variations (`bash -lc`, `&&`-chains,
`base64` pipelines, `git push -f` vs `--force`).

### Layer 2 — Git server hooks: pre-receive / pre-push (HARD at authority boundary)

**Scope:** petersimmons1972 GitHub repos + any self-hosted git servers

Server-side hooks enforce force-push and history-rewrite protection at the point
where the authority boundary actually is: the server. They are unbypassable by
quoting tricks, shell escaping, or any client-side manipulation. They cover both
human operators and automation agents with no per-host binary distribution.

**Status: PLANNED (Phase 2A, not yet implemented)**

Recommended implementation: GitHub branch protection (required status checks, no
admin bypass) + optional pre-receive hooks on self-hosted repos.

### Layer 3 — SSH keys + ForceCommand (host-level policy on automation hosts)

**Scope:** automation-only hosts (codex, grok, opencode nodes)

SSH `ForceCommand` in `~/.ssh/authorized_keys` (or `sshd_config` match blocks)
wraps every incoming SSH session in a command that:
- logs every command string to an audit file
- blocks destructive patterns (configurable blocklist)
- exits non-zero on policy violation before the inner command runs

This enforces policy at the host boundary, not the client. A compromised or
misconfigured agent that bypasses codex-guard locally cannot bypass ForceCommand
on the remote host.

**Status: PLANNED (Phase 2B, not yet implemented)**

### Layer 4 — codex-guard local binary (deterministic blocklist + audit log)

**Binary:** `codex-guard` (Rust, installed on the local Claude Code host only)

codex-guard maintains a local allowlist/blocklist and writes an audit log of every
command it passes or blocks. It is **belt-and-suspenders**, not the primary gate.

Key properties:
- Deterministic: blocklist is static config, not heuristic
- Audit log: every invocation recorded (command, timestamp, pass/block)
- Policy-mandated: `Bash(*)` is wildcard-allowed in settings.json, so codex-guard
  is enforced by mandate, not by settings. This means an agent that omits the prefix
  passes silently — codex-guard cannot catch what it never sees.

**The local-only constraint (the root cause of the 3 incidents):**

codex-guard is installed on the local host only. When an agent writes
`ssh host 'codex-guard git…'`, the remote shell cannot find the binary and the
command fails. The correct form is to guard the ssh invocation locally:

```
# CORRECT — guard runs locally, inspects the inner command
codex-guard ssh codex.petersimmons.com 'git push origin main'

# WRONG — codex-guard is absent on the remote host → command not found
ssh codex.petersimmons.com 'codex-guard git push origin main'
```

**Audit log secret redaction (open work, Phase 1.5):**
Command strings logged by codex-guard must redact tokens, env vars, kube/db
credentials. Any command containing `$TOKEN`, `$KUBECONFIG`, or credential-shaped
strings must be sanitized before the log write.

### Layer 5 — SSH identity (authentication, not authorization)

SSH keys authenticate **who** is connecting. They do not constrain **what** that
identity can do once connected. This is a trust boundary, not a capability boundary.
It is listed here for completeness; it is not a codex-guard enforcement layer.

---

## Correct invocation reference

| Scenario                              | Correct form                                              |
|---------------------------------------|-----------------------------------------------------------|
| Local git commit                      | `codex-guard git commit -m "…"`                          |
| Local kubectl                         | `codex-guard kubectl rollout restart deploy/foo -n ns`   |
| Remote git over ssh                   | `codex-guard ssh host 'git push origin main'`            |
| Remote kubectl over ssh               | `codex-guard ssh host 'kubectl apply -f manifest.yaml'`  |
| Local gh CLI                          | `codex-guard gh pr create …`                             |
| Remote gh (e.g. on codex node)        | `codex-guard ssh codex.petersimmons.com 'gh pr view 42'` |
| WRONG: codex-guard inside remote shell | `ssh host 'codex-guard git push …'` ← NEVER              |

---

## Open Phase 1.5 work

- [ ] Red-team matrix: for each security property × each invocation path, confirm
      which layer actually fires. The incidents proved we don't currently know.
- [ ] Deny-list test harness: automated suite that exercises every deny pattern.
- [ ] Audit-log secret redaction: codex-guard must not log raw credential strings.

---

## Phase 2 work (planned, not committed)

- [ ] 2A. Server-side git pre-receive/pre-push hooks (Layer 2)
- [ ] 2B. SSH ForceCommand on automation hosts (Layer 3)
- [ ] 2C. codex-guard ssh-aware mode: first-class `codex-guard ssh host '<cmd>'`
      that re-inspects the inner command and fails closed on ambiguous destructive
      patterns — making the wrong form syntactically impossible.
