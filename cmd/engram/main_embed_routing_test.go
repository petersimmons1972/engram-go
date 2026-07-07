package main

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/embed"
)

type constructedEmbedClient struct {
	baseURL string
	model   string
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

		live, reembed := newEmbedClients("http://litellm:4000", "http://litellm:4000", "bge-m3-live", reembedModelFromEnv(), "", 1024, embed.CircuitConfig{})

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

		_, reembed := newEmbedClients("http://litellm:4000", "http://litellm:4000", "bge-m3-live", reembedModelFromEnv(), "", 1024, embed.CircuitConfig{})

		if got := reembed.Name(); got != "custom-reembed" {
			t.Fatalf("reembed.Name() = %q, want %q", got, "custom-reembed")
		}
		if len(constructed) != 2 {
			t.Fatalf("constructed %d clients, want 2", len(constructed))
		}
	})
}

// TestEmbedEndpointsCoincide verifies the startup coincidence diagnostic that
// drives the WARN-vs-INFO branch (#1208): it must treat trailing-slash / case
// variants of the same host as coincident so a shared GPU is not mislabelled
// "separated", while genuinely distinct hosts read as separated.
func TestEmbedEndpointsCoincide(t *testing.T) {
	cases := []struct {
		name         string
		embedURL     string
		reembedURL   string
		wantCoincide bool
	}{
		{"identical", "http://mi50:8007", "http://mi50:8007", true},
		{"trailing slash", "http://mi50:8007", "http://mi50:8007/", true},
		{"scheme case", "HTTP://MI50:8007", "http://mi50:8007", true},
		{"distinct hosts", "http://mi50:8007", "http://w6800:8008", false},
		{"distinct ports same host", "http://mi50:8007", "http://mi50:8008", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := embedEndpointsCoincide(tc.embedURL, tc.reembedURL); got != tc.wantCoincide {
				t.Fatalf("embedEndpointsCoincide(%q, %q) = %v, want %v", tc.embedURL, tc.reembedURL, got, tc.wantCoincide)
			}
		})
	}
}

// TestReembedURLRouting verifies live/reembed GPU role-separation is enforceable
// from config (#1208): the live client is always constructed against the live
// embed URL, while the reembed client honours ENGRAM_REEMBED_URL and falls back
// to the live URL only when that override is unset.
func TestReembedURLRouting(t *testing.T) {
	orig := newLiteLLMEmbedClient
	t.Cleanup(func() { newLiteLLMEmbedClient = orig })

	var urls []string
	newLiteLLMEmbedClient = func(baseURL string, model string, _ string, _ int, _ embed.CircuitConfig) embed.Client {
		urls = append(urls, baseURL)
		return &constructedEmbedClient{baseURL: baseURL, model: model}
	}

	t.Run("reembed URL defaults to live URL when unset", func(t *testing.T) {
		t.Setenv("ENGRAM_REEMBED_URL", "")
		liveURL := "http://mi50:8007"
		reembedURL := reembedURLFromEnv(liveURL)
		if reembedURL != liveURL {
			t.Fatalf("reembedURLFromEnv(unset) = %q, want fallback %q", reembedURL, liveURL)
		}

		urls = nil
		live, reembed := newEmbedClients(liveURL, reembedURL, "bge-m3-live", "bge-m3-reembed", "", 1024, embed.CircuitConfig{})
		if got := live.(*constructedEmbedClient).baseURL; got != liveURL {
			t.Fatalf("live baseURL = %q, want %q", got, liveURL)
		}
		if got := reembed.(*constructedEmbedClient).baseURL; got != liveURL {
			t.Fatalf("reembed baseURL = %q, want %q (fallback to live)", got, liveURL)
		}
	})

	t.Run("reembed URL honours ENGRAM_REEMBED_URL override", func(t *testing.T) {
		liveURL := "http://mi50:8007"
		wantReembed := "http://w6800:8008"
		t.Setenv("ENGRAM_REEMBED_URL", wantReembed)
		reembedURL := reembedURLFromEnv(liveURL)
		if reembedURL != wantReembed {
			t.Fatalf("reembedURLFromEnv(override) = %q, want %q", reembedURL, wantReembed)
		}

		urls = nil
		live, reembed := newEmbedClients(liveURL, reembedURL, "bge-m3-live", "bge-m3-reembed", "", 1024, embed.CircuitConfig{})
		if got := live.(*constructedEmbedClient).baseURL; got != liveURL {
			t.Fatalf("live baseURL = %q, want %q (live must stay on live endpoint)", got, liveURL)
		}
		if got := reembed.(*constructedEmbedClient).baseURL; got != wantReembed {
			t.Fatalf("reembed baseURL = %q, want %q (reembed must route to override)", got, wantReembed)
		}
	})
}
