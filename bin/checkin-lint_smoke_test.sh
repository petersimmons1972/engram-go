#!/usr/bin/env bash
# checkin-lint_smoke_test.sh — TDD smoke tests for the check-in lint infra.
# Tests defined in issue #1095 / plan codex-1095-checkin-lint-infra.
# Run from repo root: bash bin/checkin-lint_smoke_test.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PASS=0
FAIL=0
RED='\033[0;31m'; GRN='\033[0;32m'; BOLD='\033[1m'; RST='\033[0m'

pass() { echo -e "${GRN}PASS${RST}  $1"; ((PASS++)) || true; }
fail() { echo -e "${RED}FAIL${RST}  $1"; ((FAIL++)) || true; }

SCOPE_REPO="$(mktemp -d)"
TEST_FIXTURE="${SCOPE_REPO}/checkin-lint-issue1388-test-$$"
DISPATCH_FIXTURE="${SCOPE_REPO}/.dispatch/worktrees/checkin-lint-issue1388-test-$$"
cleanup() {
  rm -rf "${TEST_FIXTURE}" "${DISPATCH_FIXTURE}" "${SCOPE_REPO}"
}
trap cleanup EXIT

mkdir -p "${SCOPE_REPO}/bin" "${SCOPE_REPO}/docs" "${SCOPE_REPO}/results" "${SCOPE_REPO}/scripts"
cp "${REPO_ROOT}/bin/checkin-lint.sh" "${REPO_ROOT}/bin/checkin-lint-core.sh" \
  "${REPO_ROOT}/bin/checkin-lint.baseline" "${SCOPE_REPO}/bin/"
cp "${REPO_ROOT}/scripts/check-doc-auth-headers.sh" "${SCOPE_REPO}/scripts/"
printf '%s\n' 'results/' > "${SCOPE_REPO}/.gitignore"
git -C "${SCOPE_REPO}" init -q -b main
git -C "${SCOPE_REPO}" config user.email test@example.invalid
git -C "${SCOPE_REPO}" config user.name 'Checkin Lint Test'
git -C "${SCOPE_REPO}" remote add origin https://github.com/petersimmons1972/engram-go.git
git -C "${SCOPE_REPO}" add .
git -C "${SCOPE_REPO}" commit -qm 'test fixture'

echo ""
echo -e "${BOLD}checkin-lint smoke tests${RST}"
echo "────────────────────────────────────────"

# ── test_docs_only_stage_ignores_ignored_findings (#1416) ──────────────────
echo ""
echo "test_docs_only_stage_ignores_ignored_findings"
printf '%s\n' '# check-in lint scope fixture' > "${SCOPE_REPO}/docs/scope.md"
printf '%s\n' '#!/usr/bin/env bash' 'tool=/home/private-user/bin/tool' > "${SCOPE_REPO}/results/ignored.sh"
git -C "${SCOPE_REPO}" add -- docs/scope.md
docs_output="$(cd "${SCOPE_REPO}" && bash bin/checkin-lint.sh 2>&1)" || docs_exit=$?
docs_exit="${docs_exit:-0}"
if [[ "${docs_exit}" -eq 0 ]] && ! grep -Fq 'results/ignored.sh' <<< "${docs_output}"; then
  pass "docs-only staged change is not blocked by a finding in results/"
else
  fail "ignored results/ finding blocked a docs-only staged change (exit ${docs_exit})"
  printf '%s\n' "${docs_output}" | tail -10 | sed 's/^/  /'
fi
git -C "${SCOPE_REPO}" reset -q -- docs/scope.md
rm -f "${SCOPE_REPO}/docs/scope.md" "${SCOPE_REPO}/results/ignored.sh"

# ── test_unstaged_content_of_staged_file_is_not_scanned (#1416) ────────────
echo ""
echo "test_unstaged_content_of_staged_file_is_not_scanned"
printf '%s\n' '#!/usr/bin/env bash' 'echo safe' > "${SCOPE_REPO}/partial.sh"
git -C "${SCOPE_REPO}" add -- partial.sh
printf '%s\n' '#!/usr/bin/env bash' 'tool=/home/private-user/bin/tool' > "${SCOPE_REPO}/partial.sh"
partial_output="$(cd "${SCOPE_REPO}" && bash bin/checkin-lint.sh 2>&1)" || partial_exit=$?
partial_exit="${partial_exit:-0}"
if [[ "${partial_exit}" -eq 0 ]]; then
  pass "unstaged violating content does not contaminate the staged-file scan"
else
  fail "hook scanned unstaged content instead of the staged blob (exit ${partial_exit})"
  printf '%s\n' "${partial_output}" | tail -10 | sed 's/^/  /'
fi
git -C "${SCOPE_REPO}" reset -q -- partial.sh
rm -f "${SCOPE_REPO}/partial.sh"

# ── test_staged_violation_fails_loudly (#1416) ──────────────────────────────
echo ""
echo "test_staged_violation_fails_loudly"
staged_name='staged file.env'
printf '%s\n' 'DATABASE_URL=postgres://localhost/engram' > "${SCOPE_REPO}/${staged_name}"
git -C "${SCOPE_REPO}" add -- "$staged_name"
staged_output="$(cd "${SCOPE_REPO}" && bash bin/checkin-lint.sh 2>&1)" || staged_exit=$?
staged_exit="${staged_exit:-0}"
if [[ "${staged_exit}" -ne 0 ]] && grep -Fq "$staged_name" <<< "${staged_output}"; then
  pass "staged violating content with a spaced filename fails loudly"
else
  fail "staged violation was not rejected loudly (exit ${staged_exit})"
  printf '%s\n' "${staged_output}" | tail -10 | sed 's/^/  /'
fi
git -C "${SCOPE_REPO}" reset -q -- "$staged_name"
rm -f "${SCOPE_REPO}/${staged_name}"

# ── test_tracked_scope_catches_committed_violation_staged_scope_misses (#1418) ──
# A violation committed in a tracked file (nothing staged) must be CAUGHT by
# `--tracked` (whole-tracked-tree audit mode) and MISSED by the default staged
# mode (which only scans the index) — this pair documents the CI contract:
# staged mode alone would let CI's fresh checkout scan an empty tree and
# always pass.
echo ""
echo "test_tracked_scope_catches_committed_violation_staged_scope_misses"
tracked_name="tracked-violation.env"
printf '%s\n' 'DATABASE_URL=postgres://localhost/engram-tracked' > "${SCOPE_REPO}/${tracked_name}"
git -C "${SCOPE_REPO}" add -- "$tracked_name"
git -C "${SCOPE_REPO}" commit -qm 'add tracked violation fixture'
staged_mode_output="$(cd "${SCOPE_REPO}" && bash bin/checkin-lint.sh 2>&1)" || staged_mode_exit=$?
staged_mode_exit="${staged_mode_exit:-0}"
tracked_mode_output="$(cd "${SCOPE_REPO}" && bash bin/checkin-lint.sh --tracked 2>&1)" || tracked_mode_exit=$?
tracked_mode_exit="${tracked_mode_exit:-0}"
if [[ "${staged_mode_exit}" -eq 0 ]] && ! grep -Fq "$tracked_name" <<< "${staged_mode_output}"; then
  pass "default (staged) scope does not see a committed-but-unstaged violation"
else
  fail "default scope unexpectedly caught the committed violation (exit ${staged_mode_exit})"
  printf '%s\n' "${staged_mode_output}" | tail -10 | sed 's/^/  /'
fi
if [[ "${tracked_mode_exit}" -ne 0 ]] && grep -Fq "$tracked_name" <<< "${tracked_mode_output}"; then
  pass "--tracked scope catches a committed violation the staged scope misses"
else
  fail "--tracked scope failed to catch the committed violation (exit ${tracked_mode_exit})"
  printf '%s\n' "${tracked_mode_output}" | tail -10 | sed 's/^/  /'
fi
git -C "${SCOPE_REPO}" rm -q -- "$tracked_name" >/dev/null
git -C "${SCOPE_REPO}" commit -qm 'remove tracked violation fixture'

# ── test_scan_error_fails_loudly (#1416) ────────────────────────────────────
echo ""
echo "test_scan_error_fails_loudly"
printf '%s\n' 'clean' > "${SCOPE_REPO}/clean.txt"
git -C "${SCOPE_REPO}" add -- clean.txt
fake_bin="$(mktemp -d)"
real_grep="$(command -v grep)"
printf '%s\n' '#!/usr/bin/env bash' \
  'for arg in "$@"; do [[ "$arg" == "-rn" ]] && exit 2; done' \
  "exec \"${real_grep}\" \"\$@\"" > "${fake_bin}/grep"
chmod +x "${fake_bin}/grep"
scan_output="$(cd "${SCOPE_REPO}" && PATH="${fake_bin}:${PATH}" bash bin/checkin-lint.sh 2>&1)" || scan_exit=$?
scan_exit="${scan_exit:-0}"
if [[ "${scan_exit}" -eq 2 ]] && grep -Fq 'repository scan failed' <<< "${scan_output}"; then
  pass "scan errors report an error and exit 2"
else
  fail "scan error was swallowed (exit ${scan_exit}; want 2)"
  printf '%s\n' "${scan_output}" | tail -10 | sed 's/^/  /'
fi
rm -rf "${fake_bin}"
git -C "${SCOPE_REPO}" reset -q -- clean.txt
rm -f "${SCOPE_REPO}/clean.txt"

# ── test_entrypoint_exists_and_runs ──────────────────────────────────────────
# Verify bin/checkin-lint.sh exists, is executable, and exits without
# "No such file or directory" when invoked.
echo ""
echo "test_entrypoint_exists_and_runs"
if [[ ! -f "${REPO_ROOT}/bin/checkin-lint.sh" ]]; then
  fail "bin/checkin-lint.sh does not exist"
elif [[ ! -x "${REPO_ROOT}/bin/checkin-lint.sh" ]]; then
  fail "bin/checkin-lint.sh exists but is not executable"
else
  # Run it; we only care that it doesn't produce "No such file or directory"
  output="$(cd "${REPO_ROOT}" && bash bin/checkin-lint.sh 2>&1 || true)"
  if echo "$output" | grep -q 'No such file or directory'; then
    fail "checkin-lint.sh ran but emitted 'No such file or directory'"
    echo "  output: $output"
  else
    pass "bin/checkin-lint.sh exists, is executable, and runs without missing-file errors"
  fi
fi

# ── test_baseline_survives_inserted_lines ────────────────────────────────────
echo ""
echo "test_baseline_survives_inserted_lines"
mkdir -p "${TEST_FIXTURE}"
fixture_file="${TEST_FIXTURE}/finding.env"
matched_line='DATABASE_URL=postgres://localhost/engram'
printf '%s\n' "${matched_line}" > "${fixture_file}"
fixture_rel="./${fixture_file#"${SCOPE_REPO}"/}"
content_hash="$(printf '%s' "${matched_line}" | sha1sum | awk '{print $1}')"
fixture_baseline="$(mktemp)"
cp "${BASELINE:-${REPO_ROOT}/bin/checkin-lint.baseline}" "${fixture_baseline}"
printf 'P1.hardcoded-dsn::%s::%s\n' "${fixture_rel}" "${content_hash}" >> "${fixture_baseline}"

git -C "${SCOPE_REPO}" add -- "${fixture_file#"${SCOPE_REPO}"/}"
first_output="$(cd "${SCOPE_REPO}" && CHECKIN_LINT_BASELINE="${fixture_baseline}" bash bin/checkin-lint.sh 2>&1)" || first_exit=$?
first_exit="${first_exit:-0}"
printf '# inserted above the finding\n%s\n' "${matched_line}" > "${fixture_file}"
git -C "${SCOPE_REPO}" add -- "${fixture_file#"${SCOPE_REPO}"/}"
second_output="$(cd "${SCOPE_REPO}" && CHECKIN_LINT_BASELINE="${fixture_baseline}" bash bin/checkin-lint.sh 2>&1)" || second_exit=$?
second_exit="${second_exit:-0}"
rm -f "${fixture_baseline}"

if [[ "${first_exit}" -eq 0 && "${second_exit}" -eq 0 ]]; then
  pass "content-keyed baseline remains valid when lines are inserted above a finding"
else
  fail "baselined finding became live after an unrelated line insertion (exits: ${first_exit}, ${second_exit})"
  printf '%s\n' "${first_output}" "${second_output}" | tail -20 | sed 's/^/  /'
fi
git -C "${SCOPE_REPO}" reset -q -- "${fixture_file#"${SCOPE_REPO}"/}"
rm -rf "${TEST_FIXTURE}"

# ── test_dispatch_worktrees_are_excluded ─────────────────────────────────────
echo ""
echo "test_dispatch_worktrees_are_excluded"
mkdir -p "${DISPATCH_FIXTURE}"
printf '%s\n' 'DATABASE_URL=postgres://localhost/dispatch-junk' > "${DISPATCH_FIXTURE}/finding.env"
git -C "${SCOPE_REPO}" add -f -- "${DISPATCH_FIXTURE#"${SCOPE_REPO}"/}/finding.env"
dispatch_output="$(cd "${SCOPE_REPO}" && bash bin/checkin-lint.sh 2>&1)" || dispatch_exit=$?
dispatch_exit="${dispatch_exit:-0}"
if [[ "${dispatch_exit}" -eq 0 ]] && ! grep -Fq "${DISPATCH_FIXTURE#"${REPO_ROOT}"/}" <<< "${dispatch_output}"; then
  pass "files under .dispatch/worktrees are not scanned"
else
  fail "linter scanned a file under .dispatch/worktrees"
fi
git -C "${SCOPE_REPO}" reset -q -- "${DISPATCH_FIXTURE#"${SCOPE_REPO}"/}/finding.env"
rm -rf "${DISPATCH_FIXTURE}"

# ── test_baseline_has_no_worktree_paths ──────────────────────────────────────
# The committed baseline must contain ZERO paths matching \.worktrees/ or
# \.claude/worktrees/ — the hard gate from the #1093 incident report.
echo ""
echo "test_baseline_has_no_worktree_paths"
BASELINE="${REPO_ROOT}/bin/checkin-lint.baseline"
if [[ ! -f "${BASELINE}" ]]; then
  fail "bin/checkin-lint.baseline does not exist"
else
  bad_count=$(grep -cE '\.worktrees/|\.claude/worktrees/' "${BASELINE}" 2>/dev/null || true)
  if [[ "${bad_count}" -gt 0 ]]; then
    fail "baseline contains ${bad_count} ephemeral worktree path(s) — hard gate violation"
    grep -m5 -E '\.worktrees/|\.claude/worktrees/' "${BASELINE}" | sed 's/^/  /'
  else
    pass "bin/checkin-lint.baseline contains zero worktree paths (grep exit 1 = clean)"
  fi
fi

# ── test_duplicate_cardinality_change_keeps_baseline ─────────────────────────
# Multiset semantics: a baselined unique line stays suppressed when a second
# identical line appears elsewhere in the file (1→2 must not un-baseline the
# old occurrence; the NEW occurrence must go live as a real finding).
echo ""
echo "test_duplicate_cardinality_change_keeps_baseline"
mkdir -p "${TEST_FIXTURE}"
fixture_file="${TEST_FIXTURE}/dup.env"
matched_line='DATABASE_URL=postgres://localhost/engram-dup'
printf '%s\n' "${matched_line}" > "${fixture_file}"
fixture_rel="./${fixture_file#"${SCOPE_REPO}"/}"
content_hash="$(printf '%s' "${matched_line}" | sha1sum | awk '{print $1}')"
fixture_baseline="$(mktemp)"
cp "${REPO_ROOT}/bin/checkin-lint.baseline" "${fixture_baseline}"
printf 'P1.hardcoded-dsn::%s::%s\n' "${fixture_rel}" "${content_hash}" >> "${fixture_baseline}"

printf '%s\nUNRELATED=1\n%s\n' "${matched_line}" "${matched_line}" > "${fixture_file}"
git -C "${SCOPE_REPO}" add -- "${fixture_file#"${SCOPE_REPO}"/}"
dup_output="$(cd "${SCOPE_REPO}" && CHECKIN_LINT_BASELINE="${fixture_baseline}" bash bin/checkin-lint.sh 2>&1)" || dup_exit=$?
dup_exit="${dup_exit:-0}"
rm -f "${fixture_baseline}"
dup_plain="$(sed 's/\x1b\[[0-9;]*m//g' <<< "${dup_output}")"
baselined_n="$(grep -c "^baselined .*${fixture_rel}" <<< "${dup_plain}" || true)"
live_n="$(grep -c "^FINDING .*${fixture_rel}" <<< "${dup_plain}" || true)"
if [[ "${dup_exit}" -ne 0 && "${baselined_n}" -eq 1 && "${live_n}" -eq 1 ]]; then
  pass "1→2 duplication: old occurrence stays baselined, new occurrence goes live"
else
  fail "duplicate-cardinality change mishandled (exit ${dup_exit}, baselined=${baselined_n}, live=${live_n}; want fail/1/1)"
  grep -E "${fixture_rel}" <<< "${dup_plain}" | head -5 | sed 's/^/  /'
fi
git -C "${SCOPE_REPO}" reset -q -- "${fixture_file#"${SCOPE_REPO}"/}"
rm -rf "${TEST_FIXTURE}"

# ── test_migration_preserves_unresolved_entries ──────────────────────────────
# A legacy line-keyed entry whose file no longer exists must be preserved
# unmigrated (and exit 1), never silently dropped.
echo ""
echo "test_migration_preserves_unresolved_entries"
mig_baseline="$(mktemp)"
printf 'P1.hardcoded-dsn::./no/such/file.env::42\n' > "${mig_baseline}"
mig_out="$(cd "${REPO_ROOT}" && bash bin/migrate-checkin-lint-baseline.sh "${mig_baseline}" 2>&1)" || mig_exit=$?
mig_exit="${mig_exit:-0}"
if [[ "${mig_exit}" -eq 1 ]] && grep -Fxq 'P1.hardcoded-dsn::./no/such/file.env::42' "${mig_baseline}"; then
  pass "migration preserves unresolved entries and exits 1"
else
  fail "migration dropped an unresolved entry or exited ${mig_exit} (want 1)"
  echo "${mig_out}" | tail -3 | sed 's/^/  /'
fi
rm -f "${mig_baseline}"

# ── test_clean_tree_passes ────────────────────────────────────────────────────
# Running the linter on the current tree (which should match what a clean
# checkout of this PR looks like) must exit 0 — no net-new findings beyond the
# regenerated baseline.
echo ""
echo "test_clean_tree_passes"
lint_output="$(cd "${SCOPE_REPO}" && bash bin/checkin-lint.sh 2>&1)" || lint_exit=$?
lint_exit="${lint_exit:-0}"
if [[ "${lint_exit}" -eq 0 ]]; then
  pass "linter exits 0 on current tree — 0 net-new findings"
else
  # Extract any non-baselined findings for the failure message
  net_new=$(echo "$lint_output" | sed 's/\x1b\[[0-9;]*m//g' | grep '^FINDING' || true)
  fail "linter exited ${lint_exit} — net-new findings detected"
  echo "$net_new" | head -10 | sed 's/^/  /'
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "────────────────────────────────────────"
total=$((PASS + FAIL))
if [[ "${FAIL}" -eq 0 ]]; then
  echo -e "${GRN}${BOLD}All ${total} smoke tests passed.${RST}"
  exit 0
else
  echo -e "${RED}${BOLD}${FAIL}/${total} smoke test(s) FAILED.${RST}"
  exit 1
fi
