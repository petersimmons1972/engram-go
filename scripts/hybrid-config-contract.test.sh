#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PASS=0
FAIL=0

assert_contains() {
    local file="$1"
    local needle="$2"
    local desc="$3"
    if grep -qF "$needle" "$file"; then
        echo "PASS: $desc"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $desc"
        echo "  missing: $needle"
        FAIL=$((FAIL + 1))
    fi
}

assert_count() {
    local file="$1"
    local needle="$2"
    local want="$3"
    local desc="$4"
    local got
    got="$(grep -cF "$needle" "$file" || true)"
    if [ "$got" = "$want" ]; then
        echo "PASS: $desc"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $desc"
        echo "  expected count=$want got=$got for: $needle"
        FAIL=$((FAIL + 1))
    fi
}

assert_contains "$ROOT/docker-compose.yml" '${POSTGRES_HOST:-postgres}' \
    "compose defaults POSTGRES_HOST to the bundled postgres service"
assert_count "$ROOT/docker-compose.yml" '${POSTGRES_HOST:-postgres}' 3 \
    "all hybrid DATABASE_URL entries default POSTGRES_HOST to postgres"

assert_contains "$ROOT/.env.example" 'POSTGRES_HOST=postgres' \
    ".env.example documents the default bundled PostgreSQL host"
assert_contains "$ROOT/.env.example" 'POSTGRES_PORT=5432' \
    ".env.example documents the default PostgreSQL port"
assert_contains "$ROOT/.env.example" 'Override both for an external PostgreSQL server.' \
    ".env.example explains when to override POSTGRES_HOST and POSTGRES_PORT"

assert_contains "$ROOT/README.md" 'Hybrid profile checklist before `make up`:' \
    "README lists the hybrid profile requirements before make up"
assert_contains "$ROOT/README.md" '`ENGRAM_ROUTER_URL` (or legacy `LITELLM_URL`)' \
    "README names the required external router variable"
assert_contains "$ROOT/README.md" '`POSTGRES_HOST` / `POSTGRES_PORT`' \
    "README documents the PostgreSQL host and port overrides"

assert_contains "$ROOT/docs/getting-started.md" 'Before `make up`, confirm your hybrid `.env` has:' \
    "getting-started lists the hybrid variables before make up"
assert_contains "$ROOT/docs/getting-started.md" '`POSTGRES_HOST=postgres` and `POSTGRES_PORT=5432`' \
    "getting-started documents the default hybrid PostgreSQL endpoint"
assert_contains "$ROOT/docs/getting-started.md" '`ENGRAM_ROUTER_URL` (or legacy `LITELLM_URL`)' \
    "getting-started names the required hybrid router variable"

echo
echo "$PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
