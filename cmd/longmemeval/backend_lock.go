package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

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
	// Split on "://" to get scheme+rest without importing net/url (avoids a
	// dependency on stdlib URL parsing that can behave differently for edge
	// inputs). Simple string manipulation is sufficient for our well-formed URLs.
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
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// acquireBackendLock creates (or reclaims) the per-backend lock file under cfg.
// Returns a release function (always non-nil) and nil on success.
// Returns an error with exit-code semantics (exit 2) when a live process holds
// the lock. The caller must call release() in a defer immediately after a nil
// error return.
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
		log.Printf("WARN backend lock: cannot create lock dir %q: %v — skipping lock", dir, mkErr)
		return noop, nil
	}

	lockPath := backendLockPath(dir, rawURL)

	// Open (or create) the lock file.
	fd, openErr := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if openErr != nil {
		log.Printf("WARN backend lock: cannot open lock file %q: %v — skipping lock", lockPath, openErr)
		return noop, nil
	}

	// Attempt non-blocking exclusive flock.
	flockErr := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if flockErr == nil {
		// Lock acquired cleanly. Write our PID record.
		invID := fmt.Sprintf("%d", time.Now().UnixNano())
		content := lockFileContent(os.Getpid(), time.Now().Unix(), invID)
		_ = fd.Truncate(0)
		_, _ = fd.WriteAt(content, 0)

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
		log.Printf("WARN backend lock: flock error on %q: %v — skipping lock", lockPath, flockErr)
		return noop, nil
	}

	// Read the current lock file contents to get PID/start/invocation.
	data, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		_ = fd.Close()
		return nil, fmt.Errorf(
			"ERROR another lme run holds the lock on backend %s (pid=unknown, started=unknown, invocation=unknown). "+
				"Wait for it, or pass --no-exclusive-backend if you accept result contamination", rawURL)
	}

	pid, startUnix, invID := parseLockFile(data)

	if pid > 0 && !isProcessAlive(pid) {
		// Stale lock — dead process. Log and reclaim.
		startTime := time.Unix(startUnix, 0).UTC().Format(time.RFC3339)
		log.Printf("WARN stale lock from pid=%d (dead), started=%s inv=%s — reclaiming %s",
			pid, startTime, invID, lockPath)

		// Re-try flock with a brief re-open to get a fresh fd after the original
		// locker released. The dead process's flock has already been cleared by
		// the kernel when the process died; a second non-blocking attempt should
		// now succeed.
		_ = fd.Close()
		fd2, openErr2 := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
		if openErr2 != nil {
			log.Printf("WARN backend lock: reclaim open failed: %v — skipping lock", openErr2)
			return noop, nil
		}
		flockErr2 := syscall.Flock(int(fd2.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if flockErr2 != nil {
			_ = fd2.Close()
			log.Printf("WARN backend lock: reclaim flock failed: %v — skipping lock", flockErr2)
			return noop, nil
		}
		invID2 := fmt.Sprintf("%d", time.Now().UnixNano())
		content := lockFileContent(os.Getpid(), time.Now().Unix(), invID2)
		_, _ = fd2.WriteAt(content, 0)

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
		"ERROR another lme run holds the lock on backend %s (pid=%d, started=%s, invocation=%s). "+
			"Wait for it, or pass --no-exclusive-backend if you accept result contamination",
		rawURL, pid, startTime, invID)
}
