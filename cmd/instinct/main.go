// Package main is the instinct pattern-detection daemon.
// It reads tool-use events from a JSONL buffer, groups them by session,
// calls Claude Haiku to identify patterns, and upserts those patterns
// into Engram memory.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// ── Types ────────────────────────────────────────────────────────────────────

// Event is one tool-use record written by the PostToolUse hook.
type Event struct {
	Timestamp         string `json:"timestamp"`
	SessionID         string `json:"session_id"`
	ProjectID         string `json:"project_id"`
	ToolName          string `json:"tool_name"`
	ToolInputHash     string `json:"tool_input_hash"`
	ToolOutputSummary string `json:"tool_output_summary"`
	ExitStatus        int    `json:"exit_status"`
	SchemaVersion     int    `json:"schema_version"`
}

// Pattern is the structured output produced by Haiku for a session.
type Pattern struct {
	Type         string `json:"type"`
	Description  string `json:"description"`
	Domain       string `json:"domain"`
	Evidence     string `json:"evidence"`
	TagSignature string `json:"tag_signature"`
}

// sessionKey uniquely identifies one session within one project.
type sessionKey struct {
	sessionID string
	projectID string
}

// recallResult holds an Engram memory candidate returned during dedup checks.
type recallResult struct {
	id         string
	confidence float64
}

// config holds all runtime knobs; all fields are populated by loadConfig.
type config struct {
	bufferPath    string
	minEvents     int
	engramURL     string
	engramToken   string
	anthropicKey  string
	haikuEndpoint string // empty → production endpoint; injectable for tests
}

// ── Config ───────────────────────────────────────────────────────────────────

// loadConfig builds a config from environment variables, falling back to
// ~/.claude/mcp_servers.json for Engram credentials when the env vars are
// absent.  The mcp_servers.json fallback is non-fatal: if the file is missing
// or has no engram entry, a warning is logged and loadConfig returns the
// partially-populated config so callers that only need defaults (bufferPath,
// minEvents) can still function.
func loadConfig() (config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config{}, fmt.Errorf("cannot determine home directory: %w", err)
	}
	cfg := config{
		bufferPath:   envOr("INSTINCT_BUFFER", home+"/.local/state/instinct/buffer.jsonl"),
		minEvents:    envInt("INSTINCT_MIN_EVENTS", 20),
		engramURL:    os.Getenv("ENGRAM_BASE_URL"),
		engramToken:  os.Getenv("ENGRAM_API_KEY"),
		anthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
	}

	// Fall back to ~/.claude/mcp_servers.json when env vars are absent.
	// This is best-effort: a missing file or missing engram entry is a warning,
	// not a hard error, so that tests which only care about other fields can pass.
	if cfg.engramURL == "" || cfg.engramToken == "" {
		url, tok, err := readMCPConfig(home + "/.claude/mcp_servers.json")
		if err != nil {
			slog.Warn("engram credentials not set via env and mcp_servers.json unreadable",
				"path", home+"/.claude/mcp_servers.json", "err", err)
		} else {
			if cfg.engramURL == "" {
				cfg.engramURL = url
			}
			if cfg.engramToken == "" {
				cfg.engramToken = tok
			}
		}
	}

	return cfg, nil
}

// readMCPConfig parses ~/.claude/mcp_servers.json and extracts the URL and
// Bearer token for the "engram" MCP server entry.
func readMCPConfig(path string) (url, token string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var raw struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", "", fmt.Errorf("parse %s: %w", path, err)
	}
	srv, ok := raw.MCPServers["engram"]
	if !ok {
		return "", "", fmt.Errorf("no 'engram' entry in mcpServers")
	}
	if srv.URL == "" {
		return "", "", fmt.Errorf("engram entry has no url in %s", path)
	}
	tok := strings.TrimPrefix(srv.Headers["Authorization"], "Bearer ")
	return srv.URL, tok, nil
}

// envOr returns the value of the named environment variable, or def when unset
// or empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envInt returns the integer value of the named environment variable, or def
// when unset, empty, or not parseable as a decimal integer.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// loadAndRotate reads the buffer JSONL. Returns empty if file missing or
// line count < minEvents. On success renames to .processed and returns events + new path.
func loadAndRotate(bufferPath string, minEvents int) ([]Event, string) {
	data, err := os.ReadFile(bufferPath)
	if err != nil {
		return nil, ""
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var valid []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			valid = append(valid, l)
		}
	}

	if len(valid) < minEvents {
		return nil, ""
	}

	var events []Event
	for _, l := range valid {
		var e Event
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			slog.Warn("instinct: skipping malformed line", "err", err)
			continue
		}
		events = append(events, e)
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Dir(bufferPath)
	base := filepath.Base(bufferPath)
	processedPath := filepath.Join(dir, base+"."+ts+".processed")
	if err := os.Rename(bufferPath, processedPath); err != nil {
		slog.Error("instinct: failed to rotate buffer", "err", err)
		return nil, ""
	}

	return events, processedPath
}

// groupBySession partitions events into buckets keyed by (sessionID, projectID).
// Each unique pair produces a separate entry in the returned map.
func groupBySession(events []Event) map[sessionKey][]Event {
	groups := make(map[sessionKey][]Event)
	for _, e := range events {
		k := sessionKey{e.SessionID, e.ProjectID}
		groups[k] = append(groups[k], e)
	}
	return groups
}

// ── Engram client ─────────────────────────────────────────────────────────────

type engramAPI interface {
	connect(ctx context.Context) error
	close() error
	episodeStart(ctx context.Context, sessionID, projectID string) (string, error)
	ingest(ctx context.Context, ev Event, projectID, sessionID string) error
	episodeEnd(ctx context.Context, episodeID string) error
	store(ctx context.Context, p Pattern, confidence float64, projectID string) (string, error)
	recall(ctx context.Context, tagSignature, projectID string) (*recallResult, error)
	correct(ctx context.Context, memoryID string, confidence float64) error
}

type sseEngram struct {
	c *client.Client
}

func newSSEEngram(baseURL, token string) (*sseEngram, error) {
	var opts []transport.ClientOption
	if token != "" {
		opts = append(opts, client.WithHeaders(map[string]string{
			"Authorization": "Bearer " + token,
		}))
	}
	c, err := client.NewSSEMCPClient(baseURL+"/sse", opts...)
	if err != nil {
		return nil, err
	}
	return &sseEngram{c: c}, nil
}

func (e *sseEngram) connect(ctx context.Context) error {
	if err := e.c.Start(ctx); err != nil {
		return err
	}
	req := mcp.InitializeRequest{}
	req.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcp.Implementation{Name: "instinct", Version: "1.0.0"}
	_, err := e.c.Initialize(ctx, req)
	return err
}

func (e *sseEngram) close() error {
	return e.c.Close()
}

// callTool calls an MCP tool and returns the first text content as a parsed map.
func (e *sseEngram) callTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	result, err := e.c.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if result.IsError {
		return nil, fmt.Errorf("%s: tool returned error", name)
	}
	if len(result.Content) == 0 {
		return map[string]any{}, nil
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		return map[string]any{}, nil
	}
	return out, nil
}

func (e *sseEngram) episodeStart(ctx context.Context, sessionID, projectID string) (string, error) {
	out, err := e.callTool(ctx, "memory_episode_start", map[string]any{
		"title":   "instinct-raw:" + sessionID,
		"project": projectID,
	})
	if err != nil {
		return "", err
	}
	for _, k := range []string{"episode_id", "id"} {
		if v, ok := out[k].(string); ok && v != "" {
			return v, nil
		}
	}
	return "", nil
}

func (e *sseEngram) ingest(ctx context.Context, ev Event, projectID, sessionID string) error {
	raw, _ := json.Marshal(ev)
	_, err := e.callTool(ctx, "memory_ingest", map[string]any{
		"content":     string(raw),
		"memory_type": "context",
		"project":     projectID,
		"importance":  0.2,
		"tags":        []string{"instinct-raw", "session-" + sessionID},
	})
	return err
}

func (e *sseEngram) episodeEnd(ctx context.Context, episodeID string) error {
	_, err := e.callTool(ctx, "memory_episode_end", map[string]any{"episode_id": episodeID})
	return err
}

func (e *sseEngram) store(ctx context.Context, p Pattern, confidence float64, projectID string) (string, error) {
	content := fmt.Sprintf("%s | PROVENANCE: observed 1 time, first seen %s",
		p.Description, time.Now().UTC().Format("2006-01-02"))
	out, err := e.callTool(ctx, "memory_store", map[string]any{
		"content":     content,
		"memory_type": "pattern",
		"project":     projectID,
		"importance":  confidence,
		"tags":        []string{"instinct", p.Type, p.Domain, p.TagSignature},
	})
	if err != nil {
		return "", err
	}
	if id, ok := out["id"].(string); ok {
		return id, nil
	}
	return "", nil
}

func (e *sseEngram) recall(ctx context.Context, tagSignature, projectID string) (*recallResult, error) {
	out, err := e.callTool(ctx, "memory_recall", map[string]any{
		"query":   "instinct pattern " + tagSignature,
		"project": projectID,
	})
	if err != nil {
		return nil, err
	}
	memories, _ := out["memories"].([]any)
	for _, m := range memories {
		mem, ok := m.(map[string]any)
		if !ok {
			continue
		}
		tags, _ := mem["tags"].([]any)
		for _, t := range tags {
			if s, ok := t.(string); ok && s == tagSignature {
				id, _ := mem["id"].(string)
				imp, _ := mem["importance"].(float64)
				return &recallResult{id: id, confidence: imp}, nil
			}
		}
	}
	return nil, nil
}

func (e *sseEngram) correct(ctx context.Context, memoryID string, confidence float64) error {
	_, err := e.callTool(ctx, "memory_correct", map[string]any{
		"memory_id":  memoryID,
		"importance": confidence,
	})
	return err
}

// ── Haiku client ──────────────────────────────────────────────────────────────

const instinctSystemPrompt = `You are a pattern detection system analyzing Claude Code tool call sequences.

Analyze the tool call events and identify recurring patterns of these types:

1. CORRECTION: Evidence the user corrected the AI — re-do after rollback, "don't X" instruction, same action reversed within 3 steps.
2. ERROR_RESOLUTION: The same error (matching exit_status=1 + similar output_summary) followed by the same fix tool sequence, 2+ times.
3. WORKFLOW: A sequence of 3+ tool calls that recurs within the same session or across sessions in this batch.

Return a JSON array. Each pattern object must have these exact fields:
{
  "type": "correction" | "error_resolution" | "workflow",
  "description": "<human-readable pattern, one sentence, present tense>",
  "domain": "<one word: testing | git | editing | bash | agent | general>",
  "evidence": "<brief explanation of what you observed, max 100 chars>",
  "tag_signature": "<stable slug for deduplication, e.g. 'sig-edit-test-fail-edit'>"
}

If no patterns are found, return []. Return ONLY the JSON array — no prose, no markdown fences.`

const haikuModel = "claude-haiku-4-5-20251001"
const anthropicEndpoint = "https://api.anthropic.com/v1/messages"

// callHaiku sends events to Claude Haiku for pattern detection.
// endpoint is injectable for testing; pass "" to use the production endpoint.
// Returns empty slice on any error — patterns are best-effort.
func callHaiku(ctx context.Context, apiKey string, events []Event, endpoint string) []Pattern {
	if endpoint == "" {
		endpoint = anthropicEndpoint
	}

	var sb strings.Builder
	for _, e := range events {
		fmt.Fprintf(&sb, "[%s] %s | %s | exit=%d\n",
			e.Timestamp, e.ToolName, e.ToolOutputSummary, e.ExitStatus)
	}

	body, _ := json.Marshal(map[string]any{
		"model":      haikuModel,
		"max_tokens": 1024,
		"system": []map[string]any{{
			"type":          "text",
			"text":          instinctSystemPrompt,
			"cache_control": map[string]string{"type": "ephemeral"},
		}},
		"messages": []map[string]any{{
			"role":    "user",
			"content": sb.String(),
		}},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		slog.Error("instinct: haiku request build", "err", err)
		return nil
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("instinct: haiku HTTP", "err", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("instinct: haiku non-200", "status", resp.StatusCode)
		return nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("instinct: haiku read body", "err", err)
		return nil
	}

	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil || len(apiResp.Content) == 0 {
		slog.Error("instinct: haiku parse response", "err", err)
		return nil
	}

	text := apiResp.Content[0].Text
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var rawPatterns []Pattern
	if err := json.Unmarshal([]byte(text), &rawPatterns); err != nil {
		slog.Warn("instinct: haiku JSON parse", "err", err)
		return nil
	}

	var patterns []Pattern
	for _, p := range rawPatterns {
		if p.Type == "" || p.Description == "" || p.Domain == "" || p.Evidence == "" || p.TagSignature == "" {
			slog.Warn("instinct: skipping pattern with missing fields", "tag", p.TagSignature)
			continue
		}
		patterns = append(patterns, p)
	}
	return patterns
}

// ── Confidence management ─────────────────────────────────────────────────────

var confidenceSteps = []float64{0.3, 0.5, 0.7, 0.9}

const promoteThreshold = 0.8
const confidenceTolerance = 0.01

func nextConfidence(current float64) float64 {
	for i, s := range confidenceSteps {
		if absF(current-s) < confidenceTolerance && i+1 < len(confidenceSteps) {
			return confidenceSteps[i+1]
		}
	}
	return confidenceSteps[len(confidenceSteps)-1]
}

func prevConfidence(current float64) float64 {
	for i, s := range confidenceSteps {
		if absF(current-s) < confidenceTolerance && i > 0 {
			return confidenceSteps[i-1]
		}
	}
	return confidenceSteps[0]
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// primaryProject returns the most-frequent project_id across events.
func primaryProject(events []Event) string {
	counts := make(map[string]int)
	for _, e := range events {
		counts[e.ProjectID]++
	}
	best, bestN := "", 0
	for p, n := range counts {
		if n > bestN {
			best, bestN = p, n
		}
	}
	return best
}

func upsertPattern(ctx context.Context, e engramAPI, p Pattern, events []Event) {
	proj := primaryProject(events)
	existing, err := e.recall(ctx, p.TagSignature, proj)
	if err != nil {
		slog.Error("instinct: recall failed", "sig", p.TagSignature, "err", err)
		return
	}

	var newConf float64
	if existing == nil {
		if _, err := e.store(ctx, p, confidenceSteps[0], proj); err != nil {
			slog.Error("instinct: store failed", "sig", p.TagSignature, "err", err)
		}
		newConf = confidenceSteps[0]
	} else {
		if p.Type == "correction" {
			newConf = prevConfidence(existing.confidence)
		} else {
			newConf = nextConfidence(existing.confidence)
		}
		if absF(newConf-existing.confidence) > confidenceTolerance {
			if err := e.correct(ctx, existing.id, newConf); err != nil {
				slog.Error("instinct: correct failed", "id", existing.id, "err", err)
			}
		}
	}

	// Global promotion: confidence >= threshold AND events span >= 2 projects
	if newConf >= promoteThreshold {
		projects := make(map[string]struct{})
		for _, ev := range events {
			projects[ev.ProjectID] = struct{}{}
		}
		if len(projects) >= 2 {
			global, err := e.recall(ctx, p.TagSignature, "global")
			if err == nil && global == nil {
				if _, err := e.store(ctx, p, newConf, "global"); err != nil {
					slog.Error("instinct: global store failed", "sig", p.TagSignature, "err", err)
				}
			}
		}
	}
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	if err := run(context.Background(), cfg); err != nil {
		slog.Error("run", "err", err)
		os.Exit(1)
	}
}

// run is the top-level pipeline: load buffer → group by session → write
// episodes to Engram → detect patterns via Haiku → upsert patterns.
func run(ctx context.Context, cfg config) error {
	events, processedPath := loadAndRotate(cfg.bufferPath, cfg.minEvents)
	if len(events) == 0 {
		slog.Info("instinct: noop", "reason", "buffer below min events or missing")
		return nil
	}

	slog.Info("instinct: processing", "events", len(events))

	if cfg.engramURL == "" {
		requeue(cfg.bufferPath, processedPath)
		return fmt.Errorf("ENGRAM_BASE_URL not set")
	}

	e, err := newSSEEngram(cfg.engramURL, cfg.engramToken)
	if err != nil {
		requeue(cfg.bufferPath, processedPath)
		return fmt.Errorf("newSSEEngram: %w", err)
	}
	if err := e.connect(ctx); err != nil {
		requeue(cfg.bufferPath, processedPath)
		return fmt.Errorf("connect: %w", err)
	}
	defer e.close()

	groups := groupBySession(events)
	for key, group := range groups {
		if err := writeEpisode(ctx, e, key.sessionID, key.projectID, group); err != nil {
			slog.Error("instinct: writeEpisode failed", "session", key.sessionID, "err", err)
		}
	}

	patterns := callHaiku(ctx, cfg.anthropicKey, events, cfg.haikuEndpoint)
	slog.Info("instinct: detected patterns", "count", len(patterns))
	for _, p := range patterns {
		upsertPattern(ctx, e, p, events)
	}

	slog.Info("instinct: done", "events", len(events), "patterns", len(patterns))
	return nil
}

func writeEpisode(ctx context.Context, e engramAPI, sessionID, projectID string, events []Event) error {
	epID, err := e.episodeStart(ctx, sessionID, projectID)
	if err != nil {
		return fmt.Errorf("episodeStart: %w", err)
	}
	for _, ev := range events {
		if err := e.ingest(ctx, ev, projectID, sessionID); err != nil {
			slog.Warn("instinct: ingest failed", "tool", ev.ToolName, "err", err)
		}
	}
	if epID != "" {
		if err := e.episodeEnd(ctx, epID); err != nil {
			slog.Warn("instinct: episodeEnd failed", "err", err)
		}
	}
	return nil
}

// requeue renames a .processed file back to buffer.jsonl if the buffer no longer exists.
func requeue(bufferPath, processedPath string) {
	if processedPath == "" {
		return
	}
	if _, err := os.Stat(bufferPath); os.IsNotExist(err) {
		if err := os.Rename(processedPath, bufferPath); err != nil {
			slog.Error("instinct: requeue failed", "err", err)
		}
	}
}

