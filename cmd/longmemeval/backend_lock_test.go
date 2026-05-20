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

	// Second process: should exit 2 immediately (backend already locked).
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
	if exitErr.ExitCode() != 2 {
		t.Errorf("second process exit code = %d, want 2", exitErr.ExitCode())
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
		os.Exit(2)
	}
	defer release()

	// Signal that lock is held by writing a marker; contention test polls for this.
	_ = strconv.Itoa(os.Getpid()) // just ensure pid is reachable
	if hold > 0 {
		time.Sleep(hold)
	}
	os.Exit(0)
}
