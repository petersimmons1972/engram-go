package consolidate_test

// LLM contradiction detection tests — TDD.
// These tests are written BEFORE the implementation and must fail (compile
// error or runtime) until classifyContradictionLLM and the LLM second-pass
// in DetectContradictions are implemented.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/consolidate"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── classifyContradictionLLM unit tests ─────────────────────────────────────

// TestClassifyContradictionLLM_YesResponse verifies that a mock Ollama server
// returning "YES" causes classifyContradictionLLM to return true.
func TestClassifyContradictionLLM_YesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(litellmReply("YES"))
	}))
	defer srv.Close()

	result, err := consolidate.ClassifyContradictionLLM(
		context.Background(),
		"The model is GPT-4",
		"The model is Claude",
		srv.URL,
		"llama3.2:3b",
	)
	require.NoError(t, err)
	assert.True(t, result, "YES response must return true")
}

// TestClassifyContradictionLLM_NoResponse verifies that "No, these are
// compatible" causes classifyContradictionLLM to return false.
func TestClassifyContradictionLLM_NoResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(litellmReply("No, these are compatible statements."))
	}))
	defer srv.Close()

	result, err := consolidate.ClassifyContradictionLLM(
		context.Background(),
		"Go uses goroutines for concurrency",
		"Go goroutines are lightweight threads",
		srv.URL,
		"llama3.2:3b",
	)
	require.NoError(t, err)
	assert.False(t, result, "NO response must return false")
}

// TestClassifyContradictionLLM_YesCaseInsensitive verifies that "yes" (lower
// case) is treated the same as "YES".
func TestClassifyContradictionLLM_YesCaseInsensitive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(litellmReply("yes, they contradict"))
	}))
	defer srv.Close()

	result, err := consolidate.ClassifyContradictionLLM(
		context.Background(),
		"The service is running",
		"The service is stopped",
		srv.URL,
		"llama3.2:3b",
	)
	require.NoError(t, err)
	assert.True(t, result, "yes (lowercase) must also return true")
}

// TestClassifyContradictionLLM_Timeout verifies that a mock server that delays
// longer than the context deadline causes the function to return an error.
func TestClassifyContradictionLLM_Timeout(t *testing.T) {
	// The handler uses a select so it exits promptly when either the server
	// shuts down or the client disconnects — avoiding a 5-second hang in
	// httptest.Server.Close while waiting for the active connection.
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-done:
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer func() {
		close(done)
		srv.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := consolidate.ClassifyContradictionLLM(
		ctx,
		"statement A",
		"statement B",
		srv.URL,
		"llama3.2:3b",
	)
	assert.Error(t, err, "a timed-out request must return an error")
}

// TestClassifyContradictionLLM_RequestBody verifies the request sent to LiteLLM
// contains the correct model, chat messages, and stream=false.
func TestClassifyContradictionLLM_RequestBody(t *testing.T) {
	var captured struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_ = json.NewEncoder(w).Encode(litellmReply("YES"))
	}))
	defer srv.Close()

	_, _ = consolidate.ClassifyContradictionLLM(
		context.Background(),
		"A is true",
		"A is false",
		srv.URL,
		"llama3.2:3b",
	)

	assert.Equal(t, "llama3.2:3b", captured.Model)
	assert.False(t, captured.Stream, "stream must be false for synchronous response")
	require.NotEmpty(t, captured.Messages, "request must include chat messages")
	prompt := captured.Messages[0].Content
	assert.Contains(t, prompt, "A is true", "prompt must include statement A")
	assert.Contains(t, prompt, "A is false", "prompt must include statement B")
}

// litellmReply builds an OpenAI/LiteLLM-shaped chat completion response with
// the given assistant content. Test handlers use this so that llm.Complete's
// JSON decode path matches what the consolidate package consumes.
func litellmReply(content string) map[string]any {
	return map[string]any{
		"choices": []map[string]any{
			{"message": map[string]string{"role": "assistant", "content": content}},
		},
	}
}

// ── LLM second-pass integration test ────────────────────────────────────────

// TestDetectContradictions_LLMPassCatchesAffirmative verifies that two
// memories making competing affirmative claims ("model is X" vs "model is Y")
// — which the heuristic misses — are caught when LLM detection is enabled
// and the mock Ollama responds YES.
func TestDetectContradictions_LLMPassCatchesAffirmative(t *testing.T) {
	// Integration test — requires TEST_DATABASE_URL.
	project := uniqueProject("consolidate-llm-affirmative")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	// Mock Ollama: always says YES (contradicts).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(litellmReply("YES"))
	}))
	defer srv.Close()

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 768})

	// Two affirmative claims — no negation, no versions, no tense shift.
	// The heuristic MUST miss these so the LLM pass is the only catch.
	m1 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "The current AI model is GPT-4",
		MemoryType:  types.MemoryTypeContext,
		Project:     project,
		Importance:  2,
		StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "The current AI model is Claude",
		MemoryType:  types.MemoryTypeContext,
		Project:     project,
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))

	// Identical embeddings → similarity = 1.0, pair is always examined.
	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = 0.5
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m1.ID, ChunkText: m1.Content, ChunkIndex: 0,
			ChunkHash: "llm-affirmative-1a", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m2.ID, ChunkText: m2.Content, ChunkIndex: 0,
			ChunkHash: "llm-affirmative-1b", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))

	// Confirm the heuristic alone misses this pair.
	assert.False(t,
		consolidate.IsContradiction(m1.Content, m2.Content),
		"heuristic must NOT flag competing affirmative claims (precondition for LLM test)",
	)

	stats, err := runner.RunAll(ctx, consolidate.RunOptions{
		InferRelationshipsMinSimilarity: 0.5,
		InferRelationshipsLimit:         100,
		LLMContradictionDetection:       true,
		LiteLLMURL:                      srv.URL,
		OllamaModel:                     "llama3.2:3b",
		LLMMaxCalls:                     10,
	})
	require.NoError(t, err)

	// The LLM pass must have caught the pair.
	detected, ok := stats["detected_contradictions"]
	require.True(t, ok, "stats must include detected_contradictions")
	assert.Equal(t, 1, detected, "LLM pass must catch exactly one contradicting pair")

	// Verify the edge type is actually contradicts.
	rels, err := backend.GetRelationships(ctx, project, m1.ID)
	require.NoError(t, err)
	foundContradicts := false
	for _, rel := range rels {
		if rel.RelType == types.RelTypeContradicts {
			foundContradicts = true
		}
	}
	assert.True(t, foundContradicts, "a contradicts edge must exist on m1 after LLM pass")
}

// TestDetectContradictions_LLMPassDisabled verifies that when
// LLMContradictionDetection=false the second pass is skipped and the
// competing affirmative pair is NOT flagged.
func TestDetectContradictions_LLMPassDisabled(t *testing.T) {
	project := uniqueProject("consolidate-llm-disabled")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	// Mock server that would say YES — but it must never be called.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(litellmReply("YES"))
	}))
	defer srv.Close()

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 768})

	m1 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "The current AI model is GPT-4",
		MemoryType:  types.MemoryTypeContext,
		Project:     project,
		Importance:  2,
		StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "The current AI model is Claude",
		MemoryType:  types.MemoryTypeContext,
		Project:     project,
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))

	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = 0.5
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m1.ID, ChunkText: m1.Content, ChunkIndex: 0,
			ChunkHash: "llm-disabled-1a", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m2.ID, ChunkText: m2.Content, ChunkIndex: 0,
			ChunkHash: "llm-disabled-1b", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))

	stats, err := runner.RunAll(ctx, consolidate.RunOptions{
		InferRelationshipsMinSimilarity: 0.5,
		InferRelationshipsLimit:         100,
		LLMContradictionDetection:       false, // disabled
		LiteLLMURL:                      srv.URL,
		OllamaModel:                     "llama3.2:3b",
	})
	require.NoError(t, err)

	detected := stats["detected_contradictions"]
	assert.Equal(t, 0, detected, "disabled LLM pass must detect zero contradictions for affirmative pairs")
	assert.False(t, called, "Ollama must not be called when LLMContradictionDetection=false")
}

// TestDetectContradictions_LLMMaxCalls verifies that the LLM second pass
// stops after LLMMaxCalls requests even when more uncaught pairs exist.
func TestDetectContradictions_LLMMaxCalls(t *testing.T) {
	project := uniqueProject("consolidate-llm-maxcalls")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		_ = json.NewEncoder(w).Encode(litellmReply("YES"))
	}))
	defer srv.Close()

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 768})

	// Store 5 memory pairs (10 memories total) that the heuristic won't catch.
	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = 0.5
	}

	for i := 0; i < 5; i++ {
		mA := &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     "The current AI model is GPT-4",
			MemoryType:  types.MemoryTypeContext,
			Project:     project,
			Importance:  2,
			StorageMode: "focused",
		}
		mB := &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     "The current AI model is Claude",
			MemoryType:  types.MemoryTypeContext,
			Project:     project,
			Importance:  2,
			StorageMode: "focused",
		}
		require.NoError(t, backend.StoreMemory(ctx, mA))
		require.NoError(t, backend.StoreMemory(ctx, mB))
		require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
			{ID: types.NewMemoryID(), MemoryID: mA.ID, ChunkText: mA.Content, ChunkIndex: 0,
				ChunkHash: types.NewMemoryID(), ChunkType: "sentence_window", Project: project, Embedding: vec},
		}))
		require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
			{ID: types.NewMemoryID(), MemoryID: mB.ID, ChunkText: mB.Content, ChunkIndex: 0,
				ChunkHash: types.NewMemoryID(), ChunkType: "sentence_window", Project: project, Embedding: vec},
		}))
	}

	_, err = runner.RunAll(ctx, consolidate.RunOptions{
		InferRelationshipsMinSimilarity: 0.5,
		InferRelationshipsLimit:         100,
		LLMContradictionDetection:       true,
		LiteLLMURL:                      srv.URL,
		OllamaModel:                     "llama3.2:3b",
		LLMMaxCalls:                     2, // cap at 2 regardless of pairs available
	})
	require.NoError(t, err)

	assert.LessOrEqual(t, callCount, 2, "LLM must not be called more than LLMMaxCalls times")
}
