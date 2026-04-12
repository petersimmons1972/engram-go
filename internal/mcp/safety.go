package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

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
	approvalHints   = []string{"approval", "review", "migration", "rollout", "pipeline"}
)

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

type verificationResult struct {
	Decision             string            `json:"decision"`
	ProposedAction       string            `json:"proposed_action"`
	ActionProfile        actionProfile     `json:"action_profile"`
	ChecksRun            []string          `json:"checks_run"`
	Reasons              []string          `json:"reasons,omitempty"`
	MatchedConstraints   []constraintMatch `json:"matched_constraints"`
	SuggestedSafeActions []string          `json:"suggested_safe_actions,omitempty"`
}

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

func handleCheckConstraints(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	proposedAction := getString(args, "proposed_action", "")
	if proposedAction == "" {
		return nil, fmt.Errorf("proposed_action is required")
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

func handleVerifyBeforeActing(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	proposedAction := getString(args, "proposed_action", "")
	if proposedAction == "" {
		return nil, fmt.Errorf("proposed_action is required")
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

func findConstraintMatches(ctx context.Context, h *EngineHandle, query string, profile actionProfile, limit int, staleAfterDays int) ([]constraintMatch, error) {
	ceiling := 1
	window := limit * 4
	if window < defaultConstraintWindow {
		window = defaultConstraintWindow
	}

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
			}
		}
	}

	matches := make([]constraintMatch, 0, len(candidates))
	for _, m := range candidates {
		match, ok := assessConstraintMatch(m, query, profile, staleAfterDays)
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

func buildConstraintQuery(query string) string {
	if strings.TrimSpace(query) == "" {
		return "constraint policy safety approval"
	}
	return "constraint policy safety approval " + query
}

func isConstraintMemory(m *types.Memory) bool {
	if m == nil {
		return false
	}
	if hasAnyTag(m.Tags, constraintTagMarkers) {
		return true
	}
	if m.Importance <= 1 && hasScopedConstraintTag(m.Tags) {
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

func assessConstraintMatch(m *types.Memory, query string, profile actionProfile, staleAfterDays int) (constraintMatch, bool) {
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

	match.MatchScore = score
	match.MatchReasons = dedupeStrings(reasons)
	match.ConstraintSignals = inferConstraintSignals(m)
	return match, score > 0
}

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

func requiresApproval(m *types.Memory) bool {
	for _, tag := range m.Tags {
		tagLower := strings.ToLower(tag)
		if tagLower == "requires_approval" || tagLower == "approval_required" {
			return true
		}
	}
	return textContainsAny(strings.ToLower(m.Content), []string{"requires approval", "require approval", "review required", "migration review", "deployment pipeline"})
}

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
	if ref.IsZero() {
		return false, ""
	}
	threshold := time.Duration(staleAfterDays) * 24 * time.Hour
	age := now.Sub(ref)
	if age > threshold {
		return true, fmt.Sprintf("older_than_%d_days", staleAfterDays)
	}
	return false, ""
}

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

func isStopword(token string) bool {
	switch token {
	case "this", "that", "with", "from", "into", "what", "when", "where", "then", "than", "will", "would", "should", "could", "have", "has", "been", "using":
		return true
	default:
		return false
	}
}

func textContainsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func containsAny(s string, needles []string) bool {
	return textContainsAny(s, needles)
}

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

func elevateDecision(current, next string) string {
	rank := map[string]int{
		"proceed":          0,
		"warn":             1,
		"require_approval": 2,
		"block":            3,
	}
	if rank[next] > rank[current] {
		return next
	}
	return current
}

func suggestSafeActions(profile actionProfile) []string {
	switch {
	case profile.ForcePush:
		return []string{"create a new branch", "use revert instead of rewriting shared history", "open a pull request for the corrective change"}
	case profile.Production && profile.DML:
		return []string{"review the data-retention or cleanup policy first", "archive or back up the data before any change", "use a controlled migration or approved maintenance runbook"}
	case profile.Production && profile.DDL:
		return []string{"create a migration file instead of changing production directly", "run the migration through the deployment pipeline", "get schema review or operator approval before execution"}
	case profile.Production:
		return []string{"verify current production state before acting", "prefer a reviewed change path over direct mutation"}
	default:
		return nil
	}
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

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
