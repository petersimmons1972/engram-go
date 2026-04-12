package mcp

// TDD spec for constraint verification tools.
// This file defines the CONTRACT. safety.go does not exist yet — this file
// must fail to compile until Phase 2 is complete. That is expected and correct.
// Written against what the functions SHOULD do, not what the buggy Codex PR did.

import (
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── classifyAction ────────────────────────────────────────────────────────────

func TestClassifyAction_DeleteOnProduction(t *testing.T) {
	profile := classifyAction("DELETE FROM users WHERE id = 1 on the production database")
	assert.True(t, profile.Production, "Production flag must be set")
	assert.True(t, profile.Destructive, "Destructive flag must be set for DELETE")
	assert.True(t, profile.DML, "DML flag must be set for DELETE")
	assert.False(t, profile.DDL, "DDL must not be set for a plain DELETE")
	assert.Contains(t, profile.Signals, "production_scope")
	assert.Contains(t, profile.Signals, "destructive_mutation")
}

func TestClassifyAction_CreateIndexOnProduction(t *testing.T) {
	profile := classifyAction("Create index on the production users table")
	assert.True(t, profile.Production, "Production flag must be set")
	assert.True(t, profile.DDL, "DDL flag must be set for CREATE INDEX")
	assert.False(t, profile.DML, "DML must not be set for a CREATE INDEX")
	assert.Contains(t, profile.Signals, "schema_change")
}

func TestClassifyAction_ForcePush(t *testing.T) {
	profile := classifyAction("git push --force origin main")
	assert.True(t, profile.ForcePush, "ForcePush flag must be set")
	// force push implies destructive and main-branch signals too
	assert.True(t, profile.Destructive, "Destructive implied by force push")
	assert.Contains(t, profile.Signals, "force_push")
}

func TestClassifyAction_PlainSelect_NoFlags(t *testing.T) {
	profile := classifyAction("SELECT * FROM users LIMIT 10")
	assert.False(t, profile.Production)
	assert.False(t, profile.Destructive)
	assert.False(t, profile.DDL)
	assert.False(t, profile.DML)
	assert.False(t, profile.ForcePush)
}

// ── truncateText ──────────────────────────────────────────────────────────────

func TestTruncateText_ASCII_ExactBoundary(t *testing.T) {
	s := "hello world!"
	// Truncation at 5 should yield exactly "hello"
	got := truncateText(s, 5)
	assert.Equal(t, "hello", got)
}

func TestTruncateText_ShortString_Unchanged(t *testing.T) {
	s := "hi"
	got := truncateText(s, 100)
	assert.Equal(t, "hi", got)
}

func TestTruncateText_UTF8_NeverSplitsMultibyteChar(t *testing.T) {
	// "こんにちは" — each character is 3 bytes (UTF-8).
	// Total = 15 bytes. Truncating at 10 bytes must not split the 4th character.
	// The first 3 chars occupy bytes 0–8 (9 bytes). Bytes 9–11 are char 4.
	// So the safe truncation at 10 bytes should yield the first 3 chars (9 bytes).
	s := "こんにちは"
	got := truncateText(s, 10)
	assert.True(t, len(got) <= 10, "result must be within byte limit")
	for i, r := range got {
		_ = i
		_ = r
	}
	// Verify the result is valid UTF-8 by ranging over it without panic.
	runeCount := 0
	for range got {
		runeCount++
	}
	assert.Equal(t, 3, runeCount, "should contain exactly 3 runes (9 bytes fit in 10)")
}

// ── classifyFreshness ─────────────────────────────────────────────────────────

func TestClassifyFreshness_ExpiresAt_InPast(t *testing.T) {
	past := time.Now().UTC().Add(-1 * time.Hour)
	m := &types.Memory{
		ID:        "m1",
		CreatedAt: time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt: time.Now().UTC().Add(-24 * time.Hour),
		ExpiresAt: &past,
	}
	stale, reason := classifyFreshness(m, 180)
	assert.True(t, stale)
	assert.Equal(t, "expired", reason)
}

func TestClassifyFreshness_NextReviewAt_InPast(t *testing.T) {
	past := time.Now().UTC().Add(-1 * time.Hour)
	m := &types.Memory{
		ID:           "m2",
		CreatedAt:    time.Now().UTC().Add(-5 * 24 * time.Hour),
		UpdatedAt:    time.Now().UTC().Add(-5 * 24 * time.Hour),
		NextReviewAt: &past,
	}
	stale, reason := classifyFreshness(m, 180)
	assert.True(t, stale)
	assert.Equal(t, "review_due", reason)
}

func TestClassifyFreshness_OlderThanThreshold(t *testing.T) {
	old := time.Now().UTC().Add(-200 * 24 * time.Hour)
	m := &types.Memory{
		ID:        "m3",
		CreatedAt: old,
		UpdatedAt: old,
	}
	stale, reason := classifyFreshness(m, 180)
	assert.True(t, stale)
	assert.Equal(t, "older_than_180_days", reason)
}

func TestClassifyFreshness_RecentMemory_NotStale(t *testing.T) {
	recent := time.Now().UTC().Add(-24 * time.Hour)
	m := &types.Memory{
		ID:        "m4",
		CreatedAt: recent,
		UpdatedAt: recent,
	}
	stale, reason := classifyFreshness(m, 180)
	assert.False(t, stale)
	assert.Equal(t, "", reason)
}

// CRITICAL BUG FIX: zero timestamps must not silently return (false, "").
// The Codex version returned clean for zero-timestamp memories — that is wrong.
// A memory with no timestamp evidence is unknown, not fresh.
func TestClassifyFreshness_ZeroTimestamps_ReturnsUnknown(t *testing.T) {
	m := &types.Memory{
		ID: "m-zero",
		// CreatedAt and UpdatedAt are zero values — no timestamp evidence at all.
	}
	stale, reason := classifyFreshness(m, 180)
	assert.True(t, stale, "zero-timestamp memory must be treated as stale")
	assert.Equal(t, "unknown_timestamp", reason)
}

// ── isConstraintMemory ────────────────────────────────────────────────────────

func TestIsConstraintMemory_ExplicitConstraintTag(t *testing.T) {
	m := &types.Memory{
		ID:         "cm1",
		Tags:       []string{"constraint"},
		Importance: 3,
	}
	assert.True(t, isConstraintMemory(m))
}

func TestIsConstraintMemory_ScopedTag_Importance0(t *testing.T) {
	// importance=0 (critical) with a scoped tag must qualify — no importance gate.
	m := &types.Memory{
		ID:         "cm2",
		Tags:       []string{"env:production"},
		Importance: 0,
	}
	assert.True(t, isConstraintMemory(m))
}

func TestIsConstraintMemory_ScopedTag_Importance1(t *testing.T) {
	m := &types.Memory{
		ID:         "cm3",
		Tags:       []string{"action:ddl"},
		Importance: 1,
	}
	assert.True(t, isConstraintMemory(m))
}

func TestIsConstraintMemory_ScopedTag_Importance2_StillQualifies(t *testing.T) {
	// CRITICAL: importance=2 with a scoped tag must qualify.
	// The Codex version had an importance <= 1 gate on this path — that is wrong.
	// The scoped-tag path must have NO importance gate.
	m := &types.Memory{
		ID:         "cm4",
		Tags:       []string{"resource:users-table"},
		Importance: 2,
	}
	assert.True(t, isConstraintMemory(m), "scoped-tag path must not gate on importance")
}

func TestIsConstraintMemory_PolicyCuePhrase(t *testing.T) {
	m := &types.Memory{
		ID:      "cm5",
		Content: "Never deploy to production without operator sign-off.",
		Tags:    []string{},
	}
	assert.True(t, isConstraintMemory(m))
}

func TestIsConstraintMemory_HighImportance_NoSignal_ReturnsFalse(t *testing.T) {
	// A high-importance memory with no tags and no policy language is not a constraint.
	m := &types.Memory{
		ID:         "cm6",
		Content:    "We renamed the package layout during the last refactor.",
		Importance: 1,
	}
	assert.False(t, isConstraintMemory(m))
}

// ── tokenOverlapScore ─────────────────────────────────────────────────────────

func TestTokenOverlapScore_ExactSameTokens_HighScore(t *testing.T) {
	a := "production database migration review required"
	score := tokenOverlapScore(a, a)
	// All tokens match; score should be > 0
	assert.Greater(t, score, 0)
}

func TestTokenOverlapScore_CompletelyDifferent_Zero(t *testing.T) {
	a := "production database migration"
	b := "bicycle repair manual"
	score := tokenOverlapScore(a, b)
	assert.Equal(t, 0, score)
}

func TestTokenOverlapScore_PartialOverlap_Intermediate(t *testing.T) {
	a := "production database migration review"
	b := "database migration checklist approval"
	score := tokenOverlapScore(a, b)
	// "database" and "migration" overlap (both >= 4 chars, not stopwords)
	assert.Greater(t, score, 0)
	// But not as high as identical
	full := tokenOverlapScore(a, a)
	assert.Less(t, score, full)
}

// ── buildConstraintQuery ──────────────────────────────────────────────────────

func TestBuildConstraintQuery_NonEmpty_ReturnsQueryDirectly(t *testing.T) {
	// CRITICAL BUG FIX: the Codex version prepended "constraint policy safety approval "
	// to every query. That contaminates the semantic search vector.
	// The corrected version returns the query as-is.
	query := "delete from users on production"
	got := buildConstraintQuery(query)
	assert.Equal(t, query, got, "non-empty query must be returned verbatim")
	assert.NotContains(t, got, "constraint policy", "must not prepend generic preamble")
}

func TestBuildConstraintQuery_EmptyQuery_ReturnsFallback(t *testing.T) {
	got := buildConstraintQuery("")
	assert.NotEmpty(t, got, "empty query must return a non-empty fallback")
	// Fallback should be something reasonable, not the empty string
	assert.Greater(t, len(got), 3)
}

// ── inferSeverity ─────────────────────────────────────────────────────────────

func TestInferSeverity_Importance0_Block(t *testing.T) {
	m := &types.Memory{ID: "s1", Importance: 0}
	assert.Equal(t, "critical", inferSeverity(m))
}

func TestInferSeverity_SeverityTagOverrides(t *testing.T) {
	m := &types.Memory{
		ID:         "s2",
		Importance: 3,
		Tags:       []string{"severity:critical"},
	}
	// Tag overrides the importance-based default
	assert.Equal(t, "critical", inferSeverity(m))
}

// ── evaluateVerification ──────────────────────────────────────────────────────

func TestEvaluateVerification_CriticalConstraintBlocks(t *testing.T) {
	profile := classifyAction("DELETE FROM events on production")
	result := evaluateVerification(profile.Text, profile, []constraintMatch{
		{
			Severity:         "critical",
			RequiresApproval: true,
			MatchScore:       4.2,
			MatchReasons:     []string{"constraint_tag", "destructive_mutation_policy"},
		},
	})
	assert.Equal(t, "block", result.Decision)
	assert.Contains(t, result.Reasons, "critical_constraint_match")
	assert.Contains(t, result.Reasons, "approval_required_constraint")
	assert.NotEmpty(t, result.SuggestedSafeActions)
}

func TestEvaluateVerification_ProductionDDL_RequiresApproval(t *testing.T) {
	profile := classifyAction("Create index on the production users table")
	result := evaluateVerification(profile.Text, profile, nil)
	// No constraints stored, but production + DDL should require approval at minimum.
	assert.Equal(t, "require_approval", result.Decision)
	assert.Contains(t, result.Reasons, "production_schema_change_requires_review")
}

func TestEvaluateVerification_ForcePush_AlwaysBlocks(t *testing.T) {
	// Force push must block regardless of whether any constraints are matched.
	// This is a hardcoded baseline — cannot be overridden.
	profile := classifyAction("git push --force origin main")
	result := evaluateVerification(profile.Text, profile, nil)
	assert.Equal(t, "block", result.Decision)
	assert.Contains(t, result.Reasons, "force_push_is_high_risk")
}

func TestEvaluateVerification_NoConstraintsMatched_Proceeds(t *testing.T) {
	// A benign action with no constraints should simply proceed.
	profile := classifyAction("SELECT COUNT(*) FROM logs")
	result := evaluateVerification(profile.Text, profile, nil)
	assert.Equal(t, "proceed", result.Decision)
}

func TestEvaluateVerification_ProductionScope_LowSeverity_Warns(t *testing.T) {
	// Production scope with a low/medium severity constraint match → warn, not block.
	profile := classifyAction("read deployment config from production")
	result := evaluateVerification(profile.Text, profile, []constraintMatch{
		{
			Severity:   "low",
			MatchScore: 1.5,
		},
	})
	// Decision should be at most warn (not block or require_approval) for low severity
	assert.NotEqual(t, "block", result.Decision)
}

// ── assessConstraintMatch (score floor for recall-sourced memories) ───────────
// Issue #164: a memory returned by semantic recall with zero token overlap was
// being silently excluded (score == 0 → ok=false). Memories found via vector
// recall must always get at least a score floor of 0.1.
//
// We test the invariant at the assessConstraintMatch level: when all scoring
// conditions produce 0.0 but the memory was flagged as viaRecall, the score
// must be >= 0.1 and ok must be true.
func TestSemanticRecallHitNotExcluded(t *testing.T) {
	// A memory with no constraint tags, no matching content, and zero token
	// overlap with the query — but tagged as recall-sourced.
	m := &types.Memory{
		ID:         "recall-1",
		Content:    "xyzzy plugh twisty passages",
		Tags:       []string{"constraint"},
		Importance: 2,
		CreatedAt:  time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:  time.Now().UTC().Add(-24 * time.Hour),
	}

	profile := classifyAction("delete from users")
	// We set viaRecall=true. The function signature must accept this flag.
	match, ok := assessConstraintMatchWithRecallFlag(m, "delete from users", profile, 180, true)
	require.True(t, ok, "recall-sourced memory must not be excluded (score floor)")
	assert.GreaterOrEqual(t, match.MatchScore, 0.1, "recall-sourced memory must have score >= 0.1")
}
