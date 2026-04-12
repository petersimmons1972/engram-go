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

// TestReasonSystem_ContainsRejectionInstruction guards against silent regression
// of the reasonSystem constant. The instruction to name rejected alternatives
// is load-bearing behavior for noise resistance — it must always be present.
func TestReasonSystem_ContainsRejectionInstruction(t *testing.T) {
	// ReasonSystemPrompt exposes the private constant for assertion.
	prompt := claude.ReasonSystemPrompt()
	require.Contains(t, prompt, "explicitly name the rejected alternatives",
		"reasonSystem must instruct Claude to name rejected alternatives when conflicts exist")
}

// TestReasonWithConflictAwareness_CallsAPI verifies the conflict-aware path
// hits the Claude API with a prompt containing conflict annotations.
func TestReasonWithConflictAwareness_CallsAPI(t *testing.T) {
	var capturedBody struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(textResponse("conflict-aware answer"))
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	ev := claude.EvidenceMap{
		Memories: []*types.Memory{
			makeMemory("m1", "PostgreSQL uses MVCC"),
			makeMemory("m2", "PostgreSQL does not use MVCC"),
		},
		Conflicts: []claude.ConflictPair{
			{MemoryAID: "m1", MemoryBID: "m2", Strength: 0.9},
		},
		Confidence: 0.5,
	}
	result, err := c.ReasonWithConflictAwareness(context.Background(), "Does Postgres use MVCC?", ev)
	require.NoError(t, err)
	require.Equal(t, "conflict-aware answer", result)

	// Prompt must contain conflict annotations with content excerpts.
	prompt := capturedBody.Messages[0].Content
	require.Contains(t, prompt, "CONFLICT")
	require.Contains(t, prompt, "CLAIM A")
	require.Contains(t, prompt, "CLAIM B")
	require.Contains(t, prompt, "MVCC")
}

// TestReasonWithConflictAwareness_TruncatesConflicts verifies that when memories
// exceed the cap (20), conflicts referencing truncated memories are filtered out.
func TestReasonWithConflictAwareness_TruncatesConflicts(t *testing.T) {
	var capturedBody struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(textResponse("truncated answer"))
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	// Create 25 memories — only first 20 will survive the cap.
	memories := make([]*types.Memory, 25)
	for i := range memories {
		memories[i] = makeMemory("mem-"+strings.Repeat("0", 3)+string(rune('A'+i)), "content")
	}

	ev := claude.EvidenceMap{
		Memories: memories,
		Conflicts: []claude.ConflictPair{
			// This conflict is between memories 0 and 1 — both within cap.
			{MemoryAID: memories[0].ID, MemoryBID: memories[1].ID, Strength: 0.8},
			// This conflict references memory 22 — beyond the 20 cap.
			{MemoryAID: memories[0].ID, MemoryBID: memories[22].ID, Strength: 0.7},
		},
		Confidence: 0.5,
	}
	_, err = c.ReasonWithConflictAwareness(context.Background(), "test", ev)
	require.NoError(t, err)

	prompt := capturedBody.Messages[0].Content
	// The in-cap conflict should appear.
	require.Contains(t, prompt, memories[0].ID)
	require.Contains(t, prompt, memories[1].ID)
	// The out-of-cap memory should NOT appear in the conflict section.
	require.NotContains(t, prompt, memories[22].ID,
		"conflict referencing a memory beyond the 20-cap must be filtered out")
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
