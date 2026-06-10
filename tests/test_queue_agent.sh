#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
QUEUE_AGENT="${ROOT_DIR}/bin/queue-agent"

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_eq() {
  local expected="$1" actual="$2" message="$3"
  [[ "$expected" == "$actual" ]] || fail "${message}: expected '${expected}', got '${actual}'"
}

assert_contains() {
  local needle="$1" file="$2"
  grep -Fq "$needle" "$file" || fail "expected '${needle}' in ${file}"
}

assert_not_contains() {
  local needle="$1" file="$2"
  if grep -Fq "$needle" "$file"; then
    fail "did not expect '${needle}' in ${file}"
  fi
}

run_queue_agent() {
  local temp_root="$1"
  shift

  local fake_bin="${temp_root}/bin"
  local gh_log="${temp_root}/gh.log"
  local body_capture="${temp_root}/body.txt"
  mkdir -p "$fake_bin"

  cat > "${fake_bin}/gh" <<'EOF_GH'
#!/usr/bin/env bash
set -euo pipefail

printf '%s\n' "$*" >> "$GH_LOG"

body_file=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --body-file)
      body_file="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [[ -n "${body_file}" ]]; then
  cp "${body_file}" "$BODY_CAPTURE"
fi

echo "https://github.com/example/issue/123"
EOF_GH
  chmod +x "${fake_bin}/gh"

  PATH="${fake_bin}:${PATH}" \
    GH_LOG="${gh_log}" \
    BODY_CAPTURE="${body_capture}" \
    GITHUB_OWNER="petersimmons1972" \
    CLAUDE_CODEX_CACHE="${temp_root}/cache" \
    HOME="${temp_root}/home" \
    "$QUEUE_AGENT" "$@"
}

make_brief() {
  local path="$1"
  cat > "$path" <<'EOF_BRIEF'
## Files
- bin/queue-agent

## Acceptance (TDD)
- target repo smoke

## Spec refs
- none

## Constraints
- none
EOF_BRIEF
}

test_target_repo_prepends_metadata() {
  local temp_root
  temp_root="$(mktemp -d)"
  trap 'rm -rf "$temp_root"' RETURN
  mkdir -p "${temp_root}/home" "${temp_root}/cache/config"
  printf 'petersimmons1972/homelab-config\n' > "${temp_root}/cache/config/target-repos.txt"
  local brief="${temp_root}/brief.md"
  make_brief "$brief"

  run_queue_agent "$temp_root" \
    --agent codex \
    --repo homelab-config \
    --title "target repo" \
    --brief "$brief" \
    --target-repo petersimmons1972/codex >/dev/null

  mapfile -t lines < "${temp_root}/body.txt"
  assert_eq "Target-Repo: petersimmons1972/codex" "${lines[0]}" "target repo header"
  assert_contains "Filed by queue-agent on" "${temp_root}/body.txt"
}

test_invalid_target_repo_rejected() {
  local temp_root
  temp_root="$(mktemp -d)"
  trap 'rm -rf "$temp_root"' RETURN
  mkdir -p "${temp_root}/home" "${temp_root}/cache/config"
  printf 'petersimmons1972/homelab-config\n' > "${temp_root}/cache/config/target-repos.txt"
  local brief="${temp_root}/brief.md"
  make_brief "$brief"

  set +e
  output="$(
    run_queue_agent "$temp_root" \
      --agent codex \
      --repo homelab-config \
      --title "bad target repo" \
      --brief "$brief" \
      --target-repo codex 2>&1
  )"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "expected invalid target repo to fail"
  [[ ! -f "${temp_root}/body.txt" ]] || fail "gh should not be called for invalid target repo"
  [[ "$output" == *"--target-repo must be in owner/name format"* ]] || fail "missing target repo validation error"
}

test_default_body_unchanged_without_target_repo() {
  local temp_root
  temp_root="$(mktemp -d)"
  trap 'rm -rf "$temp_root"' RETURN
  mkdir -p "${temp_root}/home" "${temp_root}/cache/config"
  printf 'petersimmons1972/homelab-config\n' > "${temp_root}/cache/config/target-repos.txt"
  local brief="${temp_root}/brief.md"
  make_brief "$brief"

  run_queue_agent "$temp_root" \
    --agent codex \
    --repo homelab-config \
    --title "no target repo" \
    --brief "$brief" >/dev/null

  first_line="$(head -n 1 "${temp_root}/body.txt")"
  [[ "$first_line" == Filed\ by\ queue-agent\ on* ]] || fail "body should start with Filed by queue-agent"
  assert_not_contains "Target-Repo:" "${temp_root}/body.txt"
}

test_target_repo_prepends_metadata
test_invalid_target_repo_rejected
test_default_body_unchanged_without_target_repo
