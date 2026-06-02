package hookdaemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// recallQuery is the canonical query the session-recall hook used.
const recallQuery = "current project status recent work decisions"

// handleSessionStart ports engram-token-refresh + engram-session-recall +
// engram-flush-fallback. It ensures Engram is reachable, validates the cached
// token, flushes any buffered fallback entries, and injects recall results into
// MEMORY.md. It never blocks the session — every failure degrades to a no-op or
// a systemMessage.
func (d *Daemon) handleSessionStart(ctx context.Context, _ Request) Response {
	// 1. Server reachable? If not, surface a message but do not block.
	if err := d.cfg.Engram.Health(ctx); err != nil {
		return Response{
			Stdout:        marshalSystemMessage("⚠️  Engram: server not responding — memory recall disabled this session."),
			SystemMessage: "⚠️  Engram: server not responding — memory recall disabled this session.",
			ExitCode:      0,
		}
	}

	// 2. Validate the cached token. The daemon owns the token in memory; only
	//    re-probe when the cache is stale.
	tok := d.currentToken()
	if tok == "" {
		return Response{ExitCode: 0}
	}
	if !d.authIsFresh() {
		ok, _ := d.cfg.Engram.CheckAuth(ctx, tok)
		if ok {
			d.markAuthOK()
		} else {
			d.markAuthFail()
			msg := "❌ Engram: MCP auth failed.\nRun: cd ~/projects/engram-go && make restart && make setup\nThen run /mcp in Claude Code."
			return Response{Stdout: marshalSystemMessage(msg), SystemMessage: msg, ExitCode: 0}
		}
	}

	// 3. Flush any buffered fallback entries now that we know auth is good.
	d.flushFallback()

	// 4. Inject recall results into MEMORY.md (best-effort).
	d.injectRecall(ctx, tok)

	return Response{ExitCode: 0}
}

// injectRecall fetches global + project recall, merges, and writes the section.
func (d *Daemon) injectRecall(ctx context.Context, tok string) {
	if d.cfg.Memory == nil {
		return
	}
	globalRaw, err := d.cfg.Engram.Recall(ctx, tok, recallQuery, "global", 3)
	if err != nil {
		return
	}
	results := parseRecallResults(globalRaw)

	if proj := d.recallProject(); proj != "" && proj != "global" {
		if projRaw, err := d.cfg.Engram.Recall(ctx, tok, recallQuery, proj, 3); err == nil {
			results = mergeRecallResults(results, parseRecallResults(projRaw))
		}
	}
	if len(results) == 0 {
		return
	}
	if len(results) > 5 {
		results = results[:5]
	}
	section := renderRecallSection(results)
	if section == "" {
		return
	}
	_ = d.cfg.Memory.WriteRecallSection(section)
}

// handleUserPromptSubmit ports engram-auth-check: a fast per-message auth check
// backed by the in-memory TTL cache. Healthy → silent. Broken → systemMessage.
func (d *Daemon) handleUserPromptSubmit(ctx context.Context, _ Request) Response {
	tok := d.currentToken()
	if tok == "" {
		return Response{ExitCode: 0}
	}
	if d.authIsFresh() {
		return Response{ExitCode: 0}
	}
	ok, _ := d.cfg.Engram.CheckAuth(ctx, tok)
	if ok {
		d.markAuthOK()
		return Response{ExitCode: 0}
	}
	d.markAuthFail()
	msg := "❌ Engram auth failed.\nRun: cd ~/projects/engram-go && make restart && make setup\nThen run /mcp in Claude Code."
	return Response{Stdout: marshalSystemMessage(msg), SystemMessage: msg, ExitCode: 0}
}

// handlePreToolUse ports engram-precheck: a fast connectivity check before any
// mcp__engram__* call. Healthy → silent. Down → systemMessage (no restart from
// the daemon; it has no shell — the operator/installer owns process management).
func (d *Daemon) handlePreToolUse(ctx context.Context, _ Request) Response {
	if err := d.cfg.Engram.Health(ctx); err == nil {
		return Response{ExitCode: 0}
	}
	msg := "⚠️  Engram health check failed. This MCP call may fail; results will be captured to fallback.md. Check: docker logs engram-go-app"
	return Response{Stdout: marshalSystemMessage(msg), SystemMessage: msg, ExitCode: 0}
}

// handlePostToolUse ports engram-mcp-error-handler: when an mcp__engram__* tool
// call failed, buffer a fallback entry in memory. The buffer is flushed on the
// next SessionStart / Stop from a single goroutine, so no flock is needed.
func (d *Daemon) handlePostToolUse(_ context.Context, req Request) Response {
	entry := extractFallbackEntry(req.Payload, d.cfg.Clock.Now())
	if entry != "" {
		d.enqueueFallback(entry)
	}
	return Response{ExitCode: 0}
}

// handlePreCompact ports pre-compact-engram: flush the fallback buffer before a
// context compaction so nothing is lost across the compaction boundary.
func (d *Daemon) handlePreCompact(_ context.Context, _ Request) Response {
	d.flushFallback()
	return Response{ExitCode: 0}
}

// handleStop ports engram-session-end. It records a session-end marker in Engram
// (synchronously), flushes the fallback buffer, then requests drain — shortening
// the idle window so the daemon winds down soon after the session, WITHOUT
// killing it outright (Option C). The shim relays the response and exits; the
// daemon handles its own wind-down.
func (d *Daemon) handleStop(ctx context.Context, _ Request) Response {
	// Flush any buffered fallback first so nothing is lost if the marker store
	// is slow or fails.
	d.flushFallback()

	summary := ""
	tok := d.currentToken()
	if tok != "" {
		if err := d.cfg.Engram.Health(ctx); err == nil {
			ts := time.Unix(d.cfg.Clock.Now(), 0).UTC().Format("2006-01-02T15:04:05Z")
			body := jsonMarshal(map[string]any{
				"content":    fmt.Sprintf("[session-end] Claude Code session ended cleanly at %s", ts),
				"project":    "global",
				"tags":       []string{"session-end", "lifecycle"},
				"importance": 1,
			})
			_ = d.cfg.Engram.QuickStore(ctx, tok, body)
		}
		summary = d.sessionSummary()
	}

	// Option C: shorten the idle window, do NOT SIGTERM. If Stop never fires,
	// the normal idle-timeout still cleans up within IdleTimeout.
	d.requestDrain()

	return Response{Stdout: summary, ExitCode: 0}
}

// sessionSummary renders the one-line session-closed summary the old
// engram-session-end.sh emitted.
func (d *Daemon) sessionSummary() string {
	d.mu.Lock()
	fallback := len(d.pendingFallback)
	authErr := d.consecutiveAuthErr
	d.mu.Unlock()
	authStatus := "ok"
	if authErr > 0 {
		authStatus = fmt.Sprintf("%d failure(s)", authErr)
	}
	return fmt.Sprintf("[engram] session closed — fallback: %d queued | auth: %s", fallback, authStatus)
}

// recall result helpers -------------------------------------------------------

type recallResult struct {
	ID      string   `json:"id"`
	Summary string   `json:"summary"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	Score   float64  `json:"score"`
}

func parseRecallResults(raw []byte) []recallResult {
	if len(raw) == 0 {
		return nil
	}
	var env struct {
		Results []recallResult `json:"results"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil
	}
	return env.Results
}

// mergeRecallResults dedupes by id (falling back to a summary prefix) and sorts
// by score descending — same semantics as the Python merge in the shell script.
func mergeRecallResults(a, b []recallResult) []recallResult {
	seen := make(map[string]bool)
	merged := make([]recallResult, 0, len(a)+len(b))
	for _, r := range append(append([]recallResult{}, a...), b...) {
		key := r.ID
		if key == "" {
			s := r.Summary
			if len(s) > 40 {
				s = s[:40]
			}
			key = s
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, r)
	}
	// Stable insertion sort by score desc (small N).
	for i := 1; i < len(merged); i++ {
		for j := i; j > 0 && merged[j].Score > merged[j-1].Score; j-- {
			merged[j], merged[j-1] = merged[j-1], merged[j]
		}
	}
	return merged
}

func renderRecallSection(results []recallResult) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Engram Session Recall\n\n")
	for i, r := range results {
		summary := r.Summary
		if summary == "" {
			summary = r.Content
			if len(summary) > 120 {
				summary = summary[:120]
			}
		}
		fmt.Fprintf(&b, "**%d.** %s\n", i+1, summary)
		if len(r.Tags) > 0 {
			tags := r.Tags
			if len(tags) > 4 {
				tags = tags[:4]
			}
			fmt.Fprintf(&b, "   *tags: %s | score: %.2f*\n", strings.Join(tags, ", "), r.Score)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// extractFallbackEntry inspects a PostToolUse payload and, when it represents a
// failed mcp__engram__* call, returns a one-line fallback entry. Returns "" when
// the payload is not a failed engram tool call.
func extractFallbackEntry(payload json.RawMessage, nowUnix int64) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		ToolName     string          `json:"tool_name"`
		ToolResponse json.RawMessage `json:"tool_response"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	if !strings.HasPrefix(p.ToolName, "mcp__engram__") {
		return ""
	}
	if !toolResponseFailed(p.ToolResponse) {
		return ""
	}
	ts := time.Unix(nowUnix, 0).UTC().Format("2006-01-02T15:04:05Z")
	return fmt.Sprintf("- [%s] %s call failed — captured for retry", ts, p.ToolName)
}

// toolResponseFailed reports whether a tool_response JSON value signals an error.
// Claude Code marks failures with an "is_error":true field or an "error" string.
func toolResponseFailed(resp json.RawMessage) bool {
	if len(resp) == 0 {
		return false
	}
	var obj map[string]any
	if err := json.Unmarshal(resp, &obj); err != nil {
		// Non-object responses (e.g. a bare string) are treated as success.
		return false
	}
	if v, ok := obj["is_error"].(bool); ok && v {
		return true
	}
	if v, ok := obj["error"]; ok {
		if s, isStr := v.(string); isStr {
			return s != ""
		}
		return v != nil
	}
	return false
}
