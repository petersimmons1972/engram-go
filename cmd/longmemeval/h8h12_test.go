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
	mu        sync.Mutex
	topKs     []int
	prompts   []string
	questions []string
}

func (c *h8h12Capture) addTopK(topK int, query string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.topKs = append(c.topKs, topK)
	c.questions = append(c.questions, query)
}

func (c *h8h12Capture) addPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = append(c.prompts, prompt)
}

func (c *h8h12Capture) lastTopK(t *testing.T) int {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.topKs) == 0 {
		t.Fatal("no topK captured")
	}
	return c.topKs[len(c.topKs)-1]
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

func runOneWithCapture(t *testing.T, cfg *Config, item longmemeval.Item) *h8h12Capture {
	t.Helper()

	var capture h8h12Capture
	url := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			topK, _ := args["top_k"].(float64)
			query, _ := args["query"].(string)
			capture.addTopK(int(topK), query)
			resp, _ := json.Marshal(map[string]any{
				"results": []map[string]any{{"memory": map[string]any{"id": "m1"}, "score": 0.9}},
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
		"memory_fetch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"memory": map[string]any{"content": "Session date: 2024-05-10\nCalled my sister."},
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
		foundUserPrompt := false
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				capture.addPrompt(msg.Content)
				foundUserPrompt = true
				break
			}
		}
		if !foundUserPrompt {
			t.Fatal("llm request missing user prompt")
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
	})
	if entry.Status != "done" {
		t.Fatalf("runOne status = %q error=%q, want done", entry.Status, entry.Error)
	}

	return &capture
}

func TestExhaustiveAggregation_DisabledIsBaseline(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-h8-disabled",
		Question:     "How many times did I call my sister?",
		QuestionType: "multi-session",
		QuestionDate: "2024-06-01",
	}
	capture := runOneWithCapture(t, &Config{RecallTopK: 100}, item)
	if got := capture.lastTopK(t); got != 100 {
		t.Fatalf("memory_recall top_k = %d, want 100", got)
	}
}

func TestExhaustiveAggregation_GateSkipsNonAgg(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-h8-noop",
		Question:     "When did I last call my sister?",
		QuestionType: "single-session-user",
		QuestionDate: "2024-06-01",
	}
	capture := runOneWithCapture(t, &Config{
		RecallTopK:            100,
		ExhaustiveAggregation: true,
	}, item)
	if got := capture.lastTopK(t); got != 100 {
		t.Fatalf("memory_recall top_k = %d, want 100", got)
	}
}

func TestExhaustiveAggregation_SetsTopK500(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-h8-enabled",
		Question:     "How many times did I call my sister?",
		QuestionType: "multi-session",
		QuestionDate: "2024-06-01",
	}
	capture := runOneWithCapture(t, &Config{
		RecallTopK:            100,
		ExhaustiveAggregation: true,
	}, item)
	if got := capture.lastTopK(t); got != 500 {
		t.Fatalf("memory_recall top_k = %d, want 500", got)
	}
}

func TestEnumerateFirst_DisabledIsBaseline(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-h12-disabled",
		Question:     "How many times did I call my sister?",
		QuestionType: "multi-session",
		QuestionDate: "2024-06-01",
	}
	capture := runOneWithCapture(t, &Config{RecallTopK: 100}, item)
	want := longmemeval.GenerationPromptForType(item.Question, item.QuestionType, item.QuestionDate, []string{
		"Session date: 2024-05-10\nCalled my sister.",
	})
	if got := capture.lastPrompt(t); got != want {
		t.Fatal("enumerate-first disabled should preserve the baseline generation prompt")
	}
}

func TestEnumerateFirst_PrefixPresent(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-h12-enabled",
		Question:     "How many times did I call my sister?",
		QuestionType: "multi-session",
		QuestionDate: "2024-06-01",
	}
	capture := runOneWithCapture(t, &Config{
		RecallTopK:     100,
		EnumerateFirst: true,
	}, item)
	if got := capture.lastPrompt(t); !strings.Contains(got, longmemeval.EnumerateFirstPrefix()) {
		t.Fatalf("prompt missing enumerate-first prefix:\n%s", got)
	}
}

func TestH8H12_CombinedFlags(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-h8h12",
		Question:     "How many times did I call my sister?",
		QuestionType: "multi-session",
		QuestionDate: "2024-06-01",
	}
	capture := runOneWithCapture(t, &Config{
		RecallTopK:            100,
		ExhaustiveAggregation: true,
		EnumerateFirst:        true,
	}, item)
	if got := capture.lastTopK(t); got != 500 {
		t.Fatalf("memory_recall top_k = %d, want 500", got)
	}
	if got := capture.lastPrompt(t); !strings.Contains(got, longmemeval.EnumerateFirstPrefix()) {
		t.Fatalf("combined flags prompt missing enumerate-first prefix:\n%s", got)
	}
}
