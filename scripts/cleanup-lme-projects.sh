#!/usr/bin/env bash
# cleanup-lme-projects.sh — delete LongMemEval isolation projects from Engram PostgreSQL
#
# LongMemEval benchmark runs create isolated projects named lme-{run_id}-{question_id}.
# All benchmark results are preserved in results/*/ as JSONL files. The DB rows
# (memories, chunks, retrieval_events, etc.) are ephemeral working state with no
# value after scoring completes.
#
# This script executes the same SQL as DeleteProject (internal/db/postgres_memory.go)
# across all lme-* projects in a single transaction.
#
# Usage:
#   ./scripts/cleanup-lme-projects.sh [OPTIONS]
#
# Options:
#   --dry-run    (DEFAULT) Show project count, date range, and per-table row counts
#   --execute    Perform deletion; explicit flag required — no accidental deletes
#   -h, --help   Show this help and exit
#
# Prerequisites:
#   - postgres container running: cd ~/projects/engram-go && docker compose up -d postgres
#   - .env file present with POSTGRES_PASSWORD set
#
# The script deletes all 13 tables that DeleteProject touches, in dependency order,
# in a single transaction. Exits non-zero if post-sweep verification finds residual rows.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# ── Defaults ──────────────────────────────────────────────────────────────────

MODE="dry-run"
PGDATABASE="${PGDATABASE:-engram}"
PGUSER="${PGUSER:-engram}"
COMPOSE_SERVICE="${COMPOSE_SERVICE:-postgres}"

# ── Help ──────────────────────────────────────────────────────────────────────

usage() {
  grep '^#' "$0" | sed 's/^# \?//' | sed -n '/^cleanup-lme/,/^Prerequisites/p'
  echo ""
  echo "Options:"
  echo "  --dry-run    (DEFAULT) Show project count, date range, and per-table row counts"
  echo "  --execute    Perform deletion; explicit flag required"
  echo "  -h, --help   Show this help and exit"
  exit 0
}

# ── Argument parsing ──────────────────────────────────────────────────────────

for arg in "$@"; do
  case "$arg" in
    --dry-run)  MODE="dry-run" ;;
    --execute)  MODE="execute" ;;
    -h|--help)  usage ;;
    *)
      echo "ERROR: unknown argument: $arg" >&2
      echo "       Run with --help for usage." >&2
      exit 1
      ;;
  esac
done

# ── psql helpers (run inside the postgres container via docker compose exec) ───

run_sql() {
  # --tuples-only --no-align: machine-readable single-value output
  docker compose -f "$PROJECT_ROOT/docker-compose.yml" exec -T "$COMPOSE_SERVICE" \
    psql -U "$PGUSER" -d "$PGDATABASE" \
    --tuples-only --no-align \
    "$@"
}

run_sql_pretty() {
  # standard psql table output
  docker compose -f "$PROJECT_ROOT/docker-compose.yml" exec -T "$COMPOSE_SERVICE" \
    psql -U "$PGUSER" -d "$PGDATABASE" \
    "$@"
}

# ── Connectivity check ────────────────────────────────────────────────────────

if ! docker compose -f "$PROJECT_ROOT/docker-compose.yml" exec -T "$COMPOSE_SERVICE" \
    pg_isready -U "$PGUSER" -d "$PGDATABASE" -q 2>/dev/null; then
  echo "ERROR: Cannot reach Engram postgres container ($COMPOSE_SERVICE)" >&2
  echo "       Start it with: cd ~/projects/engram-go && docker compose up -d postgres" >&2
  exit 1
fi

# ── Summary query (always run) ────────────────────────────────────────────────

echo ""
echo "═══════════════════════════════════════════════════════"
echo "  Engram LME Project Cleanup"
echo "═══════════════════════════════════════════════════════"
echo ""

PROJECT_COUNT=$(run_sql -c "
  SELECT COUNT(DISTINCT project)
  FROM (
    SELECT project FROM memories     WHERE project LIKE 'lme-%'
    UNION
    SELECT project FROM project_meta WHERE project LIKE 'lme-%'
  ) AS p;
")

if [[ "$PROJECT_COUNT" -eq 0 ]]; then
  echo "✓ No lme-* projects found in the database. Nothing to do."
  exit 0
fi

echo "Projects matching lme-*: $PROJECT_COUNT"
echo ""

run_sql_pretty -c "
SELECT
  'memories'            AS \"table\",
  COUNT(*)              AS rows,
  MIN(created_at)::date AS oldest,
  MAX(created_at)::date AS newest
FROM memories WHERE project LIKE 'lme-%'
UNION ALL
SELECT 'chunks',             COUNT(*), NULL, NULL
  FROM chunks              WHERE project LIKE 'lme-%'
UNION ALL
SELECT 'retrieval_events',   COUNT(*), MIN(created_at)::date, MAX(created_at)::date
  FROM retrieval_events   WHERE project LIKE 'lme-%'
UNION ALL
SELECT 'relationships',      COUNT(*), NULL, NULL
  FROM relationships       WHERE project LIKE 'lme-%'
UNION ALL
SELECT 'memory_versions',    COUNT(*), MIN(system_from)::date, MAX(system_from)::date
  FROM memory_versions     WHERE project LIKE 'lme-%'
UNION ALL
SELECT 'documents',          COUNT(*), NULL, NULL
  FROM documents           WHERE project LIKE 'lme-%'
UNION ALL
SELECT 'episodes',           COUNT(*), MIN(started_at)::date, MAX(started_at)::date
  FROM episodes            WHERE project LIKE 'lme-%'
UNION ALL
SELECT 'project_meta',       COUNT(*), NULL, NULL
  FROM project_meta        WHERE project LIKE 'lme-%'
UNION ALL
SELECT 'canonical_entities', COUNT(*), NULL, NULL
  FROM canonical_entities  WHERE project LIKE 'lme-%'
ORDER BY 1;
"

echo ""

if [[ "$MODE" == "dry-run" ]]; then
  echo "Mode: DRY RUN — no changes made."
  echo ""
  echo "To delete these $PROJECT_COUNT projects, re-run with --execute:"
  echo "  ./scripts/cleanup-lme-projects.sh --execute"
  echo ""
  exit 0
fi

# ── Execute deletion ──────────────────────────────────────────────────────────

echo "Mode: EXECUTE — deleting all lme-* projects..."
echo ""

run_sql_pretty -c "
BEGIN;

-- Delete in dependency order (mirrors DeleteProject in postgres_memory.go)
DELETE FROM retrieval_events   WHERE project LIKE 'lme-%';
DELETE FROM relationships      WHERE project LIKE 'lme-%';
DELETE FROM chunks             WHERE project LIKE 'lme-%';
DELETE FROM memory_versions    WHERE project LIKE 'lme-%';
DELETE FROM memories           WHERE project LIKE 'lme-%';
DELETE FROM documents          WHERE project LIKE 'lme-%';
DELETE FROM episodes           WHERE project LIKE 'lme-%';
DELETE FROM weight_config      WHERE project LIKE 'lme-%';
DELETE FROM weight_history     WHERE project LIKE 'lme-%';
DELETE FROM project_meta       WHERE project LIKE 'lme-%';
DELETE FROM audit_snapshots    WHERE project LIKE 'lme-%';
DELETE FROM audit_canonical_queries WHERE project LIKE 'lme-%';
DELETE FROM canonical_entities WHERE project LIKE 'lme-%';

COMMIT;
"

echo ""
echo "Deletion committed. Running post-sweep verification..."
echo ""

# ── Post-sweep verification ───────────────────────────────────────────────────

FAIL=0

for table in memories chunks retrieval_events relationships memory_versions project_meta canonical_entities; do
  residual=$(run_sql -c "SELECT COUNT(*) FROM $table WHERE project LIKE 'lme-%';")
  if [[ "$residual" -ne 0 ]]; then
    echo "  FAIL: $residual rows remain in $table" >&2
    FAIL=1
  else
    echo "  ✓ $table — clean"
  fi
done

echo ""

if [[ "$FAIL" -ne 0 ]]; then
  echo "✗ Post-sweep verification failed — see FAIL lines above." >&2
  exit 1
fi

echo "✓ All lme-* rows removed. Engram database is clean."
echo ""
