// longmemeval runs the LongMemEval benchmark against a live engram-go MCP server.
// Usage: longmemeval <ingest|run|score|all> [flags]
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
)

// Config holds flags shared across all subcommands.
type Config struct {
	DataFile   string
	Workers    int
	RunID      string
	ServerURL  string
	APIKey     string
	NoCleanup  bool
	Retries    int
	OutDir     string
	LLMBaseURL string // OpenAI-compatible base URL; bypasses claude CLI when set
	LLMModel   string // model name for LLMBaseURL endpoint

	// score-efficient flags
	ScorerURL       string // OAI endpoint for score-efficient (env: LME_SCORER_URL)
	ScorerModel     string // model name (env: LME_SCORER_MODEL)
	PreserveCorrect bool   // skip re-scoring items already CORRECT (default true)
	ForceRescore    bool   // ignore checkpoint, re-score everything
}

func main() {
	os.Exit(dispatch(os.Args, os.Stdout, os.Stderr))
}

// printUsage writes the top-level usage banner.
func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: longmemeval <subcommand> [flags]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Subcommands:")
	_, _ = fmt.Fprintln(w, "  ingest    Load the dataset into Engram (per-question isolation projects)")
	_, _ = fmt.Fprintln(w, "  run       Recall + generate hypotheses for each question")
	_, _ = fmt.Fprintln(w, "  score     Score hypotheses against gold answers")
	_, _ = fmt.Fprintln(w, "  all             Run ingest → run → score in one invocation")
	_, _ = fmt.Fprintln(w, "  score-efficient Score with olla OAI backend; preserves CORRECT items by default")
	_, _ = fmt.Fprintln(w, "  help            Print this usage and exit")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Common flags (see <subcommand> --help for the full set):")
	_, _ = fmt.Fprintln(w, "  --data <path>           Path to longmemeval_m_cleaned.json (required for ingest/run/score/all)")
	_, _ = fmt.Fprintln(w, "  --url <url>             Engram server URL                                 (env: ENGRAM_URL)")
	_, _ = fmt.Fprintln(w, "  --api-key <key>         Engram API key                                    (env: ENGRAM_API_KEY)")
	_, _ = fmt.Fprintln(w, "  --llm-url <url>         OpenAI-compatible LLM base URL                    (env: LME_LLM_URL)")
	_, _ = fmt.Fprintln(w, "  --llm-model <name>      Model name for --llm-url                          (env: LME_LLM_MODEL)")
	_, _ = fmt.Fprintln(w, "  --workers <n>           Parallel worker count (default 4)")
	_, _ = fmt.Fprintln(w, "  --out <dir>             Output directory for checkpoints (default .)")
	_, _ = fmt.Fprintln(w, "  --run-id <hex>          Run identifier (auto-generated if empty)")
	_, _ = fmt.Fprintln(w, "  --retries <n>           Retry count for generation + Engram calls (default 1)")
	_, _ = fmt.Fprintln(w, "  --no-cleanup            Skip project deletion after run stage")
}

// dispatch parses args and runs the requested subcommand. Returns the process
// exit code. Extracted from main() so it is testable without spawning a
// subprocess. Writers are injected so tests can capture output.
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		printUsage(stderr)
		return 1
	}
	subcommand := args[1]

	// #662: `help` is a first-class subcommand, not an unknown one.
	if subcommand == "help" || subcommand == "--help" || subcommand == "-h" {
		printUsage(stdout)
		return 0
	}

	fs := flag.NewFlagSet(subcommand, flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := &Config{}
	fs.StringVar(&cfg.DataFile, "data", "", "Path to longmemeval_m_cleaned.json (required)")
	fs.IntVar(&cfg.Workers, "workers", 4, "Number of parallel workers")
	fs.StringVar(&cfg.RunID, "run-id", "", "Run ID (hex); auto-generated if empty")
	defaultURL, defaultKey := mcpDefaults()
	fs.StringVar(&cfg.ServerURL, "url", envOr("ENGRAM_URL", defaultURL), "Engram server URL")
	fs.StringVar(&cfg.APIKey, "api-key", envOr("ENGRAM_API_KEY", defaultKey), "Engram API key")
	fs.BoolVar(&cfg.NoCleanup, "no-cleanup", false, "Skip Engram project deletion after run stage")
	fs.IntVar(&cfg.Retries, "retries", 1, "Retry count for generation and Engram calls")
	fs.StringVar(&cfg.OutDir, "out", ".", "Output directory for checkpoint and result files")
	fs.StringVar(&cfg.LLMBaseURL, "llm-url", envOr("LME_LLM_URL", ""), "OpenAI-compatible base URL (e.g. http://oblivion:8000/v1); bypasses claude CLI when set")
	fs.StringVar(&cfg.LLMModel, "llm-model", envOr("LME_LLM_MODEL", ""), "Model name for --llm-url endpoint")

	// score-efficient has its own flag set and early return.
	if subcommand == "score-efficient" {
		sefs := flag.NewFlagSet("score-efficient", flag.ExitOnError)
		sefs.StringVar(&cfg.DataFile, "data", "", "path to longmemeval JSON (required)")
		sefs.IntVar(&cfg.Workers, "workers", 4, "parallel workers")
		sefs.StringVar(&cfg.OutDir, "out", ".", "output directory")
		sefs.IntVar(&cfg.Retries, "retries", 1, "retry count per LLM call")
		sefs.StringVar(&cfg.ScorerURL, "scorer-url", envOr("LME_SCORER_URL", ""), "OAI base URL for scoring")
		sefs.StringVar(&cfg.ScorerModel, "scorer-model", envOr("LME_SCORER_MODEL", ""), "model name for scorer")
		sefs.BoolVar(&cfg.PreserveCorrect, "preserve-correct", true, "skip items already scored CORRECT")
		sefs.BoolVar(&cfg.ForceRescore, "force-rescore", false, "ignore checkpoint, re-score everything")
		_ = sefs.Parse(args[2:])
		if cfg.DataFile == "" {
			_, _ = fmt.Fprintln(stderr, "--data is required")
			return 1
		}
		if cfg.RunID == "" {
			cfg.RunID = newRunID()
		}
		return runScoreEfficient(cfg)
	}

	switch subcommand {
	case "ingest", "run", "score", "all":
		// known subcommand — fall through to flag parsing
	default:
		_, _ = fmt.Fprintf(stderr, "unknown subcommand %q\n", subcommand)
		printUsage(stderr)
		return 1
	}

	if err := fs.Parse(args[2:]); err != nil {
		return 2
	}

	if cfg.DataFile == "" {
		_, _ = fmt.Fprintln(stderr, "--data is required")
		return 1
	}

	if cfg.RunID == "" {
		cfg.RunID = newRunID()
	}

	switch subcommand {
	case "ingest":
		runIngest(cfg)
	case "run":
		// #703: surface non-zero exit when runRun reports any errors.
		if exit := runRun(cfg); exit != 0 {
			return exit
		}
	case "score":
		runScore(cfg)
	case "all":
		runAll(cfg)
	}
	return 0
}

func newRunID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// mcpDefaults reads the engram URL and Bearer token from ~/.claude/mcp_servers.json,
// which is kept current by the session-start hook. Falls back to localhost defaults.
func mcpDefaults() (url, token string) {
	url = "http://localhost:8788"
	home, err := os.UserHomeDir()
	if err != nil {
		return url, ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "mcp_servers.json"))
	if err != nil {
		return url, ""
	}
	var cfg struct {
		McpServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return url, ""
	}
	for name, srv := range cfg.McpServers {
		if name != "engram" {
			continue
		}
		// Strip /sse path component — the benchmark appends it in Connect().
		// Parse properly so query params don't break the suffix check.
		srvURL := srv.URL
		if u, err := neturl.Parse(srvURL); err == nil {
			u.Path = strings.TrimSuffix(u.Path, "/sse")
			u.RawQuery = ""
			srvURL = u.String()
		}
		if srvURL != "" {
			url = srvURL
		}
		if auth := srv.Headers["Authorization"]; len(auth) > 7 {
			token = auth[7:] // strip "Bearer "
		}
		return url, token
	}
	return url, token
}

func projectName(runID, questionID string) string {
	return fmt.Sprintf("lme-%s-%s", runID, questionID)
}
