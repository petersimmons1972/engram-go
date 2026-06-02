# Engram Hook Daemon (#396)

A single long-running process that handles all Claude Code hook events for
Engram, replacing the per-event shell scripts. Hook events are forwarded to the
daemon over a Unix domain socket by a thin shim; the daemon holds all mutable
state in memory, eliminating the `flock`/temp-file races the per-event scripts
had to work around.

## Why

Every Claude Code hook event (`SessionStart`, `UserPromptSubmit`, `PostToolUse`,
`Stop`, `PreCompact`, `PreToolUse`) previously spawned one or more shell scripts.
Each paid Python/`curl` startup cost, re-read `mcp_servers.json` and
`fallback.md` from disk, coordinated shared file access with `flock`, and opened
a fresh HTTP connection to Engram. The daemon collapses all of that into one
process:

| | Per-event scripts | Daemon |
|---|---|---|
| Startup cost per event | ~50–150 ms | ~1 ms socket round-trip |
| Token storage | Disk read every event | In-memory; written only on change |
| `fallback.md` concurrency | `flock` + atomic rename | Single goroutine, `sync.Mutex` |
| Engram HTTP connection | New connection per event | Persistent pooled client |
| Auth state | HTTP probe every prompt | Cached with 120 s TTL, re-probed on miss |
| Race window | Between flock release and re-acquire | Zero — single owner |

## Architecture

- **`engram hook-daemon`** — starts the daemon: binds `~/.claude/.engram-hook.sock`,
  handles each connection in its own goroutine, and self-terminates after an idle
  timeout (default 10 min). `--detach` re-execs into the background with stderr
  redirected to `~/.claude/logs/engram-hook-daemon.log` (rotated at 5 MiB).
- **`engram hook <EventName>`** — the shim client: reads the raw Claude Code hook
  stdin JSON, dials the socket (lazy-starting the daemon on the first event of a
  session), prints the daemon's stdout, and exits with the daemon's exit code.
  Pure Go — no `socat` dependency.
- **`engram hook status`** — reports whether the daemon is running.

### Socket protocol

Request (shim → daemon):

```json
{ "hook": "SessionStart", "payload": { ...raw Claude Code hook stdin JSON... } }
```

Response (daemon → shim):

```json
{ "stdout": "...", "systemMessage": "...", "exit_code": 0 }
```

One JSON request line in, one JSON response line out, then the connection closes.

### Lifecycle (Option C)

- **Lazy start.** The first shim that finds no live socket starts the daemon with
  `--detach`, waits ~150 ms, and retries the connection. No service manager
  required; survives daemon crashes (the next event restarts it).
- **Idle timeout.** With no events for the idle window (default 10 min), the
  daemon shuts itself down. This prevents accumulation if Claude Code crashes
  without sending `Stop`.
- **Stop = drain, not kill.** `Stop` records the session-end marker, flushes the
  fallback buffer, and shortens the idle window to 30 s — so the daemon winds
  down soon after the session ends, while still letting in-flight writes drain.
  `Stop` never sends `SIGTERM`; if it never fires, the normal idle timeout still
  cleans up. This is why the daemon does not depend on `Stop` firing reliably
  (Claude Code does not guarantee it on crash).

### Handler mapping

| Hook | Ported from | Behavior |
|---|---|---|
| `SessionStart` | `engram-token-refresh.sh` + `engram-session-recall.sh` + flush | Validate cached token, flush fallback buffer, inject merged global+project recall into `MEMORY.md` |
| `UserPromptSubmit` | `engram-auth-check.sh` | Fast auth check backed by 120 s in-memory TTL cache |
| `PreToolUse` | `engram-precheck.sh` | Health probe; `systemMessage` when down |
| `PostToolUse` | `engram-mcp-error-handler` | Buffer a fallback entry on a failed `mcp__engram__*` call |
| `PreCompact` | `pre-compact-engram` | Flush the fallback buffer before compaction |
| `Stop` | `engram-session-end.sh` | Store session-end marker, flush, drain |

## Rollout

The daemon path is gated behind `ENGRAM_HOOK_DAEMON=1`. When the flag is unset,
`engram-hook-shim.sh` is a no-op and the legacy per-event scripts handle events,
so the daemon can roll out behind a flag until it is proven stable. To enable:

```bash
export ENGRAM_HOOK_DAEMON=1
```

and register `engram-hook-shim.sh <EventName>` for each hook event in
`~/.claude/settings.json`.

The memory directory (`MEMORY.md` / `fallback.md`) is derived at runtime from
the home path using Claude Code's slug convention — every `/` becomes `-`, so
`/home/psimmons` → `~/.claude/projects/-home-psimmons/memory`. Set
`ENGRAM_MEMORY_DIR` to override the computed path for non-standard layouts.

## Code map

- `internal/hookdaemon/` — the testable core (protocol, daemon state, handlers,
  socket server, shim client, production adapters). Unit-tested with injected
  fakes for the HTTP client, token store, memory writer, and fallback store; no
  real Engram server or wall clock required.
- `cmd/engram/hook.go` — thin wiring: assembles the production adapters and
  dispatches the `hook-daemon` / `hook` subcommands.
- `hooks/engram/engram-hook-shim.sh` — the bash entry point that calls
  `engram hook <EventName>`.
