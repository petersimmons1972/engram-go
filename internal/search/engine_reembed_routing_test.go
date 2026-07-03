package search

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/embed"
)

type routingEmbedder struct {
	name string
	dims int
}

func (r *routingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, r.dims), nil
}

func (r *routingEmbedder) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	vec, err := r.Embed(ctx, text)
	return vec, r.name, err
}

func (r *routingEmbedder) Name() string    { return r.name }
func (r *routingEmbedder) Dimensions() int { return r.dims }

var _ embed.Client = (*routingEmbedder)(nil)

func TestPerProjectReembedUsesReembedModel(t *testing.T) {
	ctx := context.Background()
	live := &routingEmbedder{name: "bge-m3-live", dims: 1024}
	reembed := &routingEmbedder{name: "bge-m3-reembed", dims: 1024}

	eng := New(ctx, noopBackend{}, live, "routing-project", "http://litellm:4000", "llama3.2", false, nil, 0)
	t.Cleanup(func() { eng.Close() })

	eng.SetReembedEmbedder(reembed)

	if got := eng.ReembedEmbedder().Name(); got != reembed.Name() {
		t.Fatalf("ReembedEmbedder().Name() = %q, want %q", got, reembed.Name())
	}
}

func TestLiveEmbedUsesLiveModel(t *testing.T) {
	ctx := context.Background()
	live := &routingEmbedder{name: "bge-m3-live", dims: 1024}
	reembed := &routingEmbedder{name: "bge-m3-reembed", dims: 1024}

	eng := New(ctx, noopBackend{}, live, "routing-project", "http://litellm:4000", "llama3.2", false, nil, 0)
	t.Cleanup(func() { eng.Close() })

	eng.SetReembedEmbedder(reembed)

	if got := eng.Embedder().Name(); got != live.Name() {
		t.Fatalf("Embedder().Name() = %q, want %q", got, live.Name())
	}
}

func TestDefaultReembedEmbedderTracksLiveModel(t *testing.T) {
	ctx := context.Background()
	live := &routingEmbedder{name: "bge-m3-live", dims: 1024}

	eng := New(ctx, noopBackend{}, live, "routing-project", "http://litellm:4000", "llama3.2", false, nil, 0)
	t.Cleanup(func() { eng.Close() })

	if got := eng.ReembedEmbedder().Name(); got != live.Name() {
		t.Fatalf("ReembedEmbedder().Name() = %q, want %q", got, live.Name())
	}
}
