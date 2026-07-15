#!/usr/bin/env bash
# check-doc-auth-headers.sh — reject malformed Authorization: Bearer snippets.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${CHECK_DOC_ROOT:-$(cd "${SCRIPT_DIR}/.." && pwd)}"

# Explicit targets are a caller-asserted contract: the caller is telling us
# these paths exist and should be scanned. The default (docs skills) is a
# best-effort fallback for whole-repo runs and is expected to legitimately
# be absent when scanning a partial tree (e.g. a staged-files-only checkout
# that happens to include no docs/skills changes) — that is "no targets
# configured for this tree", not a failure.
explicit_targets=1
targets=("$@")
if [[ ${#targets[@]} -eq 0 ]]; then
  targets=(docs skills)
  explicit_targets=0
fi

existing_targets=()
missing_targets=()
for target in "${targets[@]}"; do
  target_path="${REPO_ROOT}/${target}"
  [[ "$target" == /* ]] && target_path="$target"
  if [[ -e "$target_path" ]]; then
    existing_targets+=("$target")
  else
    missing_targets+=("$target")
  fi
done

nothing_to_scan=0
if [[ ${#existing_targets[@]} -eq 0 ]]; then
  if [[ "$explicit_targets" -eq 1 ]]; then
    # Every explicitly-requested target is missing on disk — a scan over
    # zero existing paths is not "clean", it's "checked nothing". Fail
    # loud instead of silently reporting success (FM-18).
    echo "check-doc-auth-headers: all configured targets missing on disk: ${missing_targets[*]}" >&2
    exit 2
  fi
  # No explicit targets were requested and neither default directory
  # exists in this tree — legitimately nothing to check.
  nothing_to_scan=1
fi

failures=0
if [[ "$nothing_to_scan" -eq 0 ]]; then
  targets=("${existing_targets[@]}")

  hits=""
  scan_rc=0
  hits="$(
    cd "$REPO_ROOT"
    grep -RIn --include='*.md' 'Authorization: Bearer' "${targets[@]}" 2>&1
  )" || scan_rc=$?
  if [[ $scan_rc -gt 1 ]]; then
    echo "check-doc-auth-headers: scan failed: ${hits:-grep exited ${scan_rc}}" >&2
    exit 2
  fi

  while IFS= read -r hit; do
    [[ -z "$hit" ]] && continue
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
  done <<< "$hits"
fi

exit "$failures"
