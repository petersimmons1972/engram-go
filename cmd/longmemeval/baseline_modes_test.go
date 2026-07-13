package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

type baselineGeneratorCapture struct {
	calls  int
	prompt string
}

type baselineRoundTripFunc func(*http.Request) (*http.Response, error)

func (f baselineRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func runBaselineWithCapture(
	t *testing.T,
	cfg *Config,
	item longmemeval.Item,
) (longmemeval.RunEntry, baselineGeneratorCapture) {
	t.Helper()
	var capture baselineGeneratorCapture
	entry := runOneWithGenerator(
		context.Background(),
		cfg,
		nil,
		item,
		longmemeval.IngestEntry{QuestionID: item.QuestionID},
		func(_ context.Context, _ *Config, prompt string) (string, error) {
			capture.calls++
			capture.prompt = prompt
			return "generated answer", nil
		},
	)
	return entry, capture
}

func TestClosedBook_GeneratesWithEmptyContextAndNoRecall(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-closed-book",
		Question:     "Where did I book dinner?",
		QuestionType: "single-session-user",
		QuestionDate: "2024-06-01",
	}
	entry, capture := runBaselineWithCapture(t, &Config{
		RecallTopK:            100,
		QueryParaphrasePasses: 3,
		ClosedBook:            true,
	}, item)

	if capture.calls != 1 {
		t.Fatalf("generator calls = %d, want 1", capture.calls)
	}
	wantPrompt := longmemeval.GenerationPromptForType(
		item.Question,
		item.QuestionType,
		item.QuestionDate,
		[]string{},
	)
	if got := capture.prompt; got != wantPrompt {
		t.Fatalf("closed-book prompt mismatch\n--- got ---\n%s\n--- want ---\n%s", got, wantPrompt)
	}
	if entry.Status != "done" || entry.Hypothesis == "" {
		t.Fatalf("closed-book entry = %+v, want generated status=done entry", entry)
	}
	if entry.RetrievedIDs == nil || len(entry.RetrievedIDs) != 0 {
		t.Fatalf("closed-book retrieved_ids = %v, want empty", entry.RetrievedIDs)
	}
}

func TestFullContext_GeneratesWithAllSessionsInDateOrderAndNoRecall(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-full-context",
		Question:     "How did my plans change?",
		QuestionType: "single-session-user",
		QuestionDate: "2024-06-01",
		HaystackDates: []string{
			"2024-05-10",
			"2024-04-01",
			"2024-05-10",
		},
		HaystackSessions: [][]longmemeval.Turn{
			{{Role: "user", Content: "Canceled the Denver hotel."}},
			{{Role: "user", Content: "Booked the Seattle trip."}},
			{{Role: "assistant", Content: "Saved the replacement itinerary."}},
		},
	}
	entry, capture := runBaselineWithCapture(t, &Config{
		RecallTopK:  100,
		FullContext: true,
	}, item)

	if capture.calls != 1 {
		t.Fatalf("generator calls = %d, want 1", capture.calls)
	}
	wantPrompt := longmemeval.GenerationPromptForType(
		item.Question,
		item.QuestionType,
		item.QuestionDate,
		[]string{
			"Session date: 2024-04-01\nuser: Booked the Seattle trip.",
			"Session date: 2024-05-10\nuser: Canceled the Denver hotel.",
			"Session date: 2024-05-10\nassistant: Saved the replacement itinerary.",
		},
	)
	if got := capture.prompt; got != wantPrompt {
		t.Fatalf("full-context prompt mismatch\n--- got ---\n%s\n--- want ---\n%s", got, wantPrompt)
	}
	if entry.Status != "done" || entry.RetrievedIDs == nil || len(entry.RetrievedIDs) != 0 {
		t.Fatalf("full-context entry = %+v, want status=done and empty retrieved_ids", entry)
	}
	if entry.ContextSessionCount != 3 {
		t.Fatalf("full-context context_session_count = %d, want 3", entry.ContextSessionCount)
	}
}

func TestBaselineModes_RunWithoutIngestCheckpointOrEngram(t *testing.T) {
	tests := []struct {
		name      string
		mode      func(*Config)
		generator string
		binary    string
		script    string
		oai       bool
	}{
		{
			name: "closed book with vllm selector",
			mode: func(cfg *Config) {
				cfg.ClosedBook = true
			},
			generator: "vllm",
			oai:       true,
		},
		{
			name: "closed book with codex selector",
			mode: func(cfg *Config) {
				cfg.ClosedBook = true
			},
			generator: "codex",
			binary:    "codex",
			script: "#!/bin/sh\ncat > \"$BASELINE_PROMPT_PATH\"\n" +
				"printf 'codex\\ngenerated via codex\\ntokens used\\n1\\n'\n",
		},
		{
			name: "full context with vllm selector",
			mode: func(cfg *Config) {
				cfg.FullContext = true
			},
			generator: "vllm",
			oai:       true,
		},
		{
			name: "full context with codex selector",
			mode: func(cfg *Config) {
				cfg.FullContext = true
			},
			generator: "codex",
			binary:    "codex",
			script: "#!/bin/sh\ncat > \"$BASELINE_PROMPT_PATH\"\n" +
				"printf 'codex\\ngenerated via codex\\ntokens used\\n1\\n'\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			promptPath := filepath.Join(root, "prompt.txt")
			if tt.binary != "" {
				binDir := filepath.Join(root, "bin")
				if err := os.Mkdir(binDir, 0o700); err != nil {
					t.Fatalf("mkdir bin: %v", err)
				}
				if err := os.WriteFile(filepath.Join(binDir, tt.binary), []byte(tt.script), 0o700); err != nil {
					t.Fatalf("write fake %s: %v", tt.binary, err)
				}
				t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
				t.Setenv("BASELINE_PROMPT_PATH", promptPath)
			}

			dataPath := filepath.Join(root, "questions.json")
			data := `[{"question_id":"q-no-engram","question_type":"single-session-user","question":"What changed?","answer":"Seattle","question_date":"2024-06-01","haystack_dates":["2024-05-01"],"haystack_sessions":[[{"role":"user","content":"Changed the trip to Seattle."}]]}]`
			if err := os.WriteFile(dataPath, []byte(data), 0o600); err != nil {
				t.Fatalf("write data: %v", err)
			}

			cfg := &Config{
				DataFile:         dataPath,
				OutDir:           root,
				Workers:          1,
				Generator:        tt.generator,
				GeneratorModel:   "gpt-5.6-sol",
				GenerationModel:  "sonnet",
				CleanupPolicy:    CleanupPolicyNever,
				ExclusiveBackend: false,
			}
			if tt.oai {
				previousTransport := http.DefaultTransport
				http.DefaultTransport = baselineRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					var body struct {
						Messages []struct {
							Role    string `json:"role"`
							Content string `json:"content"`
						} `json:"messages"`
					}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						return nil, err
					}
					for _, message := range body.Messages {
						if message.Role == "user" {
							if err := os.WriteFile(promptPath, []byte(message.Content), 0o600); err != nil {
								return nil, err
							}
							break
						}
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     "200 OK",
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"generated via vllm"}}]}`)),
						Request:    req,
					}, nil
				})
				t.Cleanup(func() {
					http.DefaultTransport = previousTransport
				})
				cfg.LLMBaseURL = "http://vllm.test/v1"
				cfg.LLMModel = "test-model"
			}
			tt.mode(cfg)
			if exit := runRun(cfg); exit != 0 {
				t.Fatalf("runRun exit = %d, want 0 without ingest checkpoint or Engram", exit)
			}
			entries, err := longmemeval.ReadAllRun(filepath.Join(root, "checkpoint-run.jsonl"))
			if err != nil {
				t.Fatalf("read run checkpoint: %v", err)
			}
			if len(entries) != 1 || entries[0].Status != "done" || entries[0].RetrievedIDs == nil {
				t.Fatalf("run entries = %+v, want one done baseline entry with empty provenance", entries)
			}
			if _, err := os.Stat(filepath.Join(root, "checkpoint-ingest.jsonl")); !os.IsNotExist(err) {
				t.Fatalf("baseline unexpectedly required or created ingest checkpoint: %v", err)
			}
			if prompt, err := os.ReadFile(promptPath); err != nil || !strings.Contains(string(prompt), "What changed?") {
				t.Fatalf("generator prompt missing question: prompt=%q err=%v", prompt, err)
			}
		})
	}
}

func TestFullContext_OversizedHaystackDropsOldestSessionsAndLogsCount(t *testing.T) {
	oldest := strings.Repeat("o", 200_000)
	middle := strings.Repeat("m", 200_000)
	newest := strings.Repeat("n", 200_000)
	item := longmemeval.Item{
		QuestionID:   "q-full-context-budget",
		Question:     "What happened most recently?",
		QuestionType: "knowledge-update",
		QuestionDate: "2024-06-01",
		HaystackDates: []string{
			"2024-03-01",
			"2024-01-01",
			"2024-02-01",
		},
		HaystackSessions: [][]longmemeval.Turn{
			{{Role: "user", Content: newest}},
			{{Role: "user", Content: oldest}},
			{{Role: "user", Content: middle}},
		},
	}

	var logs bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})

	_, capture := runBaselineWithCapture(t, &Config{
		RecallTopK:  100,
		FullContext: true,
	}, item)
	prompt := capture.prompt
	if strings.Contains(prompt, oldest) {
		t.Fatal("full-context budget retained the oldest session")
	}
	if !strings.Contains(prompt, middle) || !strings.Contains(prompt, newest) {
		t.Fatal("full-context budget did not retain the newest sessions")
	}
	logText := logs.String()
	for _, want := range []string{"WARN", item.QuestionID, "full-context", "oldest-first"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("full-context truncation log = %q, want %q", logText, want)
		}
	}
	dropFields := 0
	for _, field := range strings.Fields(logText) {
		if field == "dropped_sessions=1" {
			dropFields++
		}
	}
	if dropFields != 1 {
		t.Fatalf("full-context truncation log = %q, want one exact dropped_sessions=1 field", logText)
	}
}

func TestFullContextBlocks_EmptyHaystackReturnsInitializedEmptySlice(t *testing.T) {
	blocks, dropped := fullContextBlocks(longmemeval.Item{}, 0, fullContextCharBudget)
	if blocks == nil {
		t.Fatal("fullContextBlocks returned nil, want initialized empty context")
	}
	if len(blocks) != 0 || dropped != 0 {
		t.Fatalf("fullContextBlocks(empty) = (%v, %d), want ([], 0)", blocks, dropped)
	}
}

func TestContextBlocksCharCount_IncludesPromptSeparators(t *testing.T) {
	blocks := []string{"a", "bb", "ccc"}
	want := len(strings.Join(blocks, "\n\n---\n\n"))
	if got := contextBlocksCharCount(blocks); got != want {
		t.Fatalf("contextBlocksCharCount = %d, want exact prompt length %d", got, want)
	}
}

func TestBaselineModes_GenerationFailureReturnsErrorEntry(t *testing.T) {
	item := longmemeval.Item{
		QuestionID: "q-generation-error",
		Question:   "What happened?",
	}
	entry := runOneWithGenerator(
		context.Background(),
		&Config{ClosedBook: true},
		nil,
		item,
		longmemeval.IngestEntry{QuestionID: item.QuestionID},
		func(context.Context, *Config, string) (string, error) {
			return "", errors.New("generator unavailable")
		},
	)
	if entry.Status != "error" {
		t.Fatalf("generation failure status = %q, want error", entry.Status)
	}
	if !strings.Contains(entry.Error, "generate: generator unavailable") {
		t.Fatalf("generation failure error = %q, want wrapped generator error", entry.Error)
	}
	if entry.RetrievedIDs == nil {
		t.Fatal("generation failure retrieved_ids is nil, want initialized empty provenance")
	}
}

func TestBaselineModes_GeneratorPanicReturnsInitializedErrorProvenance(t *testing.T) {
	item := longmemeval.Item{
		QuestionID: "q-generator-panic",
		Question:   "What happened?",
	}
	entry := runOneWithGenerator(
		context.Background(),
		&Config{ClosedBook: true},
		nil,
		item,
		longmemeval.IngestEntry{QuestionID: item.QuestionID},
		func(context.Context, *Config, string) (string, error) {
			panic("generator panic")
		},
	)
	if entry.Status != "error" || !strings.Contains(entry.Error, "panic: generator panic") {
		t.Fatalf("generator panic entry = %+v, want loud error", entry)
	}
	if entry.RetrievedIDs == nil {
		t.Fatal("generator panic retrieved_ids is nil, want initialized empty baseline provenance")
	}
}

func TestFullContextBlocks_RepeatInvocationIsDeterministic(t *testing.T) {
	item := longmemeval.Item{
		HaystackDates: []string{"2024-02-01", "2024-01-01"},
		HaystackSessions: [][]longmemeval.Turn{
			{{Role: "user", Content: "newer"}},
			{{Role: "user", Content: "older"}},
		},
	}
	original, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal source item: %v", err)
	}
	firstEntry, firstCapture := runBaselineWithCapture(t, &Config{FullContext: true}, item)
	secondEntry, secondCapture := runBaselineWithCapture(t, &Config{FullContext: true}, item)
	if firstCapture.prompt != secondCapture.prompt || !reflect.DeepEqual(firstEntry, secondEntry) {
		t.Fatalf("repeat full-context runs differ: first=%+v second=%+v", firstEntry, secondEntry)
	}
	after, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal source item after runs: %v", err)
	}
	if !bytes.Equal(after, original) {
		t.Fatal("full-context run mutated its source item")
	}
}

func TestFullContext_CornerCaseCardinality(t *testing.T) {
	tests := []struct {
		name            string
		item            longmemeval.Item
		wantCount       int
		wantOccurrences int
		wantContents    []string
	}{
		{
			name:      "empty haystack still generates",
			item:      longmemeval.Item{QuestionID: "q-empty", Question: "What happened?"},
			wantCount: 0,
		},
		{
			name: "identical sessions are not deduplicated",
			item: longmemeval.Item{
				QuestionID:    "q-duplicates",
				Question:      "What was repeated?",
				HaystackDates: []string{"2024-01-01", "2024-01-01"},
				HaystackSessions: [][]longmemeval.Turn{
					{{Role: "user", Content: "same session text"}},
					{{Role: "user", Content: "same session text"}},
				},
			},
			wantCount:       2,
			wantOccurrences: 2,
		},
		{
			name: "fewer dates than sessions retains every session",
			item: longmemeval.Item{
				QuestionID:    "q-short-dates",
				Question:      "What happened?",
				HaystackDates: []string{"2024-01-01"},
				HaystackSessions: [][]longmemeval.Turn{
					{{Role: "user", Content: "alpha record"}},
					{{Role: "user", Content: "beta record"}},
				},
			},
			wantCount:    2,
			wantContents: []string{"alpha record", "beta record"},
		},
		{
			name: "more dates than sessions ignores extras",
			item: longmemeval.Item{
				QuestionID:    "q-extra-dates",
				Question:      "What happened?",
				HaystackDates: []string{"2024-01-01", "2024-02-01"},
				HaystackSessions: [][]longmemeval.Turn{
					{{Role: "user", Content: "only session"}},
				},
			},
			wantCount: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, capture := runBaselineWithCapture(t, &Config{FullContext: true}, tt.item)
			if entry.Status != "done" || capture.calls != 1 {
				t.Fatalf("full-context corner entry = %+v calls=%d, want generated done entry", entry, capture.calls)
			}
			if entry.ContextSessionCount != tt.wantCount {
				t.Fatalf("context_session_count = %d, want %d", entry.ContextSessionCount, tt.wantCount)
			}
			if tt.wantOccurrences > 0 && strings.Count(capture.prompt, "same session text") != tt.wantOccurrences {
				t.Fatalf("duplicate session occurrences in prompt = %d, want %d", strings.Count(capture.prompt, "same session text"), tt.wantOccurrences)
			}
			for _, content := range tt.wantContents {
				if got := strings.Count(capture.prompt, content); got != 1 {
					t.Fatalf("prompt occurrence count for %q = %d, want 1", content, got)
				}
			}
		})
	}
}

func TestBaselineModes_DefaultSelectsRetrievalPath(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-default-retrieval",
		Question:     "How many times did I call my sister?",
		QuestionType: "multi-session",
		QuestionDate: "2024-06-01",
	}
	blocks, handled, dropped := baselineContextBlocks(&Config{}, item)
	if handled {
		t.Fatal("neither baseline flag selected: retrieval path was bypassed")
	}
	if blocks != nil || dropped != 0 {
		t.Fatalf("default baselineContextBlocks = (%v, %t, %d), want (nil, false, 0)", blocks, handled, dropped)
	}
}
