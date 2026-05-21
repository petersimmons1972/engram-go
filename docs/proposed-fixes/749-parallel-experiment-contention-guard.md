> **SUPERSEDED** — Implemented in PR #808 (merged via fix/issue-749). Lock contention uses exit code 75. This document retained for design context only.

# Issue #749 — lme: no contention guard for parallel experiments against single vLLM endpoint

**Severity:** nice-to-have
**Area:** lme-tooling
**Status:** Implemented (PR #808)

## Root cause

`cmd/longmemeval/run.go:runRun()` has no mechanism to detect or prevent concurrent `longmemeval run` processes hitting the same `--llm-url` endpoint. The Phase 0 campaign (2026-05-19) had multiple scoring jobs auto-started, causing GPU contention that required manual kill. The `runRun()` entry point at line 34 fires immediately — there is no lockfile, advisory check, or warning.

## Repro

```bash
# Terminal 1
./bin/longmemeval run --llm-url http://oblivion:8000/v1 --llm-model inference \
  --data testdata/longmemeval/longmemeval_m_cleaned.json --out /tmp/exp-a &

# Terminal 2 — fires concurrently, causes GPU contention with no warning
./bin/longmemeval run --llm-url http://oblivion:8000/v1 --llm-model inference \
  --data testdata/longmemeval/longmemeval_m_cleaned.json --out /tmp/exp-b &

# Both processes saturate oblivion; throughput halves; no error surfaced
```

## Proposed patch

Add `--exclusive-backend` flag. When set, acquire a file lock keyed to the `--llm-url` before starting workers. Exit 1 immediately if the lock is held by another process.

```diff
--- a/cmd/longmemeval/main.go
+++ b/cmd/longmemeval/main.go
@@ -27,6 +27,7 @@ type Config struct {
 	NoCleanup  bool
 	Retries    int
 	OutDir     string
+	ExclusiveBackend bool
 	LLMBaseURL string
 	LLMModel   string
@@ -111,6 +112,7 @@ func dispatch(...) int {
 	fs.BoolVar(&cfg.NoCleanup, "no-cleanup", false, "Skip Engram project deletion after run stage")
+	fs.BoolVar(&cfg.ExclusiveBackend, "exclusive-backend", false, "Acquire a per-endpoint lockfile; exit 1 if another process holds it")
```

```diff
--- a/cmd/longmemeval/run.go
+++ b/cmd/longmemeval/run.go
@@ -34,6 +34,15 @@ func runRun(cfg *Config) int {
+	if cfg.ExclusiveBackend && cfg.LLMBaseURL != "" {
+		lockPath := backendLockPath(cfg.LLMBaseURL)
+		unlock, err := acquireFileLock(lockPath, 4*time.Hour)
+		if err != nil {
+			log.Printf("ERROR exclusive-backend: lock held at %s — is another longmemeval run in progress? (%v)", lockPath, err)
+			return 1
+		}
+		defer unlock()
+		log.Printf("run: exclusive-backend lock acquired at %s", lockPath)
+	}
 	items := loadItems(cfg.DataFile)
```

New helpers (new file `cmd/longmemeval/lock.go`):
```go
// backendLockPath returns ~/.cache/longmemeval/<sha256-of-url[:16]>.lock
func backendLockPath(llmURL string) string { ... }

// acquireFileLock tries to exclusively lock the file. Returns unlock func
// and nil on success. Returns error if locked. Auto-releases if file mtime
// is older than ttl (crashed process cleanup).
func acquireFileLock(path string, ttl time.Duration) (func(), error) { ... }
```

## TDD scenarios

1. **exclusive_backend_lock_acquired** — Given `--exclusive-backend` and no competing process, when runRun starts, then a lockfile exists at `backendLockPath(llmURL)` during execution and is removed on exit.
2. **exclusive_backend_lock_held_exit1** — Given `--exclusive-backend` and a lockfile already held (simulated by pre-creating it with recent mtime), when runRun is called, then it returns exit code 1 and logs "lock held."
3. **exclusive_backend_stale_lock_acquired** — Given `--exclusive-backend` and a lockfile whose mtime is >4h old, when runRun starts, then the stale lock is overwritten and execution proceeds normally.
4. **no_exclusive_backend_no_lock** — Given `--exclusive-backend=false` (default), when runRun starts, then no lockfile is created (backwards compat).

## Risk notes

- `--exclusive-backend` is opt-in; no change to existing behaviour when flag is absent.
- Lock is process-scoped via `syscall.Flock` (Unix) or `os.OpenFile` + `O_EXCL` (portable). Use `O_EXCL` for portability.
- The 4h TTL is a safety net for crashed processes; normal runs take <2h for 500 items on oblivion.
- Does not guard against two processes on different machines hitting the same URL — this is single-host protection only.

## Rollout

No schema changes. Rebuild binary (`make build`). No infra changes.

## Out of scope (followups)

- Network-level rate limiting on the vLLM endpoint (e.g. nginx upstream concurrency limit) — would be the authoritative fix but requires infra changes.
- A `longmemeval status` subcommand that shows active runs and their lockfile state.
