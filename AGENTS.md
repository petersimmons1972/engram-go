# Local Safety Policy

- Preserve prompts for delete actions that include wildcards, such as `rm *`, `rm -rf dir/*`, `find ... -delete`, or shell-expanded destructive patterns.
- Treat Docker and Docker Compose data as protected by default. Do not run commands that can destroy container data, volumes, databases, or persistent state without explicit user approval.
- Avoid `docker compose down -v`, `docker volume rm`, `docker volume prune`, `docker system prune --volumes`, database `DROP`/`TRUNCATE`, and equivalent destructive operations unless the user has specifically authorized that data loss.
- When a container needs to be rebuilt, prefer data-preserving workarounds first: `docker compose up --build`, image rebuilds, service restarts, backups, named-volume reuse, migrations, or export/import flows.
- If data destruction seems necessary, stop and explain what data is at risk, the safer alternatives tried or available, and the exact command that needs approval.

## Engram Data Protection

- Treat the `engram` and `engram-go` projects as containing irreplaceable shared AI memory. Their Docker containers, Postgres databases, volumes, migrations, backups, and exported data must be preserved unless the user explicitly authorizes a destructive action.
- Do not assume local Engram data is test fixture data. Before any operation that could alter, rebuild, reset, truncate, migrate destructively, or remove Engram data, identify the affected container, database, schema, table, and volume when possible.
- Prefer preservation workflows for Engram: inspect first, back up before risky changes, reuse named volumes, run non-destructive migrations, rebuild images without removing volumes, and verify container/database health after changes.
- Never run Engram-related `docker compose down -v`, `docker volume rm`, `docker volume prune`, `docker system prune --volumes`, `DROP DATABASE`, `DROP SCHEMA`, `DROP TABLE`, `TRUNCATE`, destructive migration resets, or wildcard deletes without explicit user approval in that turn.
