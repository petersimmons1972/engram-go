package rag

import "time"

// AskResult is the return value from Asker.Ask.
type AskResult struct {
	Answer            string
	Citations         []Citation
	ContextTokensUsed int
}

// Citation links an answer claim back to the source memory chunk.
type Citation struct {
	Rank      int
	MemoryID  string
	Excerpt   string
	Score     float64
	Timestamp time.Time
}
