// export_test.go exposes internal symbols needed by mcp_test for integration
// testing. File is compiled only during `go test`.
package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
)

// fakeTestEmbedClient is a zero-dependency embedder for tests. It returns a
// deterministic constant vector so vector-search ranking is predictable.
type fakeTestEmbedClient struct{ dims int }

func (f *fakeTestEmbedClient) Embed(_ context.Context, _ string) ([]float32, error) {
	vec := make([]float32, f.dims)
	for i := range vec {
		vec[i] = float32(i) / float32(f.dims)
	}
	return vec, nil
}
func (f *fakeTestEmbedClient) Name() string    { return "fake" }
func (f *fakeTestEmbedClient) Dimensions() int { return f.dims }

var _ embed.Client = (*fakeTestEmbedClient)(nil)

// NewTestPoolWithDSN creates an EnginePool backed by a real PostgreSQL database
// for integration tests. The returned pool uses a fake embedder so tests do not
// require a live Ollama instance.
func NewTestPoolWithDSN(t *testing.T, ctx context.Context, dsn, project string) *EnginePool {
	t.Helper()
	embedder := &fakeTestEmbedClient{dims: 768}
	factory := func(factoryCtx context.Context, proj string) (*EngineHandle, error) {
		backend, err := db.NewPostgresBackend(factoryCtx, proj, dsn)
		if err != nil {
			return nil, err
		}
		engine := search.New(factoryCtx, backend, embedder, proj,
			"http://ollama:11434", "llama3.2", false, nil, 0)
		return &EngineHandle{Engine: engine}, nil
	}
	pool := NewEnginePool(factory)
	t.Cleanup(func() {
		h, err := pool.Get(ctx, project)
		if err == nil && h != nil && h.Engine != nil {
			h.Engine.Close()
		}
	})
	return pool
}

// CallHandleMemoryRecall invokes handleMemoryRecall for tests and returns
// the decoded output map. It bridges between the mcp_test package and the
// unexported handleMemoryRecall function.
//
// includeConflicts controls whether the include_conflicts parameter is set.
func CallHandleMemoryRecall(
	ctx context.Context,
	t *testing.T,
	pool *EnginePool,
	project, query string,
	includeConflicts bool,
) map[string]any {
	t.Helper()

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":           project,
		"query":             query,
		"top_k":             float64(10),
		"detail":            "full",
		"include_conflicts": includeConflicts,
	}

	result, err := handleMemoryRecall(ctx, pool, req, Config{})
	if err != nil {
		t.Fatalf("handleMemoryRecall: %v", err)
	}

	// The tool encodes output as JSON in the first TextContent item.
	if len(result.Content) == 0 {
		t.Fatal("tool result has no content items")
	}
	tc, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("decode tool result JSON: %v", err)
	}

	// Re-hydrate conflicting_results to []types.ConflictingResult if present,
	// so callers can do typed assertions.
	if raw, ok := out["conflicting_results"]; ok {
		b, _ := json.Marshal(raw)
		var cr []types.ConflictingResult
		if err := json.Unmarshal(b, &cr); err == nil {
			out["conflicting_results"] = cr
		}
	}
	return out
}
