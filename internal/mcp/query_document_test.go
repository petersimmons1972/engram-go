package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
