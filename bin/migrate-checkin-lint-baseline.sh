#!/usr/bin/env bash
# Convert a line-keyed checkin-lint baseline to content-keyed entries.
# Must run from the repo root (entries hold repo-relative paths); a legacy
# entry whose file cannot be found is PRESERVED as-is and reported, never
# silently dropped — rerun from the right cwd or remove it by hand.
set -euo pipefail

if [[ ! -f bin/checkin-lint.sh ]]; then
  echo "migrate-checkin-lint-baseline: run from the repo root (bin/checkin-lint.sh not found in cwd)" >&2
  exit 2
fi

baseline="${1:-bin/checkin-lint.baseline}"
output="$(mktemp)"
trap 'rm -f "$output"' EXIT
unresolved=0

while IFS= read -r entry || [[ -n "$entry" ]]; do
  if [[ -z "$entry" || "$entry" == \#* ]]; then
    printf '%s\n' "$entry" >> "$output"
    continue
  fi
  if [[ "$entry" =~ ::[[:xdigit:]]{40}(::[0-9]+)?$ ]]; then
    printf '%s\n' "$entry" >> "$output"
    continue
  fi

  rule="${entry%%::*}"
  rest="${entry#*::}"
  line="${rest##*::}"
  file="${rest%::*}"

  case "$file" in
    ./.worktrees/*|./.claude/worktrees/*|./.dispatch/*)
      # Junk paths purged by design (issue #1388 item 3) — dropping is the point.
      continue ;;
  esac
  if [[ -f "$file" ]]; then
    content="$(sed -n "${line}p" "$file")"
  elif [[ "$file" == \(*\) ]]; then
    content=""
  else
    echo "UNRESOLVED  $entry  (file not found — entry preserved unmigrated)" >&2
    printf '%s\n' "$entry" >> "$output"
    ((unresolved++)) || true
    continue
  fi
  content_hash="$(printf '%s' "$content" | sha1sum | awk '{print $1}')"
  duplicate_count=0
  if [[ -f "$file" ]]; then
    duplicate_count="$(grep -Fxc -- "$content" "$file" || true)"
  fi
  if [[ "$duplicate_count" -gt 1 ]]; then
    printf '%s::%s::%s::%s\n' "$rule" "$file" "$content_hash" "$line" >> "$output"
  else
    printf '%s::%s::%s\n' "$rule" "$file" "$content_hash" >> "$output"
  fi
done < "$baseline"

mv "$output" "$baseline"
trap - EXIT

if [[ "$unresolved" -gt 0 ]]; then
  echo "migrate-checkin-lint-baseline: ${unresolved} entr(y/ies) could not be resolved and were preserved unmigrated (see UNRESOLVED lines above)" >&2
  exit 1
fi
