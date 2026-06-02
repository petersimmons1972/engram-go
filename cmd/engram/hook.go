package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/petersimmons1972/engram/internal/config"
	"github.com/petersimmons1972/engram/internal/hookdaemon"
)

// hookSocketPath returns the daemon socket path (~/.claude/.engram-hook.sock).
func hookSocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", ".engram-hook.sock")
}

// runHookDaemon implements `engram hook-daemon [--detach]`. Thin wiring: build
// the production adapters, then hand off to the hookdaemon package. (#396)
func runHookDaemon(args []string) error {
	if len(args) > 0 && args[0] == "--detach" {
		return detachHookDaemon()
	}
	d, err := newHookDaemon()
	if err != nil {
		return err
	}
	srv, err := hookdaemon.NewServer(d, hookSocketPath())
	if err != nil {
		return err
	}
	defer srv.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	slog.Info("engram hook daemon started", "socket", srv.SocketPath())
	return srv.Run(ctx)
}

// newHookDaemon assembles the daemon with its production adapters.
func newHookDaemon() (*hookdaemon.Daemon, error) {
	home, _ := os.UserHomeDir()
	base := fmt.Sprintf("http://127.0.0.1:%d", config.DefaultPort)
	if p := os.Getenv("ENGRAM_TEST_PORT"); p != "" {
		base = "http://127.0.0.1:" + p
	}
	return hookdaemon.New(hookdaemon.Config{
		Engram:        hookdaemon.NewHTTPEngramClient(base),
		Tokens:        hookdaemon.NewMCPTokenStore(filepath.Join(home, ".claude", "mcp_servers.json")),
		Memory:        hookdaemon.NewFileMemoryWriter(filepath.Join(home, ".claude", "projects", "-home-psimmons", "memory", "MEMORY.md")),
		Fallback:      hookdaemon.NewFileFallbackStore(filepath.Join(home, ".claude", "projects", "-home-psimmons", "memory", "fallback.md")),
		RecallProject: inferEngramProject(),
	})
}

// detachHookDaemon re-execs this binary as a background hook-daemon, with stderr
// redirected to the rotating log file, then returns immediately. (#396)
func detachHookDaemon() error {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".claude", "logs")
	_ = os.MkdirAll(logDir, 0o755)
	logFile := filepath.Join(logDir, "engram-hook-daemon.log")
	rotateHookLog(logFile)
	lf, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer lf.Close()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "hook-daemon")
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from session
	cmd.Env = os.Environ()
	return cmd.Start()
}

// rotateHookLog renames the log to .1 when it exceeds 5 MiB (simple 1-file
// rotation; keeps the daemon log from growing unbounded). (#396)
func rotateHookLog(path string) {
	const maxBytes = 5 << 20
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxBytes {
		_ = os.Rename(path, path+".1")
	}
}

// runHookShim implements `engram hook <EventName>`: read stdin, forward to the
// daemon (lazy-starting it on dial failure), print response stdout, exit with
// the daemon's exit code. (#396)
func runHookShim(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: engram hook <EventName>")
	}
	event := args[0]
	if event == "status" {
		return hookStatus()
	}
	payload, _ := readAllStdin()
	req := hookdaemon.Request{Hook: event, Payload: payload}
	resp, err := hookdaemon.SendWithRetry(hookSocketPath(), req, detachHookDaemon, 150*time.Millisecond)
	if err != nil {
		// Never block the session: a daemon failure degrades to a silent no-op.
		return nil
	}
	if resp.Stdout != "" {
		fmt.Print(resp.Stdout)
	}
	if resp.ExitCode != 0 {
		os.Exit(resp.ExitCode)
	}
	return nil
}

// hookStatus implements `engram hook status`: report whether the daemon is up.
func hookStatus() error {
	sock := hookSocketPath()
	if _, err := os.Stat(sock); err != nil {
		fmt.Printf("engram hook daemon: not running (no socket at %s)\n", sock)
		return nil
	}
	resp, err := hookdaemon.SendRequest(sock, hookdaemon.Request{Hook: "PreToolUse"})
	if err != nil {
		fmt.Printf("engram hook daemon: socket present but unreachable (%v)\n", err)
		return nil
	}
	fmt.Printf("engram hook daemon: running (socket %s, exit_code %d)\n", sock, resp.ExitCode)
	return nil
}

// inferEngramProject maps the current git repo name to an Engram project,
// mirroring engram-session-recall.sh. Returns "" when no mapping applies.
func inferEngramProject() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	name := filepath.Base(strings.TrimSpace(string(out)))
	switch {
	case strings.HasPrefix(name, "clearwatch"):
		return "clearwatch"
	case strings.HasPrefix(name, "engram"):
		return "engram"
	case strings.HasPrefix(name, "homelab"):
		return "homelab"
	case strings.HasPrefix(name, "instinct"):
		return "instinct"
	default:
		return ""
	}
}

// readAllStdin reads all of stdin, capped at 8 MiB.
func readAllStdin() ([]byte, error) {
	const max = 8 << 20
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if len(buf) > max {
				return buf[:max], nil
			}
		}
		if err != nil {
			return buf, nil
		}
	}
}
