package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
)

const (
	recallEmbedDegradedWarning = "recall degraded: embed unavailable, using BM25 fallback"
	recallDroppedHitsWarning   = "recall degraded: dropped backend hits with missing memory records"
)

// degradedMap builds the "degraded" response field.
// When embed is true the reason string is included; when embed is false the
// reason key is omitted entirely so callers do not see a misleading
// embed=false + reason="embed_timeout" combination (issue #634 fix#4).
func degradedMap(embedDegraded bool, reason string) map[string]any {
	if embedDegraded {
		m := map[string]any{"embed": true}
		if reason != "" {
			m["reason"] = reason
		}
		return m
	}
	return map[string]any{"embed": false}
}

func normalizeRecallMode(rawMode string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(rawMode))
	switch mode {
	case "", "handle", "full", "summary", "id_only":
		return mode, nil
	}
	return "", fmt.Errorf("mode must be one of: handle, full, summary, id_only")
}

func federatedFailurePayload(failed []search.FailedFederatedProject) []map[string]any {
	out := make([]map[string]any, 0, len(failed))
	for _, f := range failed {
		out = append(out, map[string]any{
			"project": f.Project,
			"error":   f.Error,
		})
	}
	return out
}

func federatedFailureMessage(baseErr error, failed []search.FailedFederatedProject) string {
	parts := make([]string, 0, len(failed))
	for _, f := range failed {
		if f.Project == "" {
			parts = append(parts, f.Error)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", f.Project, f.Error))
	}
	msg := "memory_recall: all federated projects failed"
	if baseErr != nil {
		msg += ": " + baseErr.Error()
	}
	if len(parts) > 0 {
		msg += "; failures: " + strings.Join(parts, "; ")
	}
	return msg
}

func addRecallWarnings(out map[string]any, endpoint, reason string, embedDegraded bool, droppedHits int) {
	warnings := make([]string, 0, 2)

	// NOTE: RecallDegradedTotal is intentionally NOT incremented here.
	// The engine layer (RecallWithOpts / RecallWithinMemory) is the single
	// source of truth for this counter: it fires with the correct reason label
	// derived from the actual embed error, and it covers all callers (MCP and
	// non-MCP alike). Incrementing here would double-count every MCP recall
	// that goes through the engine. (#973/#917 blocker fix)
	if embedDegraded {
		slog.Warn("memory_recall degraded: embed unavailable, using BM25 fallback",
			"embed_endpoint", endpoint,
			"reason", reason)
		warnings = append(warnings, recallEmbedDegradedWarning)
	}
	if droppedHits > 0 {
		slog.Warn("memory_recall degraded: dropped backend hits with missing memory records",
			"dropped_hits", droppedHits)
		warnings = append(warnings, recallDroppedHitsWarning)
	}
	if len(warnings) > 0 {
		out["warnings"] = warnings
	}
}

func finiteOrZero(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func sanitizeMemoryFloatFields(m *types.Memory) {
	if m == nil {
		return
	}
	if m.PatternConfidence != nil {
		v := finiteOrZero(*m.PatternConfidence)
		m.PatternConfidence = &v
	}
	if m.DynamicImportance != nil {
		v := finiteOrZero(*m.DynamicImportance)
		m.DynamicImportance = &v
	}
	m.RetrievalIntervalHrs = finiteOrZero(m.RetrievalIntervalHrs)
	if m.RetrievalPrecision != nil {
		v := finiteOrZero(*m.RetrievalPrecision)
		m.RetrievalPrecision = &v
	}
}

func atomPreambleForResults(results []types.SearchResult) string {
	for _, r := range results {
		if r.AtomPreamble != "" {
			return r.AtomPreamble
		}
	}
	return ""
}

func sanitizeRecallResults(results []types.SearchResult) {
	for i := range results {
		results[i].Score = finiteOrZero(results[i].Score)
		results[i].ChunkScore = finiteOrZero(results[i].ChunkScore)
		for k, v := range results[i].ScoreBreakdown {
			results[i].ScoreBreakdown[k] = finiteOrZero(v)
		}
		sanitizeMemoryFloatFields(results[i].Memory)
		for j := range results[i].Connected {
			results[i].Connected[j].Strength = finiteOrZero(results[i].Connected[j].Strength)
			sanitizeMemoryFloatFields(results[i].Connected[j].Memory)
		}
	}
}

func sanitizeConflictingResults(conflicts []types.ConflictingResult) {
	for i := range conflicts {
		conflicts[i].Strength = finiteOrZero(conflicts[i].Strength)
		sanitizeMemoryFloatFields(conflicts[i].Memory)
	}
}

func execFetch(ctx context.Context, f backendFetcher, project, id, detail string, maxBytes int, requestedChunkIDs []string) (map[string]any, error) {
	m, err := f.GetMemoryByIDInProject(ctx, id, project)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("memory %q not found", id)
	}

	switch detail {
	case "chunk":
		chunks, err := f.GetChunksForMemory(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		if len(requestedChunkIDs) > 0 {
			want := make(map[string]bool, len(requestedChunkIDs))
			for _, cid := range requestedChunkIDs {
				want[cid] = true
			}
			filtered := chunks[:0]
			for _, c := range chunks {
				if want[c.ID] {
					filtered = append(filtered, c)
				}
			}
			chunks = filtered
		}
		return map[string]any{
			"id":          m.ID,
			"memory_type": m.MemoryType,
			"project":     m.Project,
			"chunks":      chunks,
			"chunk_count": len(chunks),
		}, nil

	case "full":
		content := m.Content
		truncated := false
		if maxBytes > 0 && len(content) > maxBytes {
			content = content[:maxBytes]
			truncated = true
		}
		out := map[string]any{
			"id":          m.ID,
			"memory_type": m.MemoryType,
			"project":     m.Project,
			"content":     content,
			"truncated":   truncated,
		}
		if m.Summary != nil {
			out["summary"] = *m.Summary
		}
		return out, nil

	default: // "summary" and anything unrecognised
		sum := ""
		if m.Summary != nil {
			sum = *m.Summary
		}
		return map[string]any{
			"id":          m.ID,
			"memory_type": m.MemoryType,
			"project":     m.Project,
			"summary":     sum,
		}, nil
	}
}

// handleMemoryFetch implements the memory_fetch MCP tool.
func handleMemoryFetch(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	errResult, id := requireString(args, "id")
	if errResult != nil {
		return errResult, nil
	}
	detail := getString(args, "detail", "summary")
	chunkIDs, err := toStringSlice(args, "chunk_ids")
	if err != nil {
		return nil, fmt.Errorf("chunk_ids: %w", err)
	}
	maxBytes := cfg.FetchMaxBytes
	if maxBytes == 0 {
		maxBytes = 65536
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	result, err := execFetch(ctx, h.Engine.Backend(), project, id, detail, maxBytes, chunkIDs)
	if err != nil {
		return nil, err
	}
	return toolResult(result)
}

// claudeMergeAdapter adapts *claude.Client to search.MergeReviewer by converting
// between the search-package and claude-package candidate/decision types.
type claudeMergeAdapter struct {
	client *claude.Client
}

func (a *claudeMergeAdapter) ReviewMergeCandidates(ctx context.Context, candidates []search.MergeCandidate) ([]search.MergeDecision, error) {
	// Convert search.MergeCandidate → claude.MergeCandidate.
	claudeCandidates := make([]claude.MergeCandidate, len(candidates))
	for i, c := range candidates {
		claudeCandidates[i] = claude.MergeCandidate{
			MemoryA:    c.MemoryA,
			MemoryB:    c.MemoryB,
			Similarity: c.Similarity,
		}
	}
	claudeDecisions, err := a.client.ReviewMergeCandidates(ctx, claudeCandidates)
	if err != nil {
		return nil, err
	}
	// Convert claude.MergeDecision → search.MergeDecision.
	decisions := make([]search.MergeDecision, len(claudeDecisions))
	for i, d := range claudeDecisions {
		decisions[i] = search.MergeDecision{
			MemoryAID:     d.MemoryAID,
			MemoryBID:     d.MemoryBID,
			ShouldMerge:   d.ShouldMerge,
			Reason:        d.Reason,
			MergedContent: d.MergedContent,
		}
	}
	return decisions, nil
}

// claudeRerankAdapter bridges search.ResultReranker to claude.Client.
type claudeRerankAdapter struct {
	client *claude.Client
}

func (a *claudeRerankAdapter) RerankResults(ctx context.Context, query string, items []search.RerankItem) ([]search.RerankResult, error) {
	// Convert search.RerankItem → claude.RerankItem.
	claudeItems := make([]claude.RerankItem, len(items))
	for i, item := range items {
		claudeItems[i] = claude.RerankItem{
			ID:      item.ID,
			Summary: item.Summary,
			Score:   item.Score,
		}
	}
	claudeResults, err := a.client.RerankResults(ctx, query, claudeItems)
	if err != nil {
		return nil, err
	}
	results := make([]search.RerankResult, len(claudeResults))
	for i, r := range claudeResults {
		results[i] = search.RerankResult{
			ID:    r.ID,
			Score: r.Score,
		}
	}
	return results, nil
}

func handleMemoryRecall(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	query := getString(args, "query", "")
	if query == "" {
		return mcpgo.NewToolResultError("query: required"), nil
	}
	if limit, ok := args["limit"]; ok {
		if _, hasTopK := args["top_k"]; !hasTopK {
			args["top_k"] = limit
		}
	}
	// #1282: lenient below — handleMemoryRecall's option flags/knobs are either
	// already range-clamped (topK) or default-off ranking/behavior toggles
	// where a bad value silently degrading to the default changes result
	// quality/shape, not correctness or a caller's cost/safety intent.
	topK := clampTopK(getInt(args, "top_k", defaultTopK), recallMaxTopK()) // #1282: lenient, clamped below
	detail := getString(args, "detail", "summary")
	includeConflicts := getBool(args, "include_conflicts", false) // #1282: lenient, opt-in enrichment toggle
	recordEvent := getBool(args, "record_event", false)           // #1282: lenient, opt-in event-recording toggle
	rawMode := getString(args, "mode", cfg.RecallDefaultMode)
	mode, err := normalizeRecallMode(rawMode)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	var opts search.RecallOpts
	opts.Mode = mode
	since, before, err := parseRecallDateBounds(args)
	if err != nil {
		return nil, err
	}
	// H-NEW-1: server-side two-pass date-windowed temporal recall (flag-gated,
	// default off). When enabled, the engine parses temporal anchors from
	// question_text (falling back to query) against question_date and runs a
	// second, date-filtered pass unioned with the first.
	temporalWindowRecall := getBool(args, "temporal_window_recall", false)                  // #1282: lenient, default-off experiment flag
	eventWindowRecall := getBool(args, "event_window_recall", false)                        // Layer C B3: additive event-atom window
	eventWindowIncludeSuperseded := getBool(args, "event_window_include_superseded", false) // Layer C B4: chronology history
	questionText := getString(args, "question_text", "")
	questionDate := getString(args, "question_date", "")
	atomRecallAsOf, err := parseRecallTimeArg(args, "atom_recall_as_of")
	if err != nil {
		return nil, err
	}
	opts.AtomRecallEnabled = getBool(args, "atom_recall", false) // #1282: lenient, default-off experiment flag
	opts.AtomRecallAsOf = atomRecallAsOf

	// Federated path: "projects" overrides the single-project recall.
	projectNames, err := toStringSlice(args, "projects")
	if err != nil {
		return nil, fmt.Errorf("projects: %w", err)
	}
	if len(projectNames) > 0 {
		if eventWindowRecall {
			return mcpgo.NewToolResultError("event_window_recall is only supported for single-project recall"), nil
		}
		if since != nil || before != nil {
			return mcpgo.NewToolResultError("since/before date filters are only supported for single-project recall"), nil
		}
		if recordEvent {
			return mcpgo.NewToolResultError("record_event is not supported for federated recall"), nil
		}
		// Expand wildcard "*" to all known projects.
		if len(projectNames) == 1 && projectNames[0] == "*" {
			h0, err := pool.Get(ctx, project)
			if err != nil {
				return nil, err
			}
			all, err := h0.Engine.Backend().ListAllProjects(ctx)
			if err != nil {
				return nil, err
			}
			projectNames = all
		}
		engines := make([]*search.SearchEngine, 0, len(projectNames))
		var firstHandle *EngineHandle // retained for conflict enrichment
		var firstProject string       // project name that firstHandle was initialized for
		failedProjects := make([]search.FailedFederatedProject, 0, len(projectNames))
		for _, p := range projectNames {
			h, err := pool.Get(ctx, p)
			if err != nil {
				// Log and keep metadata for partial success diagnostics (#130, #1038).
				slog.Warn("handleMemoryRecall: skipping project — init failed",
					"project", p, "err", err)
				failedProjects = append(failedProjects, search.FailedFederatedProject{Project: p, Error: err.Error()})
				continue
			}
			if firstHandle == nil {
				firstHandle = h
				firstProject = p
			}
			engines = append(engines, h.Engine)
		}
		if len(engines) == 0 {
			return mcpgo.NewToolResultError(
				federatedFailureMessage(
					fmt.Errorf("memory_recall: failed to initialize any requested federated project"),
					failedProjects,
				),
			), nil
		}
		results, failedRecall, err := search.RecallAcrossEnginesWithEventsAndOpts(ctx, engines, query, topK, detail, opts, recordEvent)
		failedProjects = append(failedProjects, failedRecall...)
		if err != nil {
			return mcpgo.NewToolResultError(
				federatedFailureMessage(err, failedProjects),
			), nil
		}
		sanitizeRecallResults(results)
		var out map[string]any
		var federatedDegraded bool
		var federatedReason string
		if mode == "handle" {
			ok, reason := cfg.EmbedderHealth.Snapshot(ctx)
			out = map[string]any{
				"handles":    search.ToHandles(results),
				"count":      len(results),
				"fetch_hint": "call memory_fetch with id and detail=summary|chunk|full",
				"degraded":   degradedMap(!ok, reason),
			}
			if len(failedProjects) > 0 {
				out["failed_projects"] = federatedFailurePayload(failedProjects)
			}
			federatedDegraded, federatedReason = !ok, reason
		} else {
			out = map[string]any{"results": results, "count": len(results)}
			if len(failedProjects) > 0 {
				out["failed_projects"] = federatedFailurePayload(failedProjects)
			}
			if includeConflicts && firstHandle != nil {
				// All projects share the same Postgres instance, so the backend from
				// the first successfully-initialized engine can serve cross-project
				// GetRelationships and GetMemory calls (#154).
				// EnrichWithConflicts uses each result's Memory.Project for the
				// per-memory lookup; firstProject is the fallback for the rare empty case.
				conflicts := EnrichWithConflicts(ctx, firstHandle.Engine.Backend(), firstProject, results)
				sanitizeConflictingResults(conflicts)
				out["conflicting_results"] = conflicts
				out["conflict_count"] = len(conflicts)
			}
			ok, reason := cfg.EmbedderHealth.Snapshot(ctx)
			out["degraded"] = degradedMap(!ok, reason)
			federatedDegraded, federatedReason = !ok, reason
			attachSynthesisDirective(out, query)
		}
		// Single consolidated warning-emission site for the federated path (both
		// mode=="handle" and full-result branches funnel through here) — avoids
		// the duplicate addRecallWarnings call sites the two branches used to have.
		if federatedDegraded {
			addRecallWarnings(out, cfg.RouterURL, federatedReason, true, 0)
		}
		return toolResult(out)
	}

	// Single-project path.
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	rerank := getBool(args, "rerank", false)                   // #1282: lenient, opt-in reranker toggle
	exactFactBoost := getBool(args, "exact_fact_boost", false) // #1282: lenient, default-off experiment flag
	opts.DateSince = since
	opts.DateBefore = before
	// H-TAB (LME exp #3): topic-anchor boost for preference queries.
	opts.TopicAnchorBoost = getBool(args, "topic_anchor_boost", false)                  // #1282: lenient, default-off experiment flag
	opts.SessionDiversityN = getInt(args, "session_diversity_n", cfg.SessionDiversityN) // #1282: lenient, ranking-tuning knob
	opts.TemporalWindowRecall = temporalWindowRecall
	opts.EventWindowRecall = eventWindowRecall
	opts.QuestionText = questionText
	opts.QuestionDate = questionDate
	// Claude reranker is opt-in via rerank=true.
	claudeRerankEnabled := cfg.ClaudeRerankEnabled
	if cfg.RuntimeConfig != nil {
		claudeRerankEnabled = cfg.RuntimeConfig.ClaudeRerank.Load()
	}
	if claudeRerankEnabled && rerank && cfg.claudeClient != nil {
		opts.Reranker = &claudeRerankAdapter{client: cfg.claudeClient}
	}
	// Answerability reranker (LME experiment #7): flag-gated via ENGRAM_ANSWERABILITY_RERANKER=true.
	// Reorders candidates by predicted answerability (lexical coverage of query terms).
	if arReranker := search.NewAnswerabilityRerankerFromEnv(); arReranker != nil {
		opts.Reranker = arReranker
	}
	// Cross-encoder reranker (LEVER-2): flag-gated via ENGRAM_CROSS_ENCODER_RERANK=true.
	// ENGRAM_CROSS_ENCODER_URL must be set to the TEI /rerank endpoint.
	// This reranks the dense/vector leg before hybrid fusion; it does not replace
	// the final fused-result reranker.
	if ceReranker := search.NewCrossEncoderRerankerFromEnv(); ceReranker != nil {
		opts.DenseReranker = ceReranker
	}
	// Inject current session episode for same-session score boosting (Phase 3).
	if id, ok := episodeIDFromContext(ctx); ok {
		opts.CurrentEpisodeID = id
	}
	// Exact-fact identifier boost (LME #938 improvement #3, default OFF).
	opts.ExactFactBoost = exactFactBoost
	// Paraphrase-union retrieval (LME experiment #2, default OFF).
	// Enable server-wide via ENGRAM_PARAPHRASE_UNION=true env var,
	// or per-call by passing paraphrase_union=true in the MCP args.
	{
		v := strings.ToLower(strings.TrimSpace(os.Getenv("ENGRAM_PARAPHRASE_UNION")))
		// #1282: lenient, default-off experiment flag.
		opts.ParaphraseUnion = v == "true" || v == "1" || getBool(args, "paraphrase_union", false)
	}
	// RRF fusion (LME experiment #6, issue #938 improvement #1, default OFF).
	// Enable server-wide via ENGRAM_RRF_FUSION=true|1 env var, or per-call via rrf_fusion=true.
	{
		v := strings.ToLower(strings.TrimSpace(os.Getenv("ENGRAM_RRF_FUSION")))
		// #1282: lenient, default-off experiment flag.
		opts.Fusion = v == "true" || v == "1" || getBool(args, "rrf_fusion", false)
	}

	// LEVER-8: propagate server-level session-DCG aggregation flag.
	opts.SessionNDCGAgg = cfg.SessionNDCGAgg

	// H-NEW-2: propagate server-side PreferenceMMR flag into RecallOpts.
	opts.PreferenceMMR = cfg.PreferenceMMR

	// LME Phase 3 — evidence-first packing (issue #938 improvement).
	// Re-orders results so memories with verbatim identifier matches (URLs,
	// phone numbers, quoted phrases) come first, improving LLM answer accuracy
	// for entity-specific questions. Enabled via ENGRAM_EVIDENCE_FIRST_PACK=true
	// env var or per-request evidence_first_pack=true arg. Default OFF.
	{
		v := strings.ToLower(strings.TrimSpace(os.Getenv("ENGRAM_EVIDENCE_FIRST_PACK")))
		// #1282: lenient, default-off experiment flag.
		opts.EvidenceFirstPack = v == "true" || v == "1" || getBool(args, "evidence_first_pack", false)
	}

	// Capture embedder degradation signal and reason from RecallWithOpts (#989).
	var embedDegraded bool
	var embedDegradeReason string
	opts.EmbedDegraded = &embedDegraded
	opts.EmbedDegradedReason = &embedDegradeReason
	// Capture dropped-hit count (nil-Memory recall candidates) — see RecallOpts.DroppedHits.
	var droppedHits int
	opts.DroppedHits = &droppedHits

	var results []types.SearchResult
	var eventID string
	if opts.Reranker != nil {
		results, err = h.Engine.RecallWithOpts(ctx, query, topK, detail, opts)
		if err != nil {
			return nil, err
		}
		if recordEvent {
			eventID = recordRecallEvent(ctx, h, project, query, results, "rerank path")
		}
	} else if opts.CurrentEpisodeID != "" || opts.DateSince != nil || opts.DateBefore != nil || opts.TemporalWindowRecall {
		results, err = h.Engine.RecallWithOpts(ctx, query, topK, detail, opts)
		if err != nil {
			return nil, err
		}
		if recordEvent {
			eventID = recordRecallEvent(ctx, h, project, query, results, "option path")
		}
	} else {
		results, err = h.Engine.RecallWithOpts(ctx, query, topK, detail, opts)
		if err != nil {
			return nil, err
		}
		if recordEvent {
			eventID = recordRecallEvent(ctx, h, project, query, results, "default path")
		}
	}

	// LME Phase 3: evidence-first packing — reorder results by exact-signal score.
	// Applied after all recall paths so it is consistently available regardless
	// of which path (rerank, option, default) was taken. Skipped when no signals
	// are present in the query (ExtractExactSignals returns empty, OrderResultsEvidenceFirst
	// performs a stable no-op copy). No effect when flag is off.
	if opts.EvidenceFirstPack {
		results = search.OrderResultsEvidenceFirst(results, query)
	}
	atomPreamble := atomPreambleForResults(results)
	var eventWindowContext string
	var eventWindowAtoms []atom.Atom
	if opts.EventWindowRecall {
		anchorText := questionText
		if anchorText == "" {
			anchorText = query
		}
		since, before := search.ParseTemporalWindow(anchorText, questionDate)
		if since != nil && before != nil {
			eventWindowResult, eventWindowErr := h.Engine.RecallEventWindow(
				ctx,
				project,
				since,
				before,
				eventWindowIncludeSuperseded,
			)
			err = eventWindowErr
			if err != nil {
				slog.Warn("event-window recall failed; continuing without event context",
					"project", project,
					"err", err,
				)
				eventWindowContext = ""
				eventWindowAtoms = nil
			} else {
				eventWindowContext = eventWindowResult.Context
				eventWindowAtoms = eventWindowResult.Atoms
			}
			if err == nil && eventWindowContext == "" {
				slog.Debug("event-window recall found no atoms", "project", project)
			}
		}
	}

	// When ENGRAM_DEGRADED_ERROR_MODE=structured and the embed pipeline degraded,
	// surface a structured error instead of silently returning BM25 results.
	// This prevents the MCP transport timeout from synthesising a "user denied"
	// message: the caller receives a clear code and can decide whether to accept
	// the fallback results or retry later. Opt-in; default is transparent passthrough.
	if embedDegraded && cfg.DegradedErrorMode == "structured" {
		return structuredEmbedDegradedError(results)
	}

	// finalDegraded/finalReason/finalDroppedHits are finalized by whichever
	// branch below runs, then fed into the single consolidated addRecallWarnings
	// call site just before the function's one remaining return — this replaces
	// the two separate per-branch addRecallWarnings calls that used to exist
	// here, each of which could independently fire a warning for the same
	// underlying embed-degraded/dropped-hits condition.
	var out map[string]any
	var finalDegraded bool
	var finalReason string
	if mode == "handle" {
		sanitizeRecallResults(results)
		out = map[string]any{
			"handles":    search.ToHandles(results),
			"count":      len(results) + droppedHits,
			"fetch_hint": "call memory_fetch with id and detail=summary|chunk|full",
			"degraded":   degradedMap(embedDegraded, embedDegradeReason),
		}
		if droppedHits > 0 {
			if dm, ok := out["degraded"].(map[string]any); ok {
				dm["dropped_hits"] = droppedHits
			}
		}
		if atomPreamble != "" {
			out["atom_preamble"] = atomPreamble
		}
		if eventWindowContext != "" {
			out["event_window_context"] = eventWindowContext
			out["event_window_atoms"] = eventWindowAtoms
		}
		if eventID != "" {
			out["event_id"] = eventID
			out["feedback_hint"] = "Call memory_feedback with this event_id and the memory_ids you used"
		}
		finalDegraded, finalReason = embedDegraded, embedDegradeReason
	} else {
		sanitizeRecallResults(results)
		out = map[string]any{"results": results, "count": len(results) + droppedHits}
		if summary, err := buildLayerBSummary(ctx, h.Engine.Backend(), query, results); err != nil {
			slog.Warn("layer_b recall post-pass failed", "project", project, "err", err)
		} else if summary != nil {
			out["layer_b"] = summary
		}
		attachSynthesisDirective(out, query)
		if atomPreamble != "" {
			out["atom_preamble"] = atomPreamble
		}
		if eventWindowContext != "" {
			out["event_window_context"] = eventWindowContext
			out["event_window_atoms"] = eventWindowAtoms
		}
		if eventID != "" {
			out["event_id"] = eventID
			out["feedback_hint"] = "Call memory_feedback with this event_id and the memory_ids you used"
		}
		if includeConflicts {
			conflicts := EnrichWithConflicts(ctx, h.Engine.Backend(), project, results)
			sanitizeConflictingResults(conflicts)
			out["conflicting_results"] = conflicts
			out["conflict_count"] = len(conflicts)
		}
		// Add embedder health status to response.
		ok, reason := cfg.EmbedderHealth.Snapshot(ctx)
		isDegraded := embedDegraded || !ok
		out["degraded"] = degradedMap(isDegraded, reason)
		if droppedHits > 0 {
			if dm, ok := out["degraded"].(map[string]any); ok {
				dm["dropped_hits"] = droppedHits
			}
		}
		if reason == "" && embedDegraded {
			// Use the actual degradation reason surfaced by the engine (#989).
			reason = embedDegradeReason
		}
		finalDegraded, finalReason = isDegraded, reason
	}
	if finalDegraded || droppedHits > 0 {
		addRecallWarnings(out, cfg.RouterURL, finalReason, finalDegraded, droppedHits)
	}
	return toolResult(out)
}

func recordRecallEvent(ctx context.Context, h *EngineHandle, project, query string, results []types.SearchResult, path string) string {
	resultIDs := make([]string, 0, len(results))
	for _, r := range results {
		if r.Memory != nil {
			resultIDs = append(resultIDs, r.Memory.ID)
		}
	}
	event := &types.RetrievalEvent{
		ID:        types.NewMemoryID(),
		Project:   project,
		Query:     query,
		ResultIDs: resultIDs,
		CreatedAt: time.Now().UTC(),
	}
	if storeErr := h.Engine.Backend().StoreRetrievalEvent(ctx, event); storeErr != nil {
		slog.Warn("store retrieval event failed", "path", path, "project", project, "err", storeErr)
		return ""
	}
	if incErr := h.Engine.Backend().IncrementTimesRetrieved(ctx, resultIDs); incErr != nil {
		slog.Warn("auto-increment times_retrieved failed", "path", path, "project", project, "err", incErr)
	}
	return event.ID
}

func parseRecallDateBounds(args map[string]any) (*time.Time, *time.Time, error) {
	since, err := parseRecallTimeArg(args, "since")
	if err != nil {
		return nil, nil, err
	}
	before, err := parseRecallTimeArg(args, "before")
	if err != nil {
		return nil, nil, err
	}
	if since != nil && before != nil && !since.Before(*before) {
		return nil, nil, fmt.Errorf("since must be before before")
	}
	return since, before, nil
}

func parseRecallTimeArg(args map[string]any, key string) (*time.Time, error) {
	raw := strings.TrimSpace(getString(args, key, ""))
	if raw == "" {
		return nil, nil
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("%s must be RFC3339 or YYYY-MM-DD", key)
}

// structuredEmbedDegradedError returns a structured error result when the
// embed pipeline is degraded and ENGRAM_DEGRADED_ERROR_MODE=structured.
// The result is IsError=false (so the MCP transport does not synthesise
// "user denied"), but carries code:"embed_pipeline_degraded" so that
// well-behaved clients can detect and surface the degradation (#611 fix#3).
func structuredEmbedDegradedError(bm25Results []types.SearchResult) (*mcpgo.CallToolResult, error) {
	return toolResult(map[string]any{
		"code":          "embed_pipeline_degraded",
		"message":       "embed pipeline degraded; recall fell back to BM25+recency",
		"fallback_used": true,
		"results":       bm25Results,
	})
}

// handleMemoryProjects lists all projects with their memory counts and last-active timestamps.

func handleMemoryList(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	limit, limitPresent, limitErr := requireOptionalInt(args, "limit")
	if limitErr != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("limit: %v", limitErr)), nil
	}
	if !limitPresent {
		limit = 50
	}
	if limit < 1 || limit > 500 {
		limit = 50
	}
	offset, offsetPresent, offsetErr := requireOptionalInt(args, "offset")
	if offsetErr != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("offset: %v", offsetErr)), nil
	}
	if !offsetPresent {
		offset = 0
	}
	var memType *string
	if s := getString(args, "memory_type", ""); s != "" {
		memType = &s
	}
	listTags, err := toStringSlice(args, "tags")
	if err != nil {
		return nil, fmt.Errorf("tags: %w", err)
	}
	memories, err := h.Engine.List(ctx, memType, listTags, nil, limit, offset)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"memories": memories, "count": len(memories)})
}

// handleMemoryConnect creates a directional relationship between two memories.

func handleMemoryHistory(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	errResult, id := requireString(args, "memory_id")
	if errResult != nil {
		return errResult, nil
	}
	history, err := h.Engine.MemoryHistory(ctx, id)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"memory_id": id, "versions": history, "count": len(history)})
}

// handleMemoryTimeline recalls memories that were active at a given point in time.
func handleMemoryTimeline(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	errResult, asOfStr := requireString(args, "as_of")
	if errResult != nil {
		return errResult, nil
	}
	asOf, err := time.Parse(time.RFC3339, asOfStr)
	if err != nil {
		return nil, fmt.Errorf("as_of must be RFC3339 (e.g. 2025-03-05T14:00:00Z): %w", err)
	}
	// #1282: lenient — read-only page size; a bad value silently degrading to
	// the default 20 changes result breadth, not correctness.
	limit := getInt(args, "limit", 20)
	memories, err := h.Engine.MemoryAsOf(ctx, asOf, limit)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"as_of": asOfStr, "memories": memories, "count": len(memories)})
}

// handleMemorySummarize requests Ollama to summarize a memory's content.

func handleMemoryQuery(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	// Build a merged copy so we never mutate the caller's map.
	merged := make(map[string]any, len(args)+1)
	for k, v := range args {
		merged[k] = v
	}

	// Map "limit" → "top_k". If both are supplied, top_k wins.
	if limit, ok := merged["limit"]; ok {
		if _, hasTopK := merged["top_k"]; !hasTopK {
			merged["top_k"] = limit
		}
		delete(merged, "limit")
	} else if _, hasTopK := merged["top_k"]; !hasTopK {
		merged["top_k"] = 5
	}

	req2 := req
	req2.Params.Arguments = merged
	return handleMemoryRecall(ctx, pool, req2, cfg)
}

// handleMemoryExpand explores the relationship-graph neighbourhood of a known
// memory. It calls GetConnected on the engine's backend and returns all
// reachable nodes within depth hops.
func handleMemoryExpand(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	errResult, memoryID := requireString(args, "memory_id")
	if errResult != nil {
		return errResult, nil
	}
	// #1282: lenient — graph-traversal depth already range-clamped below, so a
	// bad value silently degrading to the default 2 is harmless.
	requestedDepth := getInt(args, "depth", 2)
	depth := requestedDepth
	if depth < 1 || depth > 5 {
		depth = 2
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}

	connected, err := h.Engine.Backend().GetConnected(ctx, memoryID, depth)
	if err != nil {
		return nil, err
	}

	type connItem struct {
		ID         string   `json:"id"`
		Content    string   `json:"content"`
		MemoryType string   `json:"memory_type"`
		Project    string   `json:"project"`
		Tags       []string `json:"tags"`
		CreatedAt  string   `json:"created_at,omitempty"`
		RelType    string   `json:"rel_type"`
		Direction  string   `json:"direction"`
		Strength   float64  `json:"strength"`
	}
	items := make([]connItem, 0, len(connected))
	for _, c := range connected {
		if c.Memory == nil {
			continue
		}
		createdAt := ""
		if !c.Memory.CreatedAt.IsZero() {
			createdAt = c.Memory.CreatedAt.Format(time.RFC3339)
		}
		items = append(items, connItem{
			ID:         c.Memory.ID,
			Content:    c.Memory.Content,
			MemoryType: c.Memory.MemoryType,
			Project:    c.Memory.Project,
			Tags:       c.Memory.Tags,
			CreatedAt:  createdAt,
			RelType:    c.RelType,
			Direction:  c.Direction,
			Strength:   c.Strength,
		})
	}

	out := map[string]any{
		"memory_id": memoryID,
		"depth":     depth,
		"connected": items,
	}
	if requestedDepth != depth {
		out["requested_depth"] = requestedDepth
	}
	return toolResult(out)
}

// handleMemoryModels returns installed Ollama embedding models merged with
// the curated SuggestedModels registry.
