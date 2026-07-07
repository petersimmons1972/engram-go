#!/usr/bin/env bash
set -euo pipefail

unit="ops/instinct.service"

fail() {
    echo "FAIL: $1" >&2
    exit 1
}

[[ -f "$unit" ]] || fail "missing $unit"

exec_start_line="$(grep -E '^ExecStart=' "$unit" || true)"
[[ -n "$exec_start_line" ]] || fail "missing ExecStart in $unit"

expected='ExecStart=%h/bin/instinct-consolidate'
[[ "$exec_start_line" == "$expected" ]] || fail "expected $expected, got: $exec_start_line"

if grep -Eq '^Environment=PYTHONPATH=' "$unit"; then
    fail "unexpected PYTHONPATH environment in $unit"
fi

if grep -Fq 'python3 -m instinct.run' "$unit"; then
    fail "unexpected removed python module entrypoint in $unit"
fi

echo "PASS: $unit uses the installed Go consolidator contract"
