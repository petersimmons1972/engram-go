package embed

// ModelSpec describes a known embedding model served via LiteLLM.
type ModelSpec struct {
	Name        string
	Dimensions  int
	MaxTokens   int    // context window limit; chunks must not exceed this
	SizeMB      int
	Description string
	Recommended bool // exactly one entry should be true
}

// defaultModelMaxTokens is the safe fallback for unknown models.
const defaultModelMaxTokens = 512

// ModelMaxTokens returns the context window limit for the named model.
// Falls back to defaultModelMaxTokens for unknown model names.
func ModelMaxTokens(name string) int {
	for _, m := range SuggestedModels {
		if m.Name == name {
			if m.MaxTokens > 0 {
				return m.MaxTokens
			}
		}
	}
	return defaultModelMaxTokens
}

// SuggestedModels is the curated list of embedding models available via LiteLLM.
// Set ENGRAM_EMBED_MODEL to switch. Run memory_embedding_eval to compare before
// migrating stored embeddings.
var SuggestedModels = []ModelSpec{
	{
		Name:        "qwen3-embedding:8b",
		Dimensions:  1536,
		MaxTokens:   8192,
		SizeMB:      5400,
		Description: "Current default. Best MTEB retrieval score available locally; 8192-token context window.",
		Recommended: true,
	},
	{
		Name:        "mxbai-embed-large",
		Dimensions:  1024,
		MaxTokens:   512,
		SizeMB:      669,
		Description: "Strong general-purpose baseline with 1024 dims.",
		Recommended: false,
	},
	{
		Name:        "bge-m3",
		Dimensions:  1024,
		MaxTokens:   512,
		SizeMB:      1200,
		Description: "Best multilingual option. Recommended when memories span multiple languages.",
		Recommended: false,
	},
	{
		Name:        "nomic-embed-text",
		Dimensions:  768,
		MaxTokens:   512,
		SizeMB:      274,
		Description: "Compact baseline; smallest footprint.",
		Recommended: false,
	},
}

// DefaultRecommendedModel returns the first ModelSpec with Recommended=true,
// or nil if none exists.
func DefaultRecommendedModel() *ModelSpec {
	for i := range SuggestedModels {
		if SuggestedModels[i].Recommended {
			return &SuggestedModels[i]
		}
	}
	return nil
}
