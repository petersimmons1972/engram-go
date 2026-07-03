package hookdaemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Default lifecycle timings (issue #396).
const (
	// DefaultIdleTimeout is how long the daemon waits with no events before
	// self-terminating. Prevents accumulation if Claude Code crashes without
	// sending Stop.
	DefaultIdleTimeout = 10 * time.Minute

	// DrainIdleTimeout is the shortened idle window the Stop handler sets so the
	// daemon winds down soon after a session ends, while still giving any
	// in-flight writes time to drain (Option C — drain-not-SIGTERM).
	DrainIdleTimeout = 30 * time.Second

	// authCacheTTL mirrors the file-based auth cache TTL of the old
	// engram-auth-check.sh (120s), now held in memory.
	authCacheTTL = 120 * time.Second
)

// Config configures a Daemon. All dependencies are interfaces so the daemon is
// unit-testable without a real Engram server, filesystem, or wall clock.
type Config struct {
	Engram   EngramClient
	Tokens   TokenStore
	Memory   MemoryWriter
	Fallback FallbackStore
	Clock    Clock

	// IdleTimeout overrides DefaultIdleTimeout when non-zero.
	IdleTimeout time.Duration
	// RecallProject is the project name used for the second SessionStart recall
	// (inferred from the git repo by the caller); "" disables the project recall.
	RecallProject string
}

// Daemon owns all mutable hook state in memory, protected by a single mutex.
type Daemon struct {
	cfg Config

	mu                 sync.Mutex
	token              string
	authOKAt           int64    // unix seconds of last successful auth probe (0 = unknown)
	consecutiveAuthErr int      // tracks repeated auth failures for degraded reporting
	pendingFallback    []string // buffered fallback entries not yet flushed

	// lastEventAt and idleDeadline drive the idle-timeout self-termination. Both
	// are unix seconds. idleDeadline is recomputed on every event.
	lastEventAt  int64
	idleDeadline int64
}

// New constructs a Daemon. It loads the cached token through the TokenStore so
// the first event does not pay a disk read.
func New(cfg Config) (*Daemon, error) {
	if cfg.Engram == nil {
		return nil, fmt.Errorf("hookdaemon: Engram client is required")
	}
	if cfg.Tokens == nil {
		return nil, fmt.Errorf("hookdaemon: TokenStore is required")
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	d := &Daemon{cfg: cfg}
	tok, err := cfg.Tokens.Load()
	if err == nil {
		d.token = tok
	}
	now := cfg.Clock.Now()
	d.lastEventAt = now
	d.idleDeadline = now + int64(cfg.IdleTimeout.Seconds())
	return d, nil
}

// Handle dispatches a single hook request. It never returns an error to the
// caller in a way that would block a session — every handler degrades to a
// best-effort no-op. The returned Response is what the shim relays to Claude
// Code.
func (d *Daemon) Handle(ctx context.Context, req Request) Response {
	d.touch()

	switch req.Hook {
	case HookSessionStart:
		return d.handleSessionStart(ctx, req)
	case HookUserPromptSubmit:
		return d.handleUserPromptSubmit(ctx, req)
	case HookPreToolUse:
		return d.handlePreToolUse(ctx, req)
	case HookPostToolUse:
		return d.handlePostToolUse(ctx, req)
	case HookStop:
		return d.handleStop(ctx, req)
	case HookPreCompact:
		return d.handlePreCompact(ctx, req)
	default:
		// Unknown hook — never block the session.
		return Response{ExitCode: 0}
	}
}

// touch records event activity and extends the idle deadline.
func (d *Daemon) touch() {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.cfg.Clock.Now()
	d.lastEventAt = now
	d.idleDeadline = now + int64(d.cfg.IdleTimeout.Seconds())
}

// token snapshot helpers ------------------------------------------------------

func (d *Daemon) currentToken() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.token
}

// setToken updates the in-memory token and writes it through the TokenStore
// only when it actually changed (issue #396: write on change, not per event).
func (d *Daemon) setToken(tok string) {
	d.mu.Lock()
	changed := tok != "" && tok != d.token
	if changed {
		d.token = tok
	}
	d.mu.Unlock()
	if changed && d.cfg.Tokens != nil {
		if err := d.cfg.Tokens.Store(tok); err != nil {
			slog.Warn("hookdaemon: Tokens.Store failed — token not persisted; next session may have stale auth", "err", err)
		}
	}
}

// authIsFresh reports whether the in-memory auth-OK cache is still within TTL.
func (d *Daemon) authIsFresh() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.authOKAt == 0 {
		return false
	}
	return d.cfg.Clock.Now()-d.authOKAt < int64(authCacheTTL.Seconds())
}

func (d *Daemon) markAuthOK() {
	d.mu.Lock()
	d.authOKAt = d.cfg.Clock.Now()
	d.consecutiveAuthErr = 0
	d.mu.Unlock()
}

func (d *Daemon) markAuthFail() int {
	d.mu.Lock()
	d.authOKAt = 0
	d.consecutiveAuthErr++
	n := d.consecutiveAuthErr
	d.mu.Unlock()
	return n
}

// fallback buffer -------------------------------------------------------------

// enqueueFallback appends an entry to the in-memory buffer. Flushing happens via
// flushFallback, called from SessionStart and Stop (single-owner, no flock).
func (d *Daemon) enqueueFallback(entry string) {
	d.mu.Lock()
	d.pendingFallback = append(d.pendingFallback, entry)
	d.mu.Unlock()
}

// flushFallback drains the pending buffer to the FallbackStore. On write failure
// the entries are re-queued so they are retried on the next flush.
func (d *Daemon) flushFallback() {
	d.mu.Lock()
	if len(d.pendingFallback) == 0 || d.cfg.Fallback == nil {
		d.mu.Unlock()
		return
	}
	entries := d.pendingFallback
	d.pendingFallback = nil
	d.mu.Unlock()

	if err := d.cfg.Fallback.Append(entries); err != nil {
		// Re-queue on failure; preserve ordering with any newly added entries.
		d.mu.Lock()
		d.pendingFallback = append(entries, d.pendingFallback...)
		d.mu.Unlock()
	}
}

// PendingFallbackCount returns the number of buffered fallback entries. Exposed
// for the `hook status` subcommand and tests.
func (d *Daemon) PendingFallbackCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pendingFallback)
}

// idle-timeout ----------------------------------------------------------------

// idleDeadlineUnix returns the current idle deadline in unix seconds.
func (d *Daemon) idleDeadlineUnix() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.idleDeadline
}

// requestDrain shortens the idle window (Option C). Called by handleStop.
func (d *Daemon) requestDrain() {
	d.mu.Lock()
	now := d.cfg.Clock.Now()
	// Reset the idle deadline to the drain window, but never extend it past the
	// existing deadline — Stop should only ever bring shutdown closer.
	drainDeadline := now + int64(DrainIdleTimeout.Seconds())
	if drainDeadline < d.idleDeadline {
		d.idleDeadline = drainDeadline
	}
	d.mu.Unlock()
}

// recallProject returns the configured project recall target.
func (d *Daemon) recallProject() string {
	return strings.TrimSpace(d.cfg.RecallProject)
}

// realClock is the production Clock.
type realClock struct{}

func (realClock) Now() int64 { return time.Now().Unix() }

// jsonMarshal is a tiny helper that never panics; on error it returns "{}".
func jsonMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
