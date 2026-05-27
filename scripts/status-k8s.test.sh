#!/usr/bin/env bash
# Test suite for infra/status-k8s.sh
# Usage: bash scripts/status-k8s.test.sh

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
STATUS="${REPO_ROOT}/infra/status-k8s.sh"

PASS=0
FAIL=0

assert_exit() {
    local desc="$1" expected="$2" actual="$3"
    if [ "$expected" -eq "$actual" ]; then
        echo "PASS: $desc (exit=$actual)"
        PASS=$((PASS+1))
    else
        echo "FAIL: $desc (expected exit=$expected, got=$actual)"
        FAIL=$((FAIL+1))
    fi
}

assert_contains() {
    local desc="$1" needle="$2" haystack="$3"
    if echo "$haystack" | grep -qF "$needle"; then
        echo "PASS: $desc"
        PASS=$((PASS+1))
    else
        echo "FAIL: $desc (missing '$needle')"
        echo "$haystack"
        FAIL=$((FAIL+1))
    fi
}

make_fake_tools() {
    local d="$1"
    mkdir -p "$d/bin"
    cat > "$d/bin/kubectl" <<'FAKE_KUBECTL'
#!/usr/bin/env bash
args="$*"

if [ "$args" = "config current-context" ]; then
    echo "kind-engram"
    exit 0
fi

if [[ "$args" == *"get deploy engram-go"* && "$args" == *".status.readyReplicas"* ]]; then echo 1; exit 0; fi
if [[ "$args" == *"get deploy engram-go"* && "$args" == *".status.availableReplicas"* ]]; then echo 1; exit 0; fi
if [[ "$args" == *"get deploy engram-go"* && "$args" == *".spec.replicas"* ]]; then echo 1; exit 0; fi
if [[ "$args" == *"get deploy engram-go"* && "$args" == *".spec.template.spec.containers"* ]]; then echo "engram-go=engram-go:sha-a1"; exit 0; fi

if [[ "$args" == *"get deploy engram-reembed"* && "$args" == *".status.readyReplicas"* ]]; then echo 1; exit 0; fi
if [[ "$args" == *"get deploy engram-reembed"* && "$args" == *".status.availableReplicas"* ]]; then echo 1; exit 0; fi
if [[ "$args" == *"get deploy engram-reembed"* && "$args" == *".spec.replicas"* ]]; then echo 1; exit 0; fi
if [[ "$args" == *"get deploy engram-reembed"* && "$args" == *".spec.template.spec.containers"* ]]; then echo "reembed=engram-reembed:sha-b2"; exit 0; fi

if [[ "$args" == *"get pods -l app=engram-go"* ]]; then
    echo "engram-go-abc|engram-go:0:prev=none:exit=none"
    exit 0
fi
if [[ "$args" == *"get pods -l app=engram-reembed"* ]]; then
    echo "engram-reembed-def|reembed:12:prev=Error:exit=1"
    exit 0
fi

if [[ "$args" == *"get events"* ]]; then
    echo "LAST SEEN   TYPE      REASON    OBJECT                  MESSAGE"
    echo "2m          Warning   BackOff   pod/engram-reembed-def   Back-off restarting failed container"
    exit 0
fi

exit 0
FAKE_KUBECTL
    chmod +x "$d/bin/kubectl"

    cat > "$d/bin/curl" <<'FAKE_CURL'
#!/usr/bin/env bash
out=""
write_code=false
url=""
auth=false
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o) shift; out="$1" ;;
        -w) write_code=true; shift ;;
        -H) shift; [[ "$1" == Authorization:* ]] && auth=true ;;
        http://*) url="$1" ;;
    esac
    shift || true
done

if [[ "$url" == */health ]]; then
    echo "ok" > "$out"
    $write_code && printf "200"
    exit 0
fi
if [[ "$url" == */ready ]]; then
    echo "ready" > "$out"
    $write_code && printf "200"
    exit 0
fi
if [[ "$url" == */metrics ]]; then
    if ! $auth; then
        echo "missing auth" > "$out"
        $write_code && printf "401"
        exit 0
    fi
    cat > "$out" <<'METRICS'
engram_chunks_pending_reembed 42
engram_db_pool_acquired_conns 3
engram_db_pool_max_conns 10
METRICS
    $write_code && printf "200"
    exit 0
fi

$write_code && printf "000"
exit 0
FAKE_CURL
    chmod +x "$d/bin/curl"
}

TMPDIR=$(mktemp -d)
make_fake_tools "$TMPDIR"

out=$(PATH="$TMPDIR/bin:$PATH" ENGRAM_API_KEY=test-token bash "$STATUS" 2>&1)
rc=$?
assert_exit "status-k8s exits cleanly with fake Kubernetes and metrics" 0 "$rc"
assert_contains "deployment image identity is shown" "image=engram-go=engram-go:sha-a1" "$out"
assert_contains "reembed restart count warns" "engram-reembed pods           WARN" "$out"
assert_contains "previous crash reason is included" "prev=Error" "$out"
assert_contains "recent warning events are surfaced" "Back-off restarting failed container" "$out"
assert_contains "pending reembed backlog is parsed from metrics" "metrics backlog               WARN" "$out"
assert_contains "DB pool usage is parsed from metrics" "metrics db pool               OK" "$out"

out=$(PATH="$TMPDIR/bin:$PATH" ENGRAM_API_KEY='' bash "$STATUS" 2>&1)
rc=$?
assert_exit "status-k8s exits cleanly without metrics token" 0 "$rc"
assert_contains "missing metrics token is explicit" "metrics auth                  WARN" "$out"

rm -rf "$TMPDIR"

echo
echo "--- ${PASS} passed, ${FAIL} failed ---"
[ "$FAIL" -eq 0 ]
