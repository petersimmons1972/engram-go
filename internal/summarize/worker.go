package summarize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
)

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

// summarizeHTTPClient is shared across all SummarizeContent calls so that the
// underlying connection pool is reused rather than rebuilt on every request.
var summarizeHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConnsPerHost: 2,
	},
}

// SummarizeContent calls Ollama /api/generate synchronously. Returns the trimmed response.
func SummarizeContent(ctx context.Context, content, ollamaURL, model string) (string, error) {
	if len(content) > maxContent {
		content = content[:maxContent]
	}
	prompt := summarizePrompt + content
	body, err := json.Marshal(map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(ollamaURL, "/")+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := summarizeHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama generate: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Response), nil
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
	if mem.Summary != nil {
		return nil // already summarized
	}
	summary, err := SummarizeContent(ctx, mem.Content, ollamaURL, model)
	if err != nil {
		return err
	}
	return backend.StoreSummary(ctx, memoryID, summary)
}

// ClaudeSummarize summarizes content using the Anthropic Messages API via client.
// Content is truncated to maxContent bytes before being sent.
func ClaudeSummarize(ctx context.Context, content string, client ClaudeCompleter) (string, error) {
	if len(content) > maxContent {
		content = content[:maxContent]
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
	if mem.Summary != nil {
		return nil // already summarized
	}
	summary, err := ClaudeSummarize(ctx, mem.Content, client)
	if err != nil {
		return err
	}
	return backend.StoreSummary(ctx, memoryID, summary)
}

// Worker is a background goroutine that fills summary IS NULL rows.
type Worker struct {
	backend      db.Backend
	project      string
	ollamaURL    string
	model        string
	enabled      bool
	claudeClient ClaudeCompleter
	cancel       context.CancelFunc
	done         chan struct{}
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
	if !w.enabled {
		close(w.done)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
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
	rows, err := w.backend.GetMemoriesPendingSummary(ctx, w.project, batchSize)
	if err != nil {
		slog.Warn("summarize fetch failed", "err", err)
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
			slog.Warn("summarize failed", "id", row.ID, "err", err)
			continue
		}
		if err := w.backend.StoreSummary(ctx, row.ID, summary); err != nil {
			slog.Warn("store summary failed", "id", row.ID, "err", err)
		}
	}
}
