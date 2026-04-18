package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/petersimmons1972/engram/internal/types"
)

const (
	defaultConstraintLimit  = 10
	defaultConstraintWindow = 64
	defaultStaleAfterDays   = 180
)

var (
	constraintTagMarkers = []string{
		"constraint", "policy", "guardrail", "safety", "protected", "approval_required", "requires_approval",
	}
	policyCuePhrases = []string{
		"never ", "do not", "must not", "should not", "forbidden", "requires approval", "require approval",
		"approval required", "review required", "deployment pipeline", "migration review", "without approval",
	}
	productionHints = []string{"production", "prod", "primary", "main branch", "main"}
	deleteHints     = []string{"delete from", "delete ", "truncate", "drop ", "destroy", "recreate"}
	ddlHints        = []string{"create index", "alter table", "ddl", "schema", "migration", "index "}
	forcePushHints  = []string{"git push --force", "push --force", "force push"}
)

// actionProfile describes the risk characteristics of a proposed action.
type actionProfile struct {
	Text        string   `json:"text"`
	Signals     []string `json:"signals"`
	Production  bool     `json:"production"`
	MainBranch  bool     `json:"main_branch"`
	Destructive bool     `json:"destructive"`
	DDL         bool     `json:"ddl"`
	DML         bool     `json:"dml"`
	ForcePush   bool     `json:"force_push"`
}

// constraintMatch is one constraint memory that was found relevant to a query.
type constraintMatch struct {
	MemoryID          string    `json:"memory_id"`
	Content           string    `json:"content"`
	Tags              []string  `json:"tags,omitempty"`
	Importance        int       `json:"importance"`
	Severity          string    `json:"severity"`
	RequiresApproval  bool      `json:"requires_approval"`
	UpdatedAt         time.Time `json:"updated_at"`
	CreatedAt         time.Time `json:"created_at"`
	MatchScore        float64   `json:"match_score"`
	MatchReasons      []string  `json:"match_reasons,omitempty"`
	ConstraintSignals []string  `json:"constraint_signals,omitempty"`
	Stale             bool      `json:"stale"`
	StaleReason       string    `json:"stale_reason,omitempty"`
}

// verificationResult is the output of a full verification pipeline run.
type verificationResult struct {
	Decision             string            `json:"decision"`
	ProposedAction       string            `json:"proposed_action"`
	ActionProfile        actionProfile     `json:"action_profile"`
	ChecksRun            []string          `json:"checks_run"`
	Reasons              []string          `json:"reasons,omitempty"`
	MatchedConstraints   []constraintMatch `json:"matched_constraints"`
	SuggestedSafeActions []string          `json:"suggested_safe_actions,omitempty"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// handleGetConstraints lists constraint memories matching an optional query.
func handleGetConstraints(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	query := getString(args, "query", "")
	limit := getInt(args, "limit", defaultConstraintLimit)
	if limit < 1 || limit > 50 {
		limit = defaultConstraintLimit
	}
	staleAfterDays := getInt(args, "stale_after_days", defaultStaleAfterDays)
	if staleAfterDays < 1 || staleAfterDays > 3650 {
		staleAfterDays = defaultStaleAfterDays
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}

	profile := classifyAction(query)
	matches, err := findConstraintMatches(ctx, h, query, profile, limit, staleAfterDays)
	if err != nil {
		return nil, err
	}

	return toolResult(map[string]any{
		"query":       query,
		"constraints": matches,
		"count":       len(matches),
	})
}

// handleCheckConstraints classifies a proposed action and returns matching constraints
// with a preliminary verification decision.
func handleCheckConstraints(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	proposedAction := getString(args, "proposed_action", "")
	if proposedAction == "" {
		return mcpgo.NewToolResultError("proposed_action is required"), nil
	}
	limit := getInt(args, "limit", defaultConstraintLimit)
	if limit < 1 || limit > 50 {
		limit = defaultConstraintLimit
	}
	staleAfterDays := getInt(args, "stale_after_days", defaultStaleAfterDays)
	if staleAfterDays < 1 || staleAfterDays > 3650 {
		staleAfterDays = defaultStaleAfterDays
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}

	profile := classifyAction(proposedAction)
	matches, err := findConstraintMatches(ctx, h, proposedAction, profile, limit, staleAfterDays)
	if err != nil {
		return nil, err
	}

	result := evaluateVerification(proposedAction, profile, matches)
	result.ChecksRun = []string{"constraint_lookup", "constraint_match"}
	return toolResult(result)
}

// handleVerifyBeforeActing runs the full verification pipeline including freshness
// and risk-baseline checks, returning a decision with suggested safe actions.
func handleVerifyBeforeActing(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	proposedAction := getString(args, "proposed_action", "")
	if proposedAction == "" {
		return mcpgo.NewToolResultError("proposed_action is required"), nil
	}
	limit := getInt(args, "limit", defaultConstraintLimit)
	if limit < 1 || limit > 50 {
		limit = defaultConstraintLimit
	}
	staleAfterDays := getInt(args, "stale_after_days", defaultStaleAfterDays)
	if staleAfterDays < 1 || staleAfterDays > 3650 {
		staleAfterDays = defaultStaleAfterDays
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}

	profile := classifyAction(proposedAction)
	matches, err := findConstraintMatches(ctx, h, proposedAction, profile, limit, staleAfterDays)
	if err != nil {
		return nil, err
	}

	result := evaluateVerification(proposedAction, profile, matches)
	result.ChecksRun = []string{"constraint_lookup", "constraint_match", "freshness_scan", "risk_baseline"}
	return toolResult(result)
}

// ── Core logic ────────────────────────────────────────────────────────────────

// classifyAction inspects the action text and returns a risk profile.
func classifyAction(action string) actionProfile {
	lower := strings.ToLower(strings.TrimSpace(action))
	p := actionProfile{Text: action}
	if lower == "" {
		return p
	}
	if containsAny(lower, productionHints) {
		p.Production = true
		p.Signals = append(p.Signals, "production_scope")
	}
	if containsAny(lower, []string{" main", "main branch", " origin/main", " master"}) {
		p.MainBranch = true
		p.Signals = append(p.Signals, "protected_branch")
	}
	if containsAny(lower, deleteHints) {
		p.Destructive = true
		p.DML = true
		p.Signals = append(p.Signals, "destructive_mutation")
	}
	if containsAny(lower, ddlHints) {
		p.DDL = true
		p.Signals = append(p.Signals, "schema_change")
	}
	if containsAny(lower, forcePushHints) {
		p.ForcePush = true
		p.Destructive = true
		p.MainBranch = true
		p.Signals = append(p.Signals, "force_push")
	}
	return p
}

// findConstraintMatches gathers candidate constraint memories via List (broad
// scan) and optionally Recall (semantic search), scores them, and returns the
// top matches up to limit.
//
// Bug fix (issue #164): memories returned by Recall get a minimum score of 0.1
// so they are never silently excluded due to zero token overlap.
func findConstraintMatches(
	ctx context.Context,
	h *EngineHandle,
	query string,
	profile actionProfile,
	limit int,
	staleAfterDays int,
) ([]constraintMatch, error) {
	ceiling := 1
	window := limit * 4
	if window < defaultConstraintWindow {
		window = defaultConstraintWindow
	}

	// Broad scan: grab recent high-importance memories and keep constraint ones.
	seeded, err := h.Engine.List(ctx, nil, nil, &ceiling, window, 0)
	if err != nil {
		return nil, err
	}

	candidates := make(map[string]*types.Memory, len(seeded))
	for _, m := range seeded {
		if isConstraintMemory(m) {
			candidates[m.ID] = m
		}
	}

	// Track which IDs came from semantic recall so we can apply the score floor.
	recallIDs := make(map[string]bool)

	if query != "" {
		results, err := h.Engine.Recall(ctx, buildConstraintQuery(query), window, "full")
		if err != nil {
			return nil, err
		}
		for _, r := range results {
			if r.Memory == nil {
				continue
			}
			if isConstraintMemory(r.Memory) {
				candidates[r.Memory.ID] = r.Memory
				recallIDs[r.Memory.ID] = true
			}
		}
	}

	matches := make([]constraintMatch, 0, len(candidates))
	for _, m := range candidates {
		viaRecall := recallIDs[m.ID]
		match, ok := assessConstraintMatchWithRecallFlag(m, query, profile, staleAfterDays, viaRecall)
		if !ok && query != "" {
			continue
		}
		matches = append(matches, match)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].MatchScore != matches[j].MatchScore {
			return matches[i].MatchScore > matches[j].MatchScore
		}
		if severityRank(matches[i].Severity) != severityRank(matches[j].Severity) {
			return severityRank(matches[i].Severity) < severityRank(matches[j].Severity)
		}
		return matches[i].UpdatedAt.After(matches[j].UpdatedAt)
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

// buildConstraintQuery returns the query to use for semantic recall.
//
// Bug fix: the Codex version prepended "constraint policy safety approval " to
// every query, which contaminated the embedding vector with generic terms and
// returned irrelevant results. We return the query verbatim so the vector
// search targets the actual action described.
func buildConstraintQuery(query string) string {
	if strings.TrimSpace(query) == "" {
		return "constraint policy"
	}
	return query
}

// isConstraintMemory reports whether a memory qualifies as a constraint.
//
// Three independent paths qualify:
//  1. Explicit constraint tag marker (e.g. "constraint", "policy", "guardrail").
//  2. Scoped tag (env:, action:, resource:, severity:) — NO importance gate.
//  3. Policy cue phrase in the content.
//
// Bug fix: the Codex version gated path 2 on importance <= 1, silently
// excluding importance=2 memories that carried scoped tags. All scoped-tag
// memories are constraint candidates regardless of importance.
func isConstraintMemory(m *types.Memory) bool {
	if m == nil {
		return false
	}
	if hasAnyTag(m.Tags, constraintTagMarkers) {
		return true
	}
	// No importance gate on the scoped-tag path.
	if hasScopedConstraintTag(m.Tags) {
		return true
	}
	content := strings.ToLower(m.Content)
	for _, cue := range policyCuePhrases {
		if strings.Contains(content, cue) {
			return true
		}
	}
	return false
}

// assessConstraintMatch scores a constraint memory against the proposed action.
// This is the public form that wraps assessConstraintMatchWithRecallFlag with
// viaRecall=false, preserving call-sites that do not need the recall flag.
func assessConstraintMatch(m *types.Memory, query string, profile actionProfile, staleAfterDays int) (constraintMatch, bool) {
	return assessConstraintMatchWithRecallFlag(m, query, profile, staleAfterDays, false)
}

// assessConstraintMatchWithRecallFlag scores the memory with an explicit recall
// provenance flag. When viaRecall is true and the computed score would be 0,
// a floor of 0.1 is applied so that semantically-recalled memories are never
// silently excluded. This is the fix for issue #164.
func assessConstraintMatchWithRecallFlag(m *types.Memory, query string, profile actionProfile, staleAfterDays int, viaRecall bool) (constraintMatch, bool) {
	match := constraintMatch{
		MemoryID:         m.ID,
		Content:          truncateText(m.Content, 500),
		Tags:             m.Tags,
		Importance:       m.Importance,
		Severity:         inferSeverity(m),
		RequiresApproval: requiresApproval(m),
		UpdatedAt:        m.UpdatedAt,
		CreatedAt:        m.CreatedAt,
	}

	stale, reason := classifyFreshness(m, staleAfterDays)
	match.Stale = stale
	match.StaleReason = reason

	// Empty-query path: include everything found via List, scored by importance.
	if query == "" {
		match.MatchScore = float64(4-m.Importance) / 4
		match.ConstraintSignals = inferConstraintSignals(m)
		return match, true
	}

	actionLower := strings.ToLower(query)
	contentLower := strings.ToLower(m.Content)
	reasons := make([]string, 0, 4)
	score := 0.0

	if hasAnyTag(m.Tags, constraintTagMarkers) {
		reasons = append(reasons, "constraint_tag")
		score += 1.2
	}
	if match.RequiresApproval {
		reasons = append(reasons, "approval_gate")
		score += 0.8
	}

	if profile.Production && textContainsAny(contentLower, []string{"production", "prod", "primary"}) {
		reasons = append(reasons, "production_boundary")
		score += 1.5
	}
	if profile.ForcePush && textContainsAny(contentLower, []string{"force push", "push --force", "rewrite history"}) {
		reasons = append(reasons, "force_push_policy")
		score += 2.0
	}
	if profile.DDL && textContainsAny(contentLower, []string{"create index", "ddl", "schema", "migration", "deployment pipeline"}) {
		reasons = append(reasons, "schema_change_policy")
		score += 1.5
	}
	if profile.DML && textContainsAny(contentLower, []string{"delete", "truncate", "drop", "destructive"}) {
		reasons = append(reasons, "destructive_mutation_policy")
		score += 1.8
	}

	for _, tag := range m.Tags {
		tagLower := strings.ToLower(tag)
		switch {
		case strings.HasPrefix(tagLower, "resource:"):
			if needle := strings.TrimSpace(strings.TrimPrefix(tagLower, "resource:")); needle != "" && strings.Contains(actionLower, needle) {
				reasons = append(reasons, "resource_scope_match")
				score += 2.0
			}
		case strings.HasPrefix(tagLower, "env:"):
			if needle := strings.TrimSpace(strings.TrimPrefix(tagLower, "env:")); needle != "" && strings.Contains(actionLower, needle) {
				reasons = append(reasons, "environment_scope_match")
				score += 1.5
			}
		case strings.HasPrefix(tagLower, "action:"):
			if needle := strings.TrimSpace(strings.TrimPrefix(tagLower, "action:")); needle != "" && actionTagMatchesProfile(needle, profile, actionLower) {
				reasons = append(reasons, "action_scope_match")
				score += 1.7
			}
		}
	}

	if tokenOverlapScore(actionLower, contentLower) >= 2 {
		reasons = append(reasons, "text_overlap")
		score += 0.9
	}
	if textContainsAny(contentLower, policyCuePhrases) {
		reasons = append(reasons, "policy_language")
		score += 0.5
	}
	if m.Importance <= 1 {
		score += 0.5
	}
	if stale {
		score -= 0.25
	}

	// Issue #164: memories returned by semantic recall must not be excluded
	// even when token overlap and all other signals produce zero. Apply a
	// minimum score of 0.1 so the caller can see the recall-sourced match.
	if viaRecall && score <= 0 {
		score = 0.1
	}

	match.MatchScore = score
	match.MatchReasons = dedupeStrings(reasons)
	match.ConstraintSignals = inferConstraintSignals(m)
	return match, score > 0
}

// evaluateVerification applies the risk profile and matched constraints to
// produce a final decision. The decision is a one-directional ratchet:
// proceed < warn < require_approval < block.
//
// Hardcoded baselines that cannot be overridden by stored memory state:
//   - Force push → block
//   - Production + DML with no matching policy → block
func evaluateVerification(proposedAction string, profile actionProfile, matches []constraintMatch) verificationResult {
	result := verificationResult{
		Decision:           "proceed",
		ProposedAction:     proposedAction,
		ActionProfile:      profile,
		MatchedConstraints: matches,
	}

	if len(matches) == 0 {
		result.Reasons = append(result.Reasons, "no_matching_constraints_found")
	}

	// Hardcoded risk baselines — applied before stored constraints.
	if profile.ForcePush {
		result.Decision = elevateDecision(result.Decision, "block")
		result.Reasons = append(result.Reasons, "force_push_is_high_risk")
	}
	if profile.Production && profile.DML {
		result.Decision = elevateDecision(result.Decision, "block")
		result.Reasons = append(result.Reasons, "destructive_production_mutation")
	}
	if profile.Production && profile.DDL {
		result.Decision = elevateDecision(result.Decision, "require_approval")
		result.Reasons = append(result.Reasons, "production_schema_change_requires_review")
	}
	if profile.Production && !profile.DDL && !profile.DML && !profile.ForcePush {
		result.Decision = elevateDecision(result.Decision, "warn")
		result.Reasons = append(result.Reasons, "production_scope_detected")
	}

	// Elevate based on matched constraints.
	for _, match := range matches {
		switch match.Severity {
		case "critical":
			result.Decision = elevateDecision(result.Decision, "block")
			result.Reasons = append(result.Reasons, "critical_constraint_match")
		case "high":
			result.Decision = elevateDecision(result.Decision, "require_approval")
			result.Reasons = append(result.Reasons, "high_severity_constraint_match")
		default:
			result.Decision = elevateDecision(result.Decision, "warn")
			result.Reasons = append(result.Reasons, "constraint_match")
		}
		if match.RequiresApproval {
			result.Decision = elevateDecision(result.Decision, "require_approval")
			result.Reasons = append(result.Reasons, "approval_required_constraint")
		}
		if match.Stale {
			result.Decision = elevateDecision(result.Decision, "warn")
			result.Reasons = append(result.Reasons, "stale_constraint_evidence")
		}
	}

	// No-policy fallbacks for high-risk actions.
	if len(matches) == 0 && (profile.DML || profile.ForcePush) {
		result.Decision = elevateDecision(result.Decision, "block")
		result.Reasons = append(result.Reasons, "high_risk_action_without_matching_policy")
	}
	if len(matches) == 0 && profile.DDL {
		result.Decision = elevateDecision(result.Decision, "require_approval")
		result.Reasons = append(result.Reasons, "schema_change_without_matching_policy")
	}

	result.Reasons = dedupeStrings(result.Reasons)
	result.SuggestedSafeActions = suggestSafeActions(profile)
	return result
}

// ── Severity and approval ─────────────────────────────────────────────────────

// inferSeverity returns the effective severity for a memory.
// Explicit "severity:<level>" tags override importance-based defaults.
func inferSeverity(m *types.Memory) string {
	for _, tag := range m.Tags {
		tagLower := strings.ToLower(tag)
		if strings.HasPrefix(tagLower, "severity:") {
			switch strings.TrimSpace(strings.TrimPrefix(tagLower, "severity:")) {
			case "critical":
				return "critical"
			case "high":
				return "high"
			case "medium":
				return "medium"
			case "low":
				return "low"
			}
		}
	}
	switch m.Importance {
	case 0:
		return "critical"
	case 1:
		return "high"
	case 2:
		return "medium"
	default:
		return "low"
	}
}

// requiresApproval reports whether the memory carries an approval gate.
func requiresApproval(m *types.Memory) bool {
	for _, tag := range m.Tags {
		tagLower := strings.ToLower(tag)
		if tagLower == "requires_approval" || tagLower == "approval_required" {
			return true
		}
	}
	return textContainsAny(strings.ToLower(m.Content), []string{
		"requires approval", "require approval", "review required",
		"migration review", "deployment pipeline",
	})
}

// ── Freshness ─────────────────────────────────────────────────────────────────

// classifyFreshness reports whether a memory is stale and the reason.
//
// Bug fix: the Codex version returned (false, "") when both CreatedAt and
// UpdatedAt were zero, silently treating dateless memories as fresh. A memory
// with no timestamp evidence is of unknown age and must be treated as stale
// with reason "unknown_timestamp".
func classifyFreshness(m *types.Memory, staleAfterDays int) (bool, string) {
	now := time.Now().UTC()
	if m.ExpiresAt != nil && m.ExpiresAt.Before(now) {
		return true, "expired"
	}
	if m.NextReviewAt != nil && m.NextReviewAt.Before(now) {
		return true, "review_due"
	}
	ref := m.UpdatedAt
	if ref.IsZero() {
		ref = m.CreatedAt
	}
	// Both timestamps are zero — cannot determine age; treat as stale.
	if ref.IsZero() {
		return true, "unknown_timestamp"
	}
	threshold := time.Duration(staleAfterDays) * 24 * time.Hour
	if now.Sub(ref) > threshold {
		return true, fmt.Sprintf("older_than_%d_days", staleAfterDays)
	}
	return false, ""
}

// ── Text utilities ────────────────────────────────────────────────────────────

// truncateText shortens s to at most maxBytes bytes, walking back to a valid
// UTF-8 rune boundary if the cut point falls inside a multi-byte character.
// This is the safe pattern from internal/claude/diagnose.go.
func truncateText(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	b := []byte(s)[:maxBytes]
	for !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b)
}

// tokenOverlapScore counts how many important tokens in a appear in b.
// Both inputs should be pre-lowercased for consistent comparisons.
func tokenOverlapScore(a, b string) int {
	ta := importantTokens(a)
	tb := importantTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(tb))
	for _, token := range tb {
		set[token] = struct{}{}
	}
	score := 0
	for _, token := range ta {
		if _, ok := set[token]; ok {
			score++
		}
	}
	return score
}

// importantTokens splits s into unique tokens of at least 4 characters,
// excluding common stopwords. Punctuation is normalised to spaces first.
func importantTokens(s string) []string {
	replacer := strings.NewReplacer(
		",", " ", ".", " ", ":", " ", ";", " ", "(", " ", ")", " ",
		"[", " ", "]", " ", "{", " ", "}", " ", "/", " ", "\\", " ",
		"-", " ", "_", " ", "'", " ", "\"", " ",
	)
	clean := replacer.Replace(strings.ToLower(s))
	fields := strings.Fields(clean)
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if len(field) < 4 {
			continue
		}
		if isStopword(field) {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

// isStopword reports whether token is a common English stopword to skip.
func isStopword(token string) bool {
	switch token {
	case "this", "that", "with", "from", "into", "what", "when", "where",
		"then", "than", "will", "would", "should", "could", "have", "has",
		"been", "using":
		return true
	default:
		return false
	}
}

// ── Tag helpers ───────────────────────────────────────────────────────────────

// hasAnyTag reports whether tags contains any marker (exact match or
// "<marker>:" prefix for namespaced tags).
func hasAnyTag(tags []string, markers []string) bool {
	for _, tag := range tags {
		tagLower := strings.ToLower(tag)
		for _, marker := range markers {
			if tagLower == marker || strings.HasPrefix(tagLower, marker+":") {
				return true
			}
		}
	}
	return false
}

// hasScopedConstraintTag reports whether tags contains at least one scoped
// namespace tag (env:, action:, resource:, severity:).
func hasScopedConstraintTag(tags []string) bool {
	for _, tag := range tags {
		tagLower := strings.ToLower(tag)
		if strings.HasPrefix(tagLower, "env:") ||
			strings.HasPrefix(tagLower, "action:") ||
			strings.HasPrefix(tagLower, "resource:") ||
			strings.HasPrefix(tagLower, "severity:") {
			return true
		}
	}
	return false
}

// actionTagMatchesProfile checks whether a namespaced action tag applies to
// the current profile or appears in the action text.
func actionTagMatchesProfile(tag string, profile actionProfile, actionLower string) bool {
	switch tag {
	case "production", "prod":
		return profile.Production
	case "ddl", "schema":
		return profile.DDL
	case "dml", "delete", "destructive":
		return profile.DML || profile.Destructive
	case "force-push", "force_push":
		return profile.ForcePush
	case "main":
		return profile.MainBranch
	default:
		return strings.Contains(actionLower, tag)
	}
}

// ── Decision ratchet ──────────────────────────────────────────────────────────

// decisionRank maps decision strings to severity ordinals for elevateDecision.
// Order: proceed(0) < warn(1) < require_approval(2) < block(3).
var decisionRank = map[string]int{
	"proceed":          0,
	"warn":             1,
	"require_approval": 2,
	"block":            3,
}

// elevateDecision returns the higher-severity of current and next.
// The decision never decreases — this is the one-directional ratchet.
func elevateDecision(current, next string) string {
	if decisionRank[next] > decisionRank[current] {
		return next
	}
	return current
}

// ── Signals and suggestions ───────────────────────────────────────────────────

// inferConstraintSignals returns human-readable signal labels for a memory.
func inferConstraintSignals(m *types.Memory) []string {
	signals := make([]string, 0, 4)
	if m.Importance <= 1 {
		signals = append(signals, "high_importance")
	}
	if hasAnyTag(m.Tags, constraintTagMarkers) {
		signals = append(signals, "explicit_constraint_tag")
	}
	if requiresApproval(m) {
		signals = append(signals, "approval_language")
	}
	if textContainsAny(strings.ToLower(m.Content), []string{"production", "main", "primary"}) {
		signals = append(signals, "protected_target")
	}
	return dedupeStrings(signals)
}

// suggestSafeActions returns safer alternatives for the given risk profile.
func suggestSafeActions(profile actionProfile) []string {
	switch {
	case profile.ForcePush:
		return []string{
			"create a new branch",
			"use revert instead of rewriting shared history",
			"open a pull request for the corrective change",
		}
	case profile.Production && profile.DML:
		return []string{
			"review the data-retention or cleanup policy first",
			"archive or back up the data before any change",
			"use a controlled migration or approved maintenance runbook",
		}
	case profile.Production && profile.DDL:
		return []string{
			"create a migration file instead of changing production directly",
			"run the migration through the deployment pipeline",
			"get schema review or operator approval before execution",
		}
	case profile.Production:
		return []string{
			"verify current production state before acting",
			"prefer a reviewed change path over direct mutation",
		}
	default:
		return nil
	}
}

// ── General string helpers ────────────────────────────────────────────────────

// severityRank returns a sortable rank for a severity string.
// Lower rank = higher severity (critical=0, high=1, ...).
func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

// textContainsAny reports whether s contains any of the needles.
func textContainsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// containsAny is an alias for textContainsAny (shorter call-site name).
func containsAny(s string, needles []string) bool {
	return textContainsAny(s, needles)
}

// dedupeStrings returns a copy of values with duplicates and empty strings removed,
// preserving original order.
func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
