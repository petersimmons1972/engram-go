package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/ingestqueue"
	"github.com/petersimmons1972/engram/internal/types"
	"golang.org/x/text/unicode/norm"
)

// Config holds server-wide configuration passed to tool handlers.
type Config struct {
	RouterURL                string
	EmbedModel               string
	SummarizeModel           string
	SummarizeEnabled         bool
	ClaudeEnabled            bool // true when a claude client is present
	ClaudeConsolidateEnabled bool
	ClaudeRerankEnabled      bool
	RuntimeConfig            *RuntimeConfig
	LogLevelVar              *slog.LevelVar
	// DataDir is the base directory for all file-system operations (export,
	// import, ingest). Paths provided by callers are validated to stay within
	// this directory. Must be set; file-operation tools return an error if empty.
	DataDir string
	// RecallDefaultMode controls the default recall response format.
	// "" or "full" returns complete SearchResults; "handle" returns lightweight
	// Handle references. Set via ENGRAM_RECALL_DEFAULT_MODE env var.
	RecallDefaultMode string
	// FetchMaxBytes caps the content returned by memory_fetch detail=full.
	// Defaults to 65536 (64 KB). Set via ENGRAM_FETCH_MAX_BYTES env var.
	FetchMaxBytes int
	// ExploreMaxIters caps memory_explore loop iterations (default 5).
	ExploreMaxIters int
	// ExploreMaxWorkers bounds FanOutReason concurrency (default 8).
	ExploreMaxWorkers int
	// ExploreTokenBudget caps cumulative scoring-call tokens (default 20000).
	ExploreTokenBudget int
	// MaxDocumentBytes is the Tier-1 streaming cap: content up to this size is
	// chunked+embedded inline. Defaults to 8 MiB. Set via
	// ENGRAM_MAX_DOCUMENT_BYTES env var.
	MaxDocumentBytes int
	// RawDocumentMaxBytes is the Tier-2 raw-storage cap: content up to this
	// size is stored in the documents table as a handle-referenced blob.
	// Above this size, ingestion is refused. Defaults to 50 MiB. Set via
	// ENGRAM_RAW_DOCUMENT_MAX_BYTES env var.
	RawDocumentMaxBytes int
	// ImportMaxBytes caps local import files before any parsing work begins.
	// Defaults to 50 MiB. Set via ENGRAM_IMPORT_MAX_BYTES env var.
	ImportMaxBytes int
	// ImportExpandedMaxBytes caps total expanded bytes parsed from compressed
	// archives such as Slack exports. Defaults to 100 MiB.
	ImportExpandedMaxBytes int
	// RAGMaxTokens caps the context window assembled for memory_ask prompt
	// synthesis. Defaults to 4096. Set via ENGRAM_RAG_MAX_TOKENS env var.
	RAGMaxTokens int
	// AllowRFC1918SetupToken extends /setup-token access to RFC1918 private
	// addresses (10.x, 172.16-31.x, 192.168.x) in addition to loopback.
	// Required for Docker setups where the host appears as a bridge IP.
	// Set via ENGRAM_SETUP_TOKEN_ALLOW_RFC1918=1.
	AllowRFC1918SetupToken bool
	// EmbedDimensions is the MRL truncation target. 0 means native output dimension.
	EmbedDimensions int
	// EpisodeTTL is the maximum age of an open episode before the background
	// sweeper closes it. Handles crash-orphaned episodes where SIGKILL or a
	// container restart prevented OnUnregisterSession from firing.
	// Default: 24h. Set to 0 to disable the sweeper entirely.
	EpisodeTTL time.Duration
	// RateLimit is the per-IP sustained rate limit in req/s. Burst is 4× this value.
	// Zero disables rate limiting (recommended for loopback/single-user deployments).
	RateLimit float64
	// EmbedDegraded is set when the startup embedding probe failed but the
	// server continued anyway. /health returns 200 with "ollama":"degraded"
	// rather than 503, because the server itself is operational.
	EmbedDegraded bool
	// CircuitStateFunc, when non-nil, is called by the /health handler to
	// surface the current circuit-breaker state in the response. It must
	// return one of "closed", "half-open", or "open". (#926)
	CircuitStateFunc func() string
	// PgPool is the PostgreSQL connection pool, used by audit and weight tools.
	// When nil, audit/weight tools return an error.
	PgPool *pgxpool.Pool
	// EmbedderHealth probes the configured LiteLLM embedder with a cached 5-second
	// check. Used by memory store/recall tools to surface a degraded field on
	// tool responses when the embedder is unavailable.
	EmbedderHealth *EmbedderHealth
	// SessionDB persists MCP session registrations across server restarts (#362).
	// When nil, session persistence is disabled (sessions are lost on restart).
	SessionDB db.SessionRegistry
	// IngestQueue routes bulk ingest operations through a bounded async worker pool,
	// preventing MCP timeouts on large imports. nil = synchronous fallback.
	IngestQueue *ingestqueue.Queue
	// testHooks is nil in production; set only in tests to inject stubs.
	testHooks    *testHooks
	claudeClient *claude.Client // set via Server.SetClaudeClient

	// RateLimitRPS is the sustained request rate allowed per remote IP.
	// 0 means use the default (50 req/s). Set via ENGRAM_RATE_LIMIT_RPS env var.
	RateLimitRPS int
	// RateLimitBurst is the token-bucket burst size per remote IP.
	// 0 means use the default (200). Set via ENGRAM_RATE_LIMIT_BURST env var.
	RateLimitBurst int
	// RateLimitDisable, when true, skips the per-IP rate limiter entirely for
	// all authenticated endpoints. Intended for single-user local machines where
	// bulk writes and setup-token hammering would otherwise cause 429s.
	// Set via ENGRAM_RATE_LIMIT_DISABLE env var.
	RateLimitDisable bool

	// SessionRehydrateWindow is the lookback window for ListActiveSessions during
	// server restart. Sessions older than this are not rehydrated. 0 means default 2h.
	// Set via ENGRAM_SESSION_REHYDRATE_WINDOW env var.
	SessionRehydrateWindow time.Duration
	// EmbedRatePerSecond is the per-project sustained embed call rate (tokens/s).
	// 0 disables per-project embed rate limiting. When set, the token bucket
	// cap is 2× this value to allow short bursts. Set via
	// ENGRAM_EMBED_RATE_PER_SECOND env var.
	EmbedRatePerSecond float64
	// DegradedErrorMode controls whether embed-pipeline degradation is surfaced
	// as a structured error envelope rather than silently falling back.
	// When "structured" (set via ENGRAM_DEGRADED_ERROR_MODE=structured), recall
	// and store tools return a JSON error with code "embed_pipeline_degraded",
	// fallback_used:true, and any BM25 results that were produced before the
	// embedder gave up. Default "": transparent passthrough (original behaviour).
	DegradedErrorMode string

	// SessionNDCGAgg enables LEVER-8 session-DCG aggregation re-ranking on all
	// recall calls. When true, RecallOpts.SessionNDCGAgg is set for every
	// memory_recall invocation, grouping results by "sid:" tag and re-ranking
	// sessions by the DCG of their constituent chunk cosines.
	// Set via --session-ndcg-agg flag or ENGRAM_SESSION_NDCG_AGG=true env var.
	// Default false (ablation-safe; identical to baseline when false). (LEVER-8)
	SessionNDCGAgg bool
	// SessionDiversityN is the LEVER-9 per-session chunk cap for recall results.
	// When non-zero, recall results are post-processed to ensure no single session
	// contributes more than N chunks to the returned topK. This surfaces minority-
	// session gold chunks buried under a dominant session's higher-scoring chunks.
	//
	// Note: RecallWithOpts reads ENGRAM_SESSION_DIVERSITY_N directly as a fallback
	// when this field is zero, so the env var alone is sufficient for server-wide
	// activation without wiring through the MCP handler. This field is provided for
	// future per-request override support.
	//
	// Default 0 = off (baseline-safe). (LEVER-9, issue #1121)
	SessionDiversityN int

	// PreferenceMMR enables the H-NEW-2 centroid-MMR diversity pass for
	// preference-shaped recall queries. When true, RecallWithOpts applies an
	// MMR re-scoring post-pass that surfaces domain-specific preference sessions
	// buried under the user's dominant topic. Default false.
	// Set via ENGRAM_PREFERENCE_MMR=1. (#H-NEW-2)
	PreferenceMMR bool
}

// rateLimitRPS returns the configured RPS, or the default of 50 when unset.
func (c Config) rateLimitRPS() int {
	if c.RateLimitRPS > 0 {
		return c.RateLimitRPS
	}
	return 50
}

// rateLimitBurst returns the configured burst size, or the default of 200 when unset.
func (c Config) rateLimitBurst() int {
	if c.RateLimitBurst > 0 {
		return c.RateLimitBurst
	}
	return 200
}

// backendFetcher is the narrow interface required by execFetch.
// Satisfied by db.Backend; declared separately so tests can inject a stub.
type backendFetcher interface {
	// GetMemoryByIDInProject retrieves a memory by ID scoped to the caller's
	// declared project. The unscoped GetMemoryByID remains available on
	// db.Backend for internal cross-project reads such as EnrichWithConflicts.
	GetMemoryByIDInProject(ctx context.Context, id, project string) (*types.Memory, error)
	GetChunksForMemory(ctx context.Context, id string) ([]*types.Chunk, error)
}

func toolResult(v any) (*mcpgo.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return mcpgo.NewToolResultText(string(b)), nil
}

// extractResultID pulls the "id" field from a toolResult JSON payload.
// Returns ("", false) if the result is nil or the id field is absent/non-string.
func extractResultID(result *mcpgo.CallToolResult) (string, bool) {
	if result == nil || len(result.Content) == 0 {
		return "", false
	}
	// Content[0] is a TextContent whose Text is the JSON payload.
	text, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		return "", false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text.Text), &m); err != nil {
		return "", false
	}
	id, ok := m["id"].(string)
	return id, ok && id != ""
}

// episodeIDFromContextOrArgs resolves an episode ID from tool arguments (priority 1)
// or from the session context injected by the auto-episode hook (priority 2).
func episodeIDFromContextOrArgs(ctx context.Context, args map[string]any) string {
	if id := getString(args, "episode_id", ""); id != "" {
		return id
	}
	if id, ok := episodeIDFromContext(ctx); ok {
		return id
	}
	return ""
}

// getString extracts a string arg with a fallback default.
func getString(args map[string]any, key, def string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

// requireString returns an MCP tool-error result when args[key] is missing or
// empty. Callers use the two-return pattern:
//
//	if errResult, v := requireString(args, "query"); errResult != nil { return errResult, nil }
func requireString(args map[string]any, key string) (*mcpgo.CallToolResult, string) {
	s := getString(args, key, "")
	if s == "" {
		return mcpgo.NewToolResultError(key + " is required"), ""
	}
	return nil, s
}

// bidiAndZeroWidthRanges lists Unicode codepoints that can create trust confusion
// via bidirectional control characters or zero-width joiners/separators (#249).
// Ranges are [lo, hi] inclusive.
var bidiAndZeroWidthRanges = [][2]rune{
	{0x200B, 0x200F}, // zero-width space, ZWNJ, ZWJ, LRM, RLM
	{0x202A, 0x202E}, // LRE, RLE, PDF, LRO, RLO
	{0x2060, 0x2069}, // WJ, invisible operators, FSI/LRI/RLI/PDI
	{0xFEFF, 0xFEFF}, // BOM / zero-width no-break space
	{0x061C, 0x061C}, // Arabic letter mark
}

// validateProjectName applies NFC normalization and rejects bidi/zero-width
// codepoints and names that are empty or exceed maxProjectNameLen (#249).
const maxProjectNameLen = 128

func validateProjectName(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("project name must not be empty")
	}
	s = norm.NFC.String(s)
	if len(s) > maxProjectNameLen {
		return fmt.Errorf("project name exceeds max length %d", maxProjectNameLen)
	}
	for i, r := range s {
		for _, rng := range bidiAndZeroWidthRanges {
			if r >= rng[0] && r <= rng[1] {
				return fmt.Errorf("project name contains disallowed codepoint U+%04X at byte %d", r, i)
			}
		}
	}
	return nil
}

// getProject extracts and validates the "project" argument, applying NFC
// normalization and rejecting bidi/zero-width characters (#249).
// def is the fallback when the argument is absent or empty.
func getProject(args map[string]any, def string) (string, error) {
	raw := getString(args, "project", def)
	// Apply NFC before returning so the caller always gets a normalized name.
	normalized := norm.NFC.String(strings.TrimSpace(raw))
	if err := validateProjectName(normalized); err != nil {
		return "", fmt.Errorf("project: %w", err)
	}
	return normalized, nil
}

// validateContent rejects content strings that contain C0 control characters
// (except HT/LF/CR), DEL, and C1 control characters (U+0080–U+009F) (#253).
func validateContent(s string) error {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return fmt.Errorf("content contains invalid UTF-8 at byte %d", i)
		}
		switch {
		case r == 0x09 || r == 0x0A || r == 0x0D:
			// HT, LF, CR — allowed
		case r <= 0x08, r == 0x0B, r == 0x0C, r >= 0x0E && r <= 0x1F:
			// C0 control chars except HT/LF/CR
			return fmt.Errorf("content contains disallowed control character U+%04X at byte %d", r, i)
		case r == 0x7F:
			// DEL
			return fmt.Errorf("content contains disallowed control character U+007F (DEL) at byte %d", i)
		case r >= 0x80 && r <= 0x9F:
			// C1 control characters
			return fmt.Errorf("content contains disallowed C1 control character U+%04X at byte %d", r, i)
		}
		i += size
	}
	return nil
}

// coerceToFloat converts v to float64. It accepts a native JSON number
// (float64/int) or a JSON-encoded numeric string (e.g. "250") as a
// defense-in-depth fallback for MCP clients that stringify arguments despite
// the tool's declared schema (#1281). Returns ok=false when v is present but
// cannot be interpreted as a number by either path.
func coerceToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, false
		}
		var f float64
		if err := json.Unmarshal([]byte(s), &f); err == nil {
			return f, true
		}
		return 0, false
	}
	return 0, false
}

// coerceToInt is coerceToFloat rounded to the nearest int.
func coerceToInt(v any) (int, bool) {
	f, ok := coerceToFloat(v)
	if !ok {
		return 0, false
	}
	return int(math.Round(f)), true
}

// coerceToBool converts v to bool. It accepts a native JSON boolean or the
// exact strings "true"/"false" (case-insensitive) as a defense-in-depth
// fallback (#1281). No truthy/falsy guessing on other strings.
func coerceToBool(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		switch strings.ToLower(strings.TrimSpace(b)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
		return false, false
	}
	return false, false
}

// getInt extracts an int arg (JSON numbers arrive as float64) with a
// fallback. Accepts a JSON-encoded numeric string as a coercion fallback
// (#1281); a present-but-uncoercible value silently returns def — callers on
// load-bearing params should use requireOptionalInt instead so a genuinely
// bad value produces a loud error rather than a silent default.
func getInt(args map[string]any, key string, def int) int {
	if v, ok := args[key]; ok {
		if n, ok := coerceToInt(v); ok {
			return n
		}
	}
	return def
}

// getFloat extracts a float64 arg with a fallback. Accepts a JSON-encoded
// numeric string as a coercion fallback (#1281).
func getFloat(args map[string]any, key string, def float64) float64 {
	if v, ok := args[key]; ok {
		if f, ok := coerceToFloat(v); ok {
			return f
		}
	}
	return def
}

// getBool extracts a bool arg with a fallback default. Accepts the strings
// "true"/"false" as a coercion fallback (#1281).
func getBool(args map[string]any, key string, def bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := coerceToBool(v); ok {
			return b
		}
	}
	return def
}

// requireOptionalInt extracts an optional numeric arg. present=false means
// the key is absent or null — the caller should apply its own default. A
// non-nil err means the key was present but the value could not be
// interpreted as a number even via the coerceToInt string fallback.
//
// This is the loud-error counterpart to getInt: load-bearing numeric params
// (memory_list's limit/offset, memory_store's importance, etc.) must use this
// instead of getInt so a present-but-wrongly-typed value surfaces a tool
// error rather than silently falling back to a default that looks
// indistinguishable from a legitimate response (#1279, #1280, #1281).
func requireOptionalInt(args map[string]any, key string) (value int, present bool, err error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	if n, ok := coerceToInt(v); ok {
		return n, true, nil
	}
	return 0, true, fmt.Errorf("%s must be a number, got %T", key, v)
}

// Per-tag and count limits to prevent tag injection attacks (#149).
const (
	maxTagCount  = 50
	maxTagLength = 256
)

// toStringSlice extracts args[key] as a []string, applying per-tag/count
// limits (#149). present semantics: an absent or null key returns (nil, nil)
// — "no items", the same as an omitted optional array param always meant.
//
// A present-but-wrongly-typed value is a loud error naming the key (#1279,
// #1281) — it is never silently dropped. As a defense-in-depth fallback for
// clients that still JSON-encode the array as a string despite the tool's
// declared schema, a string value is first attempted as JSON before being
// rejected.
//
// Returns an error if any tag contains NUL or C0 control characters
// (except tab/newline/carriage-return) or DEL (#252).
func toStringSlice(args map[string]any, key string) ([]string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, nil
	}
	arr, ok := v.([]any)
	if !ok {
		switch typed := v.(type) {
		case []string:
			// Go-native callers (test helpers, REST bridges, internal
			// handler-to-handler calls) build args maps directly with
			// []string rather than the []any JSON decoding produces. It IS
			// an array of strings — route it through the same per-item
			// validation below rather than rejecting the type. (Post-#1283
			// CI regression: the tags-only memory_correct integration test's
			// helper passes []string.)
			arr = make([]any, len(typed))
			for i, s := range typed {
				arr[i] = s
			}
		case string:
			var decoded []any
			if err := json.Unmarshal([]byte(typed), &decoded); err != nil {
				return nil, fmt.Errorf("%s must be an array of strings, got a string that is not valid JSON: %w", key, err)
			}
			arr = decoded
		default:
			return nil, fmt.Errorf("%s must be an array of strings, got %T", key, v)
		}
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			continue
		}
		if len(result) >= maxTagCount {
			break // silently drop excess tags
		}
		if len(s) > maxTagLength {
			s = s[:maxTagLength] // truncate oversized tag
		}
		// Reject NUL, C0 controls (except HT/LF/CR), and DEL (#252).
		for i := 0; i < len(s); i++ {
			b := s[i]
			switch {
			case b == 0x09 || b == 0x0A || b == 0x0D:
				// allowed
			case b <= 0x08, b == 0x0B, b == 0x0C, b >= 0x0E && b <= 0x1F, b == 0x7F:
				return nil, fmt.Errorf("tag %q contains disallowed control character 0x%02X at byte %d", s, b, i)
			}
		}
		result = append(result, s)
	}
	return result, nil
}
