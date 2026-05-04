#!/usr/bin/env bash
# Run full hook test suite: shellcheck + bats
set -euo pipefail

HOOKS_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TEST_DIR="$HOOKS_DIR/test"

echo "=== shellcheck ==="
shellcheck "$HOOKS_DIR"/*.sh
[[ -d "$HOOKS_DIR/lib" ]] && shellcheck "$HOOKS_DIR"/lib/*.sh || true
echo "✅ shellcheck passed"

echo ""
echo "=== bats: instinct#8 ==="
bats "$TEST_DIR/instinct-post-tool-use.bats"

echo ""
echo "=== bats: engram-go#397 ==="
bats "$TEST_DIR/engram-flush-fallback.bats"

echo ""
echo "=== bats: engram-go#404 ==="
bats "$TEST_DIR/engram-observability.bats"

echo ""
echo "✅ All suites passed"
