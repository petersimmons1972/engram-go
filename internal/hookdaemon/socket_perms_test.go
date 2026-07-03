package hookdaemon

// Tests for issue #1191 (X4): unix socket TOCTOU and atomic 0600 permissions.
//
// The socket must be created at 0600 from the first moment the inode exists.
// These tests run against the real filesystem under t.TempDir().

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// newDaemonForTest builds a minimal *Daemon for socket permission tests.
func newDaemonForTest(t *testing.T) *Daemon {
	t.Helper()
	clk := &fakeClock{now: 1_000_000}
	d, err := New(Config{
		Engram:      &fakeEngram{authOK: true, recallByProj: map[string][]byte{}},
		Tokens:      &fakeTokens{token: "tok"},
		Clock:       clk,
		IdleTimeout: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

// TestSocketModeAtCreation verifies the socket is created at 0600 even when
// the ambient process umask is 0 (maximally permissive). With a 0 umask the
// OS would normally grant world-write permissions; the server must override
// this by manipulating the umask itself before calling Listen.
//
// This is a TDD gate: on the pre-fix code the socket would be 0777 (umask=0),
// not 0600.
func TestSocketModeAtCreation(t *testing.T) {
	// Set umask to 0 so any file not explicitly restricted gets full permissions.
	// Restore on exit so other tests in the same process are not affected.
	old := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(old) })

	sock := filepath.Join(t.TempDir(), "hook.sock")
	srv, err := NewServer(context.Background(), newDaemonForTest(t), sock)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	info, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	// Mask to lower permission bits only (ignore file type bits).
	mode := info.Mode().Perm()
	const want = os.FileMode(0o600)
	if mode != want {
		t.Errorf("socket mode = %04o, want %04o — TOCTOU fix regression (#1191)", mode, want)
	}
}

// TestSocketModeDefaultUmask verifies the socket is 0600 under the normal
// (default) ambient umask as well — belt-and-suspenders confirmation.
func TestSocketModeDefaultUmask(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "hook.sock")
	srv, err := NewServer(context.Background(), newDaemonForTest(t), sock)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	info, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	mode := info.Mode().Perm()
	const want = os.FileMode(0o600)
	if mode != want {
		t.Errorf("socket mode = %04o, want %04o (#1191)", mode, want)
	}
}

// TestExistingSocketCleaned verifies that NewServer starts successfully and
// replaces a stale socket file that has no live owner.
func TestExistingSocketCleaned(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "hook.sock")

	// Create a stale socket-shaped regular file to simulate a crashed daemon.
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatalf("create stale file: %v", err)
	}

	srv, err := NewServer(context.Background(), newDaemonForTest(t), sock)
	if err != nil {
		t.Fatalf("NewServer should start over a stale file: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	// Confirm the server is functional by checking the socket exists.
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("socket should exist after NewServer cleaned stale file: %v", err)
	}
}

// TestSocketModePersistsAfterStartup verifies that the 0600 mode on the socket
// is still correct after Run has started accepting connections (not just at
// bind time).
func TestSocketModePersistsAfterStartup(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "hook.sock")
	srv, err := NewServer(context.Background(), newDaemonForTest(t), sock)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Run(ctx) }()
	time.Sleep(20 * time.Millisecond) // let Run enter Accept

	info, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket after Run started: %v", err)
	}
	mode := info.Mode().Perm()
	const want = os.FileMode(0o600)
	if mode != want {
		t.Errorf("socket mode after startup = %04o, want %04o (#1191)", mode, want)
	}
}
