#!/usr/bin/env bash
set -euo pipefail

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  exit 0
fi

session_branch_file="$HOME/.claude/session-branch"
worktree_root="$(git rev-parse --show-toplevel 2>/dev/null || printf '')"
branch_name="$(git branch --show-current 2>/dev/null || true)"
[ -z "$branch_name" ] && branch_name='(detached)'

# Atomic write — two simultaneous worktree session starts would otherwise
# race on a bare redirect, potentially leaving a truncated or mixed file.
# mktemp + mv is atomic within the same filesystem. (FM-94)
_tmp=$(mktemp "$HOME/.claude/.session-branch.XXXXXX")
printf '%s:%s\n' "$worktree_root" "$branch_name" > "$_tmp"
mv "$_tmp" "$session_branch_file"

exit 0
