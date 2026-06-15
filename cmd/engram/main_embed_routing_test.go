package main

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/embed"
)

type constructedEmbedClient struct {
	model string
}

func (c *constructedEmbedClient) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{1, 2, 3}, nil
}

func (c *constructedEmbedClient) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	vec, err := c.Embed(ctx, text)
	return vec, c.model, err
}

func (c *constructedEmbedClient) Name() string    { return c.model }
func (c *constructedEmbedClient) Dimensions() int { return 3 }

func TestReembedModelEnvDefaultAndOverride(t *testing.T) {
	orig := newLiteLLMEmbedClient
	t.Cleanup(func() { newLiteLLMEmbedClient = orig })

	var constructed []string
	newLiteLLMEmbedClient = func(_ string, model string, _ string, _ int, _ embed.CircuitConfig) embed.Client {
		constructed = append(constructed, model)
		return &constructedEmbedClient{model: model}
	}

	t.Run("default", func(t *testing.T) {
		constructed = nil
		t.Setenv("ENGRAM_REEMBED_MODEL", "")

		live, reembed := newEmbedClients("http://litellm:4000", "bge-m3-live", reembedModelFromEnv(), "", 1024, embed.CircuitConfig{})

		if got := live.Name(); got != "bge-m3-live" {
			t.Fatalf("live.Name() = %q, want %q", got, "bge-m3-live")
		}
		if got := reembed.Name(); got != "bge-m3-reembed" {
			t.Fatalf("reembed.Name() = %q, want %q", got, "bge-m3-reembed")
		}
		if live == reembed {
			t.Fatal("expected distinct live and reembed clients")
		}
		if len(constructed) != 2 {
			t.Fatalf("constructed %d clients, want 2", len(constructed))
		}
	})

	t.Run("override", func(t *testing.T) {
		constructed = nil
		t.Setenv("ENGRAM_REEMBED_MODEL", "custom-reembed")

		_, reembed := newEmbedClients("http://litellm:4000", "bge-m3-live", reembedModelFromEnv(), "", 1024, embed.CircuitConfig{})

		if got := reembed.Name(); got != "custom-reembed" {
			t.Fatalf("reembed.Name() = %q, want %q", got, "custom-reembed")
		}
		if len(constructed) != 2 {
			t.Fatalf("constructed %d clients, want 2", len(constructed))
		}
	})
}
