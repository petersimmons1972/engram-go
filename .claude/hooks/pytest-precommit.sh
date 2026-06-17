#!/usr/bin/env bash
# PreToolUse:Bash — block git commit when pytest fails.
#
# Why: 2026-06-17 — H14 audit found the original hook always exited 0 because
# pytest output was piped through `tail -5` before the exit code was checked.
# Result: no commit was ever blocked, even on failing test suites.
#
# Fix: capture pytest output + exit code separately; emit JSON block on failure.
#
# Scope: only fires when all three conditions hold:
#   1. Command contains "git commit"
#   2. A tests/ directory exists at the git repo root
#   3. pytest is installed (missing pytest → skip silently)

set -uo pipefail

input=$(cat)

# Extract the bash command from the PreToolUse event JSON.
# Event structure: {"tool_name":"Bash","tool_input":{"command":"..."}}
cmd=$(python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('tool_input', {}).get('command', ''))
except Exception:
    print('')
" <<< "$input" 2>/dev/null || true)

# Only act on git commit commands (covers --amend, -m, etc.)
echo "$cmd" | grep -qE "\bgit\b.*\bcommit\b" || exit 0

# Find the git repo root; skip if not in a git repo
repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0

# Skip if no tests/ directory at the repo root
[ -d "$repo_root/tests" ] || exit 0

# Skip if pytest is not available
python3 -m pytest --version >/dev/null 2>&1 || exit 0

# Run pytest: capture output AND exit code before any pipe
output=$(python3 -m pytest "$repo_root/tests" -x -q --tb=short 2>&1) || true
ec=$?

if [ "$ec" -ne 0 ]; then
    # Build block JSON via Python to avoid backslash-corruption from sed escaping.
    # sed 's/"/\\"/g' double-escapes paths with backslashes and breaks JSON parsing.
    echo "$output" | tail -5 | python3 -c "
import json, sys
reason = sys.stdin.read().strip()
print(json.dumps({'decision': 'block', 'reason': 'pytest failed before commit: ' + reason}))
" 2>/dev/null || printf '{\"decision\":\"block\",\"reason\":\"pytest failed before commit (reason unavailable)\"}'
fi

exit 0
