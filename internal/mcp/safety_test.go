package mcp

import (
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyAction_DeleteOnProduction(t *testing.T) {
	profile := classifyAction("DELETE FROM events on the production database")

	assert.True(t, profile.Production)
	assert.True(t, profile.Destructive)
	assert.True(t, profile.DML)
	assert.False(t, profile.DDL)
	assert.Contains(t, profile.Signals, "production_scope")
	assert.Contains(t, profile.Signals, "destructive_mutation")
}

func TestClassifyAction_CreateIndexOnProduction(t *testing.T) {
	profile := classifyAction("Create index on the production users table")

	assert.True(t, profile.Production)
	assert.True(t, profile.DDL)
	assert.False(t, profile.DML)
	assert.Contains(t, profile.Signals, "schema_change")
}

func TestAssessConstraintMatch_UsesTagsAndFreshness(t *testing.T) {
	memory := &types.Memory{
		ID:         "mem-1",
		Content:    "Production database changes require migration review and operator approval.",
		Tags:       []string{"constraint", "env:production", "action:ddl", "requires_approval"},
		Importance: 1,
		CreatedAt:  time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:  time.Now().UTC().Add(-24 * time.Hour),
	}

	profile := classifyAction("Create index on the production users table")
	match, ok := assessConstraintMatch(memory, profile.Text, profile, 180)

	require.True(t, ok)
	assert.Equal(t, "high", match.Severity)
	assert.True(t, match.RequiresApproval)
	assert.False(t, match.Stale)
	assert.Contains(t, match.MatchReasons, "constraint_tag")
	assert.Contains(t, match.MatchReasons, "production_boundary")
	assert.Contains(t, match.MatchReasons, "schema_change_policy")
	assert.Contains(t, match.MatchReasons, "action_scope_match")
	assert.Greater(t, match.MatchScore, 0.0)
}

func TestAssessConstraintMatch_FlagsStaleEvidence(t *testing.T) {
	old := time.Now().UTC().Add(-365 * 24 * time.Hour)
	memory := &types.Memory{
		ID:         "mem-2",
		Content:    "Deployment notes from January 2025 say the connection limit is 200.",
		Tags:       []string{"constraint", "env:production"},
		Importance: 1,
		CreatedAt:  old,
		UpdatedAt:  old,
	}

	profile := classifyAction("Check current production database connection settings")
	match, ok := assessConstraintMatch(memory, profile.Text, profile, 90)

	require.True(t, ok)
	assert.True(t, match.Stale)
	assert.Equal(t, "older_than_90_days", match.StaleReason)
}

func TestIsConstraintMemory_HighImportanceNeedsConstraintSignal(t *testing.T) {
	memory := &types.Memory{
		ID:         "mem-plain",
		Content:    "We renamed the package layout during the last refactor.",
		Importance: 1,
	}

	assert.False(t, isConstraintMemory(memory))
}

func TestIsConstraintMemory_HighImportanceScopedTagQualifies(t *testing.T) {
	memory := &types.Memory{
		ID:         "mem-scope",
		Content:    "Users table changes on production require review.",
		Tags:       []string{"env:production", "action:ddl"},
		Importance: 1,
	}

	assert.True(t, isConstraintMemory(memory))
}

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

func TestEvaluateVerification_DDLRequiresApprovalWithoutConstraint(t *testing.T) {
	profile := classifyAction("Create index on the production users table")
	result := evaluateVerification(profile.Text, profile, nil)

	assert.Equal(t, "require_approval", result.Decision)
	assert.Contains(t, result.Reasons, "production_schema_change_requires_review")
	assert.Contains(t, result.Reasons, "schema_change_without_matching_policy")
}

func TestEvaluateVerification_ForcePushBlocksWithoutConstraint(t *testing.T) {
	profile := classifyAction("git push --force origin main")
	result := evaluateVerification(profile.Text, profile, nil)

	assert.Equal(t, "block", result.Decision)
	assert.Contains(t, result.Reasons, "force_push_is_high_risk")
	assert.Contains(t, result.Reasons, "high_risk_action_without_matching_policy")
}
