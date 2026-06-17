#!/usr/bin/env bash
set -euo pipefail

# BUGFIX: always drain stdin first. PreToolUse:Edit|Write pipes the full event
# JSON (including file content for Write) to stdin. Without this, a Write event
# carrying >64KB of content would block CC's pipe-write, triggering SIGPIPE and
# causing CC to silently skip the branch-warning on exactly the large writes that
# matter most. (FM-88)
INPUT=$(cat)

session_branch_file="$HOME/.claude/session-branch"
if [ ! -f "$session_branch_file" ]; then
  exit 0
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  exit 0
fi

read -r session_line < "$session_branch_file"
session_root="${session_line%%:*}"
session_branch="${session_line#*:}"

current_root="$(git rev-parse --show-toplevel 2>/dev/null || printf '')"
current_branch="$(git branch --show-current 2>/dev/null || printf '')"
[ -z "$current_branch" ] && current_branch='(detached)'

if [ -n "$session_root" ] && [ "$current_root" != "$session_root" ]; then
  exit 0
fi

if [ -n "$session_branch" ] && [ "$current_branch" != "$session_branch" ]; then
  printf '⚠️  Branch changed since session start (non-blocking):\n' >&2
  printf '   started: %s\n' "$session_branch" >&2
  printf '   now:     %s\n' "$current_branch" >&2
fi

exit 0
