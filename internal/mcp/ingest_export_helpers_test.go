package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
)

// IngestExportResult is the parsed JSON result of memory_ingest_export.
type IngestExportResult struct {
	Format         string   `json:"format"`
	MemoriesStored int      `json:"memories_stored"`
	MemoryIDs      []string `json:"memory_ids"`
}

// newToolRequest builds a mcpgo.CallToolRequest with the given arguments.
func newToolRequest(args map[string]any) mcpgo.CallToolRequest {
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// extractTextContent returns the text body of the first TextContent item in
// a tool result. Returns an empty string if there is no content or the first
// item is not a TextContent.
func extractTextContent(result *mcpgo.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	tc, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}

// storeableNoopBackend embeds noopBackend but overrides Begin to return a
// real (no-op) Tx so SearchEngine.StoreWithRawBody can complete without
// panicking on tx.Commit. Uses the noopTx already defined in simple_tools_test.go.
type storeableNoopBackend struct{ noopBackend }

func (storeableNoopBackend) Begin(_ context.Context) (db.Tx, error) {
	return noopTx{}, nil
}

// storeableNoopEmbedder is a deterministic embedder for store tests.
type storeableNoopEmbedder struct{}

func (storeableNoopEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, 384), nil
}
func (storeableNoopEmbedder) Name() string    { return "noop-store" }
func (storeableNoopEmbedder) Dimensions() int { return 384 }

var _ embed.Client = storeableNoopEmbedder{}

// NewTestStorePool builds an EnginePool where the backing engine can complete
// StoreWithRawBody calls without a live database or Ollama instance.
// Exported so mcp_test package tests can use it directly.
func NewTestStorePool(t *testing.T) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, storeableNoopBackend{}, storeableNoopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// CallHandleMemoryIngestExport is a test helper that invokes handleMemoryIngestExport.
func CallHandleMemoryIngestExport(
	ctx context.Context,
	t *testing.T,
	pool *EnginePool,
	project string,
	cfg Config,
	path string,
) (IngestExportResult, error) {
	t.Helper()
	req := newToolRequest(map[string]any{
		"path":    path,
		"project": project,
	})
	result, err := handleMemoryIngestExport(ctx, pool, req, cfg)
	if err != nil {
		return IngestExportResult{}, err
	}
	if result.IsError {
		return IngestExportResult{}, fmt.Errorf("tool error: %s", extractTextContent(result))
	}
	var out IngestExportResult
	json.Unmarshal([]byte(extractTextContent(result)), &out)
	return out, nil
}
