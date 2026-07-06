#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UNIT="${ROOT}/ops/instinct.service"

failures=0

assert_contains() {
  local description="$1"
  local needle="$2"

  if ! grep -qF "$needle" "$UNIT"; then
    echo "not ok - ${description}: missing ${needle}"
    failures=$((failures + 1))
    return
  fi

  echo "ok - ${description}"
}

assert_not_contains() {
  local description="$1"
  local needle="$2"

  if grep -qF "$needle" "$UNIT"; then
    echo "not ok - ${description}: found ${needle}"
    failures=$((failures + 1))
    return
  fi

  echo "ok - ${description}"
}

assert_contains "service runs installed Go consolidator" "ExecStart=%h/bin/instinct-consolidate"
assert_not_contains "service does not reference removed Python module" "python3 -m instinct.run"
assert_not_contains "service does not set stale Python path" "PYTHONPATH="

if (( failures > 0 )); then
  echo "FAIL: ${failures} instinct.service contract assertion(s) failed"
  exit 1
fi

echo "PASS: instinct.service contract"
