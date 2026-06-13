#!/usr/bin/env bash
# sync-failure-modes.sh — vendor checkin-lint-core.sh into all registered projects.
#
# Usage: bin/sync-failure-modes.sh
# Config: bin/failure-modes-projects.conf (relative to this script's location)

set -euo pipefail

BIN_DIR="$(cd "$(dirname "$0")" && pwd)"
CORE_SRC="${BIN_DIR}/checkin-lint-core.sh"
CONF="${BIN_DIR}/failure-modes-projects.conf"

RED='\033[0;31m'; YLW='\033[1;33m'; GRN='\033[0;32m'; BOLD='\033[1m'; RST='\033[0m'

if [[ ! -f "$CORE_SRC" ]]; then
  echo -e "${RED}ERROR${RST}: checkin-lint-core.sh not found at $CORE_SRC" >&2; exit 1
fi
if [[ ! -f "$CONF" ]]; then
  echo -e "${RED}ERROR${RST}: project config not found at $CONF" >&2; exit 1
fi

updated=0; current=0; skipped=0

while IFS=: read -r project_path _rest; do
  project_path="${project_path%%#*}"
  project_path="$(echo "$project_path" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
  [[ -z "$project_path" ]] && continue

  dest="${project_path}/bin/checkin-lint-core.sh"

  if [[ ! -d "$project_path" ]]; then
    echo -e "${YLW}skip${RST}  $project_path — directory not found"; ((skipped++)) || true; continue
  fi
  if [[ ! -d "${project_path}/bin" ]]; then
    echo -e "${YLW}skip${RST}  $project_path — no bin/ directory (run project setup first)"; ((skipped++)) || true; continue
  fi

  if [[ -f "$dest" ]] && diff -q "$CORE_SRC" "$dest" > /dev/null 2>&1; then
    echo -e "${GRN}current${RST}  $project_path"; ((current++)) || true
  else
    cp "$CORE_SRC" "$dest"
    echo -e "${GRN}updated${RST}  $project_path"; ((updated++)) || true
  fi
done < "$CONF"

echo ""
echo -e "${BOLD}sync-failure-modes: ${updated} updated, ${current} current, ${skipped} skipped${RST}"
[[ $skipped -gt 0 ]] && echo -e "${YLW}Skipped projects need bin/ setup first.${RST}"
exit 0
