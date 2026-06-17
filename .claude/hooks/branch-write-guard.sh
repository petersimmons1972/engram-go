#!/usr/bin/env bash
# PreToolUse:Edit|Write — block file writes when on main/master in a project repo.
#
# Why: CLAUDE.md Pre-Flight Protocol §1 requires halting when on the wrong branch.
# The existing branch-check-pre-tool.sh warns on stderr but exits 0 (cannot block).
# This hook enforces the same rule as a hard block.
#
# Safety design — fail-open everywhere, one hard carve-out:
#   - All git/json parse failures → exit 0 (never block on infrastructure errors)
#   - No remote → exit 0 (local-only repos have no protected branch concept)
#   - repo_root == HOME → exit 0 (home dir is on 'master' by design; blocking
#     it would prevent writing CLAUDE.md, settings.json, memory files — deadlock)
#   - ~/.claude/ paths → exit 0 (config files regardless of repo)
#   - Detached HEAD → branch="HEAD", not main/master → passes through
#
# Known limitation: cannot distinguish hotfix branches named 'main' from trunk.
# Do not add heuristics for this — accept it as a known false-negative.

set -uo pipefail

input=$(cat)

# Extract the target file path from the tool input JSON.
# Edit uses "file_path"; Write uses "file_path" also (via tool_input).
filepath=$(python3 -c "
import sys, json, pathlib
try:
    d = json.load(sys.stdin)
    inp = d.get('tool_input', {})
    p = inp.get('file_path', inp.get('path', ''))
    # Resolve to absolute path to prevent ../ traversal from bypassing carve-outs
    # e.g. ~/.claude/../projects/foo would otherwise match ~/.claude/* prefix
    print(str(pathlib.Path(p).resolve()) if p else '')
except Exception:
    print('')
" <<< "$input" 2>/dev/null || true)

[ -z "$filepath" ] && exit 0

# Always allow writes anywhere inside ~/.claude/ — config, memory, plans, hooks
case "$filepath" in
    "$HOME/.claude/"*|"$HOME/.claude") exit 0 ;;
esac

# Resolve the git repo that owns this file
repo_root=$(git -C "$(dirname "$filepath")" rev-parse --show-toplevel 2>/dev/null) || exit 0

# HOME carve-out: the home directory is a git repo on 'master' by design.
# Every config file lives here. Blocking writes here is a deadlock device.
[ "$repo_root" = "$HOME" ] && exit 0

# Only enforce protection when a remote exists (local-only repos → skip)
has_remote=$(git -C "$repo_root" remote 2>/dev/null | head -1)
[ -z "$has_remote" ] && exit 0

# Get the current branch name
branch=$(git -C "$repo_root" rev-parse --abbrev-ref HEAD 2>/dev/null) || exit 0

if [[ "$branch" == "main" || "$branch" == "master" ]]; then
    printf '{"decision":"block","reason":"Write blocked: currently on branch %s in %s. Checkout a feature branch before editing files directly on the default branch. (branch-write-guard.sh)"}' \
        "$branch" "$repo_root"
fi

exit 0
