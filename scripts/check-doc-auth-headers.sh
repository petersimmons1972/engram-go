#!/usr/bin/env bash
# check-doc-auth-headers.sh — reject malformed Authorization: Bearer snippets.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

targets=("$@")
if [[ ${#targets[@]} -eq 0 ]]; then
  targets=(docs skills)
fi

failures=0

while IFS= read -r hit; do
  file="${hit%%:*}"
  rest="${hit#*:}"
  line="${rest%%:*}"
  text="${rest#*:}"

  if [[ "$text" == *'Authorization: Bearer ***'* ]]; then
    echo "${file}:${line}: redacted bearer token breaks copy-paste syntax; use a quoted \${VAR} placeholder"
    failures=1
    continue
  fi

  is_command_header=0
  if [[ "$text" =~ ^[[:space:]]*(curl|xh)[[:space:]] ]]; then
    is_command_header=1
  elif [[ "$text" =~ (^|[[:space:]])(--header|-H)[[:space:]] ]]; then
    is_command_header=1
  elif [[ "$text" =~ ^[[:space:]]*\"Authorization:\ Bearer\  ]]; then
    is_command_header=1
  fi

  if [[ "$is_command_header" -eq 0 ]]; then
    continue
  fi

  if [[ ! "$text" =~ \"Authorization:\ Bearer\ \$\{[A-Za-z_][A-Za-z0-9_]*\}\" ]]; then
    echo "${file}:${line}: malformed Authorization header snippet; use \"Authorization: Bearer \${VAR}\""
    failures=1
  fi
done < <(
  cd "$REPO_ROOT"
  grep -RIn --include='*.md' 'Authorization: Bearer' "${targets[@]}" 2>/dev/null || true
)

exit "$failures"
