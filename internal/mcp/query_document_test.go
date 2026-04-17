package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func newQDStubClaude(t *testing.T, answer string) (*claude.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": answer}},
		})
	}))
	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL
	return c, srv
}

func TestExecQueryDocument_MissingProject(t *testing.T) {
	c, srv := newQDStubClaude(t, "ok")
	defer srv.Close()
	_, err := execQueryDocument(context.Background(), queryDocumentDeps{claudeClient: c},
		claude.DocumentQuery{MemoryID: "m1", Question: "q"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "project")
}

func TestExecQueryDocument_MissingMemoryID(t *testing.T) {
	c, srv := newQDStubClaude(t, "ok")
	defer srv.Close()
	_, err := execQueryDocument(context.Background(), queryDocumentDeps{claudeClient: c},
		claude.DocumentQuery{Project: "p", Question: "q"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "memory_id")
}

func TestExecQueryDocument_MissingQuestion(t *testing.T) {
	c, srv := newQDStubClaude(t, "ok")
	defer srv.Close()
	_, err := execQueryDocument(context.Background(), queryDocumentDeps{claudeClient: c},
		claude.DocumentQuery{Project: "p", MemoryID: "m1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "question")
}

func TestExecQueryDocument_MemoryNotFound(t *testing.T) {
	c, srv := newQDStubClaude(t, "ok")
	defer srv.Close()
	deps := queryDocumentDeps{
		claudeClient: c,
		getMemory: func(_ context.Context, _ string) (*types.Memory, error) {
			return nil, nil
		},
	}
	_, err := execQueryDocument(context.Background(), deps,
		claude.DocumentQuery{Project: "p", MemoryID: "missing", Question: "q"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestExecQueryDocument_Tier2Document(t *testing.T) {
	c, srv := newQDStubClaude(t, "Alpha failed")
	defer srv.Close()
	const docContent = "lots of prefix text\nerror: Alpha failed here\ntrailing tail"
	deps := queryDocumentDeps{
		claudeClient: c,
		getMemory: func(_ context.Context, id string) (*types.Memory, error) {
			return &types.Memory{ID: id, DocumentID: "doc-1"}, nil
		},
		getDocument: func(_ context.Context, _ string) (string, error) {
			return docContent, nil
		},
	}
	res, err := execQueryDocument(context.Background(), deps, claude.DocumentQuery{
		Project:     "p",
		MemoryID:    "m1",
		Question:    "what failed?",
		FilterSubs:  []string{"error:"},
		WindowChars: 40,
		TokenBudget: 4000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Spans)
	require.Equal(t, "Alpha failed", res.Answer)
}

func TestExecQueryDocument_Tier1FallbackToContent(t *testing.T) {
	c, srv := newQDStubClaude(t, "answered")
	defer srv.Close()
	deps := queryDocumentDeps{
		claudeClient: c,
		getMemory: func(_ context.Context, id string) (*types.Memory, error) {
			// No DocumentID — Tier-1. Content carries the body.
			return &types.Memory{ID: id, Content: "hello WORLD here"}, nil
		},
	}
	res, err := execQueryDocument(context.Background(), deps, claude.DocumentQuery{
		Project:     "p",
		MemoryID:    "m1",
		Question:    "?",
		FilterSubs:  []string{"WORLD"},
		WindowChars: 10,
		TokenBudget: 4000,
	})
	require.NoError(t, err)
	require.Len(t, res.Spans, 1)
	require.Contains(t, res.Spans[0].Text, "WORLD")
}

func TestExecQueryDocument_SemanticPath(t *testing.T) {
	c, srv := newQDStubClaude(t, "ok")
	defer srv.Close()
	deps := queryDocumentDeps{
		claudeClient: c,
		getMemory: func(_ context.Context, id string) (*types.Memory, error) {
			return &types.Memory{ID: id, Content: "ignored synopsis"}, nil
		},
		recallWithinMemory: func(_ context.Context, _, _ string, _ int, _ string) ([]*types.Memory, error) {
			return []*types.Memory{
				{ID: "m1", Content: "chunk one NEEDLE"},
				{ID: "m1", Content: "chunk two other"},
			}, nil
		},
	}
	res, err := execQueryDocument(context.Background(), deps, claude.DocumentQuery{
		Project:     "p",
		MemoryID:    "m1",
		Question:    "?",
		Semantic:    true,
		FilterSubs:  []string{"NEEDLE"},
		WindowChars: 30,
		TokenBudget: 4000,
	})
	require.NoError(t, err)
	require.Len(t, res.Spans, 1)
	require.Contains(t, res.Spans[0].Text, "NEEDLE")
}

func TestExecQueryDocument_NoClaudeClient(t *testing.T) {
	_, err := execQueryDocument(context.Background(), queryDocumentDeps{},
		claude.DocumentQuery{Project: "p", MemoryID: "m1", Question: "q"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Claude")
}

// ── handleMemoryQueryDocument: MCP entry-point tests ─────────────────────────
// These tests hit the MCP handler directly (not execQueryDocument) so the
// argument-parsing, claudeClient-presence, and early-validation paths in the
// outer handler get coverage. A real EnginePool is not needed because every
// error case returns before pool.Get is called.

func qdReq(args map[string]any) mcpgo.CallToolRequest {
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

func TestHandleMemoryQueryDocument_NoClaudeClient(t *testing.T) {
	_, err := handleMemoryQueryDocument(context.Background(), nil,
		qdReq(map[string]any{"project": "p", "memory_id": "m1", "question": "q"}),
		Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Claude")
}

func TestHandleMemoryQueryDocument_MissingProject(t *testing.T) {
	c, srv := newQDStubClaude(t, "ok")
	defer srv.Close()
	_, err := handleMemoryQueryDocument(context.Background(), nil,
		qdReq(map[string]any{"memory_id": "m1", "question": "q"}),
		Config{claudeClient: c})
	require.Error(t, err)
	require.Contains(t, err.Error(), "project")
}

func TestHandleMemoryQueryDocument_MissingMemoryID(t *testing.T) {
	c, srv := newQDStubClaude(t, "ok")
	defer srv.Close()
	_, err := handleMemoryQueryDocument(context.Background(), nil,
		qdReq(map[string]any{"project": "p", "question": "q"}),
		Config{claudeClient: c})
	require.Error(t, err)
	require.Contains(t, err.Error(), "memory_id")
}

func TestHandleMemoryQueryDocument_MissingQuestion(t *testing.T) {
	c, srv := newQDStubClaude(t, "ok")
	defer srv.Close()
	_, err := handleMemoryQueryDocument(context.Background(), nil,
		qdReq(map[string]any{"project": "p", "memory_id": "m1"}),
		Config{claudeClient: c})
	require.Error(t, err)
	require.Contains(t, err.Error(), "question")
}

// Exercises the filter-object parsing branch. Missing memory_id trips the
// validator but only after filter is parsed, so we get coverage on the
// nested-map decode path.
func TestHandleMemoryQueryDocument_FilterParsed(t *testing.T) {
	c, srv := newQDStubClaude(t, "ok")
	defer srv.Close()
	_, err := handleMemoryQueryDocument(context.Background(), nil,
		qdReq(map[string]any{
			"project":  "p",
			"question": "q",
			// memory_id intentionally omitted so the call short-circuits
			// before pool.Get — we only need the filter-parse lines covered.
			"filter": map[string]any{
				"regex":      "^err",
				"substrings": []any{"failed"},
			},
		}),
		Config{claudeClient: c})
	require.Error(t, err)
	require.Contains(t, err.Error(), "memory_id")
}
