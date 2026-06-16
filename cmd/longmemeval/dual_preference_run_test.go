package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestRunOne_DualPreferenceRecall_UsesInferredQuestionType(t *testing.T) {
	question := "Can you recommend a hotel for my upcoming trip to Miami?"
	subjectQuery := longmemeval.SubjectNPQuery(question)
	genericQuery := longmemeval.PreferenceRecallQuery(question)

	var (
		mu    sync.Mutex
		calls []string
	)
	url := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args, _ := req.Params.Arguments.(map[string]any)
			query, _ := args["query"].(string)
			mu.Lock()
			calls = append(calls, query)
			mu.Unlock()

			var handles []map[string]any
			switch query {
			case subjectQuery:
				handles = []map[string]any{
					{"id": "subject-only", "score": 0.7},
					{"id": "both", "score": 0.4},
				}
			case genericQuery:
				handles = []map[string]any{
					{"id": "generic-only", "score": 0.8},
					{"id": "both", "score": 0.9},
				}
			default:
				t.Fatalf("unexpected recall query %q", query)
			}

			resp, _ := json.Marshal(map[string]any{"handles": handles})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
		"memory_fetch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args, _ := req.Params.Arguments.(map[string]any)
			id, _ := args["id"].(string)
			resp, _ := json.Marshal(map[string]any{
				"memory": map[string]any{"content": "content for " + id},
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
	})

	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Miami hotel"}}]}`)
	}))
	defer llmSrv.Close()

	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	item := longmemeval.Item{
		QuestionID:   "q-pref-gate",
		QuestionType: "single-session-user",
		Question:     question,
		QuestionDate: "2024-01-15",
	}
	ingest := longmemeval.IngestEntry{QuestionID: "q-pref-gate", Project: "lme-r-q-pref-gate"}
	cfg := &Config{
		DualPreferenceRecall: true,
		LLMBaseURL:           llmSrv.URL,
		LLMModel:             "test",
		Retries:              0,
	}

	entry := runOne(ctx, cfg, c, item, ingest)
	if entry.Status != "done" {
		t.Fatalf("runOne status = %q error=%q, want done", entry.Status, entry.Error)
	}

	mu.Lock()
	gotCalls := append([]string(nil), calls...)
	mu.Unlock()
	wantCalls := []string{subjectQuery, genericQuery}
	if !reflect.DeepEqual(gotCalls, wantCalls) {
		t.Fatalf("memory_recall calls = %v, want %v", gotCalls, wantCalls)
	}

	wantIDs := []string{"both", "generic-only", "subject-only"}
	if !reflect.DeepEqual(entry.RetrievedIDs, wantIDs) {
		t.Fatalf("RetrievedIDs = %v, want %v", entry.RetrievedIDs, wantIDs)
	}
}
