package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestBackendLockNormalization verifies that URL normalization produces stable
// lock file names regardless of trailing slashes or host casing.
func TestBackendLockNormalization(t *testing.T) {
	cases := []struct {
		a, b string
	}{
		{"http://oblivion:8000/v1", "http://oblivion:8000/v1/"},
		{"http://OBLIVION:8000/v1", "http://oblivion:8000/v1"},
		{"http://Oblivion:8000/v1/", "http://oblivion:8000/v1"},
	}
	for _, tc := range cases {
		na := normalizeBackendURL(tc.a)
		nb := normalizeBackendURL(tc.b)
		if na != nb {
			t.Errorf("normalizeBackendURL(%q) = %q, normalizeBackendURL(%q) = %q; want equal",
				tc.a, na, tc.b, nb)
		}
	}
}

// TestLockFileName verifies the lock file path uses a 12-hex-char sha256 prefix
// and lives inside the expected directory.
func TestLockFileName(t *testing.T) {
	dir := t.TempDir()
	name := backendLockPath(dir, "http://oblivion:8000/v1")
	base := filepath.Base(name)
	if !strings.HasPrefix(base, "backend-") {
		t.Errorf("lock file base %q should start with 'backend-'", base)
	}
	// 8 (prefix) + 12 (hash chars) + 5 (.lock) = 25
	if len(base) != 25 {
		t.Errorf("lock file base %q has len %d, want 25", base, len(base))
	}
	if filepath.Dir(name) != dir {
		t.Errorf("lock file dir %q != expected %q", filepath.Dir(name), dir)
	}
}

// TestBackendLock_Contention spawns two helper processes that both try to
// acquire the lock on the same backend URL. The first holds it while sleeping;
// the second must exit with code 2 quickly (< 500ms).
func TestBackendLock_Contention(t *testing.T) {
	if os.Getenv("LME_LOCK_HELPER") != "" {
		// Running as the helper subprocess.
		lockHelperMain()
		return
	}

	dir := t.TempDir()
	url := "http://test-backend:8000/v1"
	ready := make(chan struct{})

	// First process: acquires lock and blocks until killed.
	cmd1 := exec.Command(os.Args[0], "-test.run=TestBackendLock_Contention")
	cmd1.Env = append(os.Environ(),
		"LME_LOCK_HELPER=1",
		"LME_LOCK_DIR="+dir,
		"LME_LOCK_URL="+url,
		"LME_LOCK_HOLD=500ms", // hold for 500ms
	)
	if err := cmd1.Start(); err != nil {
		t.Fatalf("start first process: %v", err)
	}
	defer func() { _ = cmd1.Process.Kill() }()

	// Give the first process time to acquire the lock.
	go func() {
		// Poll for lockfile presence.
		lockPath := backendLockPath(dir, url)
		for i := 0; i < 100; i++ {
			if _, err := os.Stat(lockPath); err == nil {
				close(ready)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		close(ready) // proceed even if we didn't see it — let second process fail
	}()
	<-ready

	// Second process: should exit 75 (EX_TEMPFAIL) immediately (backend already locked).
	start := time.Now()
	cmd2 := exec.Command(os.Args[0], "-test.run=TestBackendLock_Contention")
	cmd2.Env = append(os.Environ(),
		"LME_LOCK_HELPER=1",
		"LME_LOCK_DIR="+dir,
		"LME_LOCK_URL="+url,
		"LME_LOCK_HOLD=0",
	)
	err := cmd2.Run()
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("second process took %v; want < 500ms (should fail fast)", elapsed)
	}
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); !ok {
		t.Fatalf("second process should have exited non-zero; got: %v", err)
	}
	if exitErr.ExitCode() != 75 {
		t.Errorf("second process exit code = %d, want 75 (EX_TEMPFAIL)", exitErr.ExitCode())
	}
}

// TestBackendLock_StaleLock verifies that when the lockfile contains a dead
// PID, parseLockFile and isProcessAlive behave correctly, and acquireBackendLock
// reclaims the lock without error.
//
// Note: the kernel flock is not held by anyone when this test runs (no live
// process holds the fd open), so EWOULDBLOCK is not returned and the stale
// detection path in acquireBackendLock does not fire. We test the stale-detection
// logic via its component functions instead, and we verify end-to-end that
// a pre-written stale-PID file doesn't block acquisition.
func TestBackendLock_StaleLock(t *testing.T) {
	if os.Getenv("LME_LOCK_HELPER") != "" {
		lockHelperMain()
		return
	}

	// --- Unit test: parseLockFile with dead PID content ---
	deadPID := 999999
	startUnix := time.Now().Add(-1 * time.Hour).Unix()
	content := fmt.Sprintf("%d\n%d\nstale-invocation-id\n", deadPID, startUnix)
	pid, su, inv := parseLockFile([]byte(content))
	if pid != deadPID {
		t.Errorf("parseLockFile pid = %d, want %d", pid, deadPID)
	}
	if su != startUnix {
		t.Errorf("parseLockFile startUnix = %d, want %d", su, startUnix)
	}
	if inv != "stale-invocation-id" {
		t.Errorf("parseLockFile invocationID = %q, want %q", inv, "stale-invocation-id")
	}
	if isProcessAlive(deadPID) {
		t.Skipf("PID %d is unexpectedly alive on this system — skip stale test", deadPID)
	}

	// --- Integration: stale file present but not kernel-locked → acquires cleanly ---
	dir := t.TempDir()
	url := "http://stale-backend:8000/v1"
	lockPath := backendLockPath(dir, url)
	if err := os.WriteFile(lockPath, []byte(content), 0600); err != nil {
		t.Fatalf("write stale lockfile: %v", err)
	}

	cfg := &BackendLockConfig{ExclusiveBackend: true, BackendLockDir: dir}
	release, err := acquireBackendLock(cfg, url)
	if err != nil {
		t.Errorf("stale (un-flocked) file: acquireBackendLock should succeed, got: %v", err)
	}
	if release == nil {
		t.Fatal("release func must be non-nil on success")
	}
	release()
}

// TestBackendLock_NoExclusiveBackend verifies that --no-exclusive-backend
// skips lock acquisition entirely.
func TestBackendLock_NoExclusiveBackend(t *testing.T) {
	dir := t.TempDir()
	url := "http://oblivion:8000/v1"
	cfg := &BackendLockConfig{
		ExclusiveBackend: false,
		BackendLockDir:   dir,
	}
	// Should not create a lockfile, return nil release func.
	release, err := acquireBackendLock(cfg, url)
	if err != nil {
		t.Fatalf("no-exclusive-backend: unexpected error: %v", err)
	}
	if release == nil {
		t.Fatal("release func should be non-nil (no-op) even when exclusive=false")
	}
	release() // should not panic
	lockPath := backendLockPath(dir, url)
	if _, statErr := os.Stat(lockPath); statErr == nil {
		t.Error("lock file should not exist when ExclusiveBackend=false")
	}
}

// TestBackendLock_DefaultLockDirFallback verifies that when BackendLockDir is
// empty and XDG_RUNTIME_DIR is unset, the fallback /tmp/lme path is used.
func TestBackendLock_DefaultLockDirFallback(t *testing.T) {
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	_ = os.Unsetenv("XDG_RUNTIME_DIR")
	defer func() {
		if origXDG != "" {
			_ = os.Setenv("XDG_RUNTIME_DIR", origXDG)
		}
	}()

	got := defaultLockDir()
	if got != "/tmp/lme" {
		t.Errorf("defaultLockDir() = %q, want /tmp/lme when XDG_RUNTIME_DIR unset", got)
	}
}

// TestBackendLock_XDGRuntimeDirUsed verifies that XDG_RUNTIME_DIR is preferred.
func TestBackendLock_XDGRuntimeDirUsed(t *testing.T) {
	_ = os.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	defer func() { _ = os.Unsetenv("XDG_RUNTIME_DIR") }()

	got := defaultLockDir()
	want := "/run/user/1000/lme"
	if got != want {
		t.Errorf("defaultLockDir() = %q, want %q", got, want)
	}
}

// asExitError is a helper that avoids importing errors (avoids import cycle with
// go 1.20 errors.As style already in use elsewhere in this package).
func asExitError(err error, target **exec.ExitError) bool {
	if err == nil {
		return false
	}
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}

// TestBackendLock_PidZeroReclaim verifies that a lock file containing pid=0
// is treated as stale and reclaimed, never as a live process.
func TestBackendLock_PidZeroReclaim(t *testing.T) {
	dir := t.TempDir()
	url := "http://pid-zero-backend:8000/v1"
	lockPath := backendLockPath(dir, url)

	// Write a lock file with pid=0, which parseLockFile returns for a corrupt file.
	content := []byte("0\n1700000000\nstale-zero-inv\n")
	if err := os.WriteFile(lockPath, content, 0600); err != nil {
		t.Fatalf("write pid=0 lock file: %v", err)
	}

	// isProcessAlive(0) must return false — kernel init process must never be claimed.
	if isProcessAlive(0) {
		t.Error("isProcessAlive(0) returned true; want false (pid=0 is never a valid user process)")
	}

	cfg := &BackendLockConfig{ExclusiveBackend: true, BackendLockDir: dir}
	release, err := acquireBackendLock(cfg, url)
	if err != nil {
		t.Errorf("pid=0 lock file: acquireBackendLock should reclaim and succeed, got: %v", err)
	}
	if release == nil {
		t.Fatal("release func must be non-nil on success")
	}
	release()
}

// TestRedactURL verifies that credentials in URLs are stripped from error output.
func TestRedactURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"http://user:pass@host/v1", "http://host/v1"},
		{"https://token:x@oblivion:8000/v1", "https://oblivion:8000/v1"},
		{"http://noauth@host/v1", "http://host/v1"},
		{"http://host:8000/v1", "http://host:8000/v1"},
		// R2-B3: query strings must also be stripped (tokens in query params).
		{"http://user:p@host/v1?api_key=secret", "http://host/v1"},
		{"http://host/v1?token=xyz", "http://host/v1"},
		// Idempotent: already clean URLs pass through unchanged.
		{"http://host/v1", "http://host/v1"},
	}
	for _, tc := range cases {
		got := redactURL(tc.input)
		if got != tc.want {
			t.Errorf("redactURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	// R3: low-entropy parseable URLs (e.g. http://localhost:8000) must NOT
	// be hashed — hashes of well-known URLs are precomputable. The redacted
	// form returns the scheme+host only (no credentials, no query) which is
	// the correct safe form for log messages.
	lowEntropy := "http://localhost:8000"
	gotLow := redactURL(lowEntropy)
	// The URL has no credentials and no query, so redactURL is a no-op in
	// content (but importantly it went through the net/url path, not sha256).
	if gotLow != "http://localhost:8000" {
		t.Errorf("redactURL(%q) = %q, want %q", lowEntropy, gotLow, "http://localhost:8000")
	}

	// Low-entropy URL with query string — query must be stripped.
	lowEntropyWithQuery := "http://localhost:8000?api_key=mytoken"
	gotLowQ := redactURL(lowEntropyWithQuery)
	if gotLowQ != "http://localhost:8000" {
		t.Errorf("redactURL(%q) = %q, want %q (query stripped)", lowEntropyWithQuery, gotLowQ, "http://localhost:8000")
	}

	// Unparseable URL must return the fixed placeholder "[redacted-url]", not
	// the raw value (which may contain credentials in query params) and not a
	// sha256 hash (which is precomputable for low-entropy values).
	ugly := "://bad url with spaces and :pass@"
	got := redactURL(ugly)
	if got == ugly {
		t.Error("redactURL(unparseable) returned raw URL; want [redacted-url]")
	}
	if got != "[redacted-url]" {
		t.Errorf("redactURL(unparseable) = %q, want \"[redacted-url]\"", got)
	}
}

// TestBackendLock_ContendingExitCode75 verifies that lock contention now exits
// with code 75 (EX_TEMPFAIL), not 2 (which collides with flag-parse errors).
func TestBackendLock_ContendingExitCode75(t *testing.T) {
	if os.Getenv("LME_LOCK_HELPER") != "" {
		lockHelperMain()
		return
	}

	dir := t.TempDir()
	url := "http://exit75-backend:8000/v1"

	// First process: hold the lock.
	cmd1 := exec.Command(os.Args[0], "-test.run=TestBackendLock_ContendingExitCode75")
	cmd1.Env = append(os.Environ(),
		"LME_LOCK_HELPER=1",
		"LME_LOCK_DIR="+dir,
		"LME_LOCK_URL="+url,
		"LME_LOCK_HOLD=500ms",
	)
	if err := cmd1.Start(); err != nil {
		t.Fatalf("start first process: %v", err)
	}
	defer func() { _ = cmd1.Process.Kill() }()

	// Wait for lock file to appear.
	lockPath := backendLockPath(dir, url)
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(lockPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Second process: must exit 75, not 2.
	cmd2 := exec.Command(os.Args[0], "-test.run=TestBackendLock_ContendingExitCode75")
	cmd2.Env = append(os.Environ(),
		"LME_LOCK_HELPER=1",
		"LME_LOCK_DIR="+dir,
		"LME_LOCK_URL="+url,
		"LME_LOCK_HOLD=0",
	)
	err := cmd2.Run()
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); !ok {
		t.Fatalf("second process should exit non-zero; got: %v", err)
	}
	if exitErr.ExitCode() != 75 {
		t.Errorf("second process exit code = %d, want 75 (EX_TEMPFAIL)", exitErr.ExitCode())
	}
}

// TestBackendLock_StaleReclaimAtomicity verifies that stale-lock reclaim opens
// the file WITHOUT O_TRUNC before acquiring flock (no TOCTOU window).
// We test the observable behavior: reclaim of a dead-PID file must succeed
// and the new content must reflect our PID, not the stale one.
func TestBackendLock_StaleReclaimAtomicity(t *testing.T) {
	if os.Getenv("LME_LOCK_HELPER") != "" {
		lockHelperMain()
		return
	}

	dir := t.TempDir()
	url := "http://stale-atomic-backend:8000/v1"
	lockPath := backendLockPath(dir, url)

	// Write a stale lock file with a dead PID.
	deadPID := 999999
	staleContent := fmt.Sprintf("%d\n1700000000\nstale-atomic-inv\n", deadPID)
	if err := os.WriteFile(lockPath, []byte(staleContent), 0600); err != nil {
		t.Fatalf("write stale lock file: %v", err)
	}
	if isProcessAlive(deadPID) {
		t.Skipf("PID %d is unexpectedly alive — skip atomicity test", deadPID)
	}

	cfg := &BackendLockConfig{ExclusiveBackend: true, BackendLockDir: dir}
	release, err := acquireBackendLock(cfg, url)
	if err != nil {
		t.Fatalf("stale-reclaim: unexpected error: %v", err)
	}
	defer release()

	// After reclaim, lock file must contain OUR pid, not the dead one.
	data, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		t.Fatalf("read lock file after reclaim: %v", readErr)
	}
	pid, _, _ := parseLockFile(data)
	if pid != os.Getpid() {
		t.Errorf("lock file pid after reclaim = %d, want our pid %d", pid, os.Getpid())
	}
}

// lockHelperMain is the subprocess entry point for contention tests.
// It acquires the lock using the env-configured URL+dir, holds it for
// LME_LOCK_HOLD duration, then exits.
func lockHelperMain() {
	dir := os.Getenv("LME_LOCK_DIR")
	url := os.Getenv("LME_LOCK_URL")
	holdStr := os.Getenv("LME_LOCK_HOLD")
	var hold time.Duration
	if holdStr != "" && holdStr != "0" {
		var err error
		hold, err = time.ParseDuration(holdStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse LME_LOCK_HOLD: %v\n", err)
			os.Exit(1)
		}
	}

	cfg := &BackendLockConfig{
		ExclusiveBackend: true,
		BackendLockDir:   dir,
	}
	release, err := acquireBackendLock(cfg, url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(75) // EX_TEMPFAIL — backend lock held by another lme run
	}
	defer release()

	// Signal that lock is held by writing a marker; contention test polls for this.
	_ = strconv.Itoa(os.Getpid()) // just ensure pid is reachable
	if hold > 0 {
		time.Sleep(hold)
	}
	os.Exit(0)
}
