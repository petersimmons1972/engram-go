# Local Safety Policy

## Always Ask First

- Wildcard or expanded destructive deletes: `rm *`, `rm -rf dir/*`, `find ... -delete`, and similar shell-expanded patterns.
- Docker/database data loss: `docker compose down -v`, `docker volume rm`, `docker volume prune`, `docker system prune --volumes`, `DROP`, `TRUNCATE`, destructive migration resets, and equivalents.
- Any operation that could remove, overwrite, rebuild destructively, or discard persistent container, database, volume, backup, or exported data.

## Preservation Default

- Treat Docker and Docker Compose data as protected unless the user explicitly authorizes data loss in this turn.
- Prefer data-preserving rebuilds: `docker compose up --build`, image rebuilds, service restarts, backups, named-volume reuse, non-destructive migrations, or export/import flows.
- If destruction appears necessary, stop and state the data at risk, safer alternatives tried or available, and the exact command needing approval.

## Engram Data Protection

- Treat `engram` and `engram-go` as irreplaceable shared AI memory.
- Before risky Engram work, identify the affected container, database, schema, table, and volume when possible.
- Prefer inspect, backup, named-volume reuse, non-destructive migrations, and post-change health checks.
- Never run Engram-related destructive Docker, volume, database, migration-reset, or wildcard-delete commands without explicit approval in that turn.
