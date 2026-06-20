#!/usr/bin/env bash
# codex-guard-bash-guard.sh
# PreToolUse:Bash hook — runs every Bash command through codex-guard --json.
#
# FAIL-OPEN design: if codex-guard is missing, errors, or produces non-JSON
# output, the command is ALLOWED and the failure is logged. A broken guard
# must never block all shell operations.
#
# Output contract (Claude Code PreToolUse):
#   Allow:  exit 0 (no output required)
#   Block:  stdout JSON {"decision":"block","reason":"<text>"}  +  exit 0
#           OR: exit 2 with reason on stderr (both forms accepted by Claude Code)
#   We use the JSON stdout form to match branch-write-guard.sh convention.

set -uo pipefail

LOG="${HOME}/.claude/codex-guard-hook.log"
GUARD_BIN="${HOME}/bin/codex-guard"

# Read the PreToolUse JSON from stdin
INPUT=$(cat)

# Extract the bash command — field is .tool_input.command
CMD=$(python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    cmd = d.get('tool_input', {}).get('command', '')
    print(cmd)
except Exception:
    print('')
" <<< "$INPUT" 2>/dev/null || true)

# Empty command — nothing to guard, allow
if [[ -z "$CMD" ]]; then
    exit 0
fi

# ── FAIL-OPEN BLOCK ──────────────────────────────────────────────────────────
# Any error below this point must ALLOW the command and log the problem.
# Wrapping in a function makes the early-exit logic easier to reason about.

_run_guard() {
    # Check binary exists
    if [[ ! -x "$GUARD_BIN" ]]; then
        echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) MISS binary-not-found CMD=$(printf '%q' "$CMD")" >> "$LOG" 2>/dev/null || true
        return 0
    fi

    # Run codex-guard --json with a timeout, capture output.
    # codex-guard exits 0 when allowed, non-zero (e.g. 4) when blocked.
    # We MUST NOT treat a non-zero exit from codex-guard as a fail-open condition —
    # that is a blocking verdict, not a guard malfunction. We only fail-open when
    # the output is missing or unparseable (indicating a guard malfunction).
    local guard_out guard_exit
    guard_out=$(timeout 5s "$GUARD_BIN" --json "$CMD" 2>/dev/null) || guard_exit=$?
    guard_exit="${guard_exit:-0}"

    # If there is NO output at all, guard malfunctioned — fail-open
    if [[ -z "$guard_out" ]]; then
        echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) MISS no-output exit=${guard_exit} CMD=$(printf '%q' "$CMD")" >> "$LOG" 2>/dev/null || true
        return 0  # fail-open
    fi

    # timeout exits 124 — treat as malfunction, not a block verdict
    if [[ "$guard_exit" == "124" ]]; then
        echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) MISS timeout CMD=$(printf '%q' "$CMD")" >> "$LOG" 2>/dev/null || true
        return 0  # fail-open
    fi

    # Validate JSON and extract .allowed
    local allowed
    allowed=$(python3 -c "
import json, sys
try:
    d = json.loads(sys.stdin.read())
    print('true' if d.get('allowed', True) else 'false')
except Exception:
    print('parse-error')
" <<< "$guard_out" 2>/dev/null || echo "parse-error")

    if [[ "$allowed" == "parse-error" ]]; then
        echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) MISS parse-error CMD=$(printf '%q' "$CMD")" >> "$LOG" 2>/dev/null || true
        return 0  # fail-open
    fi

    if [[ "$allowed" == "false" ]]; then
        # Extract the finding code for the reason (never the full command — secrets live there)
        local code message
        code=$(python3 -c "
import json, sys
try:
    d = json.loads(sys.stdin.read())
    findings = d.get('findings', [])
    if findings:
        print(findings[0].get('code', 'blocked'))
    else:
        print('blocked')
except Exception:
    print('blocked')
" <<< "$guard_out" 2>/dev/null || echo "blocked")

        message=$(python3 -c "
import json, sys
try:
    d = json.loads(sys.stdin.read())
    findings = d.get('findings', [])
    if findings:
        # Return the static message, not the matched fragment (avoids leaking secrets)
        print(findings[0].get('message', 'Command blocked by codex-guard.'))
    else:
        print('Command blocked by codex-guard.')
except Exception:
    print('Command blocked by codex-guard.')
" <<< "$guard_out" 2>/dev/null || echo "Command blocked by codex-guard.")

        printf '{"decision":"block","reason":"[codex-guard:%s] %s"}\n' "$code" "$message"
        return 0
    fi

    # allowed == true — exit 0 (no output = allow)
    return 0
}

_run_guard
exit 0
