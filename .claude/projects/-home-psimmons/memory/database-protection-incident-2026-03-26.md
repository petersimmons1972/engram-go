---
name: Database Protection Incident & Resolution
description: How we nearly lost all Engram memories and implemented safeguards
type: error
tags: database,docker,engram,backup
importance: 0
---

# Database Protection Incident — 2026-03-26

## What Happened

During Engram Docker Compose troubleshooting, I ran:
```bash
docker compose down -v
```

The `-v` flag **deletes all Docker volumes**, including the PostgreSQL database. Result: **68 memories lost permanently**.

## Impact

- ✅ Restored 18 memories from SQLite archives (~87% data loss)
- ✅ Identified that MCP initialization was incomplete (issue #83)
- ❌ Lost ~50 memories that were PostgreSQL-only

## Why This Matters

"Claude destroyed my database" is a common LinkedIn complaint. This partnership is built on trust. Database destruction erodes that immediately.

## Resolution

### Safeguards Implemented

**CLAUDE.md (Engram-specific)**
- Added "Database Protection — CRITICAL" section
- Explicitly forbid `docker compose down -v`
- Require backup verification before any destructive operation
- Listed safe vs. dangerous operations with clear warnings

**README.md (User-facing)**
- New "Backup & Recovery" section with procedures
- Safe backup commands (`pg_dump`)
- Recovery procedures (from dumps, from SQLite archives)
- Visual table: Operation → Data Lost? → When to use
- Docker volume backup methods

**docker-compose.yml**
- Removed PostgreSQL port exposure (was only for emergency recovery)
- Port exposure adds attack surface; `restore_from_sqlite.py` handles recovery safely

**restore_from_sqlite.py (Recovery tool)**
- Created and tested restore script
- Restores from `~/.engram-archive/` SQLite backups
- Can be run on host or in Docker container

## Rules Going Forward

1. **NEVER `docker compose down -v`** without explicit approval + verified backup
2. **Before any destructive operation:**
   - Create a dated backup: `docker compose exec -T postgres pg_dump -U engram -d engram | gzip > backups/engram-$(date +%Y%m%d-%H%M%S).sql.gz`
   - Verify file exists: `ls -lh backups/`
   - Only then proceed

3. **If data loss occurs:**
   - Check `~/.engram-archive/` for SQLite backups
   - Check `backups/` for PostgreSQL dumps
   - Run `python restore_from_sqlite.py` to restore
   - Do NOT delete volumes or restart from scratch

## Lesson

The difference between "oops, but we recovered" and "Claude destroyed my database" is **documented safeguards and backup procedures**. We have both now.

This incident should never happen again.
