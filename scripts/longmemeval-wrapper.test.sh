#!/usr/bin/env bash
# Test suite for maintained LongMemEval wrapper scripts.
# Usage: bash scripts/longmemeval-wrapper.test.sh
#
# These checks validate repo-root discovery and binary preflight behavior using
# a temp local clone and a fake longmemeval binary. No network or real model
# endpoints are required.

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PIPELINE_REL="scripts/longmemeval-pipeline.sh"
RESUME_REL="scripts/longmemeval-resume.sh"

PASS=0
FAIL=0

assert_exit() {
    local desc="$1" expected="$2" actual="$3"
    if [ "$expected" -eq "$actual" ]; then
        echo "✓ PASS: $desc (exit=$actual)"
        PASS=$((PASS+1))
    else
        echo "✗ FAIL: $desc (expected exit=$expected, got=$actual)"
        FAIL=$((FAIL+1))
    fi
}

assert_contains() {
    local desc="$1" needle="$2" haystack="$3"
    if echo "$haystack" | grep -qF -- "$needle"; then
        echo "✓ PASS: $desc"
        PASS=$((PASS+1))
    else
        echo "✗ FAIL: $desc (missing '$needle')"
        echo "    output: $haystack"
        FAIL=$((FAIL+1))
    fi
}

make_clone() {
    local clone_dir
    clone_dir="$(mktemp -d)"
    git clone -q -- "$REPO_ROOT" "$clone_dir"
    cp "$REPO_ROOT/$PIPELINE_REL" "$clone_dir/$PIPELINE_REL"
    cp "$REPO_ROOT/$RESUME_REL" "$clone_dir/$RESUME_REL"
    echo "$clone_dir"
}

install_fake_binary() {
    local clone_dir="$1"
    cat >"$clone_dir/longmemeval" <<'EOF'
#!/usr/bin/env bash
set -eu

cmd="$1"
shift

case "$cmd" in
    ingest|run|score|score-efficient)
        while [ "$#" -gt 0 ]; do
            if [ "$1" = "--out" ]; then
                out_dir="$2"
                mkdir -p "$out_dir"
                case "$cmd" in
                    ingest) : >"$out_dir/checkpoint-ingest.jsonl" ;;
                    run) : >"$out_dir/run.log" ;;
                    score|score-efficient) : >"$out_dir/score.log" ;;
                esac
                break
            fi
            shift
        done
        ;;
esac
EOF
    chmod +x "$clone_dir/longmemeval"
}

clone_dir="$(make_clone)"

out=$(
    cd "$clone_dir" &&
        env -i HOME="${HOME:-$clone_dir}" PATH="$PATH" bash "$PIPELINE_REL" 2>&1
)
rc=$?
assert_exit "pipeline without binary fails cleanly" 1 "$rc"
assert_contains "pipeline missing binary points at clone root" "$clone_dir/longmemeval" "$out"

out=$(
    cd "$clone_dir" &&
        env -i HOME="${HOME:-$clone_dir}" PATH="$PATH" \
            LLM_URL="http://127.0.0.1:8000/v1" \
            LLM_MODEL="smoke-model" \
            bash "$RESUME_REL" 2>&1
)
rc=$?
assert_exit "resume without binary fails cleanly" 1 "$rc"
assert_contains "resume missing binary points at clone root" "$clone_dir/longmemeval" "$out"

install_fake_binary "$clone_dir"

out=$(
    cd "$clone_dir" &&
        env -i HOME="${HOME:-$clone_dir}" PATH="$PATH" bash "$PIPELINE_REL" 2>&1
)
rc=$?
assert_exit "pipeline with clone-local binary reaches config validation" 1 "$rc"
assert_contains "pipeline empty env fails on missing generation config after binary check" "GEN_URL and GEN_MODEL are required" "$out"

out=$(
    cd "$clone_dir" &&
        env -i HOME="${HOME:-$clone_dir}" PATH="$PATH" \
            LLM_URL="http://127.0.0.1:8000/v1" \
            LLM_MODEL="smoke-model" \
            bash "$RESUME_REL" 2>&1
)
rc=$?
assert_exit "resume with clone-local binary runs end-to-end" 0 "$rc"

rm -rf "$clone_dir"

echo
echo "─── ${PASS} passed, ${FAIL} failed ───"
[ "$FAIL" -eq 0 ]
