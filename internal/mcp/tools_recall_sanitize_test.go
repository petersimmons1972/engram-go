package mcp

import (
	"math"
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
)

func TestSanitizeRecallResults_NonFiniteFloatsBecomeZero(t *testing.T) {
	patternNaN := math.NaN()
	dynInf := math.Inf(1)
	precNegInf := math.Inf(-1)

	results := []types.SearchResult{
		{
			Memory: &types.Memory{
				PatternConfidence:    &patternNaN,
				DynamicImportance:    &dynInf,
				RetrievalIntervalHrs: math.NaN(),
				RetrievalPrecision:   &precNegInf,
			},
			Score:      math.NaN(),
			ChunkScore: math.Inf(1),
			ScoreBreakdown: map[string]float64{
				"cosine":  math.NaN(),
				"bm25":    math.Inf(1),
				"recency": math.Inf(-1),
			},
			Connected: []types.ConnectedMemory{
				{
					Memory:   &types.Memory{RetrievalIntervalHrs: math.Inf(1)},
					Strength: math.NaN(),
				},
			},
		},
	}

	sanitizeRecallResults(results)

	r := results[0]
	if r.Score != 0 || r.ChunkScore != 0 {
		t.Fatalf("expected non-finite result scores to be zeroed, got score=%v chunk_score=%v", r.Score, r.ChunkScore)
	}
	if r.ScoreBreakdown["cosine"] != 0 || r.ScoreBreakdown["bm25"] != 0 || r.ScoreBreakdown["recency"] != 0 {
		t.Fatalf("expected non-finite score breakdown values to be zeroed, got %#v", r.ScoreBreakdown)
	}
	if *r.Memory.PatternConfidence != 0 || *r.Memory.DynamicImportance != 0 || r.Memory.RetrievalIntervalHrs != 0 || *r.Memory.RetrievalPrecision != 0 {
		t.Fatalf("expected memory float fields to be zeroed, got pattern=%v dynamic=%v interval=%v precision=%v",
			*r.Memory.PatternConfidence, *r.Memory.DynamicImportance, r.Memory.RetrievalIntervalHrs, *r.Memory.RetrievalPrecision)
	}
	if r.Connected[0].Strength != 0 || r.Connected[0].Memory.RetrievalIntervalHrs != 0 {
		t.Fatalf("expected connected float fields to be zeroed, got strength=%v interval=%v",
			r.Connected[0].Strength, r.Connected[0].Memory.RetrievalIntervalHrs)
	}
}

func TestSanitizeConflictingResults_NonFiniteStrengthBecomesZero(t *testing.T) {
	precNaN := math.NaN()
	conflicts := []types.ConflictingResult{
		{
			Strength: math.NaN(),
			Memory:   &types.Memory{RetrievalPrecision: &precNaN},
		},
	}

	sanitizeConflictingResults(conflicts)

	if conflicts[0].Strength != 0 {
		t.Fatalf("expected non-finite conflict strength to be zeroed, got %v", conflicts[0].Strength)
	}
	if *conflicts[0].Memory.RetrievalPrecision != 0 {
		t.Fatalf("expected non-finite memory retrieval_precision to be zeroed, got %v", *conflicts[0].Memory.RetrievalPrecision)
	}
}
