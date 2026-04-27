package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
	"golang.org/x/text/unicode/norm"
)

// Config holds server-wide configuration passed to tool handlers.
type Config struct {
	OllamaURL                string
	EmbedModel               string
	SummarizeModel           string
	SummarizeEnabled         bool
	ClaudeEnabled            bool // true when a claude client is present
	ClaudeConsolidateEnabled bool
	ClaudeRerankEnabled      bool
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
	// RAGMaxTokens caps the context window assembled for memory_ask prompt
	// synthesis. Defaults to 4096. Set via ENGRAM_RAG_MAX_TOKENS env var.
	RAGMaxTokens int
	// AllowRFC1918SetupToken extends /setup-token access to RFC1918 private
	// addresses (10.x, 172.16-31.x, 192.168.x) in addition to loopback.
	// Required for Docker setups where the host appears as a bridge IP.
	// Set via ENGRAM_SETUP_TOKEN_ALLOW_RFC1918=1.
	AllowRFC1918SetupToken bool
	// EmbedDimensions is the MRL truncation target passed to NewOllamaClientWithDims.
	// 0 means use the model's native output dimension.
	EmbedDimensions int
	// OllamaDegraded is set when the startup embedding probe failed but the
	// server continued anyway. /health returns 200 with "ollama":"degraded"
	// rather than 503, because the server itself is operational.
	OllamaDegraded bool
	// PgPool is the PostgreSQL connection pool, used by audit and weight tools.
	// When nil, audit/weight tools return an error.
	PgPool *pgxpool.Pool
	// testHooks is nil in production; set only in tests to inject stubs.
	testHooks    *testHooks
	claudeClient *claude.Client // set via Server.SetClaudeClient
}

// backendFetcher is the narrow interface required by execFetch.
// Satisfied by db.Backend; declared separately so tests can inject a stub.
type backendFetcher interface {
	GetMemory(ctx context.Context, id string) (*types.Memory, error)
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

// getInt extracts an int arg (JSON numbers arrive as float64) with a fallback.
func getInt(args map[string]any, key string, def int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(math.Round(n))
		case int:
			return n
		}
	}
	return def
}

// getFloat extracts a float64 arg with a fallback.
func getFloat(args map[string]any, key string, def float64) float64 {
	if v, ok := args[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return def
}

// getBool extracts a bool arg with a fallback default.
func getBool(args map[string]any, key string, def bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

// Per-tag and count limits to prevent tag injection attacks (#149).
const (
	maxTagCount  = 50
	maxTagLength = 256
)

// toStringSlice converts []any to []string, applying per-tag/count limits (#149).
// Returns an error if any tag contains NUL or C0 control characters
// (except tab/newline/carriage-return) or DEL (#252).
func toStringSlice(v any) ([]string, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, nil
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
