#!/usr/bin/env bash
# checkin-lint-core.sh — universal automatable failure-mode checks.
# Source this from a project's bin/checkin-lint.sh — never run directly.
#
# Caller MUST set before sourcing:
#   FINDINGS=0
#   EXPECTED_REMOTE="petersimmons1972/<project>"
#   CHECKIN_K8S=0   # or 1
#   export EXPECTED_REMOTE CHECKIN_K8S
#
# Then call: run_core_checks "$@"
#
# Exports: section finding pass_rule hint run_core_checks
# K8s checks (G.*) only run when CHECKIN_K8S=1.

RED='\033[0;31m'; YLW='\033[1;33m'; GRN='\033[0;32m'; BLU='\033[0;34m'
BOLD='\033[1m'; RST='\033[0m'
_CORE_FIX_HINTS=0

section()   { echo -e "\n${BOLD}${BLU}── $* ──${RST}"; }
pass_rule() { echo -e "${GRN}ok${RST}  [$1] $2"; }
hint()      { [[ "${_CORE_FIX_HINTS:-0}" -eq 1 ]] && echo -e "    ${YLW}hint:${RST} $*" || true; }
finding() {
  local rule="$1" file="$2" line="$3" why="$4"
  echo -e "${RED}FINDING${RST} [${BOLD}${rule}${RST}] ${file}:${line}  —  ${why}"
  ((FINDINGS++)) || true
}
export -f section pass_rule hint finding

_check_remote_guard() {
  section "F.remote-guard (FM-12)"
  local expected="${EXPECTED_REMOTE:-}"
  if [[ -z "$expected" ]]; then
    echo -e "${YLW}SKIP${RST}  [F.remote-guard] EXPECTED_REMOTE not set"; return
  fi
  local repo_root
  repo_root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
  if [[ -z "$repo_root" ]]; then
    echo -e "${RED}ERROR${RST}: not inside a git repository." >&2; exit 2
  fi
  local remote_url
  remote_url="$(git -C "$repo_root" remote get-url origin 2>/dev/null || true)"
  if echo "$remote_url" | grep -q "$expected"; then
    pass_rule "F.remote-guard" "origin matches $expected"
  else
    echo -e "${RED}ERROR${RST}: origin does not match $expected (got: ${remote_url:-<none>})." >&2
    echo "       Running in the wrong repo? Refusing to lint." >&2
    exit 2
  fi
}

_check_home_literal() {
  section "C.home-literal (FM-06)"
  local n=0
  while IFS= read -r hit; do
    local file="${hit%%:*}" rest="${hit#*:}"
    local lineno="${rest%%:*}" match="${rest#*:}"
    echo "$match" | grep -qE '/home/(user|runner)([/[:space:]]|$|["\x27])' && continue
    finding "C.home-literal" "$file" "$lineno" \
      "hardcoded /home/<user> path — use %h / __HOME__ / env-var injection"
    hint "In systemd units: %h expands in ExecStart but NOT in Environment= strings."
    ((n++)) || true
  done < <(grep -rn \
    --include='*.yaml' --include='*.yml' --include='*.sh' \
    --include='*.conf' --include='*.service' --include='*.toml' --include='*.env' \
    --exclude-dir='.git' --exclude-dir='.claude' \
    --exclude='checkin-lint*.sh' --exclude='test-checkin-lint*.sh' \
    '/home/[a-z][a-z0-9_-]*' . 2>/dev/null || true)
  [[ $n -eq 0 ]] && pass_rule "C.home-literal" "no hardcoded /home/<user> paths"
}

_check_version_pinned_path() {
  section "C.version-pinned-path (FM-08)"
  local n=0
  while IFS= read -r hit; do
    local file="${hit%%:*}" rest="${hit#*:}"
    local lineno="${rest%%:*}"
    finding "C.version-pinned-path" "$file" "$lineno" \
      "version-pinned tool path breaks on upgrade — use 'command -v' or a non-versioned symlink"
    hint "Replace with: \$(command -v node)  or add a non-versioned directory to PATH."
    ((n++)) || true
  done < <(grep -rn \
    --include='*.yaml' --include='*.yml' --include='*.sh' \
    --include='*.conf' --include='*.service' \
    --exclude-dir='.git' --exclude-dir='.claude' \
    --exclude='checkin-lint*.sh' \
    -E '(\.nvm/versions/node/v[0-9]|/versions/node/v[0-9]|/node/v[0-9]+\.[0-9]+\.[0-9]+[^-])' \
    . 2>/dev/null || true)
  [[ $n -eq 0 ]] && pass_rule "C.version-pinned-path" "no version-pinned tool paths"
}

_check_exit_zero_wrapper() {
  section "D.exit-zero-wrapper (FM-18)"
  local n=0
  while IFS= read -r hit; do
    local file="${hit%%:*}" rest="${hit#*:}"
    local lineno="${rest%%:*}" match="${rest#*:}"
    local trimmed
    trimmed="$(echo "$match" | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*#.*//')"
    [[ "$trimmed" != "exit 0" ]] && continue
    finding "D.exit-zero-wrapper" "$file" "$lineno" \
      "unconditional 'exit 0' may mask child-process failure (FM-18)"
    hint "Wrappers must propagate child exit code. Use: exec child_cmd  or  child_cmd; exit \$?"
    ((n++)) || true
  done < <(grep -rn \
    --include='*.sh' \
    --exclude-dir='.git' --exclude-dir='.claude' \
    --exclude='checkin-lint*.sh' --exclude='test-checkin-lint*.sh' \
    'exit 0' . 2>/dev/null || true)
  [[ $n -eq 0 ]] && pass_rule "D.exit-zero-wrapper" "no unconditional 'exit 0' in shell scripts"
}

_check_latest_image() {
  section "G.latest-image (FM-15)"
  local n=0
  while IFS= read -r hit; do
    local file="${hit%%:*}" rest="${hit#*:}"
    local lineno="${rest%%:*}"
    finding "G.latest-image" "$file" "$lineno" \
      "':latest' image tag — pin to digest or immutable tag (FM-15)"
    hint "Replace ':latest' with a digest: image: registry/name@sha256:..."
    ((n++)) || true
  done < <(grep -rn \
    --include='*.yaml' --include='*.yml' \
    --exclude-dir='.git' \
    -E 'image:\s+[^@\s]+:latest' . 2>/dev/null || true)
  [[ $n -eq 0 ]] && pass_rule "G.latest-image" "no ':latest' image tags"
}

_check_hardcoded_ip() {
  section "G.hardcoded-ip (FM-16)"
  local n=0
  while IFS= read -r hit; do
    local file="${hit%%:*}" rest="${hit#*:}"
    local lineno="${rest%%:*}"
    grep -q 'kind: NetworkPolicy' "$file" 2>/dev/null || continue
    finding "G.hardcoded-ip" "$file" "$lineno" \
      "hardcoded IP in NetworkPolicy — use podSelector/namespaceSelector (FM-16)"
    hint "Replace ipBlock with: to: - podSelector: matchLabels: app: <name>"
    ((n++)) || true
  done < <(grep -rn \
    --include='*.yaml' --include='*.yml' \
    --exclude-dir='.git' \
    -E '(ip:|cidr:)\s+[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}' \
    . 2>/dev/null || true)
  [[ $n -eq 0 ]] && pass_rule "G.hardcoded-ip" "no hardcoded IPs in NetworkPolicy"
}

_check_missing_namespace() {
  section "G.missing-namespace (FM-17)"
  local n=0
  local system_ns="default kube-system kube-public kube-node-lease"
  local referenced_ns defined_ns
  referenced_ns=$(grep -rh --include='*.yaml' --include='*.yml' \
    --exclude-dir='.git' -E '^\s+namespace:\s+\S+' . 2>/dev/null \
    | sed 's/.*namespace:[[:space:]]*//' | sort -u || true)
  defined_ns=$(grep -rhl 'kind: Namespace' --include='*.yaml' --include='*.yml' \
    --exclude-dir='.git' . 2>/dev/null \
    | xargs grep -h '^  name:' 2>/dev/null \
    | sed 's/.*name:[[:space:]]*//' | sort -u || true)
  while IFS= read -r ns; do
    [[ -z "$ns" ]] && continue
    echo "$system_ns" | grep -qw "$ns" && continue
    echo "$defined_ns" | grep -qx "$ns" && continue
    finding "G.missing-namespace" "(manifests)" "-" \
      "namespace '$ns' referenced but no Namespace manifest found (FM-17)"
    hint "Create 00-namespace.yaml with kind: Namespace, name: $ns in the same apply bundle."
    ((n++)) || true
  done <<< "$referenced_ns"
  [[ $n -eq 0 ]] && pass_rule "G.missing-namespace" "all referenced namespaces have manifests"
}

run_core_checks() {
  for arg in "$@"; do
    case "$arg" in --fix-hints) _CORE_FIX_HINTS=1 ;; esac
  done
  _check_remote_guard
  _check_home_literal
  _check_version_pinned_path
  _check_exit_zero_wrapper
  if [[ "${CHECKIN_K8S:-0}" -eq 1 ]]; then
    _check_latest_image
    _check_hardcoded_ip
    _check_missing_namespace
  fi
}
export -f run_core_checks _check_home_literal _check_version_pinned_path
export -f _check_exit_zero_wrapper _check_latest_image _check_hardcoded_ip
export -f _check_missing_namespace _check_remote_guard
