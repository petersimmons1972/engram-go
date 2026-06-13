# engram-go Pre-Check-In Checklist

Quick start:
  bin/checkin-lint.sh           # automated checks; exit non-zero on findings
  bin/checkin-lint.sh --fix-hints

Automated checks (🤖) run via bin/checkin-lint.sh.
Universal checks inherited from ~/docs/failure-modes-standard.md.

## Judgment items (👁 — not automatable)

### H. Stateless vs. Stateful Health Probe (FM-14)
| # | Check | How |
|---|-------|-----|
| P1 | `/ready` and `/health` endpoints query a real dependency (e.g. `SELECT 1`). A 200 from a probe that doesn't hit Postgres hides a broken pod. | Code review: `internal/*/handler*.go` |
| P2 | Any new health endpoint returns 503 — not 200 — when a required dependency is down. | Integration test or manual verify |

### I. Destructive Method Self-Guard (FM-21)
| # | Check | How |
|---|-------|-----|
| P3 | Methods that delete, truncate, or overwrite data (`NullAllEmbeddings`, `DeleteProject`, pruning ops) enforce their own guard (explicit force flag or confirmation token) — not just a caller-side check. | Code review: `internal/db/` |
| P4 | A future caller of a destructive method cannot bypass the guard by omitting a flag. | Review method signature and body, not just call site |
