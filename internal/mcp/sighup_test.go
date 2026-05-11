package mcp

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// TestSIGHUPConfigReload verifies that sending SIGHUP triggers a config reload
// that atomically updates the feature flags for Claude summarize, consolidate,
// and rerank, as well as log level.
func TestSIGHUPConfigReload(t *testing.T) {
	// Create a RuntimeConfig with initial feature flags disabled
	cfg := &RuntimeConfig{}

	// Verify initial state
	if cfg.ClaudeSummarize.Load() {
		t.Fatal("expected ClaudeSummarize to start disabled")
	}
	if cfg.ClaudeConsolidate.Load() {
		t.Fatal("expected ClaudeConsolidate to start disabled")
	}
	if cfg.ClaudeRerank.Load() {
		t.Fatal("expected ClaudeRerank to start disabled")
	}

	// Create a context that we can cancel to stop the signal handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a channel to signal when the config has been reloaded
	reloaded := make(chan struct{})
	ready := make(chan struct{})
	goroutineDone := make(chan struct{})

	// Start the SIGHUP handler.
	// - ready: closed after signal.Notify so we don't send SIGHUP before the
	//   handler is registered (race that lets the default kill-on-SIGHUP fire).
	// - goroutineDone: closed on exit so signal.Stop completes before the next
	//   test registers its own handler — prevents double-delivery (#618).
	go func() {
		defer close(goroutineDone)
		handleSIGHUP(ctx, cfg, func() { close(reloaded) }, ready)
	}()
	<-ready // handler is registered; safe to send SIGHUP now

	// Set env vars that should be picked up by the reload
	t.Setenv("ENGRAM_CLAUDE_SUMMARIZE", "true")
	t.Setenv("ENGRAM_CLAUDE_CONSOLIDATE", "true")
	t.Setenv("ENGRAM_CLAUDE_RERANK", "true")
	t.Setenv("ENGRAM_LOG_LEVEL", "debug")

	// Send SIGHUP to ourselves
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := p.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("Signal SIGHUP: %v", err)
	}

	// Wait for the reload with a timeout
	select {
	case <-reloaded:
		// Config was reloaded
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for config reload")
	}

	// Verify that the flags were updated
	if !cfg.ClaudeSummarize.Load() {
		t.Error("expected ClaudeSummarize to be true after reload")
	}
	if !cfg.ClaudeConsolidate.Load() {
		t.Error("expected ClaudeConsolidate to be true after reload")
	}
	if !cfg.ClaudeRerank.Load() {
		t.Error("expected ClaudeRerank to be true after reload")
	}
	if cfg.LogLevel.Load() != 0 {
		t.Errorf("expected LogLevel to be 0 (debug), got %d", cfg.LogLevel.Load())
	}

	// Stop the signal handler and wait for it to fully exit so signal.Stop
	// is called before the next test registers its own handler (#618).
	cancel()
	<-goroutineDone
}

func TestReloadRuntimeConfigUpdatesServerLogLevelVar(t *testing.T) {
	levelVar := &slog.LevelVar{}
	levelVar.Set(slog.LevelInfo)
	s := &Server{
		cfg:        Config{LogLevelVar: levelVar},
		runtimeCfg: &RuntimeConfig{},
	}
	t.Setenv("ENGRAM_LOG_LEVEL", "debug")

	s.ReloadRuntimeConfig()

	if levelVar.Level() != slog.LevelDebug {
		t.Fatalf("expected shared LevelVar to update to debug, got %v", levelVar.Level())
	}
}

// TestSIGHUPPartialReload verifies that only changed flags are updated and
// unset env vars do not toggle existing flags.
func TestSIGHUPPartialReload(t *testing.T) {
	// Create a RuntimeConfig with initial state
	cfg := &RuntimeConfig{}
	cfg.ClaudeSummarize.Store(true)  // Pre-set one flag
	cfg.ClaudeRerank.Store(true)     // Pre-set another flag

	ctx, cancel := context.WithCancel(context.Background())
	reloaded := make(chan struct{})
	ready := make(chan struct{})
	goroutineDone := make(chan struct{})
	go func() {
		defer close(goroutineDone)
		handleSIGHUP(ctx, cfg, func() { close(reloaded) }, ready)
	}()
	<-ready

	// Set only one env var (summarize), leaving others unset
	t.Setenv("ENGRAM_CLAUDE_SUMMARIZE", "false")
	t.Setenv("ENGRAM_LOG_LEVEL", "info")

	// Send SIGHUP
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := p.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("Signal SIGHUP: %v", err)
	}

	// Wait for reload
	select {
	case <-reloaded:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for config reload")
	}

	// Verify results: summarize should be false (changed), rerank should stay true
	if cfg.ClaudeSummarize.Load() {
		t.Error("expected ClaudeSummarize to be false after reload")
	}
	if !cfg.ClaudeRerank.Load() {
		t.Error("expected ClaudeRerank to remain true (unchanged)")
	}

	cancel()
	<-goroutineDone
}

// handleSIGHUP runs the SIGHUP signal handler loop.
// It blocks until ctx is cancelled. If ready is non-nil it is closed immediately
// after signal.Notify so callers can synchronise before sending SIGHUP.
func handleSIGHUP(ctx context.Context, cfg *RuntimeConfig, onReload func(), ready chan<- struct{}) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)
	defer signal.Stop(sigCh)
	if ready != nil {
		close(ready)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			reloadRuntimeConfig(cfg)
			if onReload != nil {
				onReload()
			}
		}
	}
}
