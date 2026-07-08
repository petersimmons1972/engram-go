package layerb_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/types"
)

func memory(content string) types.SearchResult {
	return types.SearchResult{
		Memory: &types.Memory{ID: "m1", Content: content, Project: "p"},
	}
}

func TestDiagnoseBuildSummary_NotAggregation(t *testing.T) {
	d := layerb.DiagnoseBuildSummary("What is Alex's favorite color?", nil)
	if d.FailedGate != "not_aggregation" || d.WouldFire {
		t.Fatalf("Diagnosis = %+v, want not_aggregation", d)
	}
}

func TestDiagnoseBuildSummary_NoContributions(t *testing.T) {
	d := layerb.DiagnoseBuildSummary("How many times did I visit the doctor?", []types.SearchResult{
		memory("unrelated budget planning for March"),
	})
	if d.FailedGate != "no_contributions" {
		t.Fatalf("FailedGate = %q, want no_contributions", d.FailedGate)
	}
}

func TestDiagnoseBuildSummary_ConnectivityFailure(t *testing.T) {
	d := layerb.DiagnoseBuildSummary(
		"How many bikes did I service or plan to service in March?",
		[]types.SearchResult{
			memory("I serviced the bike rack in January."),
			memory("I plan the March budget next week."),
		},
	)
	if d.FailedGate != "connectivity" {
		t.Fatalf("FailedGate = %q, want connectivity; diag=%+v", d.FailedGate, d)
	}
	if d.ConnectedComponents < 2 {
		t.Fatalf("ConnectedComponents = %d, want >= 2", d.ConnectedComponents)
	}
}