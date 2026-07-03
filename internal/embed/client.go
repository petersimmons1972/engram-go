package embed

import "context"

// Client is the embedding provider interface. The live implementation is
// LiteLLMClient (see litellm.go); it is satisfied by any backend reachable
// through the Olla router.
type Client interface {
	// Embed returns a float32 vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)
	// EmbedWithModel returns a vector plus the model identifier reported by the embedding backend.
	EmbedWithModel(ctx context.Context, text string) ([]float32, string, error)
	// Name returns the model identifier (e.g. "nomic-embed-text").
	Name() string
	// Dimensions returns the vector size, or 0 before the first successful embed.
	Dimensions() int
}
