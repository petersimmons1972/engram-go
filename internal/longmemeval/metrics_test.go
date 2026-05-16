package longmemeval_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestRecallAny(t *testing.T) {
	retrieved := []string{"sid-a", "sid-b", "sid-c"}
	relevant := map[string]bool{"sid-b": true, "sid-z": true}

	if got := longmemeval.RecallAny(retrieved, relevant, 3); got != 1.0 {
		t.Errorf("RecallAny@3 = %.2f, want 1.0", got)
	}
	if got := longmemeval.RecallAny(retrieved, relevant, 2); got != 1.0 {
		t.Errorf("RecallAny@2 = %.2f, want 1.0", got)
	}
	if got := longmemeval.RecallAny(retrieved, relevant, 1); got != 0.0 {
		t.Errorf("RecallAny@1 = %.2f, want 0.0", got)
	}
}

func TestRecallAll(t *testing.T) {
	retrieved := []string{"sid-a", "sid-b", "sid-c", "sid-d"}
	relevant := map[string]bool{"sid-b": true, "sid-c": true}

	if got := longmemeval.RecallAll(retrieved, relevant, 4); got != 1.0 {
		t.Errorf("RecallAll@4 = %.2f, want 1.0", got)
	}
	if got := longmemeval.RecallAll(retrieved, relevant, 2); got != 0.0 {
		t.Errorf("RecallAll@2 = %.2f, want 0.0", got)
	}
}

func TestRecallAny_Empty(t *testing.T) {
	if got := longmemeval.RecallAny(nil, map[string]bool{"x": true}, 5); got != 0.0 {
		t.Errorf("RecallAny with nil retrieved = %.2f, want 0.0", got)
	}
	if got := longmemeval.RecallAny([]string{"a"}, nil, 5); got != 0.0 {
		t.Errorf("RecallAny with nil relevant = %.2f, want 0.0", got)
	}
}

func TestSessionIDs(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sid-a",
		"mem-2": "sid-b",
		"mem-3": "sid-c",
	}
	retrieved := []string{"mem-2", "mem-3", "mem-1"}
	want := []string{"sid-b", "sid-c", "sid-a"}
	got := longmemeval.SessionIDs(retrieved, memoryMap)
	for i, g := range got {
		if g != want[i] {
			t.Errorf("SessionIDs[%d] = %q, want %q", i, g, want[i])
		}
	}
}

func TestSessionContentIncludesAllRoles(t *testing.T) {
	turns := []longmemeval.Turn{
		{Role: "user", Content: "What's the best ramen place?"},
		{Role: "assistant", Content: "Ippudo in the East Village is excellent."},
		{Role: "user", Content: "Thanks!"},
	}
	got := longmemeval.SessionContent(turns)

	// Must include the assistant turn — single-session-assistant questions
	// have their gold answers in assistant replies.
	if !contains(got, "Ippudo") {
		t.Errorf("SessionContent dropped assistant turn; got %q", got)
	}
	if !contains(got, "ramen") {
		t.Errorf("SessionContent dropped user turn; got %q", got)
	}
	if !contains(got, "assistant:") || !contains(got, "user:") {
		t.Errorf("SessionContent missing role labels; got %q", got)
	}
}

func TestSessionContentEmptyTurnsSkipped(t *testing.T) {
	turns := []longmemeval.Turn{
		{Role: "user", Content: ""},
		{Role: "assistant", Content: "hello"},
	}
	got := longmemeval.SessionContent(turns)
	if !contains(got, "hello") {
		t.Errorf("expected assistant content, got %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSessionContentStripsControlChars(t *testing.T) {
	turns := []longmemeval.Turn{
		{Role: "user", Content: "hello\x0Bworld"},              // VT
		{Role: "assistant", Content: "answer\x00with\x7Fchar"}, // NUL, DEL
		{Role: "user", Content: "tab\there\nnew"},              // tab/newline preserved
	}
	got := longmemeval.SessionContent(turns)
	for _, bad := range []string{"\x00", "\x0B", "\x7F"} {
		if contains(got, bad) {
			t.Errorf("SessionContent leaked control char %q: %q", bad, got)
		}
	}
	if !contains(got, "\t") || !contains(got, "\n") {
		t.Errorf("SessionContent stripped tab/newline: %q", got)
	}
}
