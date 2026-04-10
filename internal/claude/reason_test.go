package claude_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// makeMemory is a helper that constructs a minimal *types.Memory for testing.
func makeMemory(id, content string) *types.Memory {
	return &types.Memory{
		ID:      id,
		Content: content,
	}
}

// textResponse returns a standard Anthropic messages response with the given text.
func textResponse(text string) map[string]interface{} {
	return map[string]interface{}{
		"content": []map[string]string{
			{"type": "text", "text": text},
		},
	}
}

func TestReasonOverMemories_ReturnsAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(textResponse("The answer is 42"))
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	memories := []*types.Memory{makeMemory("mem-1", "some context")}
	result, err := c.ReasonOverMemories(context.Background(), "what is the answer?", memories)
	require.NoError(t, err)
	require.Equal(t, "The answer is 42", result)
}

func TestReasonOverMemories_TruncatesMemoryContent(t *testing.T) {
	// Build a content string longer than maxMemoryContentInReason (1000 chars).
	longContent := strings.Repeat("x", 2000)
	// The 1001st character is "x" at index 1000 — if it appears in the prompt,
	// truncation did not happen.  We check that the prompt body does NOT contain
	// the 1001st char by verifying the total content block is ≤ 1000 chars.
	// Because all chars are identical we instead check the request body length
	// for the substring that would only exist beyond the 1000-char cut.
	// Simpler: build a content where char[1001] is uniquely 'Z'.
	controlled := strings.Repeat("a", 1000) + "Z" + strings.Repeat("a", 999)

	var capturedBody struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(textResponse("truncated ok"))
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	_ = longContent // not used below — keep the controlled one
	memories := []*types.Memory{makeMemory("mem-trunc", controlled)}
	_, err = c.ReasonOverMemories(context.Background(), "test question", memories)
	require.NoError(t, err)

	// The unique 'Z' at position 1000 must NOT appear in the prompt.
	require.NotContains(t, capturedBody.Messages[0].Content, "Z",
		"memory content beyond 1000 chars must be truncated")
}

func TestReasonOverMemories_LimitsMemories(t *testing.T) {
	var capturedBody struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(textResponse("limited ok"))
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	// Create 30 memories.
	memories := make([]*types.Memory, 30)
	for i := 0; i < 30; i++ {
		memories[i] = makeMemory("mem-id-"+strings.Repeat("0", 2-len(string(rune('0'+i))))+string(rune('0'+i)), "content")
	}
	_, err = c.ReasonOverMemories(context.Background(), "test question", memories)
	require.NoError(t, err)

	// Count occurrences of "ID: " in the prompt — each memory contributes one.
	count := strings.Count(capturedBody.Messages[0].Content, "ID: ")
	require.LessOrEqual(t, count, 20,
		"at most 20 memories should appear in the prompt; got %d", count)
}

func TestReasonOverMemories_EmptyMemories(t *testing.T) {
	called := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(textResponse("no context answer"))
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	result, err := c.ReasonOverMemories(context.Background(), "what do you know?", []*types.Memory{})
	require.NoError(t, err)
	require.Equal(t, "no context answer", result)
	require.True(t, called, "API must be called even with an empty memory list")
}

func TestReasonOverMemories_AdvisorMaxUsesIsTwo(t *testing.T) {
	var capturedBody struct {
		Tools []struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Model   string `json:"model"`
			MaxUses int    `json:"max_uses"`
		} `json:"tools"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(textResponse("advisor check ok"))
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	memories := []*types.Memory{makeMemory("mem-a", "some data")}
	_, err = c.ReasonOverMemories(context.Background(), "advisor test", memories)
	require.NoError(t, err)

	require.Len(t, capturedBody.Tools, 1)
	require.Equal(t, 2, capturedBody.Tools[0].MaxUses,
		"advisor max_uses must be 2 for ReasonOverMemories")
}
