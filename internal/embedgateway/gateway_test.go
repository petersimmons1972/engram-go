package embedgateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/embedmodel"
)

type stubEmbedder struct {
	delay   time.Duration
	dims    int
	modelID string
}

func (s stubEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vec, _, err := s.EmbedWithModel(ctx, text)
	return vec, err
}

func (s stubEmbedder) EmbedWithModel(ctx context.Context, _ string) ([]float32, string, error) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	dims := s.dims
	if dims == 0 {
		dims = embedmodel.RequiredDims
	}
	modelID := s.modelID
	if modelID == "" {
		modelID = embedmodel.CanonicalBGEM3
	}
	return make([]float32, dims), modelID, nil
}

func (s stubEmbedder) Name() string    { return embedmodel.CanonicalBGEM3 }
func (s stubEmbedder) Dimensions() int { return embedmodel.RequiredDims }

func TestGateway_EnqueueWakesLoop(t *testing.T) {
	called := make(chan struct{}, 1)
	g := NewForTest(stubEmbedder{}, func(context.Context) (int, error) {
		called <- struct{}{}
		return 0, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g.Start(ctx)
	defer g.Stop()

	g.Enqueue([]string{"id1"})

	select {
	case <-called:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Enqueue did not wake drain loop within 100ms")
	}
}

func TestGateway_AdaptiveThrottle_BacksOffUnderLoad(t *testing.T) {
	throttle := NewAdaptiveThrottle(8)
	throttle.Update(ThrottleStats{AcquiredConns: 9, MaxConns: 10})
	if got := throttle.Concurrency(); got != 1 {
		t.Fatalf("Concurrency() = %d, want 1 under >80%% utilization", got)
	}
}

func TestGateway_AdaptiveThrottle_ScalesUpUnderLowLoad(t *testing.T) {
	throttle := NewAdaptiveThrottle(8)
	for range 10 {
		throttle.Update(ThrottleStats{AcquiredConns: 1, MaxConns: 10})
	}
	if got := throttle.Concurrency(); got != 8 {
		t.Fatalf("Concurrency() = %d, want max concurrency 8 under low utilization", got)
	}
}

func TestGateway_Stop_DrainsInFlight(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	g := NewForTest(stubEmbedder{}, func(ctx context.Context) (int, error) {
		once.Do(func() { close(started) })
		select {
		case <-release:
			return 1, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g.Start(ctx)
	g.Enqueue([]string{"id1"})

	select {
	case <-started:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("drain did not start")
	}

	done := make(chan struct{})
	go func() {
		g.Stop()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Stop returned before in-flight drain completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Stop did not return after in-flight drain completed")
	}
}

func TestGateway_DegradedHoldAfterValidationRejections(t *testing.T) {
	g := NewForTest(stubEmbedder{}, nil)
	g.holdFor = time.Hour

	for range consecutiveValidationThreshold {
		g.noteValidationRejected("nomic-embed-text", embedmodel.RequiredDims)
	}

	if _, held := g.inDegradedHold(time.Now()); !held {
		t.Fatal("gateway did not enter degraded hold after validation rejection threshold")
	}
}
