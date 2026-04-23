// longmemeval runs the LongMemEval benchmark against a live engram-go MCP server.
// Usage: longmemeval <ingest|run|score|all> [flags]
package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
)

// Config holds flags shared across all subcommands.
type Config struct {
	DataFile  string
	Workers   int
	RunID     string
	ServerURL string
	APIKey    string
	NoCleanup bool
	Retries   int
	OutDir    string
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
	fs.StringVar(&cfg.ServerURL, "url", envOr("ENGRAM_URL", "http://localhost:8788"), "Engram server URL")
	fs.StringVar(&cfg.APIKey, "api-key", os.Getenv("ENGRAM_API_KEY"), "Engram API key")
	fs.BoolVar(&cfg.NoCleanup, "no-cleanup", false, "Skip Engram project deletion after run stage")
	fs.IntVar(&cfg.Retries, "retries", 1, "Retry count for claude --print and Engram calls")
	fs.StringVar(&cfg.OutDir, "out", ".", "Output directory for checkpoint and result files")

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

func projectName(runID, questionID string) string {
	return fmt.Sprintf("lme-%s-%s", runID, questionID)
}
