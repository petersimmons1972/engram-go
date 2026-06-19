#!/usr/bin/env bats
# Tests for bin/attach-tokens-update.sh
# Mocks all kubectl and curl calls so no cluster is required.
#
# NOTE: All token bytes used in mocks are computed at runtime from short plaintext
# words ("oldtoken", "newtoken", "wrongchecksum") to avoid false-positive secret
# scanners triggering on high-entropy base64 literals.

SCRIPT="$BATS_TEST_DIRNAME/../attach-tokens-update.sh"

# ---------------------------------------------------------------------------
# Helpers — mock kubectl and curl via PATH injection
# ---------------------------------------------------------------------------

setup() {
  MOCK_DIR="$(mktemp -d)"
  export PATH="$MOCK_DIR:$PATH"
  export MOCK_DIR

  # Compute test fixture bytes at runtime so no high-entropy literals appear
  # in source. The mock scripts reference these via $OLD_TOKEN_B64 /
  # $DEFAULT_TOKEN_B64 (exported here, available to every kubectl invocation).
  export OLD_TOKEN_B64;     OLD_TOKEN_B64=$(printf '%s' 'oldtoken'  | base64 -w0)
  export DEFAULT_TOKEN_B64; DEFAULT_TOKEN_B64=$(printf '%s' 'newtoken' | base64 -w0)

  # Default mocks (override per test as needed)
  _write_kubectl_mock
  _write_curl_mock
}

teardown() {
  rm -rf "$MOCK_DIR"
}

_write_kubectl_mock() {
  # Single-quoted heredoc: written verbatim. $OLD_TOKEN_B64 / $DEFAULT_TOKEN_B64 /
  # $MOCK_ATTACH_TOKENS / $MOCK_REFRESH_TIME / $MOCK_DIR expand at RUNTIME
  # (the mock script inherits the exported env from setup()).
  cat > "$MOCK_DIR/kubectl" <<'MOCK'
#!/usr/bin/env bash
# Records calls; produces minimal valid responses.
echo "$@" >> "$MOCK_DIR/kubectl_calls"

case "$*" in
  # preflight: ExternalSecret exists
  "get externalsecret fleet-dispatch-tokens -n ai-fleet"*)
    echo '{"metadata":{"name":"fleet-dispatch-tokens"}}'
    exit 0 ;;
  # preflight: Secret exists
  "get secret fleet-dispatch-tokens -n ai-fleet"*)
    echo '{"metadata":{"resourceVersion":"100"},"data":{"ATTACH_TOKENS":"'"$OLD_TOKEN_B64"'"}}'
    exit 0 ;;
  # preflight: Deployment exists
  "get deployment fleet-dispatch -n ai-fleet"*)
    echo '{"metadata":{"name":"fleet-dispatch"}}'
    exit 0 ;;
  # resourceVersion baseline / polling
  *"jsonpath={.metadata.resourceVersion}"*)
    # First call: 100 (baseline), subsequent: 101 (changed)
    COUNT=$(grep -c 'jsonpath={.metadata.resourceVersion}' "$MOCK_DIR/kubectl_calls" 2>/dev/null || echo 1)
    [ "$COUNT" -le 1 ] && echo "100" || echo "101"
    exit 0 ;;
  # Secret data for checksum polling
  *"jsonpath={.data.ATTACH_TOKENS}"*)
    # Return computed base64 of expected new value; tests override via MOCK_ATTACH_TOKENS
    echo "${MOCK_ATTACH_TOKENS:-$DEFAULT_TOKEN_B64}"
    exit 0 ;;
  # ESO refreshTime polling
  *"refreshTime"*)
    echo "${MOCK_REFRESH_TIME:-2099-01-01T00:01:00Z}"
    exit 0 ;;
  # force-sync annotation on ExternalSecret
  "annotate externalsecret fleet-dispatch-tokens"*)
    exit 0 ;;
  # causal annotation on deployment
  "annotate deployment fleet-dispatch"*)
    exit 0 ;;
  # rollout restart
  "rollout restart deployment/fleet-dispatch -n ai-fleet"*)
    exit 0 ;;
  # rollout status
  "rollout status deployment/fleet-dispatch -n ai-fleet"*)
    echo "deployment successfully rolled out"
    exit 0 ;;
  *)
    echo "UNMATCHED kubectl: $*" >&2
    exit 1 ;;
esac
MOCK
  chmod +x "$MOCK_DIR/kubectl"
}

_write_curl_mock() {
  cat > "$MOCK_DIR/curl" <<'MOCK'
#!/usr/bin/env bash
echo "$@" >> "$MOCK_DIR/curl_calls"
# Default: smoke test /items returns 200
echo '[]'
exit 0
MOCK
  chmod +x "$MOCK_DIR/curl"
}

# Compute expected checksum for "newtoken" (the intended new ATTACH_TOKENS value)
_expected_checksum() {
  printf '%s' 'newtoken' | sha256sum | awk '{print $1}'
}

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

@test "happy_path: exits 0 when all stages pass" {
  export MOCK_ATTACH_TOKENS; MOCK_ATTACH_TOKENS=$(printf '%s' 'newtoken' | base64 -w0)
  run "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123"
  [ "$status" -eq 0 ]
}

@test "missing_expected_checksum: exits non-zero; no kubectl calls" {
  run "$SCRIPT" --smoke-token "smoketoken123"
  [ "$status" -ne 0 ]
  [ ! -f "$MOCK_DIR/kubectl_calls" ]
}

@test "missing_smoke_token: exits non-zero; no kubectl calls" {
  run "$SCRIPT" --expected-checksum "abc123"
  [ "$status" -ne 0 ]
  [ ! -f "$MOCK_DIR/kubectl_calls" ]
}

@test "preflight_missing_externalsecret: exits non-zero before mutation" {
  cat > "$MOCK_DIR/kubectl" <<'MOCK'
#!/usr/bin/env bash
echo "$@" >> "$MOCK_DIR/kubectl_calls"
case "$*" in
  "get externalsecret"*) exit 1 ;;
  *) exit 0 ;;
esac
MOCK
  chmod +x "$MOCK_DIR/kubectl"

  run "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123"
  [ "$status" -ne 0 ]
  # No annotation or rollout calls
  ! grep -q 'annotate\|rollout restart' "$MOCK_DIR/kubectl_calls" 2>/dev/null
}

@test "preflight_missing_deployment: exits non-zero before mutation" {
  # OLD_TOKEN_B64 is exported from setup(); available to this inline mock at runtime
  cat > "$MOCK_DIR/kubectl" <<'MOCK'
#!/usr/bin/env bash
echo "$@" >> "$MOCK_DIR/kubectl_calls"
case "$*" in
  "get deployment"*) exit 1 ;;
  "get externalsecret"*) echo '{"metadata":{"name":"fleet-dispatch-tokens"}}'; exit 0 ;;
  "get secret"*) echo '{"metadata":{"resourceVersion":"100"},"data":{"ATTACH_TOKENS":"'"$OLD_TOKEN_B64"'"}}'; exit 0 ;;
  *) exit 0 ;;
esac
MOCK
  chmod +x "$MOCK_DIR/kubectl"

  run "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123"
  [ "$status" -ne 0 ]
  ! grep -q 'annotate\|rollout restart' "$MOCK_DIR/kubectl_calls" 2>/dev/null
}

@test "eso_refreshtime_timeout: exits non-zero; no rollout restart" {
  # refreshTime always in the past
  export MOCK_REFRESH_TIME="2000-01-01T00:00:00Z"

  run timeout 90 "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123" \
    --eso-timeout 3 \
    --checksum-timeout 3
  [ "$status" -ne 0 ]
  ! grep -q 'rollout restart' "$MOCK_DIR/kubectl_calls" 2>/dev/null
}

@test "checksum_mismatch_timeout: exits non-zero; no rollout restart" {
  # ESO refreshTime advances but checksum never matches expected
  export MOCK_REFRESH_TIME="2099-01-01T00:01:00Z"
  export MOCK_ATTACH_TOKENS; MOCK_ATTACH_TOKENS=$(printf '%s' 'wrongchecksum' | base64 -w0)

  run timeout 90 "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123" \
    --eso-timeout 3 \
    --checksum-timeout 3
  [ "$status" -ne 0 ]
  ! grep -q 'rollout restart' "$MOCK_DIR/kubectl_calls" 2>/dev/null
}

@test "force_sync_uses_uuid: annotation value matches UUID format" {
  export MOCK_ATTACH_TOKENS; MOCK_ATTACH_TOKENS=$(printf '%s' 'newtoken' | base64 -w0)

  run "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123"
  [ "$status" -eq 0 ]

  # Extract the force-sync annotation value from kubectl calls
  SYNC_VAL=$(grep 'force-sync=' "$MOCK_DIR/kubectl_calls" | grep -oE 'force-sync=[^ ]+' | cut -d= -f2)
  [[ "$SYNC_VAL" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]]
}

@test "restart_only_flag: skips ESO annotation and checksum poll; does rollout" {
  export MOCK_ATTACH_TOKENS; MOCK_ATTACH_TOKENS=$(printf '%s' 'newtoken' | base64 -w0)

  run "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123" \
    --restart-only
  [ "$status" -eq 0 ]

  # No force-sync annotation
  ! grep -q 'force-sync' "$MOCK_DIR/kubectl_calls" 2>/dev/null
  # Rollout restart still happens
  grep -q 'rollout restart' "$MOCK_DIR/kubectl_calls"
}

@test "no_secret_value_in_output: stdout+stderr contain no base64 token values" {
  export MOCK_ATTACH_TOKENS; MOCK_ATTACH_TOKENS=$(printf '%s' 'newtoken' | base64 -w0)

  run "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123"

  # Combined output must not contain the raw token value or its base64 encoding
  ! echo "$output" | grep -q 'newtoken'
  ! echo "$output" | grep -q "$(printf '%s' 'newtoken' | base64 -w0)"
  ! echo "$output" | grep -q 'smoketoken123'
}

@test "rollback_printed_on_failure: stderr has key path, baseline checksum, rollback steps" {
  # Trigger failure via ESO timeout
  export MOCK_REFRESH_TIME="2000-01-01T00:00:00Z"

  run timeout 30 "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123" \
    --eso-timeout 2 \
    --checksum-timeout 2
  [ "$status" -ne 0 ]
  # Rollback instructions appear in output
  echo "$output" | grep -q 'ROLLBACK'
  echo "$output" | grep -q 'force-sync\|infisical\|Infisical'
}

@test "deployment_annotated_before_restart: annotate call precedes rollout restart call" {
  export MOCK_ATTACH_TOKENS; MOCK_ATTACH_TOKENS=$(printf '%s' 'newtoken' | base64 -w0)

  run "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123"
  [ "$status" -eq 0 ]

  ANNOTATE_LINE=$(grep -n 'annotate deployment' "$MOCK_DIR/kubectl_calls" | head -1 | cut -d: -f1)
  RESTART_LINE=$(grep -n 'rollout restart' "$MOCK_DIR/kubectl_calls" | head -1 | cut -d: -f1)
  [ -n "$ANNOTATE_LINE" ]
  [ -n "$RESTART_LINE" ]
  [ "$ANNOTATE_LINE" -lt "$RESTART_LINE" ]
}

@test "smoke_test_uses_items_endpoint: curl hits /items with Bearer header" {
  export MOCK_ATTACH_TOKENS; MOCK_ATTACH_TOKENS=$(printf '%s' 'newtoken' | base64 -w0)

  run "$SCRIPT" \
    --expected-checksum "$(_expected_checksum)" \
    --smoke-token "smoketoken123"
  [ "$status" -eq 0 ]

  grep -q '/items' "$MOCK_DIR/curl_calls"
  grep -q 'smoketoken123' "$MOCK_DIR/curl_calls"
}
