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

# Superpowers Planning Override

- When using superpowers planning, brainstorming, or design/spec skills, ask as many clarifying questions as needed to produce a correct plan; do not stop because a generic three-question limit has been reached.
- Preserve the superpowers preference for one question at a time when practical. If the active tool or UI can only batch a limited number of questions, continue with additional follow-up batches until the plan has enough information.

# Global Multi-Agent Preference

- For all non-trivial work, default to a multi-phase approach and use multiple agents when they add meaningful leverage. This global preference applies especially to planning, design review, implementation planning, issue triage, audits, debugging, and tasks with independent workstreams.
- Before execution on complex tasks, split the work into explicit phases such as discovery, analysis, plan drafting, plan review, implementation, verification, and handoff. Use only the phases that fit the task.
- Use agents for parallelism, independent review, specialized analysis, adversarial critique, or context isolation. Do not use agents for tiny, single-step, clearly serial, or low-risk tasks where coordination cost exceeds the benefit.
- Agent selection MUST be cost conscious. Choose the most token-efficient agent or model that can reliably complete each subtask. Reserve larger or more expensive agents for ambiguity, high-risk decisions, broad-context synthesis, adversarial review, or final validation.
- Every agent assignment MUST be bounded with clear inputs, expected output, stop conditions, and cost expectations. The main agent MUST merge, dedupe, verify, and resolve conflicts instead of treating agent output as authoritative.

## Claude ↔ Codex Handoff

Codex implements work queued by Claude via GitHub Issues (`~/bin/queue-agent`).
Use `codex-handoff` MCP tool for repo context. Full workflow:
`~/projects/codex/README.md` § Claude ↔ Codex Hybrid Workflow.
