package embed

// ModelSpec describes a curated Ollama embedding model.
type ModelSpec struct {
	Name        string
	Dimensions  int
	MaxTokens   int    // context window limit; chunks must not exceed this
	SizeMB      int
	Description string
	Recommended bool // exactly one entry should be true
}

// defaultModelMaxTokens is the safe fallback for unknown models. All currently
// supported Ollama embedding models cap at 512 tokens.
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

// SuggestedModels is the curated list of Ollama embedding models recommended
// for engram-go 3.x. Users can pull any of these via `ollama pull <Name>` and
// then set ENGRAM_OLLAMA_MODEL to switch. Run memory_embedding_eval to compare
// before migrating stored embeddings.
var SuggestedModels = []ModelSpec{
	{
		Name:        "mxbai-embed-large",
		Dimensions:  1024,
		MaxTokens:   512,
		SizeMB:      669,
		Description: "Best MTEB retrieval score of locally-available Ollama models. Recommended upgrade from nomic-embed-text.",
		Recommended: true,
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
		Description: "Current default. Solid general-purpose baseline; smallest footprint.",
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
