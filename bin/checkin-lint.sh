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
# Baseline key format: <rule>::<file>::<sha1-of-matched-line-content>[::<line>]
# The optional line suffix is informational only (kept for humans reading the
# baseline). Matching is multiset by the rule::file::sha1 prefix: N baseline
# entries with the same prefix suppress up to N occurrences, so a change in
# duplicate cardinality never flips the key format of unchanged content.
baseline_key() {
  local rule="$1" file="$2" line="$3"
  local content="" content_hash

  if [[ "$line" =~ ^[0-9]+$ && -f "$file" ]]; then
    content="$(sed -n "${line}p" "$file")"
  fi
  content_hash="$(printf '%s' "$content" | sha1sum | awk '{print $1}')"
  printf '%s::%s::%s\n' "$rule" "$file" "$content_hash"
}

# Count baseline entries whose rule::file::sha1 prefix matches (entry may
# carry an informational ::<line> suffix).
_baseline_allowance() {
  local prefix="$1"
  awk -v p="$prefix" 'index($0, p) == 1 && (length($0) == length(p) || substr($0, length(p) + 1, 2) == "::") { c++ } END { print c + 0 }' \
    "${CHECKIN_LINT_BASELINE}"
}

finding() {
  local rule="$1" file="$2" line="$3" why="$4"
  local key allowed=0 used=0
  key="$(baseline_key "$rule" "$file" "$line")"
  [[ -f "${CHECKIN_LINT_BASELINE}" ]] && allowed="$(_baseline_allowance "$key")"
  [[ -n "${_BASELINE_USED_FILE:-}" && -f "${_BASELINE_USED_FILE}" ]] && \
    used="$(grep -Fxc -- "$key" "${_BASELINE_USED_FILE}" 2>/dev/null || true)"
  if [[ "$allowed" -gt "$used" ]]; then
    [[ -n "${_BASELINE_USED_FILE:-}" ]] && echo "$key" >> "${_BASELINE_USED_FILE}"
    echo -e "${YLW}baselined${RST} [${BOLD}${rule}${RST}] ${file}:${line}  —  ${why}"
    ((BASELINED++)) || true
    [[ -n "${_ALL_FINDING_KEYS_FILE:-}" ]] && \
      echo "$key" >> "$_ALL_FINDING_KEYS_FILE"
    return 0
  fi
  echo -e "${RED}FINDING${RST} [${BOLD}${rule}${RST}] ${file}:${line}  —  ${why}"
  [[ -n "${_ALL_FINDING_KEYS_FILE:-}" ]] && \
    echo "$key" >> "$_ALL_FINDING_KEYS_FILE"
  ((FINDINGS++)) || true
}
# Re-export so subprocesses spawned after this point see the overridden version, not core's.
export -f baseline_key _baseline_allowance finding

# Per-run multiset state: how many occurrences each baseline prefix has already
# suppressed (file-backed so finding() works from subshells too).
_BASELINE_USED_FILE="$(mktemp "${TMPDIR:-/tmp}/checkin-lint-used.XXXXXX")"
export _BASELINE_USED_FILE
trap 'rm -f "${_BASELINE_USED_FILE}"' EXIT

_core_exit=0
run_core_checks "$@" || _core_exit=$?
[[ $_core_exit -eq 2 ]] && exit 2

# ── D2. Documentation auth headers must stay copy-paste valid (#1340) ────────
section "D2. Documentation auth header snippets"
if out="$(scripts/check-doc-auth-headers.sh 2>&1)"; then
  pass_rule "D2.doc-auth-headers" "all Authorization header snippets use quoted \${VAR} placeholders"
else
  while IFS= read -r hit; do
    [[ -z "$hit" ]] && continue
    file="${hit%%:*}"
    rest="${hit#*:}"
    lineno="${rest%%:*}"
    why="${rest#*: }"
    finding "D2.doc-auth-headers" "$file" "$lineno" "$why"
  done <<< "$out"
fi

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
  --exclude-dir='.git' --exclude-dir='.claude' --exclude-dir='.worktrees' --exclude-dir='.dispatch' \
  'postgres://[^$][^{]' . 2>/dev/null || true)
[[ $p1_n -eq 0 ]] && pass_rule "P1.hardcoded-dsn" "no hardcoded postgres:// DSNs"

if [[ "${_CORE_AUDIT_BASELINE:-0}" -eq 1 ]]; then
  _do_baseline_audit
fi

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
