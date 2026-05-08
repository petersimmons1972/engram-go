package embed

// ModelSpec describes a known embedding model served via LiteLLM.
type ModelSpec struct {
	Name        string
	Dimensions  int
	MaxTokens   int // context window limit; chunks must not exceed this
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
//
// SINGLE SOURCE OF TRUTH: The entry with Recommended=true is the official embedding
// model for this deployment. docker-compose.yml fallback defaults and .env must match
// this entry. When changing the recommended model, update all three in one commit.
var SuggestedModels = []ModelSpec{
	{
		Name:        "jinaai/jina-embeddings-v5-text-small",
		Dimensions:  1024,
		MaxTokens:   8192,
		SizeMB:      560,
		Description: "Official embedding model. Jina v5-text-small via vLLM on oblivion (GB10, 128 GB unified). 118 emb/s vs ~12 emb/s GGUF. Sole embedding endpoint as of 2026-05-08.",
		Recommended: true,
	},
	{
		Name:        "diqiuzhuanzhuan/jina-embeddings-v4-text-retrieval-Q8_0.gguf:latest",
		Dimensions:  1024, // Matryoshka truncation from native 2048; set ENGRAM_EMBED_DIMENSIONS=1024
		MaxTokens:   8192,
		SizeMB:      9200,
		Description: "Legacy. Jina v4 Q8 GGUF. Retained on engram-ollama only; all other nodes migrated to v5.",
		Recommended: false,
	},
	{
		Name:        "qwen3-embedding:8b",
		Dimensions:  1536,
		MaxTokens:   8192,
		SizeMB:      5400,
		Description: "High-quality alternative. Best MTEB retrieval score; 8192-token context window.",
		Recommended: false,
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
