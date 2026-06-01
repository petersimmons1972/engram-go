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
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// Config holds flags shared across all subcommands.
type Config struct {
	DataFile       string
	Workers        int
	RunID          string
	ServerURL      string
	APIKey         string
	NoCleanup      bool          // Deprecated: use CleanupPolicy=never
	CleanupPolicy  CleanupPolicy // "auto" | "always" | "never" (default: auto)
	Retries        int
	OutDir         string
	LLMBaseURL     string // OpenAI-compatible base URL; bypasses claude CLI when set
	LLMModel       string // model name for LLMBaseURL endpoint
	EnableThinking bool   // enable chain-of-thought for models that support it (Qwen3)
	LLMMaxTokens   int    // output token budget; 0 → default (2048 thinking-off, 8192 thinking-on)
	ScoreOutput    string // score stdout mode: text, json, or quiet
	Output         io.Writer

	// score-efficient flags
	ScorerURL       string // OAI endpoint for score-efficient (env: LME_SCORER_URL)
	ScorerModel     string // model name (env: LME_SCORER_MODEL)
	ScorerMaxTokens int    // max_tokens for scoring requests (default 2048)
	PreserveCorrect bool   // skip re-scoring items already CORRECT (default true)
	ForceRescore    bool   // ignore checkpoint, re-score everything

	// score-batch flags
	ScorerBatchAPIKey string // Anthropic API key (env: ANTHROPIC_API_KEY)

	// generation flags
	GenerationModel string // claude model alias for answer generation (default "sonnet")
	ContextTopKBump bool   // raise all contextTopK categories to 15 when true

	// retrieval ablation flags
	RecallTopK          int  // memories recalled before context trim (default 100)
	ContextTopKOverride int  // explicit context topK; 0 = per-type default
	ChronoSort          bool // sort context blocks by Session date ascending before prompt assembly
	DisableQueryRewrite bool // use raw question as recall query; skip temporal/preference rewriting
	MaxBlockChars       int  // truncate each context block to this many chars before prompt assembly; 0 = no truncation
	RepairPreset        string

	// H16: question_date injection
	InjectQuestionDate bool // prepend "Today's date is: {question_date}" to temporal-reasoning prompts (default off)

	// Exp-14: H-M5 chrono-sort forcing + H-M1 entity enumeration pass
	TemporalPromptAug bool // inject H-M5 ordering instruction and H-M1 entity enumeration step into temporal-reasoning prompts (default off)

	// H15: paraphrased multi-pass BM25 union
	QueryParaphrasePasses int // Haiku paraphrase variants to generate per query; union retrieved IDs (default 0 = off)

	// H15: dual-query preference recall (lme-h8h12h15 branch)
	DualPreferenceRecall bool // run a second subject-anchor recall for preference questions and union results

	// PA: preference-anchoring generation prompt (#938)
	PreferenceAnchor bool // inject single-session anchoring instructions for preference questions (default off)

	// H8: exhaustive aggregation recall (lme-h8h12h15 branch)
	ExhaustiveAggregation bool // run a topK=500 sweep for count-shaped questions and union with primary results

	// H12: enumerate-first generation prompt (lme-h8h12h15 branch)
	EnumerateFirst bool // inject enumerate-then-total instruction for aggregation questions (default off)

	// #749: contention guard
	ExclusiveBackend bool   // guard the vLLM endpoint with a PID-liveness lockfile (default true)
	BackendLockDir   string // override lock file directory (default: $XDG_RUNTIME_DIR/lme or /tmp/lme)

	// #754/#837: scratch TTL. Applied at ingest time via the /quick-store
	// expires_at field so the `prune` subcommand can sweep expired lme-*
	// projects later. Zero means durable (no expiry).
	ScratchTTL time.Duration
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
	_, _ = fmt.Fprintln(w, "  score-batch     Score all items in one Anthropic Message Batches API call")
	_, _ = fmt.Fprintln(w, "  sample-prepare  Prepare a no-ingest representative sample output directory")
	_, _ = fmt.Fprintln(w, "  sample-analyze  Summarize existing sample checkpoints without generation/scoring")
	_, _ = fmt.Fprintln(w, "  analyze         Summarize result checkpoints and classify score failures")
	_, _ = fmt.Fprintln(w, "  route-discover  Resolve Olla/OpenAI flags from AI Flight Controller + Olla")
	_, _ = fmt.Fprintln(w, "  prune           Delete expired lme-* scratch projects (TTL sweep, #754)")
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
	_, _ = fmt.Fprintln(w, "  --cleanup-policy <val>  Project cleanup after run: auto (default), always, never")
	_, _ = fmt.Fprintln(w, "                          auto: delete only projects created by this run invocation")
	_, _ = fmt.Fprintln(w, "                          always: unconditional deletion (pre-v0 behavior)")
	_, _ = fmt.Fprintln(w, "                          never: preserve all projects (use if reusing data in a follow-up experiment)")
	_, _ = fmt.Fprintln(w, "  --no-cleanup            DEPRECATED: alias for --cleanup-policy=never")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Backend contention guard (--exclusive-backend is on by default):")
	_, _ = fmt.Fprintln(w, "  --exclusive-backend         Guard the vLLM endpoint with a PID-liveness lockfile (default true)")
	_, _ = fmt.Fprintln(w, "  --no-exclusive-backend      Disable backend lock; accept result contamination from parallel runs")
	_, _ = fmt.Fprintln(w, "  --backend-lock-dir <dir>    Override lock file directory (default: $XDG_RUNTIME_DIR/lme or /tmp/lme)")
	_, _ = fmt.Fprintln(w, "  Lock file path: <lock-dir>/backend-<sha256(normalized_url)[:12]>.lock")
	_, _ = fmt.Fprintln(w, "  Exit 75 (EX_TEMPFAIL): backend lock held by another lme run — wait and retry")
}

func parseFlagSet(fs *flag.FlagSet, args []string) int {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	return -1
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
	fs.StringVar(&cfg.ServerURL, "url", "", "Engram server URL")
	fs.StringVar(&cfg.APIKey, "api-key", "", "Engram API key")
	// #751: cleanup-policy enum replaces the old boolean --no-cleanup flag.
	// v0.x: cleanup is now scoped to ephemeral projects only. Pass --cleanup-policy=always to restore prior unconditional deletion.
	fs.StringVar((*string)(&cfg.CleanupPolicy), "cleanup-policy", string(CleanupPolicyAuto), "Project cleanup after run stage: auto (default, delete only projects created by this run), always (unconditional), never (preserve all)")
	// Deprecated: --no-cleanup is an alias for --cleanup-policy=never. Emits a deprecation WARN at parse time.
	fs.BoolVar(&cfg.NoCleanup, "no-cleanup", false, "DEPRECATED: use --cleanup-policy=never instead")
	fs.IntVar(&cfg.Retries, "retries", 1, "Retry count for generation and Engram calls")
	fs.StringVar(&cfg.OutDir, "out", ".", "Output directory for checkpoint and result files")
	fs.StringVar(&cfg.LLMBaseURL, "llm-url", envOr("LME_LLM_URL", ""), "OpenAI-compatible base URL (e.g. http://oblivion:8000/v1); bypasses claude CLI when set")
	fs.StringVar(&cfg.LLMModel, "llm-model", envOr("LME_LLM_MODEL", ""), "Model name for --llm-url endpoint")
	fs.BoolVar(&cfg.EnableThinking, "enable-thinking", false, "Enable chain-of-thought reasoning (Qwen3 and compatible models; do NOT use with Nemotron v3)")
	fs.IntVar(&cfg.LLMMaxTokens, "max-tokens", 0, "Output token budget for OAI endpoint; 0 = auto (2048 without thinking, 8192 with thinking)")
	fs.StringVar(&cfg.ScoreOutput, "score-output", envOr("LME_SCORE_OUTPUT", "text"), "score summary stdout mode: text, json, or quiet")
	fs.StringVar(&cfg.GenerationModel, "generation-model", "sonnet", "Claude model for answer generation: opus, sonnet, or haiku")
	fs.BoolVar(&cfg.ContextTopKBump, "context-topk-bump", false, "Raise context topK to 15 for all question types")
	fs.IntVar(&cfg.RecallTopK, "recall-topk", 100, "memories to recall before context trim (1–500)")
	fs.IntVar(&cfg.ContextTopKOverride, "context-topk", 0, "explicit context topK; 0 = per-type default")
	fs.BoolVar(&cfg.ChronoSort, "chrono-sort", false, "sort context blocks by Session date ascending before prompt assembly")
	fs.BoolVar(&cfg.DisableQueryRewrite, "disable-query-rewrite", false, "use raw question as recall query; skip temporal/preference rewriting")
	fs.IntVar(&cfg.MaxBlockChars, "max-block-chars", 0, "truncate each context block to this many chars before prompt assembly; 0 = no limit (use with large --context-topk to stay within vLLM max_model_len)")
	fs.StringVar(&cfg.RepairPreset, "repair-preset", "", "named LongMemEval repair preset to enable known repair switches: recall-repair")
	// H16: prepend question_date as first line of temporal-reasoning prompts
	fs.BoolVar(&cfg.InjectQuestionDate, "inject-question-date", false, "prepend 'Today's date is: {question_date}' as the first line of temporal-reasoning prompts to anchor relative-time references before the model reads memory context (default off)")
	// Exp-14: H-M5 chrono-sort forcing + H-M1 entity enumeration pass
	fs.BoolVar(&cfg.TemporalPromptAug, "temporal-prompt-aug", false, "inject ordering and entity-enumeration instructions into temporal-reasoning prompts: asks the model to list events chronologically and enumerate all matching events before committing to an answer (default off)")
	// H15: paraphrased multi-pass BM25 union
	fs.IntVar(&cfg.QueryParaphrasePasses, "query-paraphrase-passes", 0, "number of paraphrased query variants to generate per question and union with the primary recall pass; 0 = off (default); each variant is generated by Haiku emphasising different verbs and synonyms")
	// H15: dual-query preference recall (lme-h8h12h15 branch)
	fs.BoolVar(&cfg.DualPreferenceRecall, "dual-preference-recall", false, "H15: run a second subject-anchor recall for preference questions and union both result sets (default off)")
	// PA: preference-anchoring generation prompt (#938)
	fs.BoolVar(&cfg.PreferenceAnchor, "preference-anchor", false, "PA: inject single-session anchoring instructions into the generation prompt for preference questions; targets session-averaging failures (default off)")
	// H8: exhaustive aggregation recall (lme-h8h12h15 branch)
	fs.BoolVar(&cfg.ExhaustiveAggregation, "exhaustive-aggregation", false, "H8: run a topK=500 sweep recall for count-shaped questions and union with primary results (default off)")
	// H12: enumerate-first generation prompt (lme-h8h12h15 branch)
	fs.BoolVar(&cfg.EnumerateFirst, "enumerate-first", false, "H12: inject enumerate-then-total generation instruction for aggregation questions (default off)")
	// #749: contention guard. --no-exclusive-backend is the negation flag.
	// Default is exclusive=true; --no-exclusive-backend sets it false.
	var noExclusiveBackend bool
	cfg.ExclusiveBackend = true // default on
	fs.BoolVar(&noExclusiveBackend, "no-exclusive-backend", false, "disable the backend lock; use when you accept result contamination from parallel runs")
	fs.StringVar(&cfg.BackendLockDir, "backend-lock-dir", "", "override lock file directory (default: $XDG_RUNTIME_DIR/lme or /tmp/lme)")
	// #754: TTL stamped on per-question scratch projects at ingest time. The
	// prune subcommand sweeps anything older than this.
	fs.DurationVar(&cfg.ScratchTTL, "scratch-ttl", defaultScratchTTL(), "TTL applied to ephemeral lme-* projects at ingest time (e.g. 168h = 7 days); 0 = durable, no expiry")

	// prune has its own flag set and early return — it does not share the
	// ingest/run/score data-file workflow. See cmd/longmemeval/prune.go (#754).
	if subcommand == "prune" {
		return dispatchPrune(args[2:], stdout, stderr)
	}

	// score-efficient has its own flag set and early return.
	if subcommand == "score-efficient" {
		sefs := flag.NewFlagSet("score-efficient", flag.ContinueOnError)
		sefs.SetOutput(stderr)
		sefs.StringVar(&cfg.DataFile, "data", "", "path to longmemeval JSON (required)")
		sefs.IntVar(&cfg.Workers, "workers", 4, "parallel workers")
		sefs.StringVar(&cfg.OutDir, "out", ".", "output directory")
		sefs.IntVar(&cfg.Retries, "retries", 1, "retry count per LLM call")
		sefs.StringVar(&cfg.ScorerURL, "scorer-url", envOr("LME_SCORER_URL", ""), "OAI base URL for scoring")
		sefs.StringVar(&cfg.ScorerModel, "scorer-model", envOr("LME_SCORER_MODEL", ""), "model name for scorer")
		sefs.IntVar(&cfg.ScorerMaxTokens, "scorer-max-tokens", longmemeval.DefaultScorerMaxTokens, "max_tokens for scoring requests (default 2048)")
		sefs.BoolVar(&cfg.PreserveCorrect, "preserve-correct", true, "skip items already scored CORRECT")
		sefs.BoolVar(&cfg.ForceRescore, "force-rescore", false, "ignore checkpoint, re-score everything")
		if exit := parseFlagSet(sefs, args[2:]); exit >= 0 {
			return exit
		}
		if cfg.DataFile == "" {
			_, _ = fmt.Fprintln(stderr, "--data is required")
			return 1
		}
		if cfg.RunID == "" {
			cfg.RunID = newRunID()
		}
		return runScoreEfficient(cfg)
	}

	// score-batch has its own flag set and early return.
	if subcommand == "score-batch" {
		sbfs := flag.NewFlagSet("score-batch", flag.ContinueOnError)
		sbfs.SetOutput(stderr)
		sbfs.StringVar(&cfg.DataFile, "data", "", "path to longmemeval JSON (required)")
		sbfs.StringVar(&cfg.OutDir, "out", ".", "output directory")
		sbfs.StringVar(&cfg.ScorerModel, "scorer-model", "claude-haiku-4-5", "Anthropic model ID for batch scoring")
		sbfs.BoolVar(&cfg.PreserveCorrect, "preserve-correct", true, "skip items already scored CORRECT")
		sbfs.BoolVar(&cfg.ForceRescore, "force-rescore", false, "ignore checkpoint, re-score everything")
		sbfs.StringVar(&cfg.ScorerBatchAPIKey, "api-key-anthropic", envOr("ANTHROPIC_API_KEY", ""), "Anthropic API key (env: ANTHROPIC_API_KEY)")
		if exit := parseFlagSet(sbfs, args[2:]); exit >= 0 {
			return exit
		}
		if cfg.DataFile == "" {
			_, _ = fmt.Fprintln(stderr, "--data is required")
			return 1
		}
		if cfg.RunID == "" {
			cfg.RunID = newRunID()
		}
		return runScoreBatch(cfg)
	}

	if subcommand == "sample-prepare" {
		spfs := flag.NewFlagSet("sample-prepare", flag.ContinueOnError)
		spfs.SetOutput(stderr)
		var sp samplePrepareConfig
		spfs.StringVar(&sp.DataFile, "data", "", "path to LongMemEval cohort JSON")
		spfs.StringVar(&sp.Source, "source", "", "source results directory containing checkpoint-ingest.jsonl")
		spfs.StringVar(&sp.OutDir, "out", "", "output directory for the prepared sample")
		spfs.IntVar(&sp.Limit, "limit", 0, "maximum items to include; 0 = no global limit")
		spfs.IntVar(&sp.MaxPerType, "max-per-type", 0, "maximum items per question_type; 0 = no per-type limit")
		spfs.BoolVar(&sp.CopyRun, "copy-run", false, "copy matching checkpoint-run.jsonl rows from source")
		spfs.BoolVar(&sp.CopyScore, "copy-score", false, "copy matching checkpoint-score.jsonl rows from source")
		spfs.StringVar(&sp.Description, "description", "", "description recorded in sample_manifest.json")
		if exit := parseFlagSet(spfs, args[2:]); exit >= 0 {
			return exit
		}
		result, err := prepareSample(sp)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "sample-prepare: %v\n", err)
			return 1
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return 0
	}

	if subcommand == "sample-analyze" || subcommand == "analyze" {
		safs := flag.NewFlagSet(subcommand, flag.ContinueOnError)
		safs.SetOutput(stderr)
		var sa sampleAnalyzeConfig
		safs.StringVar(&sa.DataFile, "data", "", "path to LongMemEval cohort JSON")
		safs.StringVar(&sa.ResultsDir, "results", "", "results directory containing checkpoints")
		if exit := parseFlagSet(safs, args[2:]); exit >= 0 {
			return exit
		}
		summary, err := analyzeSample(sa)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", subcommand, err)
			return 1
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(summary)
		return 0
	}

	if subcommand == "route-discover" {
		rdfs := flag.NewFlagSet("route-discover", flag.ContinueOnError)
		rdfs.SetOutput(stderr)
		var rd routeDiscoverConfig
		rdfs.StringVar(&rd.FleetURL, "fleet-url", envOr("LME_FLEET_URL", "https://ai-fleet.petersimmons.com"), "AI Flight Controller base URL")
		rdfs.StringVar(&rd.OllaURL, "olla-url", envOr("LME_OLLA_URL", "https://olla.petersimmons.com"), "Olla base URL")
		rdfs.StringVar(&rd.Model, "model", envOr("LME_ROUTE_MODEL", ""), "required model name; empty selects the first compatible live model")
		rdfs.StringVar(&rd.Purpose, "purpose", envOr("LME_ROUTE_PURPOSE", "generation"), "route purpose: generation, scoring, or embedding")
		rdfs.StringVar(&rd.FleetCert, "fleet-cert", envOr("LME_FLEET_CERT", ""), "AI Flight Controller mTLS client certificate")
		rdfs.StringVar(&rd.FleetKey, "fleet-key", envOr("LME_FLEET_KEY", ""), "AI Flight Controller mTLS client key")
		rdfs.StringVar(&rd.FleetCA, "fleet-ca", envOr("LME_FLEET_CA", ""), "AI Flight Controller CA certificate")
		rdfs.DurationVar(&rd.RequestLimit, "timeout", 10*time.Second, "request timeout for discovery calls")
		if exit := parseFlagSet(rdfs, args[2:]); exit >= 0 {
			return exit
		}
		result, err := discoverRoute(rd)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "route-discover: %v\n", err)
			return 1
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return 0
	}

	switch subcommand {
	case "ingest", "run", "score", "all":
		// known subcommand — fall through to flag parsing
	case "prune":
		// handled above; dispatch never reaches here for prune
	default:
		_, _ = fmt.Fprintf(stderr, "unknown subcommand %q\n", subcommand)
		printUsage(stderr)
		return 1
	}

	if exit := parseFlagSet(fs, args[2:]); exit >= 0 {
		return exit
	}
	applySharedDefaults(cfg, fs)
	if noExclusiveBackend {
		cfg.ExclusiveBackend = false
	}
	if err := applyRepairPreset(cfg); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	// B2 (#807): validate --cleanup-policy against the known enum set.
	// Must fire before any other validation so the error is clearly attributable.
	switch cfg.CleanupPolicy {
	case CleanupPolicyAuto, CleanupPolicyAlways, CleanupPolicyNever:
		// valid
	default:
		_, _ = fmt.Fprintf(stderr, "invalid --cleanup-policy %q: must be one of auto|always|never\n", cfg.CleanupPolicy)
		return 1
	}

	// B3 (#807): --no-cleanup is deprecated; coerce to CleanupPolicyNever and warn.
	// If user explicitly set a non-default policy alongside --no-cleanup, reject
	// the combination — silently overwriting would hide intent bugs.
	if cfg.NoCleanup {
		if cfg.CleanupPolicy != CleanupPolicyAuto {
			// User passed both --no-cleanup and an explicit --cleanup-policy.
			_, _ = fmt.Fprintf(stderr,
				"conflicting flags: --no-cleanup is deprecated alias for --cleanup-policy=never; cannot combine with --cleanup-policy=%s\n",
				cfg.CleanupPolicy)
			return 1
		}
		_, _ = fmt.Fprintln(stderr, "WARN: --no-cleanup is deprecated; use --cleanup-policy=never instead")
		cfg.CleanupPolicy = CleanupPolicyNever
	}

	if cfg.DataFile == "" {
		_, _ = fmt.Fprintln(stderr, "--data is required")
		return 1
	}
	cfg.Output = stdout

	switch cfg.ScoreOutput {
	case "", "text", "json", "quiet":
		if cfg.ScoreOutput == "" {
			cfg.ScoreOutput = "text"
		}
	default:
		_, _ = fmt.Fprintf(stderr, "--score-output %q is not allowed; must be one of: text, json, quiet\n", cfg.ScoreOutput)
		return 1
	}

	validGenerationModels := map[string]bool{"opus": true, "sonnet": true, "haiku": true}
	if !validGenerationModels[cfg.GenerationModel] {
		_, _ = fmt.Fprintf(stderr, "--generation-model %q is not allowed; must be one of: opus, sonnet, haiku\n", cfg.GenerationModel)
		return 1
	}

	if cfg.RecallTopK <= 0 || cfg.RecallTopK > 500 {
		_, _ = fmt.Fprintf(stderr, "--recall-topk %d is out of range; must be 1–500\n", cfg.RecallTopK)
		return 1
	}
	if cfg.ContextTopKOverride < 0 || cfg.ContextTopKOverride > cfg.RecallTopK {
		_, _ = fmt.Fprintf(stderr, "--context-topk %d is out of range; must be 0–%d (recall-topk)\n", cfg.ContextTopKOverride, cfg.RecallTopK)
		return 1
	}

	if cfg.RunID == "" {
		cfg.RunID = newRunID()
	}

	switch subcommand {
	case "ingest":
		return runStageWithStatus(cfg, subcommand, args, func() int {
			runIngest(cfg)
			return 0
		})
	case "run":
		// #703: surface non-zero exit when runRun reports any errors.
		return runStageWithStatus(cfg, subcommand, args, func() int {
			return runRun(cfg)
		})
	case "score":
		return runStageWithStatus(cfg, subcommand, args, func() int {
			return runScore(cfg)
		})
	case "all":
		return runStageWithStatus(cfg, subcommand, args, func() int {
			return runAll(cfg)
		})
	}
	return 0
}

func runStageWithStatus(cfg *Config, stage string, commandLine []string, run func() int) int {
	startedAt := time.Now().UTC()
	exitCode := run()
	writeRunStatus(cfg, stage, startedAt, time.Now().UTC(), exitCode, commandLine)
	return exitCode
}

func newRunID() string {
	// S7 (#807): 8 bytes = 16 hex chars = 64 bits; reduces prefix-match collision
	// risk on shared infra from 1/16M (24-bit) to ~1/18E (64-bit). Callers treat
	// RunID as an opaque string so length change is backward-compatible.
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func applyRepairPreset(cfg *Config) error {
	switch strings.TrimSpace(cfg.RepairPreset) {
	case "":
		return nil
	case "recall-repair":
		cfg.DualPreferenceRecall = true
		cfg.ExhaustiveAggregation = true
		cfg.EnumerateFirst = true
		cfg.TemporalPromptAug = true
		cfg.ChronoSort = true
		return nil
	default:
		return fmt.Errorf("invalid --repair-preset %q: must be recall-repair", cfg.RepairPreset)
	}
}

func defaultAPIKey() string {
	_, token := mcpDefaults()
	return envOr("ENGRAM_API_KEY", token)
}

func defaultServerURL() string {
	url, _ := mcpDefaults()
	return envOr("ENGRAM_URL", url)
}

func applySharedDefaults(cfg *Config, fs *flag.FlagSet) {
	if !flagWasProvided(fs, "api-key") {
		cfg.APIKey = defaultAPIKey()
	}
	if !flagWasProvided(fs, "url") {
		cfg.ServerURL = defaultServerURL()
	}
}

func flagWasProvided(fs *flag.FlagSet, name string) bool {
	provided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
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
