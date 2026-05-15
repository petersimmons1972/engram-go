// longmemeval runs the LongMemEval benchmark against a live engram-go MCP server.
// Usage: longmemeval <ingest|run|score|all> [flags]
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
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
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: longmemeval <ingest|run|score|all> [flags]")
		os.Exit(1)
	}
	subcommand := os.Args[1]

	fs := flag.NewFlagSet(subcommand, flag.ExitOnError)
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

	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}

	if cfg.DataFile == "" && subcommand != "help" {
		log.Fatal("--data is required")
	}

	if cfg.RunID == "" {
		cfg.RunID = newRunID()
	}

	switch subcommand {
	case "ingest":
		runIngest(cfg)
	case "run":
		runRun(cfg)
	case "score":
		runScore(cfg)
	case "all":
		runAll(cfg)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", subcommand)
		os.Exit(1)
	}
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
