#!/usr/bin/env bash
# Convert a line-keyed checkin-lint baseline to content-keyed entries.
set -euo pipefail

baseline="${1:-bin/checkin-lint.baseline}"
output="$(mktemp)"
trap 'rm -f "$output"' EXIT

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
    ./.worktrees/*|./.claude/worktrees/*) continue ;;
  esac
  if [[ -f "$file" ]]; then
    content="$(sed -n "${line}p" "$file")"
  elif [[ "$file" == \(*\) ]]; then
    content=""
  else
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
