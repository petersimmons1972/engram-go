#!/usr/bin/env bats
# Tests for codex-poll.sh post_run_verify() — Protocol 19 PR Packaging enforcement
#
# Protocol 19: every CODEX REPORT PR must include a "## PR Packaging" section.
# post_run_verify() must:
#   - Apply agent/codex/done when PR body contains "## PR Packaging"
#   - Apply agent/codex/stalled + needs-fix/pr-packaging label + post comment
#     when the section is absent

SCRIPT="/tmp/hc-proto19/scripts/codex-poll.sh"

# ---------------------------------------------------------------------------
# Test infrastructure
# ---------------------------------------------------------------------------

setup_file() {
    export MOCK_DIR
    MOCK_DIR="$(mktemp -d)"
    export MOCK_BIN="${MOCK_DIR}/bin"
    mkdir -p "${MOCK_BIN}"
    export MOCK_STATE="${MOCK_DIR}/state"
    mkdir -p "${MOCK_STATE}"
    export MOCK_CACHE="${MOCK_DIR}/cache/config"
    mkdir -p "${MOCK_CACHE}"
    printf 'petersimmons1972/homelab-config\n' > "${MOCK_CACHE}/target-repos.txt"
}

teardown_file() {
    rm -rf "${MOCK_DIR}"
}

setup() {
    rm -f "${MOCK_DIR}/gh-calls"
    rm -f "${MOCK_DIR}/gh-config"
    # Defaults: branch present, PR present, body has ## PR Packaging
    printf 'branch=exists\npr=1\nbody=has_packaging\n' > "${MOCK_DIR}/gh-config"
    _write_mock_gh
}

# ---------------------------------------------------------------------------
# Mock gh binary
# Records every call to $MOCK_DIR/gh-calls.
# Returns canned responses based on $MOCK_DIR/gh-config.
# ---------------------------------------------------------------------------

_write_mock_gh() {
    # MOCK_DIR must be baked in at write time; it's exported so we reference it
    # via the env inside the script.
    cat > "${MOCK_BIN}/gh" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${MOCK_DIR}/gh-calls"

branch_mode=$(grep '^branch=' "${MOCK_DIR}/gh-config" | cut -d= -f2)
pr_count=$(grep '^pr=' "${MOCK_DIR}/gh-config" | cut -d= -f2)
body_mode=$(grep '^body=' "${MOCK_DIR}/gh-config" | cut -d= -f2)

subcmd="$1 $2"
case "${subcmd}" in
  "api repos"*)
    if [[ "${branch_mode}" == "exists" ]]; then
      printf '{"ref":"refs/heads/test"}\n'; exit 0
    else
      exit 1
    fi
    ;;
  "pr list")
    if [[ "${pr_count}" -gt 0 ]]; then
      printf '[{"number":101}]\n'
    else
      printf '[]\n'
    fi
    exit 0
    ;;
  "pr view")
    if [[ "${body_mode}" == "has_packaging" ]]; then
      printf '## PR Packaging\nscope_class: hotfix\nmerge_order: 1\n'
    else
      printf 'Some PR description without the required section.\n'
    fi
    exit 0
    ;;
  "pr edit")
    exit 0
    ;;
  "issue edit")
    exit 0
    ;;
  "issue comment"|"pr comment")
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
MOCK
    chmod +x "${MOCK_BIN}/gh"
}

# Run post_run_verify in a subprocess with the mock gh injected.
# $1: body mode (has_packaging | missing_packaging)
# $2: branch mode (exists | missing)   default: exists
# $3: pr count                          default: 1
_run_post_verify() {
    local body_mode="${1:-has_packaging}"
    local branch_mode="${2:-exists}"
    local pr_count="${3:-1}"

    printf 'branch=%s\npr=%s\nbody=%s\n' \
        "${branch_mode}" "${pr_count}" "${body_mode}" > "${MOCK_DIR}/gh-config"
    _write_mock_gh

    env \
        PATH="${MOCK_BIN}:${PATH}" \
        MOCK_DIR="${MOCK_DIR}" \
        CLAUDE_CODEX_CACHE="${MOCK_DIR}/cache" \
        TARGET_REPOS_FILE="${MOCK_CACHE}/target-repos.txt" \
        PROJECTS_ROOT="${MOCK_DIR}/projects" \
        WORKTREES_ROOT="${MOCK_DIR}/worktrees" \
        STATE_DIR="${MOCK_STATE}" \
        LOG_FILE="${MOCK_STATE}/poll.log" \
        LOCK_FILE="${MOCK_STATE}/poll.lock" \
        TICK_FILE="${MOCK_STATE}/tick" \
        DAILY_STAMP="${MOCK_STATE}/last-daily" \
    bash -c '
        # Strip the final "main "$@"" dispatch line so sourcing only loads
        # function definitions without triggering a live GitHub API call.
        _defs=$(mktemp /tmp/codex-poll-defs-XXXXXX.sh)
        grep -v "^main \"\$@\"" "'"${SCRIPT}"'" > "${_defs}"
        set +euo pipefail
        source "${_defs}"
        rm -f "${_defs}"
        set -euo pipefail
        sleep 0 &
        fake_pid=$!
        wait "${fake_pid}" 2>/dev/null || true
        post_run_verify \
            "${fake_pid}" \
            "petersimmons1972/homelab-config" \
            "42" \
            "agent/codex/issue-42-poll" \
            "/tmp/fake-codex.log" \
            2>&1
    '
}

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

@test "pr_packaging_present_applies_done_label" {
    run _run_post_verify "has_packaging" "exists" "1"
    [ "${status}" -eq 0 ]
    [[ "${output}" == *"done"* ]] \
        || { echo "Expected 'done' in output, got: ${output}"; return 1; }
    grep -q "issue edit" "${MOCK_DIR}/gh-calls" \
        || { echo "gh issue edit was not called"; return 1; }
    grep "issue edit" "${MOCK_DIR}/gh-calls" | grep -q "agent/codex/done" \
        || { echo "done label was not applied"; return 1; }
}

@test "pr_packaging_present_does_not_apply_stalled" {
    run _run_post_verify "has_packaging" "exists" "1"
    [ "${status}" -eq 0 ]
    ! grep "issue edit" "${MOCK_DIR}/gh-calls" | grep -q "agent/codex/stalled" \
        || { echo "stalled label incorrectly applied when PR Packaging present"; return 1; }
}

@test "pr_packaging_missing_applies_stalled_label" {
    run _run_post_verify "missing_packaging" "exists" "1"
    [ "${status}" -eq 0 ]
    grep "issue edit" "${MOCK_DIR}/gh-calls" | grep -q "agent/codex/stalled" \
        || { echo "stalled label not applied when ## PR Packaging absent"; return 1; }
    ! grep "issue edit" "${MOCK_DIR}/gh-calls" | grep -q "agent/codex/done" \
        || { echo "done label incorrectly applied when ## PR Packaging absent"; return 1; }
}

@test "pr_packaging_missing_applies_needs_fix_label_on_pr" {
    run _run_post_verify "missing_packaging" "exists" "1"
    [ "${status}" -eq 0 ]
    grep "pr edit" "${MOCK_DIR}/gh-calls" | grep -q "needs-fix/pr-packaging" \
        || { echo "needs-fix/pr-packaging label not added to PR"; return 1; }
}

@test "pr_packaging_missing_logs_warning" {
    run _run_post_verify "missing_packaging" "exists" "1"
    [ "${status}" -eq 0 ]
    [[ "${output}" == *"PR Packaging"* || "${output}" == *"pr-packaging"* ]] \
        || { echo "Expected PR Packaging warning, got: ${output}"; return 1; }
}

@test "pr_packaging_missing_posts_comment" {
    run _run_post_verify "missing_packaging" "exists" "1"
    [ "${status}" -eq 0 ]
    { grep -q "pr comment" "${MOCK_DIR}/gh-calls" || grep -q "issue comment" "${MOCK_DIR}/gh-calls"; } \
        || { echo "No comment posted when ## PR Packaging absent"; return 1; }
}

@test "no_remote_branch_applies_stalled_regardless_of_packaging" {
    run _run_post_verify "has_packaging" "missing" "0"
    [ "${status}" -eq 0 ]
    grep "issue edit" "${MOCK_DIR}/gh-calls" | grep -q "agent/codex/stalled" \
        || { echo "stalled not applied when remote branch absent"; return 1; }
}

@test "no_pr_applies_stalled_and_skips_packaging_check" {
    run _run_post_verify "has_packaging" "exists" "0"
    [ "${status}" -eq 0 ]
    grep "issue edit" "${MOCK_DIR}/gh-calls" | grep -q "agent/codex/stalled" \
        || { echo "stalled not applied when no open PR"; return 1; }
    ! grep -q "^pr view" "${MOCK_DIR}/gh-calls" \
        || { echo "gh pr view called when no PR exists — unnecessary API call"; return 1; }
}

@test "prompt_template_contains_pr_packaging_requirement" {
    grep -q "PR Packaging" "${SCRIPT}" \
        || { echo "Script missing '## PR Packaging' requirement in prompt template"; return 1; }
    grep -q "scope_class" "${SCRIPT}" \
        || { echo "Script missing scope_class field in prompt template"; return 1; }
}

@test "prompt_template_contains_all_required_fields" {
    local field
    for field in scope_class merge_order depends_on conflict_surface risk rollback; do
        grep -q "${field}" "${SCRIPT}" \
            || { echo "Script missing Protocol 19 field: ${field}"; return 1; }
    done
}
