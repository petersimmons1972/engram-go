package ollama

import "encoding/json"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model     string          `json:"model"`
	Messages  []Message       `json:"messages"`
	Stream    bool            `json:"stream"`
	Format    json.RawMessage `json:"format,omitempty"`
	Options   map[string]any  `json:"options,omitempty"`
	KeepAlive *int            `json:"keep_alive,omitempty"`
}

type ChatResponse struct {
	Model   string `json:"model"`
	Message struct {
		Role     string `json:"role"`
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	Done bool `json:"done"`
}

type TagsResponse struct {
	Models []struct {
		Name   string `json:"name"`
		Digest string `json:"digest"`
	} `json:"models"`
}

type ShowResponse struct {
	Modelfile string `json:"modelfile"`
	Details   struct {
		Family string `json:"family"`
	} `json:"details"`
	ModelInfo map[string]any `json:"model_info"`
}

type VersionResponse struct {
	Version string `json:"version"`
}
