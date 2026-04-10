package reembed_test

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/reembed"
)

type fakeEmbedder struct{ dims int }

func (f *fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, f.dims), nil
}
func (f *fakeEmbedder) Name() string    { return "fake" }
func (f *fakeEmbedder) Dimensions() int { return f.dims }

func TestWorker_StartsAndStops(t *testing.T) {
	w := reembed.NewWorker(nil, &fakeEmbedder{dims: 768}, "proj", false)
	w.Start()
	time.Sleep(20 * time.Millisecond)
	w.Stop()
	// reached here without hanging
}
