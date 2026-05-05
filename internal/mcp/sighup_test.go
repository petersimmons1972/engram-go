package mcp

import (
	"context"
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

	// Start the SIGHUP handler
	go handleSIGHUP(ctx, cfg, func() {
		// This callback is invoked when config is reloaded
		close(reloaded)
	})

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

	// Stop the signal handler
	cancel()
}

// TestSIGHUPPartialReload verifies that only changed flags are updated and
// unset env vars do not toggle existing flags.
func TestSIGHUPPartialReload(t *testing.T) {
	// Create a RuntimeConfig with initial state
	cfg := &RuntimeConfig{}
	cfg.ClaudeSummarize.Store(true)  // Pre-set one flag
	cfg.ClaudeRerank.Store(true)     // Pre-set another flag

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reloaded := make(chan struct{})

	go handleSIGHUP(ctx, cfg, func() {
		close(reloaded)
	})

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
}

// handleSIGHUP runs the SIGHUP signal handler loop.
// It blocks until ctx is cancelled.
func handleSIGHUP(ctx context.Context, cfg *RuntimeConfig, onReload func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)
	defer signal.Stop(sigCh)

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
