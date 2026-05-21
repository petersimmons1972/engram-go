#!/usr/bin/env bash
# =============================================================================
# test-hermes-deployment.sh
# TDD suite for Hermes Agent deployment on hermes.petersimmons.com
#
# Usage:
#   ./test-hermes-deployment.sh           # run all tests
#   ./test-hermes-deployment.sh --stage 1  # run only stage 1 tests
#   ./test-hermes-deployment.sh --fast     # skip slow Docker build tests
#
# Each test maps to a numbered stage (matching the plan tasks).
# Tests are designed to fail before implementation and pass after.
# Exit code: 0 = all pass, 1 = one or more failures.
# =============================================================================

set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
HOST="hermes.petersimmons.com"
SSH_KEY="$HOME/.ssh/hermes_agent_key"
SSH_PSIMMONS="ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 psimmons@${HOST}"
SSH_HERMES="ssh -i ${SSH_KEY} -o StrictHostKeyChecking=no -o ConnectTimeout=10 hermes@${HOST}"
HERMES_UID=65532
INFISICAL_PROJECT_ID="40891bed-df74-4967-a556-ab5bfff2d654"

# ── Args ──────────────────────────────────────────────────────────────────────
STAGE_FILTER=""
FAST_MODE=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --stage) STAGE_FILTER="$2"; shift 2 ;;
    --fast)  FAST_MODE=true; shift ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

# ── Output helpers ────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

pass() { echo -e "  ${GREEN}✓ PASS${RESET}  $1"; (( ++PASS_COUNT )); }
fail() { echo -e "  ${RED}✗ FAIL${RESET}  $1"; (( ++FAIL_COUNT )); }
skip() { echo -e "  ${YELLOW}⊘ SKIP${RESET}  $1"; (( ++SKIP_COUNT )); }

stage() {
  local num="$1"; shift
  if [[ -n "$STAGE_FILTER" && "$STAGE_FILTER" != "$num" ]]; then return 0; fi
  echo ""
  echo -e "${CYAN}${BOLD}── Stage ${num}: $* ──────────────────────────────────────────${RESET}"
}

assert_ssh() {
  # assert_ssh <label> <user: psimmons|hermes> <command> <expected_pattern>
  local label="$1" user="$2" cmd="$3" pattern="$4"
  local ssh_cmd
  if [[ "$user" == "hermes" ]]; then ssh_cmd="$SSH_HERMES"
  else ssh_cmd="$SSH_PSIMMONS"; fi
  local out
  out=$($ssh_cmd "$cmd" 2>&1) || true
  if echo "$out" | grep -qE "$pattern"; then
    pass "$label"
  else
    fail "$label — got: $(echo "$out" | head -1)"
  fi
}

assert_ssh_fail() {
  # assert_ssh_fail <label> <user> <command> <pattern that should NOT match>
  local label="$1" user="$2" cmd="$3" anti_pattern="$4"
  local ssh_cmd
  if [[ "$user" == "hermes" ]]; then ssh_cmd="$SSH_HERMES"
  else ssh_cmd="$SSH_PSIMMONS"; fi
  local out
  out=$($ssh_cmd "$cmd" 2>&1) || true
  if echo "$out" | grep -qE "$anti_pattern"; then
    fail "$label — matched forbidden pattern: $anti_pattern"
  else
    pass "$label"
  fi
}

assert_local() {
  local label="$1" cmd="$2" pattern="$3"
  local out
  out=$(eval "$cmd" 2>&1) || true
  if echo "$out" | grep -qE "$pattern"; then
    pass "$label"
  else
    fail "$label — got: $(echo "$out" | head -1)"
  fi
}

# =============================================================================
# STAGE 1: Infisical project and Engram seeded
# =============================================================================
stage 1 "Infisical + Engram bootstrap"

# Test 1.1: SSH reachability (prereq for everything)
assert_ssh "1.1 psimmons SSH reachable" "psimmons" "echo REACHABLE" "REACHABLE"

# Test 1.2: Local SSH key exists
assert_local "1.2 hermes SSH private key exists locally" \
  "test -f $HOME/.ssh/hermes_agent_key && echo EXISTS" "EXISTS"

# Test 1.3: Local SSH key is ed25519
assert_local "1.3 hermes SSH key is ed25519" \
  "ssh-keygen -l -f $HOME/.ssh/hermes_agent_key" "ED25519|256"

# =============================================================================
# STAGE 2: hermes user created on remote host
# =============================================================================
stage 2 "hermes system user"

# Test 2.1: hermes user exists
assert_ssh "2.1 hermes user exists" "psimmons" \
  "id hermes 2>/dev/null" "hermes"

# Test 2.2: hermes UID is exactly 65532
assert_ssh "2.2 hermes UID is 65532 (Chainguard nonroot)" "psimmons" \
  "id -u hermes" "^65532$"

# Test 2.3: hermes GID is 65532
assert_ssh "2.3 hermes GID is 65532" "psimmons" \
  "id -g hermes" "^65532$"

# Test 2.4: hermes home directory exists
assert_ssh "2.4 hermes home /home/hermes exists" "psimmons" \
  "test -d /home/hermes && echo EXISTS" "EXISTS"

# Test 2.5: hermes has NO passwordless sudo
assert_ssh "2.5 hermes has no NOPASSWD sudo (not in sudoers)" "psimmons" \
  "sudo grep -r hermes /etc/sudoers /etc/sudoers.d/ 2>/dev/null || echo CLEAN" "CLEAN"

# Test 2.6: hermes not in sudo group
assert_ssh_fail "2.6 hermes not in sudo group" "psimmons" \
  "groups hermes" "sudo"

# =============================================================================
# STAGE 3: SSH keypair installed
# =============================================================================
stage 3 "SSH keypair"

# Test 3.1: hermes SSH login works with certificate
assert_ssh "3.1 SSH as hermes works (cert auth)" "hermes" \
  "echo HERMES_OK" "HERMES_OK"

# Test 3.2: hermes SSH confirms UID 65532
assert_ssh "3.2 hermes SSH session has UID 65532" "hermes" \
  "id -u" "^65532$"

# Test 3.3: authorized_keys exists and is mode 600
assert_ssh "3.3 authorized_keys mode is 600" "hermes" \
  "stat -c '%a' ~/.ssh/authorized_keys" "^600$"

# Test 3.4: .ssh directory is mode 700
assert_ssh "3.4 .ssh directory mode is 700" "hermes" \
  "stat -c '%a' ~/.ssh" "^700$"

# Test 3.5: authorized_keys owned by hermes
assert_ssh "3.5 authorized_keys owned by hermes (uid 65532)" "hermes" \
  "stat -c '%u' ~/.ssh/authorized_keys" "^65532$"

# =============================================================================
# STAGE 4: Docker installed
# =============================================================================
stage 4 "Docker CE"

# Test 4.1: docker binary present
assert_ssh "4.1 docker binary installed" "psimmons" \
  "which docker" "docker"

# Test 4.2: docker daemon running
assert_ssh "4.2 docker daemon active (systemctl)" "psimmons" \
  "sudo systemctl is-active docker" "^active$"

# Test 4.3: hermes user is in docker group
assert_ssh "4.3 hermes user in docker group" "psimmons" \
  "groups hermes" "docker"

# Test 4.4: hermes can run docker (group membership active)
assert_ssh "4.4 hermes can run docker hello-world" "hermes" \
  "docker run --rm hello-world 2>&1" "Hello from Docker"

# Test 4.5: docker compose plugin installed
assert_ssh "4.5 docker compose plugin available" "psimmons" \
  "docker compose version" "Docker Compose"

# =============================================================================
# STAGE 5: Chainguard image built
# =============================================================================
stage 5 "Chainguard Docker image"

if [[ "$FAST_MODE" == "true" ]]; then
  skip "5.x image build tests (--fast mode)"
else
  # Test 5.1: Dockerfile exists in deploy dir
  assert_ssh "5.1 Dockerfile exists at /home/hermes/deploy/Dockerfile" "hermes" \
    "test -f /home/hermes/deploy/Dockerfile && echo EXISTS" "EXISTS"

  # Test 5.2: hermes-agent image built
  assert_ssh "5.2 hermes-agent:latest image exists" "hermes" \
    "docker images hermes-agent:latest --format '{{.Repository}}'" "hermes-agent"

  # Test 5.3: Image runs as UID 65532
  assert_ssh "5.3 container runs as UID 65532" "hermes" \
    "docker run --rm hermes-agent:latest id" "65532"

  # Test 5.4: hermes binary present in image
  assert_ssh "5.4 hermes binary present in image" "hermes" \
    "docker run --rm hermes-agent:latest hermes --version" "hermes|version|[0-9]+\."

  # Test 5.5: Image base is wolfi/chainguard (no debian/ubuntu shell artifacts)
  assert_ssh "5.5 image uses apk (wolfi-base, not apt)" "hermes" \
    "docker run --rm hermes-agent:latest sh -c 'which apk && echo APK_OK'" "APK_OK"

  # Test 5.6: tini present as PID 1 entrypoint
  assert_ssh "5.6 tini available as /sbin/tini in image" "hermes" \
    "docker run --rm hermes-agent:latest ls /sbin/tini" "/sbin/tini"
fi

# =============================================================================
# STAGE 6: docker-compose.yaml written
# =============================================================================
stage 6 "docker-compose configuration"

# Test 6.1: compose file exists
assert_ssh "6.1 docker-compose.yaml exists" "hermes" \
  "test -f /home/hermes/deploy/docker-compose.yaml && echo EXISTS" "EXISTS"

# Test 6.2: compose file declares no-new-privileges
assert_ssh "6.2 compose declares no-new-privileges security opt" "hermes" \
  "grep -c 'no-new-privileges' /home/hermes/deploy/docker-compose.yaml" "^[1-9]"

# Test 6.3: compose declares cap_drop ALL
assert_ssh "6.3 compose declares cap_drop ALL" "hermes" \
  "grep -c 'ALL' /home/hermes/deploy/docker-compose.yaml" "^[1-9]"

# Test 6.4: compose does NOT bind 0.0.0.0 for any port
assert_ssh_fail "6.4 compose does NOT expose ports on 0.0.0.0" "hermes" \
  "grep -E '^.*- [\"0-9]' /home/hermes/deploy/docker-compose.yaml || echo CLEAN" \
  "- \"[0-9]+:|    - [0-9]+:"

# Test 6.5: dashboard bound to 127.0.0.1 only
assert_ssh "6.5 dashboard port bound to 127.0.0.1 only" "hermes" \
  "grep '127.0.0.1' /home/hermes/deploy/docker-compose.yaml" "127.0.0.1"

# Test 6.6: Docker GID env file present and mode 600
assert_ssh "6.6 compose .env exists and mode 600" "hermes" \
  "stat -c '%a' /home/hermes/deploy/.env 2>/dev/null || echo MISSING" "^600$"

# =============================================================================
# STAGE 7: Hermes Agent configured
# =============================================================================
stage 7 "Hermes Agent configuration"

# Test 7.1: .hermes directory exists, mode 700
assert_ssh "7.1 ~/.hermes dir exists and is mode 700" "hermes" \
  "stat -c '%a' /home/hermes/.hermes" "^700$"

# Test 7.2: config.yaml exists
assert_ssh "7.2 config.yaml exists" "hermes" \
  "test -f /home/hermes/.hermes/config.yaml && echo EXISTS" "EXISTS"

# Test 7.3: Docker backend configured
assert_ssh "7.3 config.yaml uses terminal.backend: docker" "hermes" \
  "grep 'backend: docker' /home/hermes/.hermes/config.yaml" "backend: docker"

# Test 7.4: approvals mode is manual (not off)
assert_ssh "7.4 approvals.mode is manual (not off)" "hermes" \
  "grep 'mode:' /home/hermes/.hermes/config.yaml" "manual"

# Test 7.5: allow_private_urls is false
assert_ssh "7.5 allow_private_urls is false" "hermes" \
  "grep 'allow_private_urls' /home/hermes/.hermes/config.yaml" "false"

# Test 7.6: .env exists and mode 600
assert_ssh "7.6 .hermes/.env exists and mode is 600" "hermes" \
  "stat -c '%a' /home/hermes/.hermes/.env 2>/dev/null || echo MISSING" "^600$"

# Test 7.7: .env NOT world-readable
assert_ssh_fail "7.7 .hermes/.env NOT world-readable" "hermes" \
  "stat -c '%a' /home/hermes/.hermes/.env 2>/dev/null || echo MISSING" "^6[46][46]$|^644$|^664$|^666$"

# Test 7.8: GATEWAY_ALLOW_ALL_USERS not set in .env (fail-closed)
assert_ssh_fail "7.8 GATEWAY_ALLOW_ALL_USERS not in .env (fail-closed)" "hermes" \
  "grep 'GATEWAY_ALLOW_ALL_USERS' /home/hermes/.hermes/.env 2>/dev/null || echo CLEAN" \
  "GATEWAY_ALLOW_ALL_USERS=true"

# =============================================================================
# STAGE 8: systemd service
# =============================================================================
stage 8 "systemd service"

# Test 8.1: service unit file exists
assert_ssh "8.1 hermes-agent.service unit file exists" "psimmons" \
  "test -f /etc/systemd/system/hermes-agent.service && echo EXISTS" "EXISTS"

# Test 8.2: service enabled (autostart)
assert_ssh "8.2 hermes-agent.service is enabled" "psimmons" \
  "sudo systemctl is-enabled hermes-agent.service" "enabled"

# Test 8.3: service running
assert_ssh "8.3 hermes-agent.service is active/running" "psimmons" \
  "sudo systemctl is-active hermes-agent.service" "^active$"

# Test 8.4: service runs as hermes user
assert_ssh "8.4 service unit sets User=hermes" "psimmons" \
  "grep 'User=' /etc/systemd/system/hermes-agent.service" "User=hermes"

# =============================================================================
# STAGE 9: Container runtime security
# =============================================================================
stage 9 "Container security verification"

# Test 9.1: container running
assert_ssh "9.1 hermes-agent container is running" "psimmons" \
  "sudo docker ps --filter name=hermes-agent --format '{{.Names}}'" "hermes-agent"

# Test 9.2: container UID is 65532
assert_ssh "9.2 running container process UID is 65532" "psimmons" \
  "sudo docker exec hermes-agent id -u" "^65532$"

# Test 9.3: no capabilities (CapEff all zeros)
assert_ssh "9.3 container has no effective capabilities (CapEff=0)" "psimmons" \
  "sudo docker exec hermes-agent sh -c 'cat /proc/1/status | grep CapEff'" \
  "CapEff:[[:space:]]*0+$"

# Test 9.4: NoNewPrivileges set on container process
assert_ssh "9.4 NoNewPrivileges set on container process" "psimmons" \
  "sudo docker exec hermes-agent sh -c 'cat /proc/1/status | grep NoNewPrivs'" \
  "NoNewPrivs:[[:space:]]*1"

# Test 9.5: container health status healthy
assert_ssh "9.5 container health status is healthy" "psimmons" \
  "sudo docker inspect hermes-agent --format '{{.State.Health.Status}}'" "healthy"

# =============================================================================
# STAGE 10: Network / firewall
# =============================================================================
stage 10 "Firewall and network isolation"

# Test 10.1: UFW is active
assert_ssh "10.1 UFW is active" "psimmons" \
  "sudo ufw status | head -1" "Status: active"

# Test 10.2: UFW default incoming is deny
assert_ssh "10.2 UFW default incoming: deny" "psimmons" \
  "sudo ufw status verbose | grep 'Default:'" "deny.*incoming"

# Test 10.3: Only SSH port open (no extra ports)
# (replaced by direct count test below)
# assert_ssh removed — empty grep pattern matches everything
# (empty pattern = no matches = pass — we test for absence)

# Better version: count non-SSH listeners on 0.0.0.0
# (passes if count is 0)
COUNT_CMD="sudo ss -tlnp | grep '0.0.0.0:' | grep -v ':22 ' | grep -v '127\.' | wc -l | tr -d ' '"
LISTENER_COUNT=$($SSH_PSIMMONS "$COUNT_CMD" 2>/dev/null || echo "999")
if [[ "$LISTENER_COUNT" == "0" ]]; then
  pass "10.3 no unexpected 0.0.0.0 listeners (non-SSH)"
else
  fail "10.3 unexpected external listeners found: $LISTENER_COUNT"
fi

# Test 10.4: Hermes dashboard not on 0.0.0.0
assert_ssh_fail "10.4 hermes dashboard NOT on 0.0.0.0:7788" "psimmons" \
  "sudo ss -tlnp | grep ':7788'" "0.0.0.0:7788"

# Test 10.5: SSH still reachable after UFW
assert_ssh "10.5 SSH still reachable with UFW active" "hermes" \
  "echo FIREWALL_OK" "FIREWALL_OK"

# =============================================================================
# Summary
# =============================================================================
echo ""
echo -e "${BOLD}═══════════════════════════════════════════════════════════${RESET}"
TOTAL=$(( PASS_COUNT + FAIL_COUNT + SKIP_COUNT ))
echo -e "${BOLD}  Test Summary:${RESET}  ${GREEN}${PASS_COUNT} passed${RESET}  ${RED}${FAIL_COUNT} failed${RESET}  ${YELLOW}${SKIP_COUNT} skipped${RESET}  (${TOTAL} total)"
echo -e "${BOLD}═══════════════════════════════════════════════════════════${RESET}"

if [[ $FAIL_COUNT -gt 0 ]]; then
  echo -e "${RED}  DEPLOYMENT NOT READY — ${FAIL_COUNT} gate(s) failed${RESET}"
  exit 1
else
  echo -e "${GREEN}  ALL GATES CLEAR${RESET}"
  exit 0
fi
