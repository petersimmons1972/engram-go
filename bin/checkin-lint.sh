#!/usr/bin/env bash
# checkin-lint.sh — engram-go pre-check-in guard.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RED='\033[0;31m'; YLW='\033[1;33m'; GRN='\033[0;32m'; BOLD='\033[1m'; RST='\033[0m'

EXPECTED_REMOTE="petersimmons1972/engram-go"
CHECKIN_K8S=1
export EXPECTED_REMOTE CHECKIN_K8S

FINDINGS=0
BASELINED=0
export FINDINGS BASELINED

# ── Baseline file path (exported so finding() subshells can read it) ──────────
BASELINE_FILE_DEFAULT="${SCRIPT_DIR}/checkin-lint.baseline"
BASELINE_FILE="${CHECKIN_LINT_BASELINE:-$BASELINE_FILE_DEFAULT}"
export CHECKIN_LINT_BASELINE="$BASELINE_FILE"

# Source core first (it defines and exports finding()), then override it.
source "${SCRIPT_DIR}/checkin-lint-core.sh"

# ── Override finding() to suppress baselined entries ──────────────────────────
# Baseline key format: <rule>::<file>::<line>
finding() {
  local rule="$1" file="$2" line="$3" why="$4"
  local key="${rule}::${file}::${line}"
  if [[ -f "${CHECKIN_LINT_BASELINE}" ]] && \
     grep -Fxq "$key" "${CHECKIN_LINT_BASELINE}" 2>/dev/null; then
    echo -e "${YLW}baselined${RST} [${BOLD}${rule}${RST}] ${file}:${line}  —  ${why}"
    ((BASELINED++)) || true
    [[ -n "${_ALL_FINDING_KEYS_FILE:-}" ]] && \
      echo "${rule}::${file}::${line}" >> "$_ALL_FINDING_KEYS_FILE"
    return 0
  fi
  echo -e "${RED}FINDING${RST} [${BOLD}${rule}${RST}] ${file}:${line}  —  ${why}"
  [[ -n "${_ALL_FINDING_KEYS_FILE:-}" ]] && \
    echo "${rule}::${file}::${line}" >> "$_ALL_FINDING_KEYS_FILE"
  ((FINDINGS++)) || true
}
# Re-export so subprocesses spawned after this point see the overridden version, not core's.
export -f finding

_core_exit=0
run_core_checks "$@" || _core_exit=$?
[[ $_core_exit -eq 2 ]] && exit 2

# ── P1. No hardcoded DB connection strings (FM-06 / secrets) ─────────────────
section "P1. Hardcoded DB connection strings"
p1_n=0
while IFS= read -r hit; do
  file="${hit%%:*}"; rest="${hit#*:}"; lineno="${rest%%:*}"
  finding "P1.hardcoded-dsn" "$file" "$lineno" \
    "hardcoded postgres:// DSN — use environment variable injection"
  hint "Replace with: os.Getenv(\"DATABASE_URL\") or equivalent config struct."
  ((p1_n++)) || true
# postgres://[^$][^{] — excludes $VAR and ${VAR} style env references; flags bare DSNs
# *_test.go files are excluded: test DSNs are never production secrets.
done < <(grep -rn \
  --include='*.go' --include='*.yaml' --include='*.yml' --include='*.env' \
  --exclude='*_test.go' \
  --exclude-dir='.git' --exclude-dir='.claude' --exclude-dir='.worktrees' \
  'postgres://[^$][^{]' . 2>/dev/null || true)
[[ $p1_n -eq 0 ]] && pass_rule "P1.hardcoded-dsn" "no hardcoded postgres:// DSNs"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
baseline_note=""
[[ $BASELINED -gt 0 ]] && baseline_note=" (${BASELINED} baselined)"
if [[ $FINDINGS -eq 0 ]]; then
  echo -e "${GRN}${BOLD}✓ checkin-lint PASSED — 0 findings${baseline_note}${RST}"; exit 0
else
  echo -e "${RED}${BOLD}✗ checkin-lint FAILED — ${FINDINGS} finding(s)${RST}"
  echo "  Re-run with --fix-hints for remediation guidance."; exit 1
fi
