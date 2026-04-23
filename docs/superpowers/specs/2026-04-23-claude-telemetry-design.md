# Claude Telemetry Sink — Design

**Date:** 2026-04-23
**Status:** Design — awaiting user review before implementation plan
**Owner:** Peter Simmons
**Related:** `~/CLAUDE.md` (self-learning, retry-limit, escalation rules)

## Goal

Capture every Claude Code tool call, error, session boundary, user prompt, commit, and escalation into a queryable store so that:

1. **Human-facing:** Peter can see tool-usage patterns, error frequency, session duration, and retry loops over time.
2. **Agent-facing (v1, narrow):** Claude can answer "have I hit this error signature N times already?" mid-session via a stable `just` interface.

Secondary goals derived from the primary pair:

- Feed the 3-strike retry rule in `CLAUDE.md` with real data instead of honor-system counting.
- Give the memory janitor and future Shewhart-style analysis a common-cause / special-cause signal source.
- Produce a cost proxy (tool-call counts, durations) without standing up a dedicated billing pipeline.

## Non-Goals

- Not a SIEM. No cross-host correlation, no alerting DSL, no compliance reporting.
- Not a distributed tracing system. No span trees, no OpenTelemetry conformance (may map to it later).
- Not real-time. 5-minute lag between event and Postgres is fine.
- Not multi-tenant. One user, one workstation, one cluster.

## Decisions Captured (from brainstorming)

| # | Question | Decision |
|---|---|---|
| 1 | Primary consumer | Human-first, with one narrow agent-facing query path (retry check). |
| 2 | Event set | Everything — tool calls, errors, sessions, commits, escalations, agent dispatches, Engram recalls, full payloads. |
| 3 | Delivery path | Local JSONL + K8s CronJob shipper. Fully asynchronous — zero session impact. |
| 4 | Storage location | New database `claude_telemetry` on the existing TruNAS Postgres instance. |
| 5 | Failure handling | Dead-letter for data failures, health-check surfacing for infra failures, idempotent shipping. |
| 6 | Schema shape | Typed core columns + `payload jsonb`. Promote fields via Postgres generated columns as queries demand. |
| 7 | Agent query interface | `just claude-events <recipe>` that unions local JSONL + Postgres. |
| 8 | Retention & privacy | Keep everything forever. Raw local JSONL (workstation trust boundary); scrub secrets at the shipper before Postgres insert. |
| 9 | Language | Go for the shipper. Bash for the hook. `just` + `psql` + `jq` for recipes. |

## Architecture

```
Claude Code session (workstation)
        │
        │ hooks: PostToolUse, Stop, SessionStart, UserPromptSubmit
        ▼
~/.claude/events/YYYY-MM-DD.jsonl       ← append-only, local, never blocks
        │
        │ read by K8s CronJob (every 5 min, mtime > 10 min)
        ▼
claude-telemetry-shipper (Go, FROM scratch container)
  ├─ parse JSONL line by line
  ├─ scrub secrets via regex ruleset
  ├─ batch INSERT ... ON CONFLICT DO NOTHING
  ├─ malformed lines → ~/.claude/events/deadletter/
  └─ delete source file only on full success
        │
        ▼
TruNAS Postgres → database: claude_telemetry → table: events
        │
        ├─→ human: Grafana, ad-hoc SQL
        └─→ agent: just claude-events <recipe> (unions local JSONL + Postgres)
```

**Trust boundary:** local JSONL sits on the workstation (same trust level as `~/.claude/` creds, shell history, Infisical cache). Postgres is scrubbed because it's reachable from anywhere in the cluster.

**Blast radius:**

- Hook failure → one missing event. Session unaffected.
- Shipper failure → files accumulate locally, ship on recovery.
- Postgres failure → invisible to session; surfaces via `health-check.sh` on next session start.
- Scrub regex false negative → secret reaches Postgres. Mitigated by periodic `scrub-audit` recipe.

## Components

### 1. Hook — `~/.claude/hooks/telemetry-emit.sh`

A single bash script wired into `settings.json` under four events:

- `PostToolUse` — every tool call result.
- `Stop` — end of assistant turn.
- `SessionStart` — new session boundary.
- `UserPromptSubmit` — user prompt received.

**Contract:**

- Reads hook JSON from stdin.
- Augments with `ts` (RFC3339 UTC, millisecond precision), `session_id`, `event_type`, `tool`, `status`, `duration_ms`, `error_signature`, `project`.
- Appends one JSON line to `~/.claude/events/$(date -u +%Y-%m-%d).jsonl`.
- Exits 0 unconditionally (`trap 'exit 0' ERR`). Any internal failure is silent from Claude's perspective.
- Uses `flock` with a 100ms timeout on a per-day lockfile. If the lock can't be acquired in 100ms, writes to `~/.claude/events/.hook-errors.log` and exits 0.
- No network calls. No subprocesses beyond `date`, `sha1sum`, `jq` (for field extraction).
- Performance target: p95 < 10ms per invocation.

**Derived fields:**

- `session_id` — from `$CLAUDE_SESSION_ID` if set; else read from `~/.claude/events/.current-session` (written on `SessionStart`); else `UNKNOWN-<pid>`.
- `error_signature` — if `status == "error"` or payload contains an error string: SHA-1 of first 200 characters of the error text. Computed at emit time so queries don't regex-parse JSON.
- `project` — heuristic from `$PWD` matching a known set of project roots. Maintained as an ordered list in the hook script; unknown paths → `null`.
- `duration_ms` — from the hook payload if available (Claude Code provides it for `PostToolUse`); else null.

### 2. Shipper — `claude-telemetry-shipper` (Go)

A single Go binary, deployed as a K8s CronJob every 5 minutes. Container image: `FROM scratch` or `gcr.io/distroless/static`, final size ~15MB.

**Dependencies:**

- `github.com/jackc/pgx/v5` — Postgres driver, native batch INSERT support.
- Standard library: `encoding/json`, `regexp`, `os`, `path/filepath`, `time`, `crypto/sha1`, `log/slog`.
- No Cobra, no Viper. Env-var config, flag-parsing only if needed.

**Control flow:**

```
1. Load config from env (PG_DSN from Infisical, EVENTS_DIR, DEADLETTER_DIR).
2. Connect to Postgres; fail fast if unreachable.
3. Walk EVENTS_DIR for *.jsonl files where now() - mtime > 10 minutes.
4. For each file:
   a. Open and read line by line.
   b. For each line:
      - Parse JSON. If parse fails → append raw line to deadletter/<date>.jsonl, continue.
      - Run scrub ruleset. Replace matches with [REDACTED:<rule-name>]. Set scrubbed=true if any rule hit.
      - Add to batch.
   c. Flush batch via pgx.Batch with INSERT ... ON CONFLICT (session_id, ts, event_type, tool) DO NOTHING.
   d. On success → delete source file.
   e. On Postgres error → log, exit 1, leave file in place.
5. Emit run metrics (files processed, rows inserted, scrubs applied, deadletter count) to stdout.
6. Exit 0.
```

**Scrub ruleset (v1):**

| Name | Regex | Notes |
|---|---|---|
| `api-key-generic` | `sk-[A-Za-z0-9]{20,}` | OpenAI/Anthropic shape |
| `github-token` | `ghp_[A-Za-z0-9]{36}` | GitHub PAT |
| `github-app-token` | `ghs_[A-Za-z0-9]{36}` | GitHub App installation token |
| `jwt` | `eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}` | JWT shape |
| `password-assign` | `(?i)password["']?\s*[:=]\s*["'][^"']{1,}["']` | assignment literal |
| `bearer` | `(?i)bearer\s+[A-Za-z0-9._\-]{20,}` | Authorization header |

Replacement preserves the rule name so Peter can see *what* was scrubbed without seeing *what it was*.

**Idempotency:** `ON CONFLICT (session_id, ts, event_type, tool) DO NOTHING`. A re-run of the same file is safe. If the same four-tuple legitimately repeats within a single millisecond, one row is dropped; tolerable for v1.

**Failure semantics:**

- File parse failure (entire file unreadable): whole file to deadletter, continue.
- Single-line parse failure: that line to deadletter, continue with rest of file.
- Postgres connection / auth failure: exit 1 without deleting any source file. CronJob marks failed. Health-check picks it up.
- Partial batch insert failure: pgx returns error; exit 1 without deleting. Next run retries — idempotent.

**Workstation → cluster data path:** the shipper CronJob needs to read files from `~/.claude/events/` on the workstation. Two concrete options to resolve during implementation:

- **Option A (preferred if pattern exists):** NFS export of `~/.claude/events/` read-only to the cluster, mounted into the CronJob pod. Matches how other workstation-origin data reaches the cluster.
- **Option B (fallback):** systemd user timer on the workstation runs the shipper binary locally; Postgres is reachable from the workstation.

The shipper binary is identical in both cases — only the orchestration differs. Decision deferred to implementation plan once the existing pattern is confirmed.

### 3. Query recipes — `~/.claude/justfile` (or `~/justfile`)

A handful of small recipes. Each is 5–20 lines of shell, directly readable.

**v1 recipes:**

```
claude-events retry-check <error_signature>
    # Count matches in today's local JSONL + last 24h in Postgres.
    # Returns a single integer (total matches) and exits 0.

claude-events session-summary
    # Current session stats from local JSONL only.
    # Tool-call count, error count, duration, unique tools used.

claude-events recent-errors
    # Last 10 error events this session from local JSONL.
    # Tool, error_signature, timestamp — one per line.

claude-events scrub-audit
    # Run each scrub regex against the last 7 days of Postgres events.
    # Report any hits (should be zero — indicates a scrub false negative).

claude-events tail
    # tail -f today's local JSONL, pretty-printed via jq.
    # For live debugging.
```

**Why `just` + `psql` + `jq` and not a Go CLI:** v1 recipes are one SQL query or one `jq` expression each. A dedicated binary is premature. If recipes grow teeth (complex joins, custom formatting, charts), promote to a `claude-events` Go binary at that point.

### 4. Health-check integration — `~/bin/health-check.sh`

Add one check: most recent successful shipper run within the last 15 minutes.

- Check: query Postgres for `MAX(ts)` in the `events` table, or query K8s for the CronJob's last successful run.
- Surface: add a line to the existing warnings section in MEMORY.md (same pattern as other health-check outputs).
- No new alerting channel. No pager. Just visibility on next session start.

## Data Model

**Database:** `claude_telemetry` (new, on the TruNAS Postgres instance)
**Schema:** `public` (single-purpose DB, no subdivision needed)

```sql
CREATE TABLE events (
  id              bigserial PRIMARY KEY,
  ts              timestamptz NOT NULL,
  session_id      text        NOT NULL,
  event_type      text        NOT NULL,  -- tool_call | tool_result | error | session_start
                                         -- session_stop | user_prompt | commit | escalation
                                         -- (agent dispatches and Engram recalls arrive as tool_call
                                         --  events with tool = Task or mcp__engram__memory_recall)
  tool            text,                  -- null for non-tool events
  status          text,                  -- ok | error | timeout | null
  duration_ms     int,
  error_signature text,                  -- sha1 of first 200 chars of error; null if not an error
  project         text,                  -- from PWD heuristic; null if unknown
  scrubbed        boolean     NOT NULL DEFAULT false,
  payload         jsonb       NOT NULL,  -- full hook JSON + any fields not yet promoted
  UNIQUE (session_id, ts, event_type, tool)
);

CREATE INDEX events_ts_brin   ON events USING BRIN (ts);
CREATE INDEX events_session   ON events (session_id, ts);
CREATE INDEX events_error_sig ON events (error_signature) WHERE error_signature IS NOT NULL;
CREATE INDEX events_tool_ts   ON events (tool, ts)        WHERE tool IS NOT NULL;
```

**Roles:**

- `claude_telemetry_writer` — `INSERT` only on `events`. Used by shipper.
- `claude_telemetry_reader` — `SELECT` only on `events`. Used by `just` recipes and Grafana.

Credentials: managed in Infisical under a new path (e.g. `claude-telemetry/postgres`). Shipper CronJob mounts them via the existing Infisical pattern used by other cluster workloads.

### JSONL line shape (on disk + pre-ship)

```json
{
  "ts": "2026-04-23T14:32:11.482Z",
  "session_id": "20260423-055606",
  "event_type": "tool_result",
  "tool": "Bash",
  "status": "error",
  "duration_ms": 1243,
  "error_signature": "a3f8c29b1e...",
  "project": "clearwatch",
  "payload": {
    "input":  { "command": "kubectl get pods" },
    "output": "error: ...",
    "hook_source": "PostToolUse"
  }
}
```

### Field promotion — the escape hatch

When a `payload->>'x'` field proves query-worthy, promote it with a generated column:

```sql
ALTER TABLE events
  ADD COLUMN agent_type text
  GENERATED ALWAYS AS (payload->>'agent_type') STORED;
CREATE INDEX ON events (agent_type) WHERE agent_type IS NOT NULL;
```

Zero backfill. Zero shipper changes. This is the mechanism that lets v1 stay small without boxing in v2+.

## Data Flow

**Happy path, single tool call:**

1. Claude invokes `Bash("kubectl get pods")`.
2. Claude Code fires `PostToolUse` hook with tool JSON on stdin.
3. Hook: acquire `flock` (sub-ms), compute derived fields, append one JSON line, release lock, exit 0. ~2ms total. Claude's session continues instantly.
4. Up to 5 minutes later, CronJob wakes. Today's file has recent `mtime` → skipped. Yesterday's closed file is processed.
5. Shipper: parse → scrub → batch insert with `ON CONFLICT DO NOTHING` → delete file on full success.
6. Next session start: `health-check.sh` confirms shipper ran within 15 min. MEMORY.md header is clean.

**Retry-check path (agent-facing):**

1. Claude hits the same error for the third time in a session.
2. Before attempting a fourth retry, Claude runs `just claude-events retry-check <error_signature>`.
3. Recipe greps today's local JSONL (captures the most recent occurrences, including the one seconds ago) + queries Postgres for the last 24h. Sums the counts.
4. If result ≥ 3, Claude escalates per `CLAUDE.md` rules instead of retrying.

## Failure Handling Matrix

| Failure | Impact on session | Detection | Recovery |
|---|---|---|---|
| Hook script missing/broken | Event lost, session unaffected | Gaps in `events/` files | Fix script; past events unrecoverable |
| Disk full on events dir | Append fails, hook still exits 0 | Existing host disk alert | Free space; events during outage lost |
| Malformed JSON from Claude Code | Line written as-is; shipper sends to deadletter | Lines in `deadletter/` | Inspect, fix scrub/schema, optionally replay |
| Secret in payload | Written raw to local JSONL (trusted); scrubbed before Postgres | `scrubbed=true` rows | Already handled |
| Postgres unreachable | Shipper exits non-zero; files remain | CronJob failure + health-check | Automatic on DB recovery |
| Scrub false positive | Postgres has redacted payload; local JSONL still has raw | Compare local vs DB during query | Update scrub rules; raw on workstation |
| Scrub false negative | Secret reaches Postgres | Periodic `scrub-audit` recipe | Add rule; optional `UPDATE` to redact retroactively |
| Hook races shipper | `mtime > 10 min` rule prevents it | Not possible by construction | N/A |
| Two concurrent sessions | `flock` serializes (~1ms wait) | N/A | N/A |
| Clock skew workstation vs cluster | `ts` comes from workstation clock at emit time | Compare `ts` to `now()` during shipper run | NTP already running |
| Schema migration mid-flight | New column with `DEFAULT NULL`; generated columns compute on read | N/A | Backward-compatible by construction |

## Testing Strategy

**Hook (bash, `bats` test harness):**

- Sample `PostToolUse` stdin → expected JSONL line.
- Malformed stdin → exits 0, no blocking behavior.
- Missing `$CLAUDE_SESSION_ID` → fallback to `.current-session` file.
- Two concurrent invocations → both lines present, neither corrupted.
- 100-invocation loop → p95 < 10ms.
- Single integration test: real Claude Code session, one Bash call, assert a JSONL line within 1 second.

**Shipper (Go, stdlib `testing` + `testcontainers-go`):**

- Good file → rows inserted, file deleted.
- One malformed line in a good file → good lines inserted, bad line in deadletter, file deleted.
- Secret in payload → row has `[REDACTED:<name>]`, `scrubbed=true`.
- Same file shipped twice → second run inserts zero rows (idempotency).
- Postgres unreachable → exit non-zero, file NOT deleted.
- File with `mtime < 10 min` → skipped.
- Dead-letter files never re-processed (no loops).
- Scrub rule table: parametrized positive + negative examples per rule. New rule = one new test row.

**Query recipes (golden-output tests):**

- Seed fixture JSONL + fixture Postgres rows, run each recipe, assert exact stdout.
- Specifically for `retry-check`: 2 events in Postgres (yesterday) + 1 in local JSONL (today) → recipe returns `3`. This is the load-bearing case; without it, the agent-facing half of the design is untested.

**End-to-end smoke test (manual, not CI):**

- Script that runs a fresh Claude session, makes 5 mixed tool calls, sleeps 11 minutes, manually triggers the shipper, queries Postgres, asserts ≥5 rows with expected shape.

**Explicitly not tested:**

- Disk-full behavior (trust the OS).
- NFS / workstation-to-cluster mount reliability (infra, not our code).
- `flock` on NFS (local FS only, not a concern).
- Real secrets in real payloads (obvious reasons).

## Implementation Order (high-level, not a plan)

Each stage is independently useful. You could stop after any stage and still have a working subset.

1. **Hook + JSONL emission** → events are captured locally from the first deploy. No queryability yet, but no data is lost while building the rest.
2. **Schema + shipper** → backfill Postgres from accumulated JSONL. Human-queryable from this point.
3. **`just` recipes** → agent-facing interface lights up. Retry-check is the load-bearing recipe.
4. **Health-check integration** → trivial, last.

The detailed plan belongs to the writing-plans phase.

## Open Questions Deferred to Plan

1. **Workstation → cluster file access pattern.** NFS export vs. systemd user timer. Need to confirm what existing cluster workloads use for workstation-origin data.
2. **Exact Infisical path** for `claude_telemetry` credentials — follow the existing convention.
3. **Initial Grafana dashboard** — out of scope for v1 code; likely a follow-up once data exists. Maybe two or three panels: events per hour, top error signatures, tool-call duration percentiles.
4. **Hook script's project heuristic** — exact ordered list of project roots to check. Likely: `~/projects/clearwatch`, `~/homelab`, `~/projects/engram-go`, `~/projects/security-program`. Maintained in the script; updated by editing.

## Success Criteria

- Zero measurable impact on Claude Code session UX (hook p95 < 10ms, no network calls in hot path).
- Events captured for 100% of tool calls in a 24-hour smoke period.
- `just claude-events retry-check` returns correct counts across local + Postgres within <2 seconds.
- Shipper can tolerate a 24-hour Postgres outage and catch up cleanly on recovery.
- Scrub ruleset catches all v1 patterns against a synthetic "secret soup" fixture.
- `health-check.sh` surfaces shipper staleness within 15 minutes.
