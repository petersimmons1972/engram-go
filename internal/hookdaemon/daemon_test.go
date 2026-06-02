package hookdaemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- test doubles -----------------------------------------------------------

type fakeClock struct {
	mu  sync.Mutex
	now int64
}

func (c *fakeClock) Now() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
func (c *fakeClock) advance(sec int64) {
	c.mu.Lock()
	c.now += sec
	c.mu.Unlock()
}

type fakeEngram struct {
	mu sync.Mutex

	healthErr    error
	authOK       bool
	authErr      error
	recallByProj map[string][]byte
	recallErr    error

	authCalls  int
	storeCalls int
	storeBody  []byte
}

func (f *fakeEngram) Health(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.healthErr
}
func (f *fakeEngram) CheckAuth(_ context.Context, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authCalls++
	return f.authOK, f.authErr
}
func (f *fakeEngram) Recall(_ context.Context, _, _, project string, _ int) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recallErr != nil {
		return nil, f.recallErr
	}
	return f.recallByProj[project], nil
}
func (f *fakeEngram) QuickStore(_ context.Context, _ string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeCalls++
	f.storeBody = body
	return nil
}

type fakeTokens struct {
	mu      sync.Mutex
	token   string
	stored  []string
	loadErr error
}

func (t *fakeTokens) Load() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.token, t.loadErr
}
func (t *fakeTokens) Store(tok string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stored = append(t.stored, tok)
	t.token = tok
	return nil
}

type fakeMemory struct {
	mu       sync.Mutex
	sections []string
}

func (m *fakeMemory) WriteRecallSection(s string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sections = append(m.sections, s)
	return nil
}

type fakeFallback struct {
	mu       sync.Mutex
	appended []string
	failNext bool
}

func (f *fakeFallback) Append(entries []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return errors.New("disk full")
	}
	f.appended = append(f.appended, entries...)
	return nil
}

// newTestDaemon wires a daemon with all fakes and a controllable clock.
func newTestDaemon(t *testing.T, cfg Config) (*Daemon, *fakeEngram, *fakeTokens, *fakeMemory, *fakeFallback, *fakeClock) {
	t.Helper()
	eng := &fakeEngram{authOK: true, recallByProj: map[string][]byte{}}
	tok := &fakeTokens{token: "tok-abc"}
	mem := &fakeMemory{}
	fb := &fakeFallback{}
	clk := &fakeClock{now: 1_000_000}
	cfg.Engram = eng
	cfg.Tokens = tok
	cfg.Memory = mem
	cfg.Fallback = fb
	cfg.Clock = clk
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d, eng, tok, mem, fb, clk
}

// ---- constructor ------------------------------------------------------------

func TestNew_RequiresEngramAndTokens(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error when Engram client missing")
	}
	if _, err := New(Config{Engram: &fakeEngram{}}); err == nil {
		t.Fatal("expected error when TokenStore missing")
	}
}

func TestNew_LoadsCachedTokenAndDefaultIdle(t *testing.T) {
	d, _, _, _, _, _ := newTestDaemon(t, Config{})
	if d.currentToken() != "tok-abc" {
		t.Fatalf("token not loaded: %q", d.currentToken())
	}
	if d.cfg.IdleTimeout != DefaultIdleTimeout {
		t.Fatalf("default idle timeout not applied: %v", d.cfg.IdleTimeout)
	}
}

// ---- SessionStart -----------------------------------------------------------

func TestHandleSessionStart_HealthDownSurfacesMessage(t *testing.T) {
	d, eng, _, _, _, _ := newTestDaemon(t, Config{})
	eng.healthErr = errors.New("down")
	resp := d.Handle(context.Background(), Request{Hook: HookSessionStart})
	if resp.SystemMessage == "" || !strings.Contains(resp.SystemMessage, "not responding") {
		t.Fatalf("expected server-down systemMessage, got %+v", resp)
	}
}

func TestHandleSessionStart_AuthFailSurfacesMessage(t *testing.T) {
	d, eng, _, _, _, _ := newTestDaemon(t, Config{})
	eng.authOK = false
	resp := d.Handle(context.Background(), Request{Hook: HookSessionStart})
	if !strings.Contains(resp.SystemMessage, "auth failed") {
		t.Fatalf("expected auth-failed systemMessage, got %+v", resp)
	}
}

func TestHandleSessionStart_InjectsMergedRecallAndFlushes(t *testing.T) {
	d, eng, _, mem, fb, _ := newTestDaemon(t, Config{RecallProject: "engram"})
	eng.recallByProj["global"] = []byte(`{"results":[{"id":"g1","summary":"global one","score":0.5,"tags":["a"]}]}`)
	eng.recallByProj["engram"] = []byte(`{"results":[{"id":"e1","summary":"engram one","score":0.9,"tags":["b"]}]}`)
	// queue a fallback entry so SessionStart flush has something to drain
	d.enqueueFallback("- pending entry")

	resp := d.Handle(context.Background(), Request{Hook: HookSessionStart})
	if resp.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", resp.ExitCode)
	}
	if len(mem.sections) != 1 {
		t.Fatalf("expected one recall section written, got %d", len(mem.sections))
	}
	sec := mem.sections[0]
	// engram (0.9) should sort above global (0.5)
	if strings.Index(sec, "engram one") > strings.Index(sec, "global one") {
		t.Fatalf("recall not sorted by score desc:\n%s", sec)
	}
	if !strings.Contains(sec, recallHeading) {
		t.Fatalf("section missing heading:\n%s", sec)
	}
	if len(fb.appended) != 1 || fb.appended[0] != "- pending entry" {
		t.Fatalf("fallback not flushed on SessionStart: %v", fb.appended)
	}
	if d.PendingFallbackCount() != 0 {
		t.Fatalf("pending buffer not drained: %d", d.PendingFallbackCount())
	}
}

// ---- UserPromptSubmit + auth cache -----------------------------------------

func TestHandleUserPromptSubmit_AuthCacheAvoidsReprobe(t *testing.T) {
	d, eng, _, _, _, clk := newTestDaemon(t, Config{})
	// first prompt → probes auth
	d.Handle(context.Background(), Request{Hook: HookUserPromptSubmit})
	if eng.authCalls != 1 {
		t.Fatalf("expected 1 auth probe, got %d", eng.authCalls)
	}
	// within TTL → no reprobe
	clk.advance(60)
	d.Handle(context.Background(), Request{Hook: HookUserPromptSubmit})
	if eng.authCalls != 1 {
		t.Fatalf("expected cache hit (still 1 probe), got %d", eng.authCalls)
	}
	// past TTL → reprobe
	clk.advance(int64(authCacheTTL.Seconds()) + 1)
	d.Handle(context.Background(), Request{Hook: HookUserPromptSubmit})
	if eng.authCalls != 2 {
		t.Fatalf("expected reprobe after TTL, got %d", eng.authCalls)
	}
}

func TestHandleUserPromptSubmit_NoTokenIsSilent(t *testing.T) {
	d, eng, _, _, _, _ := newTestDaemon(t, Config{})
	d.token = ""
	resp := d.Handle(context.Background(), Request{Hook: HookUserPromptSubmit})
	if resp.SystemMessage != "" || eng.authCalls != 0 {
		t.Fatalf("expected silent no-op without token, got %+v (auth calls %d)", resp, eng.authCalls)
	}
}

// L1 — handleUserPromptSubmit auth-fail branch: token present, CheckAuth returns
// false → systemMessage surfaced, no exit code change.
func TestHandleUserPromptSubmit_AuthFailSurfacesMessage(t *testing.T) {
	d, eng, _, _, _, _ := newTestDaemon(t, Config{})
	eng.authOK = false
	resp := d.Handle(context.Background(), Request{Hook: HookUserPromptSubmit})
	if !strings.Contains(resp.SystemMessage, "auth failed") {
		t.Fatalf("expected auth-failed systemMessage, got %+v", resp)
	}
	if resp.ExitCode != 0 {
		t.Fatalf("auth failure must not change exit code, got %d", resp.ExitCode)
	}
}

// ---- PreToolUse -------------------------------------------------------------

func TestHandlePreToolUse_HealthyIsSilent(t *testing.T) {
	d, _, _, _, _, _ := newTestDaemon(t, Config{})
	resp := d.Handle(context.Background(), Request{Hook: HookPreToolUse})
	if resp.SystemMessage != "" {
		t.Fatalf("expected silent on healthy, got %+v", resp)
	}
}

func TestHandlePreToolUse_DownSurfacesMessage(t *testing.T) {
	d, eng, _, _, _, _ := newTestDaemon(t, Config{})
	eng.healthErr = errors.New("down")
	resp := d.Handle(context.Background(), Request{Hook: HookPreToolUse})
	if !strings.Contains(resp.SystemMessage, "health check failed") {
		t.Fatalf("expected health-failed message, got %+v", resp)
	}
}

// ---- PostToolUse / fallback buffer -----------------------------------------

func TestHandlePostToolUse_FailedEngramCallBuffers(t *testing.T) {
	d, _, _, _, _, _ := newTestDaemon(t, Config{})
	payload := json.RawMessage(`{"tool_name":"mcp__engram__memory_store","tool_response":{"is_error":true}}`)
	d.Handle(context.Background(), Request{Hook: HookPostToolUse, Payload: payload})
	if d.PendingFallbackCount() != 1 {
		t.Fatalf("expected 1 buffered entry, got %d", d.PendingFallbackCount())
	}
}

func TestHandlePostToolUse_SuccessfulCallNoBuffer(t *testing.T) {
	d, _, _, _, _, _ := newTestDaemon(t, Config{})
	payload := json.RawMessage(`{"tool_name":"mcp__engram__memory_store","tool_response":{"ok":true}}`)
	d.Handle(context.Background(), Request{Hook: HookPostToolUse, Payload: payload})
	if d.PendingFallbackCount() != 0 {
		t.Fatalf("expected no buffer for success, got %d", d.PendingFallbackCount())
	}
}

func TestHandlePostToolUse_NonEngramToolIgnored(t *testing.T) {
	d, _, _, _, _, _ := newTestDaemon(t, Config{})
	payload := json.RawMessage(`{"tool_name":"Bash","tool_response":{"is_error":true}}`)
	d.Handle(context.Background(), Request{Hook: HookPostToolUse, Payload: payload})
	if d.PendingFallbackCount() != 0 {
		t.Fatalf("expected non-engram tool ignored, got %d", d.PendingFallbackCount())
	}
}

func TestFlushFallback_RequeuesOnWriteFailure(t *testing.T) {
	d, _, _, _, fb, _ := newTestDaemon(t, Config{})
	fb.failNext = true
	d.enqueueFallback("- entry one")
	d.flushFallback()
	if d.PendingFallbackCount() != 1 {
		t.Fatalf("expected entry re-queued after failure, got %d", d.PendingFallbackCount())
	}
	// retry succeeds
	d.flushFallback()
	if d.PendingFallbackCount() != 0 || len(fb.appended) != 1 {
		t.Fatalf("expected successful retry, pending=%d appended=%v", d.PendingFallbackCount(), fb.appended)
	}
}

// ---- PreCompact -------------------------------------------------------------

func TestHandlePreCompact_FlushesBuffer(t *testing.T) {
	d, _, _, _, fb, _ := newTestDaemon(t, Config{})
	d.enqueueFallback("- before compact")
	d.Handle(context.Background(), Request{Hook: HookPreCompact})
	if len(fb.appended) != 1 {
		t.Fatalf("expected flush on PreCompact, got %v", fb.appended)
	}
}

// ---- Stop / drain (Option C) ------------------------------------------------

func TestHandleStop_StoresMarkerFlushesAndDrains(t *testing.T) {
	d, eng, _, _, fb, clk := newTestDaemon(t, Config{IdleTimeout: 10 * time.Minute})
	d.enqueueFallback("- unflushed")

	before := d.idleDeadlineUnix()
	resp := d.Handle(context.Background(), Request{Hook: HookStop})

	if eng.storeCalls != 1 {
		t.Fatalf("expected session-end marker stored once, got %d", eng.storeCalls)
	}
	if !strings.Contains(string(eng.storeBody), "session-end") {
		t.Fatalf("marker body missing session-end: %s", eng.storeBody)
	}
	if len(fb.appended) != 1 {
		t.Fatalf("expected fallback flushed on Stop, got %v", fb.appended)
	}
	if !strings.Contains(resp.Stdout, "session closed") {
		t.Fatalf("expected session summary, got %q", resp.Stdout)
	}
	// drain: idle deadline must be brought closer, not pushed out, and the
	// daemon must NOT be killed (no SIGTERM semantics here).
	after := d.idleDeadlineUnix()
	wantDrain := clk.Now() + int64(DrainIdleTimeout.Seconds())
	if after != wantDrain {
		t.Fatalf("expected drain deadline %d, got %d (before=%d)", wantDrain, after, before)
	}
	if after >= before {
		t.Fatalf("drain should bring deadline closer: before=%d after=%d", before, after)
	}
}

func TestRequestDrain_NeverExtendsDeadline(t *testing.T) {
	d, _, _, _, _, clk := newTestDaemon(t, Config{IdleTimeout: 5 * time.Second})
	// idle timeout (5s) is already shorter than the 30s drain window — drain must
	// not push the deadline out.
	before := d.idleDeadlineUnix()
	d.requestDrain()
	after := d.idleDeadlineUnix()
	if after != before {
		t.Fatalf("drain must not extend a sooner deadline: before=%d after=%d", before, after)
	}
	_ = clk
}

// ---- token write-on-change --------------------------------------------------

func TestSetToken_WritesOnlyOnChange(t *testing.T) {
	d, _, tok, _, _, _ := newTestDaemon(t, Config{})
	d.setToken("tok-abc") // same as loaded → no write
	if len(tok.stored) != 0 {
		t.Fatalf("expected no write for unchanged token, got %v", tok.stored)
	}
	d.setToken("tok-new") // changed → one write
	if len(tok.stored) != 1 || tok.stored[0] != "tok-new" {
		t.Fatalf("expected one write for changed token, got %v", tok.stored)
	}
	d.setToken("") // empty → no write
	if len(tok.stored) != 1 {
		t.Fatalf("empty token must not write, got %v", tok.stored)
	}
}

// ---- idle deadline extends on activity --------------------------------------

func TestTouch_ExtendsIdleDeadline(t *testing.T) {
	d, _, _, _, _, clk := newTestDaemon(t, Config{IdleTimeout: 100 * time.Second})
	clk.advance(50)
	d.touch()
	want := clk.Now() + 100
	if got := d.idleDeadlineUnix(); got != want {
		t.Fatalf("touch did not extend deadline: want %d got %d", want, got)
	}
}

// ---- unknown hook -----------------------------------------------------------

func TestHandle_UnknownHookIsNoOp(t *testing.T) {
	d, _, _, _, _, _ := newTestDaemon(t, Config{})
	resp := d.Handle(context.Background(), Request{Hook: "Frobnicate"})
	if resp.ExitCode != 0 || resp.SystemMessage != "" || resp.Stdout != "" {
		t.Fatalf("unknown hook should be a clean no-op, got %+v", resp)
	}
}

// ---- recall merge unit ------------------------------------------------------

func TestMergeRecallResults_DedupAndSort(t *testing.T) {
	a := []recallResult{{ID: "x", Score: 0.2}, {ID: "y", Score: 0.9}}
	b := []recallResult{{ID: "y", Score: 0.9}, {ID: "z", Score: 0.5}}
	got := mergeRecallResults(a, b)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique results, got %d", len(got))
	}
	if got[0].ID != "y" || got[1].ID != "z" || got[2].ID != "x" {
		t.Fatalf("wrong sort order: %+v", got)
	}
}

func TestExtractFallbackEntry(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    bool
	}{
		{"engram failure is_error bool", `{"tool_name":"mcp__engram__x","tool_response":{"is_error":true}}`, true},
		{"engram failure is_error int 1 (M3)", `{"tool_name":"mcp__engram__x","tool_response":{"is_error":1}}`, true},
		{"engram failure is_error int 0 (M3)", `{"tool_name":"mcp__engram__x","tool_response":{"is_error":0}}`, false},
		{"engram failure is_error string true (M3)", `{"tool_name":"mcp__engram__x","tool_response":{"is_error":"true"}}`, true},
		{"engram failure is_error string false (M3)", `{"tool_name":"mcp__engram__x","tool_response":{"is_error":"false"}}`, false},
		{"engram failure error string", `{"tool_name":"mcp__engram__x","tool_response":{"error":"boom"}}`, true},
		{"engram success", `{"tool_name":"mcp__engram__x","tool_response":{"ok":1}}`, false},
		{"array tool_response treated as success", `{"tool_name":"mcp__engram__x","tool_response":[{"type":"text"}]}`, false},
		{"non-engram", `{"tool_name":"Read","tool_response":{"is_error":true}}`, false},
		{"empty payload", ``, false},
		{"bad json", `{not json`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractFallbackEntry(json.RawMessage(c.payload), 1000)
			if (got != "") != c.want {
				t.Fatalf("want failure=%v, got entry=%q", c.want, got)
			}
		})
	}
}
