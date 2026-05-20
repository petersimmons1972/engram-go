package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/petersimmons1972/engram/internal/llmclient"
)

// Version is injected at build time via -ldflags "-X main.Version=<ver>".
var Version = "dev"

func main() {
	os.Exit(run())
}

// run is the CLI entrypoint for instinct-audit-go.
// Flag parsing, Engram client setup, LLM client construction, and the
// orchestration loop all live here.
func run() int {
	llmBackend := flag.String("llm-backend", envOr("LLM_BACKEND", "anthropic"), "LLM backend: anthropic or olla")
	timeout := flag.Duration("timeout", defaultTimeout, "Per-inference timeout")
	engramBase := flag.String("engram", "", "Engram base URL (overrides ENGRAM_URL env var and ~/.claude/mcp_servers.json)")
	engramToken := flag.String("token", "", "Engram Bearer token (overrides ENGRAM_TOKEN env var and ~/.claude/mcp_servers.json)")
	flag.Parse()

	// Resolve Engram coordinates.
	base, token, err := resolveEngram(*engramBase, *engramToken)
	if err != nil {
		slog.Error("resolve engram config", "err", err)
		return 1
	}

	// Construct LLM client. Unlike the consolidator (which is long-running and
	// tolerates a missing backend), audit is a one-shot batch tool — failure
	// should be loud and non-zero so cron jobs surface the problem.
	// Pass Backend explicitly via Config instead of setting LLM_BACKEND in the
	// process environment — keeps configuration in the call chain and avoids
	// race-unsafe global state under parallel tests (Blocker 3).
	client, err := llmclient.NewClient(llmclient.Config{
		Backend: *llmBackend,
		Timeout: *timeout,
	})
	if err != nil {
		slog.Error("create LLM client", "backend", *llmBackend, "err", err)
		return 1
	}

	if err := runAudit(base, token, client, *timeout, os.Stdout); err != nil {
		slog.Error("audit failed", "err", err)
		return 1
	}
	return 0
}

// runAudit fetches patterns, judges each, and writes the JSON report to out.
// Extracted from run() so it can be tested without exec.
func runAudit(base, token string, client llmclient.LLMClient, timeout time.Duration, out io.Writer) error {
	if timeout == 0 {
		timeout = defaultTimeout
	}

	slog.Info("fetching instinct patterns", "engram", base)
	patterns, err := fetchPatterns(base, token)
	if err != nil {
		return err
	}
	slog.Info("found patterns", "count", len(patterns))

	results := make([]auditResult, 0, len(patterns))
	for i, p := range patterns {
		slog.Info("auditing pattern", "n", i+1, "total", len(patterns), "id", p.ID)
		res := Judge(context.Background(), client, timeout, p)
		results = append(results, res)
	}

	r := buildReport(results)
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
