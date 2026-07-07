package search

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/stretchr/testify/require"
)

type droppedHitTemporalBackend struct {
	noopBackend
	ftsCalls atomic.Int64
}

func (b *droppedHitTemporalBackend) FTSSearch(_ context.Context, _, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	b.ftsCalls.Add(1)
	return []db.FTSResult{{
		Memory: nil,
		Score:  1,
	}}, nil
}

func TestRecallTwoPassTemporal_AccumulatesDroppedHitsFromBothPasses(t *testing.T) {
	ctx := context.Background()
	backend := &droppedHitTemporalBackend{}
	engine := New(ctx, backend, &constEmbedder{name: "const", dims: 4}, "test-temporal-dropped-hits",
		"http://ollama-test:11434", "", false, nil, 0)
	t.Cleanup(engine.Close)

	var droppedHits int
	results, err := engine.RecallWithOpts(ctx, "dentist appointment", 10, "summary", RecallOpts{
		TemporalWindowRecall: true,
		QuestionText:         "What did I schedule 3 days ago?",
		QuestionDate:         "2023/06/09 (Fri)",
		DroppedHits:          &droppedHits,
	})
	require.NoError(t, err)
	require.Empty(t, results, "nil-memory hits must never surface in recall results")
	require.Equal(t, 2, droppedHits, "DroppedHits must accumulate one dropped hit from each temporal recall pass")
	require.Equal(t, int64(2), backend.ftsCalls.Load(), "temporal two-pass recall must execute both FTS passes")
}
