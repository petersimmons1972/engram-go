package rag

import "time"

// AskResult is the return value from Asker.Ask.
type AskResult struct {
	Answer            string     `json:"answer"`
	Citations         []Citation `json:"citations"`
	ContextTokensUsed int        `json:"context_tokens_used"`
}

// Citation links an answer claim back to the source memory chunk.
type Citation struct {
	Rank      int       `json:"rank"`
	MemoryID  string    `json:"memory_id"`
	Excerpt   string    `json:"excerpt"`
	Score     float64   `json:"score"`
	Timestamp time.Time `json:"timestamp"`
}
