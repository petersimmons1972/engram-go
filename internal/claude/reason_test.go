package claude_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

type reasonRT func(*http.Request) (*http.Response, error)

func (f reasonRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func reasonJSON(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func makeMemory(id, content string) *types.Memory { return &types.Memory{ID: id, Content: content} }

func textResponse(text string) map[string]interface{} {
	return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": text}}}
}

func testReasonClient(rt http.RoundTripper) *claude.Client {
	return claude.NewWithTransport("test-key", rt)
}

func TestReasonOverMemories_ReturnsAnswer(t *testing.T) {
	c := testReasonClient(reasonRT(func(*http.Request) (*http.Response, error) {
		payload, _ := json.Marshal(textResponse("The answer is 42"))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))

	memories := []*types.Memory{makeMemory("mem-1", "some context")}
	result, err := c.ReasonOverMemories(context.Background(), "what is the answer?", memories)
	require.NoError(t, err)
	require.Equal(t, "The answer is 42", result)
}

func TestReasonOverMemories_TruncatesMemoryContent(t *testing.T) {
	controlled := strings.Repeat("a", 1000) + "Z" + strings.Repeat("a", 999)
	var capturedBody struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	c := testReasonClient(reasonRT(func(r *http.Request) (*http.Response, error) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		payload, _ := json.Marshal(textResponse("truncated ok"))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))

	memories := []*types.Memory{makeMemory("mem-trunc", controlled)}
	_, err := c.ReasonOverMemories(context.Background(), "test question", memories)
	require.NoError(t, err)
	require.NotContains(t, capturedBody.Messages[0].Content, "Z")
}

func TestReasonOverMemories_LimitsMemories(t *testing.T) {
	var capturedBody struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	c := testReasonClient(reasonRT(func(r *http.Request) (*http.Response, error) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		payload, _ := json.Marshal(textResponse("limited ok"))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))

	memories := make([]*types.Memory, 30)
	for i := 0; i < 30; i++ {
		memories[i] = makeMemory(fmt.Sprintf("mem-id-%02d", i), "content")
	}
	_, err := c.ReasonOverMemories(context.Background(), "test question", memories)
	require.NoError(t, err)
	require.LessOrEqual(t, strings.Count(capturedBody.Messages[0].Content, "ID: "), 20)
}

func TestReasonOverMemories_EmptyMemories(t *testing.T) {
	called := false
	c := testReasonClient(reasonRT(func(r *http.Request) (*http.Response, error) {
		called = true
		payload, _ := json.Marshal(textResponse("no context answer"))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))

	result, err := c.ReasonOverMemories(context.Background(), "what do you know?", []*types.Memory{})
	require.NoError(t, err)
	require.Equal(t, "no context answer", result)
	require.True(t, called)
}

func TestReasonSystem_ContainsRejectionInstruction(t *testing.T) {
	prompt := claude.ReasonSystemPrompt()
	require.Contains(t, prompt, "explicitly name the rejected alternatives")
}

func TestReasonWithConflictAwareness_CallsAPI(t *testing.T) {
	var capturedBody struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	c := testReasonClient(reasonRT(func(r *http.Request) (*http.Response, error) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		payload, _ := json.Marshal(textResponse("conflict-aware answer"))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))

	ev := claude.EvidenceMap{
		Memories:   []*types.Memory{makeMemory("m1", "PostgreSQL uses MVCC"), makeMemory("m2", "PostgreSQL does not use MVCC")},
		Conflicts:  []claude.ConflictPair{{MemoryAID: "m1", MemoryBID: "m2", Strength: 0.9}},
		Confidence: 0.5,
	}
	result, err := c.ReasonWithConflictAwareness(context.Background(), "Does Postgres use MVCC?", ev)
	require.NoError(t, err)
	require.Equal(t, "conflict-aware answer", result)
	require.Contains(t, capturedBody.Messages[0].Content, "CONFLICT")
	require.Contains(t, capturedBody.Messages[0].Content, "CLAIM A")
	require.Contains(t, capturedBody.Messages[0].Content, "CLAIM B")
	require.Contains(t, capturedBody.Messages[0].Content, "MVCC")
}

func TestReasonWithConflictAwareness_TruncatesConflicts(t *testing.T) {
	var capturedBody struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	c := testReasonClient(reasonRT(func(r *http.Request) (*http.Response, error) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		payload, _ := json.Marshal(textResponse("truncated answer"))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))

	memories := make([]*types.Memory, 25)
	for i := range memories {
		memories[i] = makeMemory("mem-"+strings.Repeat("0", 3)+string(rune('A'+i)), "content")
	}
	ev := claude.EvidenceMap{
		Memories: memories,
		Conflicts: []claude.ConflictPair{
			{MemoryAID: memories[0].ID, MemoryBID: memories[1].ID, Strength: 0.8},
			{MemoryAID: memories[0].ID, MemoryBID: memories[22].ID, Strength: 0.7},
		},
		Confidence: 0.5,
	}
	_, err := c.ReasonWithConflictAwareness(context.Background(), "test", ev)
	require.NoError(t, err)
	require.Contains(t, capturedBody.Messages[0].Content, memories[0].ID)
	require.Contains(t, capturedBody.Messages[0].Content, memories[1].ID)
	require.NotContains(t, capturedBody.Messages[0].Content, memories[22].ID)
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
	c := testReasonClient(reasonRT(func(r *http.Request) (*http.Response, error) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		payload, _ := json.Marshal(textResponse("advisor check ok"))
		return reasonJSON(http.StatusOK, string(payload)), nil
	}))

	memories := []*types.Memory{makeMemory("mem-a", "some data")}
	_, err := c.ReasonOverMemories(context.Background(), "advisor test", memories)
	require.NoError(t, err)
	require.Len(t, capturedBody.Tools, 1)
	require.Equal(t, 2, capturedBody.Tools[0].MaxUses)
}
