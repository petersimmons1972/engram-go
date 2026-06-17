#!/usr/bin/env bash
# validate-readonly-bash.sh
# PreToolUse hook for Tier 3 validators (Spruance, Rickover-validator).
# Allows read-only commands. Blocks destructive operations.
# Exit 0 = allow. Exit 2 = block (Claude Code convention).
#
# SECURITY FIX (FM-86): bare python3/python on allowlist granted arbitrary
# code execution to validator agents (python3 -c "..."). curl on allowlist
# allowed data exfiltration. Both are now restricted to specific safe forms.

set -euo pipefail

# Opt-in enforcement: only active when coordinator has set the validator guard.
# Coordinators dispatching Tier-3 validators must: touch ~/.claude/.validator-bash-guard
# Remove after validation: rm ~/.claude/.validator-bash-guard
GUARD_FILE="${HOME}/.claude/.validator-bash-guard"
if [[ ! -f "$GUARD_FILE" ]]; then
    exit 0  # not a validator session; allow all
fi

# Read the tool input JSON from stdin
INPUT=$(cat)

# Extract the bash command
CMD=$(echo "$INPUT" | python3 -c "
import json, sys
data = json.load(sys.stdin)
# Handle both direct input and nested tool_input formats
cmd = data.get('command') or data.get('tool_input', {}).get('command', '')
print(cmd.strip())
" 2>/dev/null || echo "")

if [ -z "$CMD" ]; then
    exit 0  # Can't parse — allow and let Claude handle it
fi

# Extract the base command (first word, stripping leading whitespace/newlines)
BASE=$(echo "$CMD" | awk '{print $1}' | tr -d '\n')

# Git requires special handling — allow read-only subcommands only
if [ "$BASE" = "git" ]; then
    SUBCMD=$(echo "$CMD" | awk '{print $2}' | tr -d '\n')
    case "$SUBCMD" in
        status|diff|log|show|branch|stash|describe|shortlog|rev-parse|ls-files|ls-tree|cat-file|blame|grep|bisect)
            exit 0
            ;;
        *)
            echo "BLOCKED by validate-readonly-bash.sh: 'git $SUBCMD' is not on the validator allowlist." >&2
            echo "Permitted git subcommands: status, diff, log, show, branch, stash, describe, ls-files" >&2
            exit 2
            ;;
    esac
fi

# npm requires special handling — allow test/audit/list, block install/publish/etc.
if [ "$BASE" = "npm" ] || [ "$BASE" = "npx" ]; then
    SUBCMD=$(echo "$CMD" | awk '{print $2}' | tr -d '\n')
    case "$SUBCMD" in
        test|run|exec|audit|ls|list|outdated|ci)
            exit 0
            ;;
        install|uninstall|publish|pack|link|rebuild|update|version|deprecate)
            echo "BLOCKED by validate-readonly-bash.sh: 'npm $SUBCMD' is not on the validator allowlist." >&2
            exit 2
            ;;
        *)
            # Allow 'npm run <script>' patterns
            exit 0
            ;;
    esac
fi

# python3/python: ONLY allow pytest invocations — bare python3 -c "..." grants
# arbitrary code execution and must be blocked in read-only validator sessions.
# SECURITY: do NOT add bare 'python3' or 'python' to ALLOWED below.
if [[ "$BASE" == python* ]]; then
    SUBCMD=$(echo "$CMD" | awk '{print $2}' | tr -d '\n')
    THIRD=$(echo "$CMD" | awk '{print $3}' | tr -d '\n')
    # Allow: python3 -m pytest <args>, python -m pytest <args>
    if [[ "$SUBCMD" == "-m" && "$THIRD" == "pytest" ]]; then
        exit 0
    fi
    # Allow: python3 /path/to/pytest or python3 -m coverage run ...
    if [[ "$SUBCMD" == *pytest* ]]; then
        exit 0
    fi
    echo "BLOCKED by validate-readonly-bash.sh: '$BASE' invocation not permitted." >&2
    echo "Only 'python3 -m pytest ...' is allowed. Use pytest directly or pass -m pytest." >&2
    exit 2
fi

# Allowlist: non-git/npm/python commands validators are permitted to run.
# SECURITY: do NOT add curl, wget, nc, ssh, or any network-capable tool here.
# Do NOT add python3/python — handled above with argument inspection.
ALLOWED=(
    pytest
    cat
    head
    tail
    grep
    rg
    ripgrep
    find
    ls
    wc
    diff
    jq
    echo
    printf
    which
    type
    true
    false
    node
)

for allowed in "${ALLOWED[@]}"; do
    if [ "$BASE" = "$allowed" ]; then
        exit 0
    fi
done

# Block everything else
echo "BLOCKED by validate-readonly-bash.sh: '$BASE' is not on the validator allowlist." >&2
echo "Permitted commands: pytest, python3 -m pytest, npm test, git status/diff/log, cat, head, tail, grep, rg, find, ls" >&2
exit 2
