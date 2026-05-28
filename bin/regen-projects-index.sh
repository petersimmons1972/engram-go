#!/usr/bin/env bash
# regen-projects-index.sh — Generate ~/PROJECTS.md from per-project CLAUDE.md frontmatter.
# Usage: ~/bin/regen-projects-index.sh
set -euo pipefail

OUT="$HOME/PROJECTS.md"
TS=$(date -u +"%Y-%m-%dT%H:%MZ")

# ── 1. Collect candidate CLAUDE.md files ──────────────────────────────────────
mapfile -t FILES < <(
  fd "CLAUDE.md" "$HOME/projects/" "$HOME/archive/bambu/" --type f 2>/dev/null \
  | grep -vE '(\.worktrees/|/aifleet-fix-|/aifleet-issue|/aifleet-stop-|/olla-aifleet|/3dprint/challenge_coins|/kubernetes/|/locked-shields/|/art-direction-research/|/aifleet-worktrees/|/archive/claude-md-improvements/)' \
  | sort
)

# ── 2. Parse frontmatter; collect rows per section ────────────────────────────
# Each row is pipe-delimited: project|purpose|stack|display_path|notes
PRIO=(); ACTIVE=(); REFERENCE=(); DORMANT=(); ARCHIVED=(); FAILURES=()

# Extract a single yq field from extracted frontmatter text
yqf() { echo "$1" | yq ".$2 // \"\"" 2>/dev/null; }

for f in "${FILES[@]}"; do
  [[ "$(head -1 "$f" 2>/dev/null)" == "---" ]] || { FAILURES+=("$f (no frontmatter)"); continue; }
  fm=$(awk 'NR==1&&/^---$/{on=1;next} on&&/^---$/{exit} on{print}' "$f")
  proj=$(yqf "$fm" project); [[ -z "$proj" || "$proj" == "null" ]] && FAILURES+=("$f (missing project)") && continue
  stat=$(yqf "$fm" status); [[ -z "$stat" || "$stat" == "null" ]] && stat=dormant
  prio=$(yqf "$fm" priority)
  purp=$(yqf "$fm" purpose)
  stk=$(echo "$fm"  | yq '.stack | join(", ")' 2>/dev/null || true)
  note=$(yqf "$fm" notes)
  path="${f/$HOME/\~}"; path="$(dirname "$path")/"
  row="${proj}|${purp}|${stk}|${path}|${note}"
  case "$stat" in
    active)    [[ "$prio" =~ ^[0-9]+$ ]] && PRIO+=("${prio}|${row}") || ACTIVE+=("${proj}|${row}") ;;
    reference) REFERENCE+=("${proj}|${row}") ;;
    dormant)   DORMANT+=("${proj}|${row}") ;;
    archived)  ARCHIVED+=("${proj}|${row}") ;;
    *)         ACTIVE+=("${proj}|${row}") ;;
  esac
done

# ── 3. Table-printing helpers ─────────────────────────────────────────────────
# Prints sorted rows; $1=header line, $2=separator, $3+=data rows (sort key is col 1)
prio_table() {
  printf "| # | Project | Purpose | Path |\n|---|---------|---------|------|\n"
  [[ ${#PRIO[@]} -eq 0 ]] && return
  printf '%s\n' "${PRIO[@]}" | sort -t'|' -k1,1n \
    | while IFS='|' read -r p proj purp stk path note; do
        printf "| %s | **%s** | %s | \`%s\` |\n" "$p" "$proj" "$purp" "$path"; done
}
active_table() {
  printf "| Project | Purpose | Stack | Path |\n|---------|---------|-------|------|\n"
  [[ ${#ACTIVE[@]} -eq 0 ]] && return
  printf '%s\n' "${ACTIVE[@]}" | sort -t'|' -k1,1 \
    | while IFS='|' read -r _ proj purp stk path note; do
        printf "| **%s** | %s | %s | \`%s\` |\n" "$proj" "$purp" "$stk" "$path"; done
}
simple_table() {        # args: array_name — prints Project|Purpose|Path
  local -n _arr=$1
  printf "| Project | Purpose | Path |\n|---------|---------|------|\n"
  [[ ${#_arr[@]} -eq 0 ]] && return
  printf '%s\n' "${_arr[@]}" | sort -t'|' -k1,1 \
    | while IFS='|' read -r _ proj purp stk path note; do
        printf "| **%s** | %s | \`%s\` |\n" "$proj" "$purp" "$path"; done
}
archived_table() {
  printf "| Project | Purpose | Path | Notes |\n|---------|---------|------|-------|\n"
  [[ ${#ARCHIVED[@]} -eq 0 ]] && return
  printf '%s\n' "${ARCHIVED[@]}" | sort -t'|' -k1,1 \
    | while IFS='|' read -r _ proj purp stk path note; do
        printf "| **%s** | %s | \`%s\` | %s |\n" "$proj" "$purp" "$path" "$note"; done
}

# ── 4. Emit output ────────────────────────────────────────────────────────────
TMP=$(mktemp); trap 'rm -f "$TMP"' EXIT
{
printf '# Project Index\n\n'
printf '_Generated from per-project CLAUDE.md frontmatter. Regenerate with `bin/regen-projects-index.sh`._\n'
printf '_Last generated: %s_\n\n' "$TS"
printf '## Active — Priority Stack\n\n'; prio_table
printf '\n## Active — Other\n\n';        active_table
printf '\n## Reference\n\n';             simple_table REFERENCE
printf '\n## Dormant\n\n';               simple_table DORMANT
printf '\n## Archived\n\n';              archived_table
printf '\n## Not Indexed\n'
printf '_Active projects deliberately excluded from this index:_\n'
echo '- `generals` and `security-intelligence-business` — deny-listed for direct edit; manually add frontmatter if/when needed.'
echo '- `kubernetes` — subsumed by `infrastructure`.'
echo '- `locked-shields`, `art-direction-research`, and `aifleet-*` worktree directories — no git remote.'
if [[ ${#FAILURES[@]} -gt 0 ]]; then
  printf '\n## Parse Warnings\n_The following files were skipped — no frontmatter or missing required fields:_\n'
  for f in "${FAILURES[@]}"; do echo "- \`$f\`"; done
fi
} > "$TMP"

mv "$TMP" "$OUT"
echo "Written: $OUT ($(wc -l < "$OUT") lines)"
[[ ${#FAILURES[@]} -gt 0 ]] && printf 'WARNINGS — skipped %d file(s):\n' "${#FAILURES[@]}" && printf '  %s\n' "${FAILURES[@]}"
true
