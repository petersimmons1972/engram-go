package hookdaemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newServerForTest(t *testing.T, idle time.Duration) (*Server, *fakeClock) {
	t.Helper()
	clk := &fakeClock{now: 1_000_000}
	d, err := New(Config{
		Engram:      &fakeEngram{authOK: true, recallByProj: map[string][]byte{}},
		Tokens:      &fakeTokens{token: "tok"},
		Clock:       clk,
		IdleTimeout: idle,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sock := filepath.Join(t.TempDir(), "hook.sock")
	srv, err := NewServer(d, sock)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv, clk
}

func TestServer_RoundTripViaClient(t *testing.T) {
	srv, _ := newServerForTest(t, 10*time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	// UserPromptSubmit with a valid token + healthy fake → silent success.
	resp, err := SendRequest(srv.SocketPath(), Request{Hook: HookUserPromptSubmit})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", resp.ExitCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

func TestServer_RejectsLiveSocket(t *testing.T) {
	srv, _ := newServerForTest(t, 10*time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Run(ctx) }()
	// give Run a moment to start accepting
	time.Sleep(50 * time.Millisecond)

	d, _ := New(Config{Engram: &fakeEngram{}, Tokens: &fakeTokens{}})
	if _, err := NewServer(d, srv.SocketPath()); err == nil {
		t.Fatal("expected NewServer to reject an already-live socket")
	}
}

func TestServer_IdleTimeoutShutsDown(t *testing.T) {
	// 1s idle window; advance the fake clock past it and confirm Run returns.
	srv, clk := newServerForTest(t, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	clk.advance(5) // push past the idle deadline; watchdog polls every 1s
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error on idle shutdown: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("daemon did not idle-timeout")
	}
}

func TestSendWithRetry_StartsDaemonOnDialFailure(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "absent.sock")
	started := false
	// start callback brings up a real server so the retry can connect.
	start := func() error {
		started = true
		clk := &fakeClock{now: 1_000_000}
		d, _ := New(Config{
			Engram: &fakeEngram{authOK: true, recallByProj: map[string][]byte{}},
			Tokens: &fakeTokens{token: "tok"}, Clock: clk, IdleTimeout: time.Minute,
		})
		srv, err := NewServer(d, sock)
		if err != nil {
			return err
		}
		go func() { _ = srv.Run(context.Background()) }()
		return nil
	}
	resp, err := SendWithRetry(sock, Request{Hook: HookUserPromptSubmit}, start, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("SendWithRetry: %v", err)
	}
	if !started {
		t.Fatal("expected start callback to fire on dial failure")
	}
	if resp.ExitCode != 0 {
		t.Fatalf("expected exit 0 after retry, got %d", resp.ExitCode)
	}
}
