package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ExitCodeLockContention is returned when a live process holds the backend
// lock. Uses EX_TEMPFAIL (75, from sysexits.h) — semantically "temporary
// failure, try again later" — to avoid colliding with exit code 2 which the
// flag package uses for parse errors.
const ExitCodeLockContention = 75

// redactURL strips credentials and query strings from a URL before it appears
// in error messages or logs. Uses net/url to parse.
//
// Redaction rules:
//   - User-info (user:password@) is always removed.
//   - Query string is always removed — tokens and API keys commonly appear
//     there (e.g. ?api_key=…, ?token=…) and must not appear in logs.
//   - Fragment is dropped by net/url.String() when RawQuery is cleared.
//
// If parsing fails or the URL has no host (e.g. unparseable strings that may
// contain credentials in the query), returns the fixed placeholder
// "[redacted-url]" rather than a sha256 hash. A hash of a low-entropy URL
// (e.g. "http://localhost:8000") is precomputable and provides false
// redaction assurance.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Unparseable or host-less — do not leak any fragment of the raw
		// value; return a fixed placeholder.
		return "[redacted-url]"
	}
	// Strip user-info and query string. The redacted form intentionally drops
	// both so neither credentials (in userinfo) nor tokens (in query params)
	// appear in logs or error messages.
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	return u.String()
}

// BackendLockConfig carries the locking-related flags parsed from the CLI.
type BackendLockConfig struct {
	// ExclusiveBackend guards the vLLM endpoint with a PID-liveness lockfile.
	// Default: true. Set false via --no-exclusive-backend.
	ExclusiveBackend bool

	// BackendLockDir overrides the directory where lock files are written.
	// When empty, defaultLockDir() is used.
	BackendLockDir string
}

// defaultLockDir returns the preferred lock directory:
//   - $XDG_RUNTIME_DIR/lme  if XDG_RUNTIME_DIR is set
//   - /tmp/lme               otherwise
func defaultLockDir() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "lme")
	}
	return "/tmp/lme"
}

// normalizeBackendURL lowercases the host, strips trailing slashes from the
// path, and drops any query string. This ensures that
// "http://OBLIVION:8000/v1/" and "http://oblivion:8000/v1" hash identically.
func normalizeBackendURL(rawURL string) string {
	// Simple string manipulation rather than net/url.Parse for consistent
	// hash-key construction: net/url round-trips may alter percent-encoding
	// or drop opaque path components for unusual but well-formed URLs, causing
	// the same logical backend URL to produce different lock-file names across
	// Go versions. The string-split approach is stable for our URL shape.
	scheme := ""
	rest := rawURL
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		scheme = strings.ToLower(rawURL[:idx+3])
		rest = rawURL[idx+3:]
	}

	// Drop query string.
	if qIdx := strings.Index(rest, "?"); qIdx >= 0 {
		rest = rest[:qIdx]
	}

	// Separate authority (host:port) from path.
	pathStart := strings.Index(rest, "/")
	authority := rest
	path := ""
	if pathStart >= 0 {
		authority = rest[:pathStart]
		path = rest[pathStart:]
	}

	// Lowercase host (includes port — safe since ports are digits).
	authority = strings.ToLower(authority)

	// Strip trailing slashes from path.
	path = strings.TrimRight(path, "/")

	return scheme + authority + path
}

// backendLockPath returns the absolute path of the lock file for the given
// backend URL inside dir. The filename is:
//
//	backend-<sha256(normalized_url)[:12]>.lock
func backendLockPath(dir, rawURL string) string {
	normalized := normalizeBackendURL(rawURL)
	sum := sha256.Sum256([]byte(normalized))
	hex12 := fmt.Sprintf("%x", sum[:6]) // 6 bytes → 12 hex chars
	return filepath.Join(dir, "backend-"+hex12+".lock")
}

// lockFileContent formats the content written to the lock file:
//
//	<pid>\n<start_unix>\n<invocation_id>\n
func lockFileContent(pid int, startUnix int64, invocationID string) []byte {
	return []byte(fmt.Sprintf("%d\n%d\n%s\n", pid, startUnix, invocationID))
}

// parseLockFile parses the three fields from a lock file. Returns pid=0 on
// any parse error so callers can treat it as a safe-to-reclaim stale file.
func parseLockFile(data []byte) (pid int, startUnix int64, invocationID string) {
	lines := strings.SplitN(strings.TrimRight(string(data), "\n"), "\n", 3)
	if len(lines) < 3 {
		return 0, 0, ""
	}
	p, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, 0, ""
	}
	s, err := strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
	if err != nil {
		return 0, 0, ""
	}
	return p, s, strings.TrimSpace(lines[2])
}

// isProcessAlive returns true if pid is a live process on this machine.
// Uses kill(pid, 0) — no signal is sent; ESRCH means the process is gone.
//
// Guard: pid <= 0 is never a valid user-space process and always returns
// false. This prevents a deadlock where parseLockFile returns pid=0 on a
// corrupt or partially-written lock file, which would otherwise be mistaken
// for a live process on Linux (kill(0, 0) returns success for the process
// group).
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// acquireBackendLock creates (or reclaims) the per-backend lock file under cfg.
// Returns a release function (always non-nil) and nil on success.
// Returns an error (caller should exit ExitCodeLockContention = 75) when a
// live process holds the lock. The caller must call release() in a defer
// immediately after a nil error return.
//
// When cfg.ExclusiveBackend is false, the function is a no-op and the release
// function is also a no-op.
func acquireBackendLock(cfg *BackendLockConfig, rawURL string) (release func(), err error) {
	noop := func() {}
	if !cfg.ExclusiveBackend {
		return noop, nil
	}

	dir := cfg.BackendLockDir
	if dir == "" {
		dir = defaultLockDir()
	}
	if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
		// Non-fatal: if we can't create the dir, skip locking and warn.
		// TODO(#808-S3): migrate to slog when slog is wired into cmd/longmemeval.
		log.Printf("WARN backend lock: cannot create lock dir %q: %v — skipping lock", dir, mkErr)
		return noop, nil
	}

	lockPath := backendLockPath(dir, rawURL)

	// Open (or create) the lock file.
	fd, openErr := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if openErr != nil {
		// TODO(#808-S3): migrate to slog when slog is wired into cmd/longmemeval.
		log.Printf("WARN backend lock: cannot open lock file %q: %v — skipping lock", lockPath, openErr)
		return noop, nil
	}

	// Attempt non-blocking exclusive flock.
	flockErr := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if flockErr == nil {
		// Lock acquired cleanly. Write our PID record.
		invID := fmt.Sprintf("%d", time.Now().UnixNano())
		content := lockFileContent(os.Getpid(), time.Now().Unix(), invID)
		if err := fd.Truncate(0); err != nil {
			// Write setup failed — release flock, remove lock file, return error.
			_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
			_ = fd.Close()
			_ = os.Remove(lockPath)
			return nil, fmt.Errorf("backend lock: truncate %q failed: %w", lockPath, err)
		}
		if _, writeErr := fd.WriteAt(content, 0); writeErr != nil {
			// Write failed — release flock, remove lock file, return error.
			_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
			_ = fd.Close()
			_ = os.Remove(lockPath)
			return nil, fmt.Errorf("backend lock: write pid record to %q failed: %w", lockPath, writeErr)
		}

		releaseFunc := func() {
			_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
			_ = fd.Close()
		}
		return releaseFunc, nil
	}

	// EWOULDBLOCK — someone else holds the flock. Check liveness.
	if flockErr != syscall.EWOULDBLOCK {
		// Unexpected flock error; skip locking with a warning.
		_ = fd.Close()
		// TODO(#808-S3): migrate to slog when slog is wired into cmd/longmemeval.
		log.Printf("WARN backend lock: flock error on %q: %v — skipping lock", lockPath, flockErr)
		return noop, nil
	}

	// Read the current lock file contents to get PID/start/invocation.
	data, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		_ = fd.Close()
		return nil, fmt.Errorf(
			"ERROR another lme run holds the lock on backend %s (pid=unknown, started=unknown, invocation=unknown). "+
				"Wait for it, or pass --no-exclusive-backend if you accept result contamination",
			redactURL(rawURL))
	}

	pid, startUnix, invID := parseLockFile(data)

	// pid <= 0 means corrupt/partial lock file — isProcessAlive guards this
	// too, but the explicit check here makes the reclaim path clear.
	if !isProcessAlive(pid) {
		// Stale lock — dead or invalid pid. Log and reclaim.
		startTime := time.Unix(startUnix, 0).UTC().Format(time.RFC3339)
		// TODO(#808-S3): migrate to slog when slog is wired into cmd/longmemeval.
		log.Printf("WARN stale lock from pid=%d (dead), started=%s inv=%s — reclaiming %s",
			pid, startTime, invID, lockPath)

		// S1 fix: close the old fd, re-open WITHOUT O_TRUNC, acquire flock
		// FIRST, THEN truncate+write under the lock. Opening with O_TRUNC
		// before holding the flock creates a TOCTOU race window.
		_ = fd.Close()
		fd2, openErr2 := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
		if openErr2 != nil {
			// TODO(#808-S3): migrate to slog when slog is wired into cmd/longmemeval.
			log.Printf("WARN backend lock: reclaim open failed: %v — skipping lock", openErr2)
			return noop, nil
		}
		// Acquire flock (blocking is fine — we've decided to reclaim a dead lock).
		flockErr2 := syscall.Flock(int(fd2.Fd()), syscall.LOCK_EX)
		if flockErr2 != nil {
			_ = fd2.Close()
			// TODO(#808-S3): migrate to slog when slog is wired into cmd/longmemeval.
			log.Printf("WARN backend lock: reclaim flock failed: %v — skipping lock", flockErr2)
			return noop, nil
		}
		// Truncate and write under the lock.
		if truncErr := fd2.Truncate(0); truncErr != nil {
			_ = syscall.Flock(int(fd2.Fd()), syscall.LOCK_UN)
			_ = fd2.Close()
			_ = os.Remove(lockPath)
			return nil, fmt.Errorf("backend lock: reclaim truncate %q failed: %w", lockPath, truncErr)
		}
		invID2 := fmt.Sprintf("%d", time.Now().UnixNano())
		content := lockFileContent(os.Getpid(), time.Now().Unix(), invID2)
		if _, writeErr := fd2.WriteAt(content, 0); writeErr != nil {
			_ = syscall.Flock(int(fd2.Fd()), syscall.LOCK_UN)
			_ = fd2.Close()
			_ = os.Remove(lockPath)
			return nil, fmt.Errorf("backend lock: reclaim write to %q failed: %w", lockPath, writeErr)
		}

		releaseFunc := func() {
			_ = syscall.Flock(int(fd2.Fd()), syscall.LOCK_UN)
			_ = fd2.Close()
		}
		return releaseFunc, nil
	}

	// Live process holds the lock.
	_ = fd.Close()
	startTime := "unknown"
	if startUnix > 0 {
		startTime = time.Unix(startUnix, 0).UTC().Format(time.RFC3339)
	}
	return nil, fmt.Errorf(
		"ERROR another lme run holds the lock on backend %s (pid=%d, started=%s, invocation=%s); "+
			"wait for it or pass --no-exclusive-backend to accept result contamination "+
			"(exit %d = EX_TEMPFAIL)",
		redactURL(rawURL), pid, startTime, invID, ExitCodeLockContention)
}
