#!/usr/bin/env bash
# PostToolUse hook: run ruff on edited Python files.
# BUGFIX: was reading d.get('file_path') from top-level JSON, but CC PostToolUse
# nests the path at tool_input.file_path. Hook was silently never running. (FM-87)
input=$(cat)
file=$(echo "$input" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path','') or d.get('tool_input',{}).get('path',''))" 2>/dev/null)
if [[ "$file" == *.py ]] && command -v ruff &>/dev/null; then
  # Auto-fix what ruff can.
  ruff check --fix "$file" >/dev/null 2>&1 || true
  # Second pass (read-only): if violations still remain after auto-fix,
  # emit a non-blocking warning to stderr so they're visible to the user.
  # --quiet suppresses the "All checks passed!" banner on success.
  # Never exit non-zero — this is a PostToolUse hook and must not block.
  if remaining=$(ruff check --quiet "$file" 2>&1); then
    : # clean — nothing to report
  else
    echo "ruff: unfixable violations remain in $file:" >&2
    echo "$remaining" >&2
  fi
fi
