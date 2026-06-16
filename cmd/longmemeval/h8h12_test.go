package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

type h8h12Capture struct {
	mu      sync.Mutex
	topKs   []int
	prompts []string
}

func (c *h8h12Capture) addTopK(topK int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.topKs = append(c.topKs, topK)
}

func (c *h8h12Capture) addPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = append(c.prompts, prompt)
}

func (c *h8h12Capture) gotTopKs() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]int, len(c.topKs))
	copy(out, c.topKs)
	return out
}

func (c *h8h12Capture) lastPrompt(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.prompts) == 0 {
		t.Fatal("no prompt captured")
	}
	return c.prompts[len(c.prompts)-1]
}

func runOneWithCapture(t *testing.T, cfg *Config, item longmemeval.Item, recallIDs []string) *h8h12Capture {
	t.Helper()

	capture := &h8h12Capture{}
	url := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			topK, _ := args["top_k"].(float64)
			capture.addTopK(int(topK))
			results := make([]map[string]any, 0, len(recallIDs))
			for _, id := range recallIDs {
				results = append(results, map[string]any{
					"memory": map[string]any{"id": id},
					"score":  0.9,
				})
			}
			resp, _ := json.Marshal(map[string]any{"results": results})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
		"memory_fetch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			id, _ := args["id"].(string)
			resp, _ := json.Marshal(map[string]any{
				"memory": map[string]any{"content": fmt.Sprintf("Session date: 2024-05-10\nMemory %s", id)},
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
	})
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode llm request: %v", err)
		}
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				capture.addPrompt(msg.Content)
				break
			}
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"2"}}]}`)
	}))
	defer llmSrv.Close()

	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	cfgCopy := *cfg
	cfgCopy.LLMBaseURL = llmSrv.URL
	cfgCopy.LLMModel = "test"
	cfgCopy.Retries = 0
	if cfgCopy.RecallTopK == 0 {
		cfgCopy.RecallTopK = 100
	}

	entry := runOne(ctx, &cfgCopy, c, item, longmemeval.IngestEntry{
		QuestionID: item.QuestionID,
		Project:    "lme-r-" + item.QuestionID,
		Status:     "done",
	})
	if entry.Status != "done" {
		t.Fatalf("runOne status = %q error=%q, want done", entry.Status, entry.Error)
	}

	return capture
}

func TestExhaustiveAggregation_UsesSingleTopK500Recall(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-h8-enabled",
		Question:     "How many times did I call my sister?",
		QuestionType: "multi-session",
		QuestionDate: "2024-06-01",
	}
	capture := runOneWithCapture(t, &Config{
		RecallTopK:            100,
		ExhaustiveAggregation: true,
	}, item, []string{"m1", "m2"})

	got := capture.gotTopKs()
	if len(got) != 1 || got[0] != 500 {
		t.Fatalf("memory_recall topKs = %v, want [500]", got)
	}
}

func TestExhaustiveAggregation_UsesFullContextSweep(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-h8-context",
		Question:     "How many times did I call my sister?",
		QuestionType: "multi-session",
		QuestionDate: "2024-06-01",
	}
	recallIDs := make([]string, 0, 20)
	for i := 1; i <= 20; i++ {
		recallIDs = append(recallIDs, fmt.Sprintf("m%02d", i))
	}
	capture := runOneWithCapture(t, &Config{
		RecallTopK:            100,
		ExhaustiveAggregation: true,
	}, item, recallIDs)

	prompt := capture.lastPrompt(t)
	if !strings.Contains(prompt, "Memory m20") {
		t.Fatalf("full-context aggregation sweep should include the tail of the recall set in the prompt:\n%s", prompt)
	}
}
