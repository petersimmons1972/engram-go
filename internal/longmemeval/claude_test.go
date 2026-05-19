package longmemeval_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestParseScoreLabel_Valid(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"CORRECT\nBecause X.", "CORRECT"},
		{"PARTIALLY_CORRECT\nSome details missing.", "PARTIALLY_CORRECT"},
		{"INCORRECT\nWrong answer.", "INCORRECT"},
		{"  correct  \nexplanation here", "CORRECT"},
	}
	for _, c := range cases {
		got, _ := longmemeval.ParseScoreLabel(c.input)
		if got != c.want {
			t.Errorf("ParseScoreLabel(%q) label = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestParseScoreLabel_Invalid(t *testing.T) {
	label, _ := longmemeval.ParseScoreLabel("I'm not sure about this one.")
	if label != "PARTIALLY_CORRECT" {
		t.Errorf("unrecognised label default = %q, want PARTIALLY_CORRECT", label)
	}
}

func TestParseScoreLabel_Explanation(t *testing.T) {
	_, explanation := longmemeval.ParseScoreLabel("CORRECT\nThe answer matches the reference exactly.")
	if !strings.Contains(explanation, "matches") {
		t.Errorf("explanation = %q, want it to contain 'matches'", explanation)
	}
}

func TestGenerateOAI_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Content-Type: %s", r.Header.Get("Content-Type"))
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"hello world"}}]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	out, err := longmemeval.GenerateOAI(ctx, "say hello", srv.URL, "test-model", 0)
	if err != nil {
		t.Fatalf("GenerateOAI: %v", err)
	}
	if out != "hello world" {
		t.Errorf("output = %q, want %q", out, "hello world")
	}
}

func TestGenerateOAI_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := longmemeval.GenerateOAI(ctx, "prompt", srv.URL, "test-model", 0)
	if err == nil {
		t.Error("expected error for empty choices, got nil")
	}
}

func TestGenerateOAI_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := longmemeval.GenerateOAI(ctx, "prompt", srv.URL, "test-model", 0)
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestGenerateOAI_TrimsWhitespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"  trimmed\n"}}]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	out, err := longmemeval.GenerateOAI(ctx, "prompt", srv.URL, "model", 0)
	if err != nil {
		t.Fatalf("GenerateOAI: %v", err)
	}
	if out != "trimmed" {
		t.Errorf("output = %q, want %q", out, "trimmed")
	}
}

func TestScoreOAI_ReturnsCorrect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"CORRECT\nMatches reference."}}]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	result, err := longmemeval.ScoreOAI(ctx, "What is X?", "X is 5", "X is 5", srv.URL, "test-model", 0)
	if err != nil {
		t.Fatalf("ScoreOAI: %v", err)
	}
	if result.Label != "CORRECT" {
		t.Errorf("label = %q, want CORRECT", result.Label)
	}
	if !strings.Contains(result.Explanation, "Matches") {
		t.Errorf("explanation = %q, want it to contain 'Matches'", result.Explanation)
	}
}

func TestScoreOAI_HTTPError_ReturnsPartiallyCorrect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx := context.Background()
	result, err := longmemeval.ScoreOAI(ctx, "Q", "A", "B", srv.URL, "model", 0)
	if err == nil {
		t.Error("expected error for HTTP 503, got nil")
	}
	if result.Label != "PARTIALLY_CORRECT" {
		t.Errorf("label = %q, want PARTIALLY_CORRECT as default on error", result.Label)
	}
}

func TestBuildScoringRequestBody(t *testing.T) {
	body, err := longmemeval.BuildScoringRequestBody("mymodel", "Q?", "A", "A")
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		MaxTokens   int     `json:"max_tokens"`
		Temperature float64 `json:"temperature"`
		Model       string  `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if req.MaxTokens != 100 {
		t.Errorf("want max_tokens=100 got %d", req.MaxTokens)
	}
	if req.Temperature != 0 {
		t.Errorf("want temperature=0 got %f", req.Temperature)
	}
	if req.Model != "mymodel" {
		t.Errorf("want mymodel got %s", req.Model)
	}
}

// --- ScoreBatch tests ---

// batchTestServer builds an httptest.Server that handles the three Anthropic
// batch endpoints. pollResponses is a slice of processing_status values
// returned on sequential GET /v1/messages/batches/{id} calls; the last value
// must be "ended". resultsNDJSON is the raw NDJSON to return from the results
// endpoint.
func batchTestServer(t *testing.T, pollResponses []string, resultsNDJSON string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var pollCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/batches":
			// Verify required headers.
			if r.Header.Get("x-api-key") == "" {
				t.Errorf("missing x-api-key header")
			}
			if r.Header.Get("anthropic-beta") != "message-batches-2024-09-24" {
				t.Errorf("missing/wrong anthropic-beta header: %s", r.Header.Get("anthropic-beta"))
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"batch_test123","processing_status":"in_progress"}`)

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/results"):
			w.Header().Set("Content-Type", "application/x-ndjson")
			fmt.Fprint(w, resultsNDJSON)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/messages/batches/"):
			idx := int(pollCount.Add(1)) - 1
			status := pollResponses[idx%len(pollResponses)]
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"id":"batch_test123","processing_status":"%s"}`, status)

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return srv, &pollCount
}

func TestScoreBatch_happyPath(t *testing.T) {
	ndjson := `{"custom_id":"q1","result":{"type":"succeeded","message":{"content":[{"type":"text","text":"CORRECT\nMatches exactly."}]}}}` + "\n" +
		`{"custom_id":"q2","result":{"type":"succeeded","message":{"content":[{"type":"text","text":"INCORRECT\nDoes not match."}]}}}` + "\n"

	srv, _ := batchTestServer(t, []string{"ended"}, ndjson)
	defer srv.Close()

	longmemeval.SetAnthropicBaseURL(srv.URL)
	defer longmemeval.SetAnthropicBaseURL("https://api.anthropic.com")

	items := []longmemeval.BatchScoringItem{
		{QuestionID: "q1", Question: "Q1?", ReferenceAnswer: "A1", Hypothesis: "A1"},
		{QuestionID: "q2", Question: "Q2?", ReferenceAnswer: "A2", Hypothesis: "wrong"},
	}
	ctx := context.Background()
	results, err := longmemeval.ScoreBatch(ctx, items, "test-key", "claude-haiku-4-5")
	if err != nil {
		t.Fatalf("ScoreBatch: %v", err)
	}
	if results["q1"].Label != "CORRECT" {
		t.Errorf("q1 label = %q, want CORRECT", results["q1"].Label)
	}
	if results["q2"].Label != "INCORRECT" {
		t.Errorf("q2 label = %q, want INCORRECT", results["q2"].Label)
	}
	if !strings.Contains(results["q1"].Explanation, "Matches") {
		t.Errorf("q1 explanation = %q, expected to contain 'Matches'", results["q1"].Explanation)
	}
}

func TestScoreBatch_pollsUntilEnded(t *testing.T) {
	ndjson := `{"custom_id":"q1","result":{"type":"succeeded","message":{"content":[{"type":"text","text":"CORRECT\nGood."}]}}}` + "\n"

	// First two polls return "in_progress", third returns "ended".
	srv, pollCount := batchTestServer(t, []string{"in_progress", "in_progress", "ended"}, ndjson)
	defer srv.Close()

	longmemeval.SetAnthropicBaseURL(srv.URL)
	defer longmemeval.SetAnthropicBaseURL("https://api.anthropic.com")

	items := []longmemeval.BatchScoringItem{
		{QuestionID: "q1", Question: "Q?", ReferenceAnswer: "A", Hypothesis: "A"},
	}
	ctx := context.Background()
	results, err := longmemeval.ScoreBatch(ctx, items, "test-key", "claude-haiku-4-5")
	if err != nil {
		t.Fatalf("ScoreBatch: %v", err)
	}
	if results["q1"].Label != "CORRECT" {
		t.Errorf("q1 label = %q, want CORRECT", results["q1"].Label)
	}
	// Expect exactly 3 poll calls (in_progress, in_progress, ended).
	if n := pollCount.Load(); n != 3 {
		t.Errorf("poll count = %d, want 3", n)
	}
}

func TestScoreBatch_handlesErroredItem(t *testing.T) {
	ndjson := `{"custom_id":"q1","result":{"type":"errored","error":{"type":"server_error","message":"timeout"}}}` + "\n"

	srv, _ := batchTestServer(t, []string{"ended"}, ndjson)
	defer srv.Close()

	longmemeval.SetAnthropicBaseURL(srv.URL)
	defer longmemeval.SetAnthropicBaseURL("https://api.anthropic.com")

	items := []longmemeval.BatchScoringItem{
		{QuestionID: "q1", Question: "Q?", ReferenceAnswer: "A", Hypothesis: "B"},
	}
	ctx := context.Background()
	results, err := longmemeval.ScoreBatch(ctx, items, "test-key", "claude-haiku-4-5")
	if err != nil {
		t.Fatalf("ScoreBatch returned error, want nil: %v", err)
	}
	if results["q1"].Label != "PARTIALLY_CORRECT" {
		t.Errorf("errored item label = %q, want PARTIALLY_CORRECT", results["q1"].Label)
	}
}

func TestScoreBatch_emptyAPIKey(t *testing.T) {
	ctx := context.Background()
	_, err := longmemeval.ScoreBatch(ctx, []longmemeval.BatchScoringItem{{QuestionID: "q1"}}, "", "model")
	if err == nil {
		t.Error("expected error for empty apiKey, got nil")
	}
}

func TestScoreBatch_emptyItems(t *testing.T) {
	ctx := context.Background()
	results, err := longmemeval.ScoreBatch(ctx, nil, "key", "model")
	if err != nil {
		t.Fatalf("ScoreBatch(nil items): %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty map for no items, got %d entries", len(results))
	}
}

// TestGenerate_RequiresClaude is skipped in short mode.
func TestGenerate_RequiresClaude(t *testing.T) {
	// #678: the claude binary is an undocumented prerequisite for this test.
	// In CI it is not in PATH; skip rather than fail. Locally (with claude
	// installed) the test still exercises the real code path.
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not in PATH — skipping (#678)")
	}
	if testing.Short() {
		t.Skip("short mode")
	}
	ctx := context.Background()
	out, err := longmemeval.Generate(ctx, "Reply with only the word: HELLO", 1)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out == "" {
		t.Error("Generate returned empty output")
	}
}
