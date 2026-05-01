package summarize

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/llm"
	"github.com/petersimmons1972/engram/internal/metrics"
)

// ErrModelNotFound is returned by SummarizeContent when the generation
// endpoint responds with HTTP 404 (model not available).
var ErrModelNotFound = errors.New("model not found")

const modelNotFoundBackoff = 10 * time.Minute

// ClaudeCompleter is the subset of claude.Client used for summarization.
type ClaudeCompleter interface {
	Complete(ctx context.Context, system, prompt, execModel, advModel string,
		advisorMaxUses, maxTokens int) (string, error)
}

const (
	pollInterval = 30 * time.Second
	batchSize    = 10
	maxContent   = 2000
)

var summarizePrompt = "Summarize the following memory in 1-2 concise sentences. Focus on the key fact or decision. No preamble.\n\n"

// truncateRunes trims content to at most n UTF-8 characters (#121).
// Using byte slicing on a rune string can split multi-byte characters.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n])
}

// SummarizeContent calls the LiteLLM /v1/chat/completions endpoint synchronously.
// litellmURL is the base URL (e.g. "http://litellm:4000"). Returns the trimmed response.
//
// LiteLLM returns HTTP 500 (not 404) when the upstream Ollama reports "model not found".
// We detect that case by inspecting the error body and wrap it as ErrModelNotFound so
// the 10-minute backoff in runOnce fires instead of spamming WARN on every 30s tick.
func SummarizeContent(ctx context.Context, content, litellmURL, model string) (string, error) {
	if utf8.RuneCountInString(content) > maxContent {
		content = truncateRunes(content, maxContent)
	}
	result, err := llm.Complete(ctx, litellmURL, "", model, summarizePrompt+content)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return "", fmt.Errorf("%w: model=%q: %w", ErrModelNotFound, model, err)
		}
		return "", fmt.Errorf("summarize: %w", err)
	}
	return result, nil
}

// SummarizeOne immediately summarizes a single memory by ID and stores the result.
// Returns nil if the memory is already summarized.
func SummarizeOne(ctx context.Context, backend db.Backend, memoryID, ollamaURL, model string) error {
	mem, err := backend.GetMemory(ctx, memoryID)
	if err != nil {
		return fmt.Errorf("get memory %s: %w", memoryID, err)
	}
	if mem == nil {
		return fmt.Errorf("memory %s not found", memoryID)
	}
	if mem.Summary != nil && *mem.Summary != mem.Content {
		return nil // already has a real summary
	}
	summary, err := SummarizeContent(ctx, mem.Content, ollamaURL, model)
	if err != nil {
		return err
	}
	return backend.StoreSummary(ctx, memoryID, summary)
}

// ClaudeSummarize summarizes content using the Anthropic Messages API via client.
// Content is truncated to maxContent runes before being sent (#121).
func ClaudeSummarize(ctx context.Context, content string, client ClaudeCompleter) (string, error) {
	if utf8.RuneCountInString(content) > maxContent {
		content = truncateRunes(content, maxContent)
	}
	system := "You are a memory summarizer. Respond only with a 1-2 sentence summary of the key fact or decision. No preamble."
	result, err := client.Complete(ctx, system, content,
		"claude-sonnet-4-6", "claude-opus-4-6", 2, 256)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result), nil
}

// SummarizeOneWithClaude immediately summarizes a single memory by ID using the
// Claude API and stores the result. Returns nil if the memory is already summarized.
func SummarizeOneWithClaude(ctx context.Context, backend db.Backend, memoryID string, client ClaudeCompleter) error {
	mem, err := backend.GetMemory(ctx, memoryID)
	if err != nil {
		return fmt.Errorf("get memory %s: %w", memoryID, err)
	}
	if mem == nil {
		return fmt.Errorf("memory %s not found", memoryID)
	}
	if mem.Summary != nil && *mem.Summary != mem.Content {
		return nil // already has a real summary
	}
	summary, err := ClaudeSummarize(ctx, mem.Content, client)
	if err != nil {
		return err
	}
	return backend.StoreSummary(ctx, memoryID, summary)
}

// Worker is a background goroutine that fills summary IS NULL rows.
type Worker struct {
	backend             db.Backend
	project             string
	ollamaURL           string
	model               string
	enabled             bool
	claudeClient        ClaudeCompleter
	cancel              context.CancelFunc
	done                chan struct{}
	modelNotFoundUntil  time.Time // backoff expiry after ErrModelNotFound (#151)
}

// NewWorker creates a Worker. enabled=false makes Start a no-op.
func NewWorker(backend db.Backend, project, ollamaURL, model string, enabled bool) *Worker {
	return &Worker{
		backend:   backend,
		project:   project,
		ollamaURL: ollamaURL,
		model:     model,
		enabled:   enabled,
		done:      make(chan struct{}),
	}
}

// NewWorkerWithClaude creates a Worker that uses the Claude API for summarization
// when claudeClient is non-nil. Falls back to Ollama otherwise.
func NewWorkerWithClaude(backend db.Backend, project, ollamaURL, model string, enabled bool, claudeClient ClaudeCompleter) *Worker {
	w := NewWorker(backend, project, ollamaURL, model, enabled)
	w.claudeClient = claudeClient
	return w
}

// Start launches the background goroutine. Safe to call if disabled.
func (w *Worker) Start() {
	w.StartWithContext(context.Background())
}

// StartWithContext launches the background goroutine using ctx as the parent
// lifecycle context. The worker stops when ctx is cancelled.
func (w *Worker) StartWithContext(ctx context.Context) {
	if !w.enabled {
		close(w.done)
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	go w.run(ctx)
}

// Stop signals the worker to stop and waits for it to exit (max 35s).
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	select {
	case <-w.done:
	case <-time.After(35 * time.Second):
		slog.Warn("summarize worker did not stop within 35s", "project", w.project)
	}
}

const batchTimeout = 5 * time.Minute // max time for one runOnce iteration (#120)

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	w.timedRunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.timedRunOnce(ctx)
		}
	}
}

// timedRunOnce wraps safeRunOnce with a per-iteration context timeout (#120).
func (w *Worker) timedRunOnce(ctx context.Context) {
	metrics.WorkerTicks.WithLabelValues("summarize").Inc()
	iterCtx, cancel := context.WithTimeout(ctx, batchTimeout)
	defer cancel()
	w.safeRunOnce(iterCtx)
}

// safeRunOnce wraps runOnce with per-iteration panic recovery (#106).
// A panic logs an error and sleeps 1s so the loop can continue rather than
// killing the worker goroutine permanently.
func (w *Worker) safeRunOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("summarize worker panic — will retry next tick",
				"project", w.project, "panic", r)
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}()
	w.runOnce(ctx)
}

func (w *Worker) runOnce(ctx context.Context) {
	if w.backend == nil {
		return
	}
	// Skip the entire batch while in model-not-found backoff (#151).
	if !w.modelNotFoundUntil.IsZero() && time.Now().Before(w.modelNotFoundUntil) {
		return
	}
	rows, err := w.backend.GetMemoriesPendingSummary(ctx, w.project, batchSize)
	if err != nil {
		slog.Warn("summarize fetch failed", "err", err)
		metrics.WorkerErrors.WithLabelValues("summarize").Inc()
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		var summary string
		var err error
		if w.claudeClient != nil {
			summary, err = ClaudeSummarize(ctx, row.Content, w.claudeClient)
		} else {
			summary, err = SummarizeContent(ctx, row.Content, w.ollamaURL, w.model)
		}
		if err != nil {
			if errors.Is(err, ErrModelNotFound) {
				// Log once at ERROR level then back off — do not spam on every tick (#151).
				slog.Error("summarize model not found — backing off",
					"project", w.project, "model", w.model,
					"backoff", modelNotFoundBackoff, "err", err)
				w.modelNotFoundUntil = time.Now().Add(modelNotFoundBackoff)
				return
			}
			slog.Warn("summarize failed", "id", row.ID, "err", err)
			continue
		}
		// Successful summarization — clear any previous backoff.
		w.modelNotFoundUntil = time.Time{}
		if err := w.backend.StoreSummary(ctx, row.ID, summary); err != nil {
			slog.Warn("store summary failed", "id", row.ID, "err", err)
		}
	}
}
