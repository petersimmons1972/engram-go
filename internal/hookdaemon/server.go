package hookdaemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

// perConnTimeout bounds how long the daemon will spend handling a single
// connection, so a wedged hook handler can never hold the socket forever.
const perConnTimeout = 15 * time.Second

// Server wraps a Daemon with a Unix-domain-socket listener and the idle-timeout
// shutdown loop.
type Server struct {
	daemon   *Daemon
	sockPath string
	ln       net.Listener

	wg sync.WaitGroup
}

// NewServer binds a Unix socket at sockPath and returns a ready Server. Any
// stale socket file at sockPath is removed first (a leftover from a crashed
// daemon). The caller owns Run and Close. ctx scopes the bind and the
// liveness-probe dial.
func NewServer(ctx context.Context, d *Daemon, sockPath string) (*Server, error) {
	// Remove a stale socket if no daemon is actually listening. If a live daemon
	// owns it, the bind below will fail and we surface that to the caller.
	if _, err := os.Stat(sockPath); err == nil {
		var dialer net.Dialer
		probeCtx, cancel := context.WithTimeout(ctx, time.Second)
		c, derr := dialer.DialContext(probeCtx, "unix", sockPath)
		cancel()
		if derr == nil {
			_ = c.Close()
			return nil, fmt.Errorf("hookdaemon: socket %s already has a live daemon", sockPath)
		}
		_ = os.Remove(sockPath)
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("hookdaemon: listen %s: %w", sockPath, err)
	}
	// Owner-only socket — no other local user should drive the daemon.
	_ = os.Chmod(sockPath, 0o600)
	return &Server{daemon: d, sockPath: sockPath, ln: ln}, nil
}

// SocketPath returns the path the server is listening on.
func (s *Server) SocketPath() string { return s.sockPath }

// Run accepts connections until ctx is cancelled or the idle timeout elapses.
// It returns nil on a clean idle/ctx shutdown.
func (s *Server) Run(ctx context.Context) error {
	// Watchdog goroutine: closes the listener when ctx is cancelled or the idle
	// deadline passes, which unblocks Accept.
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go s.idleWatchdog(watchCtx)

	for {
		conn, err := s.ln.Accept()
		if err != nil {
			// Listener closed by watchdog (ctx cancel or idle) → clean shutdown.
			if errors.Is(err, net.ErrClosed) {
				break
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return err
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(ctx, conn)
		}()
	}
	s.wg.Wait()
	return nil
}

// idleWatchdog closes the listener when ctx is done or the daemon's idle
// deadline is reached. It polls once per second; the resolution is well within
// the multi-minute idle window.
func (s *Server) idleWatchdog(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = s.ln.Close()
			return
		case <-ticker.C:
			if s.daemon.cfg.Clock.Now() >= s.daemon.idleDeadlineUnix() {
				slog.Info("hook daemon idle timeout reached — shutting down")
				_ = s.ln.Close()
				return
			}
		}
	}
}

// handleConn reads one Request, dispatches it, and writes one Response. The
// protocol is one JSON request line in, one JSON response line out, then close.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(perConnTimeout))

	var req Request
	dec := json.NewDecoder(bufio.NewReader(io.LimitReader(conn, 8<<20))) // 8 MiB cap
	if err := dec.Decode(&req); err != nil {
		// Malformed request — return a non-fatal error response.
		s.writeResponse(conn, Response{ExitCode: 0})
		return
	}

	connCtx, cancel := context.WithTimeout(ctx, perConnTimeout)
	defer cancel()

	resp := s.daemon.Handle(connCtx, req)
	s.writeResponse(conn, resp)
}

func (s *Server) writeResponse(conn net.Conn, resp Response) {
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(b, '\n'))
}

// Close stops the listener and removes the socket file. Safe to call multiple
// times.
func (s *Server) Close() error {
	err := s.ln.Close()
	_ = os.Remove(s.sockPath)
	return err
}
