---
name: SIGHUP test signal races in Go
description: Two race patterns that cause 'signal: hangup' kills in Go tests that send signals to themselves — and the deterministic fixes
type: feedback
originSessionId: a3a168d6-3b5a-4e24-8770-cfe80a12748b
---
When a Go test sends `SIGHUP` to itself via `p.Signal(syscall.SIGHUP)` with a goroutine running `signal.Notify`, there are TWO distinct races to guard against. Both manifested in `internal/mcp/sighup_test.go` (engram-go #618).

**Race 1 — Pre-signal: goroutine not yet registered**

The goroutine is `go`-launched but hasn't reached `signal.Notify` when the test fires the signal. With no handler registered, the default action (kill the process) fires.

**Fix:** Have `handleSIGHUP` (or any test helper that calls `signal.Notify`) close a `ready chan<- struct{}` immediately after `signal.Notify`. Block on `<-ready` in the test before sending the signal.

```go
func handleSIGHUP(ctx context.Context, cfg *RuntimeConfig, onReload func(), ready chan<- struct{}) {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGHUP)
    defer signal.Stop(sigCh)
    if ready != nil { close(ready) } // signal: handler registered
    ...
}

// In test:
ready := make(chan struct{})
go func() { handleSIGHUP(ctx, cfg, cb, ready) }()
<-ready  // safe to send SIGHUP now
p.Signal(syscall.SIGHUP)
```

**Race 2 — Post-test: goroutine outlives the test**

`cancel()` is non-blocking. The goroutine may still be running (and still registered with `signal.Notify`) when the next sequential test starts its own goroutine and fires its SIGHUP. Both goroutines receive it — causing double-close panics or unexpected callbacks.

**Fix:** Track goroutine completion and wait before returning.

```go
goroutineDone := make(chan struct{})
go func() {
    defer close(goroutineDone)
    handleSIGHUP(ctx, cfg, cb, ready)
}()
// ... test body ...
cancel()
<-goroutineDone  // signal.Stop has completed; next test is safe to register
```

**Why:** `signal.Notify` is additive — every registered channel receives the signal. `signal.Stop` removes the registration, but only after the goroutine's defer runs. Without the `<-goroutineDone` wait, deferred `signal.Stop` races with the next test's `signal.Notify`.

**Verified:** 20/20 clean runs with `-count=10` and `go test ./...` at 0 failures after applying both fixes. Commit: 8cbf155 (engram-go main).
