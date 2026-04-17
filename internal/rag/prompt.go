package rag

import (
	"fmt"
	"strings"

	"github.com/petersimmons1972/engram/internal/types"
)

const systemPrompt = `You are a memory assistant. Answer the question using only the provided memory excerpts. Cite your sources using [N] notation. If no excerpts are relevant, say so.`

// AssemblePrompt builds the user-facing prompt from the question and context chunks.
// Format:
//
//	Question: <question>
//	[1] (timestamp) excerpt_text
//	[2] (timestamp) excerpt_text
//	...
//
//	Answer based on the above excerpts.
func AssemblePrompt(question string, chunks []types.SearchResult) string {
	var sb strings.Builder

	sb.WriteString("Question: ")
	sb.WriteString(question)

	for i, chunk := range chunks {
		var ts string
		if chunk.Memory != nil {
			ts = chunk.Memory.CreatedAt.Format("2006-01-02T15:04:05Z")
		}
		sb.WriteString(fmt.Sprintf("\n[%d] (%s) %s", i+1, ts, chunk.MatchedChunk))
	}

	sb.WriteString("\n\nAnswer based on the above excerpts.")
	return sb.String()
}

// BuildCitations maps each SearchResult to a Citation with 1-based rank ordering.
func BuildCitations(chunks []types.SearchResult) []Citation {
	citations := make([]Citation, 0, len(chunks))
	for i, chunk := range chunks {
		c := Citation{
			Rank:    i + 1,
			Excerpt: chunk.MatchedChunk,
			Score:   chunk.Score,
		}
		if chunk.Memory != nil {
			c.MemoryID = chunk.Memory.ID
			c.Timestamp = chunk.Memory.CreatedAt
		}
		citations = append(citations, c)
	}
	return citations
}
