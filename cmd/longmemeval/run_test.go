package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// TestRunLogFormat_ErrorIncludesCause verifies that when runOne returns an
// error entry, the log message format string produced by runWorker would
// include the error cause — not just hypothesis_len=0.
//
// We test the format string directly since runWorker logs via log.Printf and
// the real worker requires a live server. The assertion is on the format
// string constant in the source; this test documents the contract so that
// any future change to the log line that drops the error field will fail CI.
func TestRunLogFormat_ErrorIncludesCause(t *testing.T) {
	// Build a synthetic error entry as runOne would return.
	entry := longmemeval.RunEntry{
		QuestionID: "q-001",
		Status:     "error",
		Error:      "recall: connection refused",
	}

	// The format string used in runWorker must contain %s for the error field.
	// We verify this by formatting the log line ourselves.
	msg := runEntryLogLine(entry)

	if !strings.Contains(msg, "status=error") {
		t.Errorf("log line missing status=error: %q", msg)
	}
	if !strings.Contains(msg, "recall: connection refused") {
		t.Errorf("log line missing error cause: %q", msg)
	}
}

// TestRunLogFormat_SuccessNoError verifies that successful entries do not
// spuriously include an error field in the log line.
func TestRunLogFormat_SuccessNoError(t *testing.T) {
	entry := longmemeval.RunEntry{
		QuestionID: "q-002",
		Hypothesis: "The answer is 42.",
		Status:     "done",
	}
	msg := runEntryLogLine(entry)
	if !strings.Contains(msg, "status=done") {
		t.Errorf("log line missing status=done: %q", msg)
	}
	if !strings.Contains(msg, "hypothesis_len=17") {
		t.Errorf("log line missing hypothesis_len: %q", msg)
	}
	// No error field should appear on success.
	if strings.Contains(msg, "error=") {
		t.Errorf("log line should not contain error= on success: %q", msg)
	}
}

// TestRunRun_AllErrors_ReturnsNonZero — #703: when every attempted item fails,
// runRun must return a non-zero exit code so scripted pipelines don't proceed.
// We can't easily exercise the full pipeline in a unit test (it requires a live
// MCP server). Instead this test exercises the exit-code computation by feeding
// an empty ingest checkpoint (which causes runRun to attempt zero items and
// return 0 — the resume-clean case), then verifying the contract at the boundary
// our code controls: a function that decides the exit code given counts.
func TestExitCodeForRunOutcome(t *testing.T) {
	cases := []struct {
		name      string
		attempted int64
		errors    int64
		want      int
	}{
		{"clean resume (0 attempted)", 0, 0, 0},
		{"all succeeded", 10, 0, 0},
		{"some failed, some succeeded", 10, 3, 0},
		{"all failed", 10, 10, 1},
		{"single attempt, failed", 1, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := exitCodeForRunOutcome(tc.attempted, tc.errors)
			if got != tc.want {
				t.Errorf("exitCodeForRunOutcome(att=%d, err=%d) = %d, want %d",
					tc.attempted, tc.errors, got, tc.want)
			}
		})
	}
}

// TestRunWorker_HasPerItemCleanup — #669: each work-item iteration must
// close its MCP client so SSE goroutines + connections don't accumulate.
// We assert the structural pattern in run.go (an IIFE with deferred Close)
// because a behavioural test would need a live MCP server + goroutine
// accounting; brittle for the value it gives.
func TestRunWorker_HasPerItemCleanup(t *testing.T) {
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "mcpClient.Close()") {
		t.Errorf("run.go missing mcpClient.Close() — per-item SSE leak risk (#669)")
	}
}

// ---------------------------------------------------------------------------
// sortBlocksChronologically
// ---------------------------------------------------------------------------

func TestSortBlocksChronologically_AscendingOrder(t *testing.T) {
	b1 := "Session date: 2024-03-15\nSome content"
	b2 := "Session date: 2024-01-01\nOlder content"
	b3 := "Session date: 2024-06-30\nNewer content"

	got := sortBlocksChronologically([]string{b1, b2, b3})
	want := []string{b2, b1, b3}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("position %d: got %q, want %q", i, got[i][:30], w[:30])
		}
	}
}

func TestSortBlocksChronologically_NoParsableDate_SortsFirst(t *testing.T) {
	noDate := "No date header here\nSome content"
	dated := "Session date: 2020-05-10\nOld but dated"

	got := sortBlocksChronologically([]string{dated, noDate})
	if got[0] != noDate {
		t.Errorf("block with no date should sort first (treated as 1970); got %q first", got[0][:20])
	}
}

func TestSortBlocksChronologically_DoesNotMutateInput(t *testing.T) {
	b1 := "Session date: 2024-03-15\nContent A"
	b2 := "Session date: 2024-01-01\nContent B"
	input := []string{b1, b2}
	orig := []string{b1, b2}

	_ = sortBlocksChronologically(input)
	for i := range orig {
		if input[i] != orig[i] {
			t.Errorf("input slice was mutated at index %d", i)
		}
	}
}

// ---------------------------------------------------------------------------
// R2-B1: runRun must return ExitCodeLockContention (75), not 2, on lock
// contention. We test this via (a) a source-level structural guard that the
// literal `2` is not used as the lock-contention exit code, and (b) a
// subprocess test that wires runRun through a locked backend.
// ---------------------------------------------------------------------------

// TestRunRun_LockContention_ExitCode75_StructuralGuard asserts that run.go
// does NOT contain `return 2` as the lock-contention exit path. This is a
// source-level regression guard — the functional assertion is in
// TestRunRun_LockContention_ExitCode75_Subprocess below.
func TestRunRun_LockContention_ExitCode75_StructuralGuard(t *testing.T) {
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}
	text := string(src)
	// The lock-contention branch must use ExitCodeLockContention, not a
	// literal 2. We check that ExitCodeLockContention is referenced and
	// that the pattern `return 2` does not appear in the lock-contention
	// block (i.e., adjacent to lockErr != nil check).
	if !strings.Contains(text, "ExitCodeLockContention") {
		t.Error("run.go missing ExitCodeLockContention — lock-contention exit code not wired (#808 R2-B1)")
	}
	// Detect the old bug: `return 2` immediately after the lockErr check.
	// We look for the pattern in the lock-acquisition block context.
	if strings.Contains(text, "return 2") {
		t.Error("run.go still contains `return 2` — must use ExitCodeLockContention (#808 R2-B1)")
	}
}

// TestRunRun_LockContention_ExitCode75_Subprocess verifies that when runRun
// finds the backend locked, the process exits with ExitCodeLockContention (75).
// Uses the lockHelper subprocess to hold the lock, then spawns a second
// subprocess that calls runRun via a minimal Config pointing at the same URL.
func TestRunRun_LockContention_ExitCode75_Subprocess(t *testing.T) {
	if os.Getenv("LME_LOCK_HELPER") != "" {
		lockHelperMain()
		return
	}
	if os.Getenv("LME_RUNRUN_HELPER") != "" {
		runRunLockHelper()
		return
	}

	dir := t.TempDir()
	url := "http://runrun-exit75:8000/v1"

	// First subprocess: hold the lock.
	cmd1 := exec.Command(os.Args[0], "-test.run=TestRunRun_LockContention_ExitCode75_Subprocess")
	cmd1.Env = append(os.Environ(),
		"LME_LOCK_HELPER=1",
		"LME_LOCK_DIR="+dir,
		"LME_LOCK_URL="+url,
		"LME_LOCK_HOLD=1s",
	)
	if err := cmd1.Start(); err != nil {
		t.Fatalf("start lock-holder: %v", err)
	}
	defer func() { _ = cmd1.Process.Kill() }()

	// Wait for lockfile to appear.
	lockPath := backendLockPath(dir, url)
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(lockPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Second subprocess: calls runRun with the locked backend. Must exit 75.
	cmd2 := exec.Command(os.Args[0], "-test.run=TestRunRun_LockContention_ExitCode75_Subprocess")
	cmd2.Env = append(os.Environ(),
		"LME_RUNRUN_HELPER=1",
		"LME_LOCK_DIR="+dir,
		"LME_LOCK_URL="+url,
	)
	err := cmd2.Run()
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); !ok {
		t.Fatalf("runRun subprocess should exit non-zero; got: %v", err)
	}
	if exitErr.ExitCode() != ExitCodeLockContention {
		t.Errorf("runRun exit code = %d, want ExitCodeLockContention (%d) (#808 R2-B1)",
			exitErr.ExitCode(), ExitCodeLockContention)
	}
}

// runRunLockHelper is the subprocess entry for TestRunRun_LockContention_ExitCode75_Subprocess.
// It calls runRun with a minimal Config pointing at the locked backend URL.
// The data file and out dir are intentionally invalid — runRun must exit at
// the lock-acquisition step before reaching them.
func runRunLockHelper() {
	dir := os.Getenv("LME_LOCK_DIR")
	url := os.Getenv("LME_LOCK_URL")
	outDir, _ := os.MkdirTemp("", "lme-runrun-test-*")
	defer os.RemoveAll(outDir)
	cfg := &Config{
		LLMBaseURL:       url,
		ExclusiveBackend: true,
		BackendLockDir:   dir,
		OutDir:           outDir,
		DataFile:         "/dev/null",
		Workers:          1,
	}
	code := runRun(cfg)
	os.Exit(code)
}

// ---------------------------------------------------------------------------
// DisableQueryRewrite — structural guard
// ---------------------------------------------------------------------------

// TestDisableQueryRewrite_StructuralGuard verifies that run.go gates the
// rewrite logic on cfg.DisableQueryRewrite. This is a source-level assertion
// since exercising runOne requires a live MCP server.
func TestDisableQueryRewrite_StructuralGuard(t *testing.T) {
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "cfg.DisableQueryRewrite") {
		t.Error("run.go missing cfg.DisableQueryRewrite gate — rewrite bypass flag not wired")
	}
}

// ---------------------------------------------------------------------------
// H15 — paraphrase union structural guards
// ---------------------------------------------------------------------------

// TestQueryParaphrasePassesFlag_StructuralGuard verifies that run.go gates the
// paraphrase-union logic on cfg.QueryParaphrasePasses and calls both
// GenerateParaphrases and DeduplicateIDs — the core H15 union mechanism.
func TestQueryParaphrasePassesFlag_StructuralGuard(t *testing.T) {
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}
	text := string(src)
	checks := []struct {
		substr string
		label  string
	}{
		{"cfg.QueryParaphrasePasses", "H15 flag gate"},
		{"GenerateParaphrases", "H15 paraphrase call"},
		{"DeduplicateIDs", "H15 dedup union"},
	}
	for _, c := range checks {
		if !strings.Contains(text, c.substr) {
			t.Errorf("run.go missing %s (%q)", c.label, c.substr)
		}
	}
}

// TestQueryParaphrasePassesFlag_DefaultZero verifies that the Config field
// defaults to 0 (off) so existing runs are unaffected.
func TestQueryParaphrasePassesFlag_DefaultZero(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	// The flag registration must use default value 0.
	if !strings.Contains(string(src), `"query-paraphrase-passes", 0`) {
		t.Error("main.go: --query-paraphrase-passes default must be 0 (off by default)")
	}
}

// ---------------------------------------------------------------------------
// R2-B2: preservedLog — deadlock-free collection (mutex-protected slice)
// ---------------------------------------------------------------------------

// TestPreservedLog_NoDeadlockOnHighCount verifies that N concurrent goroutines
// can each append a name to a preservedLog without blocking, even when N far
// exceeds any channel buffer size. This is the regression guard for R2-B2
// (#807): the old chan-based approach deadlocked when preserved-project count
// exceeded cfg.Workers*2.
func TestPreservedLog_NoDeadlockOnHighCount(t *testing.T) {
	const N = 100
	pl := &preservedLog{}
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pl.add(fmt.Sprintf("project-%03d", n))
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("preservedLog.add deadlocked under high concurrency (R2-B2 regression)")
	}

	names := pl.names()
	if len(names) != N {
		t.Errorf("preservedLog collected %d names, want %d", len(names), N)
	}
}

// TestPreservedLog_NamesReturnsCopy verifies that mutating the returned slice
// does not affect the underlying preservedLog state.
func TestPreservedLog_NamesReturnsCopy(t *testing.T) {
	pl := &preservedLog{}
	pl.add("alpha")
	pl.add("beta")

	got := pl.names()
	got[0] = "mutated"

	got2 := pl.names()
	for _, n := range got2 {
		if n == "mutated" {
			t.Error("names() returned a slice sharing the backing array — mutations affect internal state")
		}
	}
}

// TestCleanupSummary_TokenIsCleanupSummary verifies the greppable log token
// used for the end-of-run preserved-project summary is exactly "cleanup-summary"
// as specified in S9 (#807 Round 3). (Source-level structural guard.)
func TestCleanupSummary_TokenIsCleanupSummary(t *testing.T) {
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "cleanup-summary") {
		t.Error("run.go missing 'cleanup-summary' log token (S9 #807 Round 3 spec)")
	}
	// Ensure the old token is gone.
	if strings.Contains(text, "INFO run: preserved") {
		t.Error("run.go still contains old log token 'INFO run: preserved' — should be replaced by 'cleanup-summary'")
	}
}

// ---------------------------------------------------------------------------
// buildRecallQuery — F2 temporal classifier signal preservation
// ---------------------------------------------------------------------------

// temporalSignalWords mirrors the set in internal/search/query_signal.go.
// We don't import search here (different package), so we enumerate a subset
// of the well-known signals that should appear in the rewritten query.
var temporalSignalWords = []string{
	"recent", "recently", "ago", "last", "when", "since", "before", "after",
	"first", "latest", "earliest", "previous", "prior",
}

func hasTemporalSignal(q string) bool {
	lower := strings.ToLower(q)
	for _, w := range temporalSignalWords {
		// word-boundary check: preceded/followed by non-alpha or string edge
		idx := strings.Index(lower, w)
		if idx < 0 {
			continue
		}
		before := idx == 0 || !isAlpha(lower[idx-1])
		after := idx+len(w) >= len(lower) || !isAlpha(lower[idx+len(w)])
		if before && after {
			return true
		}
	}
	return false
}

func isAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// TestBuildRecallQuery_TemporalReasoningPreservesSignal is the F2 regression
// test. Before the fix, buildRecallQuery strips all temporal words from the
// interrogative prefix and returns a bare semantic fragment — causing
// isTemporalQuery() on the server to return false and apply DefaultWeights.
// After the fix, the returned query must begin with a temporal signal word
// ("recent") so TemporalWeights fire on the server side.
func TestBuildRecallQuery_TemporalReasoningPreservesSignal(t *testing.T) {
	cases := []struct {
		name     string
		question string
		// wantSemanticFragment is a substring that must appear in the result
		// to confirm the semantic content was not lost.
		wantSemanticFragment string
	}{
		{
			name:                 "days-ago interrogative",
			question:             "How many days ago did I attend the baking class?",
			wantSemanticFragment: "baking class",
		},
		{
			name:                 "weeks-ago interrogative",
			question:             "How many weeks ago did I start learning guitar?",
			wantSemanticFragment: "guitar",
		},
		{
			name:                 "when-did interrogative",
			question:             "When did I visit my grandmother in Portland?",
			wantSemanticFragment: "grandmother",
		},
		{
			name:                 "months-ago interrogative",
			question:             "How many months ago was I promoted at work?",
			wantSemanticFragment: "promoted",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildRecallQuery(tc.question, "temporal-reasoning", false)

			// F2 contract: the result must carry a temporal signal word so that
			// isTemporalQuery() returns true on the Engram server.
			if !hasTemporalSignal(got) {
				t.Errorf("buildRecallQuery(%q) = %q — no temporal signal word found; isTemporalQuery will return false (F2 regression)", tc.question, got)
			}

			// Semantic content must be preserved.
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tc.wantSemanticFragment)) {
				t.Errorf("buildRecallQuery(%q) = %q — missing expected semantic fragment %q", tc.question, got, tc.wantSemanticFragment)
			}
		})
	}
}

// TestBuildRecallQuery_TemporalReasoningDisableRewrite verifies that when
// DisableQueryRewrite is true the raw question is returned unchanged (no
// "recent " prefix is prepended).
func TestBuildRecallQuery_TemporalReasoningDisableRewrite(t *testing.T) {
	q := "How many days ago did I attend the baking class?"
	got := buildRecallQuery(q, "temporal-reasoning", true)
	if got != q {
		t.Errorf("with disableRewrite=true, buildRecallQuery should return question unchanged; got %q", got)
	}
}

// TestBuildRecallQuery_NonTemporalUnchanged verifies that non-temporal question
// types are not prefixed with "recent ".
func TestBuildRecallQuery_NonTemporalUnchanged(t *testing.T) {
	q := "What is my favorite restaurant?"
	got := buildRecallQuery(q, "single-hop-factual", false)
	if got != q {
		t.Errorf("non-temporal question should be returned unchanged; got %q, want %q", got, q)
	}
}
