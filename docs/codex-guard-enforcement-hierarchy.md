# codex-guard Enforcement Hierarchy

**Status:** Layer 1 LIVE (PreToolUse hook); Layer 2 LIVE (settings.json deny-list); Layers 3–4 PLANNED
**Last updated:** 2026-06-20
**Prefix mandate:** RETIRED as of PR #135

---

## Why the prefix mandate was retired

The original mandate — "prefix every git/gh/kubectl/helm call with `codex-guard`" —
was intended to interpose the binary on destructive commands. Three problems made it
ineffective:

1. **Token cost with no enforcement value.** With `Bash(*)` wildcard-allowed in
   settings.json, any command that omitted the prefix ran unchecked. The mandate was
   advisory, not mechanical.

2. **Bypassed when forgotten.** An agent that skipped the prefix encountered no error
   and no block — the destructive command ran silently.

3. **Remote-ssh confusion.** Three separate incidents had agents write
   `ssh host 'codex-guard git…'` — the binary does not exist on remote hosts, so
   the call failed with `command not found`. The correct form (`codex-guard ssh host
   '<cmd>'`) was documented but not enforced.

The replacement is a PreToolUse hook that intercepts **every Bash call automatically**
with no per-command effort from agents or orchestrators.

---

## Enforcement layers (strongest → weakest)

### Layer 1 — PreToolUse hook: codex-guard (PRIMARY CLIENT GATE)

**File:** `~/.claude/hooks/codex-guard-bash-guard.sh`
**Registered in:** `~/.claude/settings.json` `hooks.PreToolUse` Bash matcher

Every Bash command passes through the hook before execution. The hook:
- Extracts `.tool_input.command` from the PreToolUse JSON
- Runs `codex-guard --json "<command>"` (assessment-only, does not execute)
- If `allowed == false`: emits `{"decision":"block","reason":"[codex-guard:<code>] <message>"}` to stdout and returns 0
- If `allowed == true`: exits 0 (allow), no output
- **FAIL-OPEN:** if codex-guard is missing, times out, or produces non-JSON output ->
  the command is ALLOWED and the failure is appended to `~/.claude/codex-guard-hook.log`

The hook is **automatic**. Agents write plain `git commit`, `kubectl apply`, etc.
The hook guards them transparently.

codex-guard blocks 6 destructive classes:
- `destructive_git` -- `git push --force`, `git reset --hard`, history rewrites (detected inside `bash -c` / `ssh` payloads)
- `dangerous_rm` -- `rm -rf` targeting absolute or HOME paths outside the ephemeral-build allowlist
- `docker_data_destruction` -- `kubectl delete namespace`, `docker rm -v`, volume destruction
- (additional classes per codex-guard v0.1.0 `--json` schema)

**Status: ACTIVE**

### Layer 2 — settings.json deny-list (MECHANICAL BACKSTOP)

**File:** `~/.claude/settings.json` `permissions.deny`

Claude Code refuses to run a matched command regardless of hook output. This covers
the residual risk that the hook itself is bypassed (e.g. via settings corruption or a
hook crash that returns non-zero instead of failing open).

Current deny patterns: `git push --force *`, `git push -f *`, `git reset --hard *`,
`git clean -f *`, `rm -rf /`, `kubectl delete namespace *`, `kubectl delete pv *`,
and related variants.

**The KUBECTL entries are KEPT** -- they remain a necessary mechanical backstop for
production namespace destruction, regardless of the hook.

**Status: ACTIVE**

### Layer 3 -- Git server hooks: pre-receive / pre-push (AUTHORITY BOUNDARY)

**Scope:** petersimmons1972 GitHub repos + any self-hosted git servers

Server-side hooks enforce force-push and history-rewrite protection at the authority
boundary: the server. They are unbypassable by any client-side mechanism. They cover
both human operators and automation agents with no per-host binary distribution.

**Status: PLANNED (not yet implemented)**

Recommended implementation: GitHub branch protection (required status checks, no
admin bypass) + optional pre-receive hooks on self-hosted repos.

### Layer 4 -- SSH identity + ForceCommand (HOST-LEVEL POLICY)

**Scope:** automation-only hosts (codex, grok, opencode nodes)

SSH `ForceCommand` wraps every incoming SSH session in a wrapper that logs and
optionally blocks destructive patterns at the host boundary. A compromised agent
that bypasses local hooks cannot bypass ForceCommand on the remote host.

SSH keys authenticate who is connecting; ForceCommand constrains what they can do.
Both are listed here for completeness.

**Status: PLANNED (not yet implemented)**

---

## Fail-open log

When the hook cannot assess a command (binary missing, timeout, non-JSON output),
it appends a line to `~/.claude/codex-guard-hook.log`:

```
2026-06-20T10:00:00Z MISS binary-not-found CMD=git\ push\ --force
2026-06-20T10:00:01Z MISS timeout CMD=...
2026-06-20T10:00:02Z MISS no-output exit=1 CMD=...
```

Monitor this file after any codex-guard binary update or PATH change.

---

## What agents write now

Agents write plain commands. No prefix required.

```bash
# Before (RETIRED mandate)
codex-guard git commit -m "fix: foo"
codex-guard kubectl rollout restart deploy/foo -n ns

# After (current)
git commit -m "fix: foo"
kubectl rollout restart deploy/foo -n ns
```

The hook intercepts both forms transparently.

---

## Open work

- [ ] Red-team matrix: for each security property x each invocation path, confirm
      which layer fires. Verify deny-list patterns vs quoting variations.
- [ ] Audit-log secret redaction: codex-guard must not log raw credential strings
      in its own output or the hook's miss log.
- [ ] Layer 3: GitHub branch protection enforcement (no admin bypass).
- [ ] Layer 4: SSH ForceCommand on automation hosts.
