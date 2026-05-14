---
name: Postgres Restart Root Cause — DeleteProject Schema Bug
description: Critical bug that caused Postgres cascade during LongMemEval scoring
type: error
originSessionId: f3ae2e31-3715-47ec-a406-f213ad3ab159
---
# Postgres Restart Root Cause: DeleteProject Schema Mismatch

## The Bug

`DeleteProject()` in `internal/db/postgres_memory.go` (lines 709, 714) referenced non-existent table names:
- Line 709: `DELETE FROM project_metadata` → table is actually `project_meta`
- Line 714: `DELETE FROM decay_audit_snapshots` → table is actually `audit_snapshots`

Schema defined in migration `001_initial.sql:75` as `project_meta` (singular), but code queries `project_metadata` (plural).

## Cascade Effect

When `DeleteProject()` was called during eval isolation cleanup (2026-05-02 01:46:05 UTC):

1. **First Query Failure**: `ERROR: relation "project_metadata" does not exist`
2. **Retry Loop**: Each subsequent retry fails identically
3. **Log Spam**: 100+ error messages per minute in Postgres logs for ~5 minutes
4. **Database Hammer**: Error handling/retry logic sends more failed queries → more errors
5. **Write Load Spike**: Cascading errors trigger high write activity
6. **Checkpoint Cascade**: Postgres switches from 1 checkpoint/min to 1 every 6-12 seconds
7. **I/O Contention**: Checkpoint times balloon from 5-10s to 50-96s each
8. **Responsiveness Collapse**: Database I/O saturation causes sluggish responses
9. **Possible Restart**: Extreme load may trigger automatic Postgres restart

**Timeline in Postgres logs:**
- 01:46:05.773 UTC: First error appears
- 01:46:37.381 UTC: Checkpoints increasing in frequency (28 seconds apart)
- 01:47:08 onwards: Checkpoint times hit 50-96 seconds
- 01:51:10 UTC: Final checkpoint at 59+ seconds before pattern stabilizes

## Fix Applied

**Commit**: `4f20616`
**Issue**: #409

Changes in `internal/db/postgres_memory.go`:
- Corrected `project_metadata` → `project_meta`
- Corrected `decay_audit_snapshots` → `audit_snapshots`
- Added missing cleanup: `audit_canonical_queries`, `canonical_entities`

## Key Lesson

**Never hard-code table names in DELETE statements without cross-referencing the schema migrations.** The cascade effect of a single typo in a cleanup function can destabilize the entire database instance due to cascading error-retry loops.

**Prevention**: Add a pre-commit hook or migration validator that:
1. Ensures all table names in code match actual schema
2. Flags any DELETE queries where table doesn't exist in latest migration
3. Runs on every commit to catch this class of bug early

## Impact

- Blocked eval isolation projects (like longmemeval scoring) from completing cleanup
- Risk of database instance becoming unresponsive during normal operations
- Demonstrated fragility of error handling in high-write scenarios
