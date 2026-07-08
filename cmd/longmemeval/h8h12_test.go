package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

type h8h12Capture struct {
	mu        sync.Mutex
	topKs     []int
	listCalls []int
	prompts   []string
	questions []string
	recalls   int
	fetches   int
}

func (c *h8h12Capture) addTopK(topK int, query string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.topKs = append(c.topKs, topK)
	c.questions = append(c.questions, query)
	c.recalls++
}

func (c *h8h12Capture) addListOffset(offset int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listCalls = append(c.listCalls, offset)
}

func (c *h8h12Capture) addPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = append(c.prompts, prompt)
}

func (c *h8h12Capture) addFetch() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fetches++
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

func (c *h8h12Capture) recallCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recalls
}

func (c *h8h12Capture) fetchCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fetches
}

func (c *h8h12Capture) fetchCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fetches
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
		"memory_list": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			offset, _ := args["offset"].(float64)
			limit, _ := args["limit"].(float64)
			if int(limit) != 500 {
				t.Fatalf("memory_list limit = %v, want 500", args["limit"])
			}
			capture.addListOffset(int(offset))
			if int(offset) > 0 {
				resp, _ := json.Marshal(map[string]any{
					"memories": []map[string]any{},
					"count":    0,
				})
				return &mcp.CallToolResult{
					Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
				}, nil
			}
			resp, _ := json.Marshal(map[string]any{
				"memories": []map[string]any{{
					"id":      "m1",
					"content": "Session date: 2024-05-10\nCalled my sister.",
					"project": "lme-r-" + item.QuestionID,
					"tags":    []string{"session:s-001"},
				}},
				"count": 1,
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
		"memory_fetch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			capture.addFetch()
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

func TestFullTimelineContext_UsesHaystackTimelineWithoutRecallOrFetch(t *testing.T) {
	// Question is phrased to also satisfy IsInferredPreferenceQuestion, so that
	// enabling --dual-preference-recall below actually exercises the gate
	// instead of being a no-op due to the question shape.
	item := longmemeval.Item{
		QuestionID:   "q-full-timeline",
		Question:     "What restaurant do I prefer for dinner and how did my travel plans change?",
		QuestionType: "single-session-user",
		QuestionDate: "2024-06-01",
		HaystackDates: []string{
			"2024-04-01",
			"2024-05-10",
		},
		HaystackSessions: [][]longmemeval.Turn{
			{
				{Role: "user", Content: "Booked the Seattle trip."},
				{Role: "assistant", Content: "Saved the itinerary."},
			},
			{
				{Role: "user", Content: "Canceled the Denver hotel."},
			},
		},
	}
	// Enable every recall-augmentation lever (retrieval-fusion, dual-preference
	// recall, paraphrase passes) alongside --full-timeline-context. None of them
	// may fire a single memory_recall/memory_fetch call: full-timeline mode must
	// stay a hard gate regardless of which other benchmark levers are toggled on.
	capture := runOneWithCapture(t, &Config{
		RecallTopK:            100,
		FullTimelineContext:   true,
		RetrievalFusion:       true,
		DualPreferenceRecall:  true,
		QueryParaphrasePasses: 3,
	}, item)

	if got := capture.recallCalls(); got != 0 {
		t.Fatalf("memory_recall calls = %d, want 0 in full timeline context mode", got)
	}
	if got := capture.fetchCalls(); got != 0 {
		t.Fatalf("memory_fetch calls = %d, want 0 in full timeline context mode", got)
	}

	want := longmemeval.GenerationPromptForType(item.Question, item.QuestionType, item.QuestionDate, []string{
		"Session date: 2024-04-01\nuser: Booked the Seattle trip.\nassistant: Saved the itinerary.",
		"Session date: 2024-05-10\nuser: Canceled the Denver hotel.",
	})
	if got := capture.lastPrompt(t); got != want {
		t.Fatalf("full timeline prompt mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFullTimelineContext_AtomModeAddsNoBlock verifies that --atom-mode is
// also gated by --full-timeline-context: no atom fetch happens (no request
// to the /atoms REST endpoint) and the generation prompt is exactly the
// full-timeline prompt with no atom preamble prepended.
func TestFullTimelineContext_AtomModeAddsNoBlock(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-full-timeline-atoms",
		Question:     "What travel plans changed?",
		QuestionType: "single-session-user",
		QuestionDate: "2024-06-01",
		HaystackDates: []string{
			"2024-04-01",
		},
		HaystackSessions: [][]longmemeval.Turn{
			{
				{Role: "user", Content: "Booked the Seattle trip."},
			},
		},
	}

	var atomsRequests int
	var mu sync.Mutex
	mcpServer := server.NewMCPServer("stub-engram", "1.0.0", server.WithToolCapabilities(true))
	streamable := server.NewStreamableHTTPServer(mcpServer)
	mux := http.NewServeMux()
	mux.HandleFunc("/atoms", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		atomsRequests++
		mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
	})
	mux.Handle("/", streamable)
	ts := httptest.NewServer(mux)
	defer ts.Close()

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
		var prompt string
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				prompt = msg.Content
				break
			}
		}
		if prompt == "" {
			t.Fatal("llm request missing user prompt")
		}
		if strings.Contains(prompt, "Extracted Preference Atoms") {
			t.Error("prompt contains atom context block in full-timeline-context mode")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"2"}}]}`)
	}))
	defer llmSrv.Close()

	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, ts.URL, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	cfg := &Config{
		RecallTopK:          100,
		FullTimelineContext: true,
		AtomMode:            true,
		LLMBaseURL:          llmSrv.URL,
		LLMModel:            "test",
		Retries:             0,
	}
	entry := runOne(ctx, cfg, c, item, longmemeval.IngestEntry{
		QuestionID: item.QuestionID,
		Project:    "lme-r-" + item.QuestionID,
	})
	if entry.Status != "done" {
		t.Fatalf("runOne status = %q error=%q, want done", entry.Status, entry.Error)
	}

	mu.Lock()
	got := atomsRequests
	mu.Unlock()
	if got != 0 {
		t.Fatalf("/atoms requests = %d, want 0 in full timeline context mode", got)
	}
}

func TestExhaustiveAggregation_PaginatesPast500AndUsesListedContent(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-h8-pagination",
		Question:     "How many times did I call my sister?",
		QuestionType: "multi-session",
		QuestionDate: "2024-06-01",
	}

	var capture h8h12Capture
	url := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			topK, _ := args["top_k"].(float64)
			query, _ := args["query"].(string)
			capture.addTopK(int(topK), query)
			resp, _ := json.Marshal(map[string]any{
				"results": []map[string]any{{"memory": map[string]any{"id": "m-000"}, "score": 0.9}},
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
		"memory_list": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			limit, _ := args["limit"].(float64)
			offset, _ := args["offset"].(float64)
			if int(limit) != 500 {
				t.Fatalf("memory_list limit = %v, want 500", args["limit"])
			}
			capture.addListOffset(int(offset))

			count := 500
			if int(offset) == 500 {
				count = 2
			}
			memories := make([]map[string]any, 0, count)
			for i := 0; i < count; i++ {
				id := fmt.Sprintf("m-%03d", int(offset)+i)
				content := fmt.Sprintf("Session date: 2024-05-10\nPage one memory %03d.", int(offset)+i)
				if int(offset) == 500 {
					content = fmt.Sprintf("Session date: 2024-05-11\nBoundary page memory %03d.", int(offset)+i)
				}
				memories = append(memories, map[string]any{
					"id":      id,
					"content": content,
					"project": "lme-r-" + item.QuestionID,
					"tags":    []string{fmt.Sprintf("session:s-%03d", int(offset)+i)},
				})
			}
			resp, _ := json.Marshal(map[string]any{
				"memories": memories,
				"count":    count,
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
		"memory_fetch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			capture.addFetch()
			resp, _ := json.Marshal(map[string]any{
				"memory": map[string]any{"content": "unexpected fetch fallback"},
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
		fmt.Fprint(w, `{"choices":[{"message":{"content":"502"}}]}`)
	}))
	defer llmSrv.Close()

	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	entry := runOne(ctx, &Config{
		LLMBaseURL:            llmSrv.URL,
		LLMModel:              "test",
		RecallTopK:            100,
		ExhaustiveAggregation: true,
		Retries:               0,
	}, c, item, longmemeval.IngestEntry{
		QuestionID: item.QuestionID,
		Project:    "lme-r-" + item.QuestionID,
	})
	if entry.Status != "done" {
		t.Fatalf("runOne status = %q error=%q, want done", entry.Status, entry.Error)
	}
	if got := capture.lastTopK(t); got != 500 {
		t.Fatalf("memory_recall top_k = %d, want 500", got)
	}
	if !strings.Contains(capture.lastPrompt(t), "Boundary page memory 500.") {
		t.Fatalf("generation prompt missing second-page memory:\n%s", capture.lastPrompt(t))
	}
	if !reflect.DeepEqual(capture.listCalls, []int{0, 500}) {
		t.Fatalf("memory_list offsets = %v, want [0 500]", capture.listCalls)
	}
	if capture.fetchCount() != 0 {
		t.Fatalf("memory_fetch calls = %d, want 0 when listed content is reused", capture.fetchCount())
	}
	if len(entry.RetrievedIDs) != 502 {
		t.Fatalf("retrieved IDs len = %d, want 502", len(entry.RetrievedIDs))
	}
}
