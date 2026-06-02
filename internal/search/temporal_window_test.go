// temporal_window_test.go — pure (no-DB) unit tests for the server-side
// temporal anchor parser used by H-NEW-1 two-pass date-windowed recall.
package search

import (
	"testing"
	"time"
)

func twDate(t *testing.T, y int, m time.Month, d int) time.Time {
	t.Helper()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestParseTemporalWindow_RelativeDaysAgo(t *testing.T) {
	// question_date 2023-06-09; "3 days ago" -> 2023-06-06, padded +/-1 day.
	since, before := ParseTemporalWindow("What did I do 3 days ago?", "2023/06/09 (Fri)")
	if since == nil || before == nil {
		t.Fatalf("expected non-nil window, got since=%v before=%v", since, before)
	}
	target := twDate(t, 2023, 6, 6)
	wantSince := target.AddDate(0, 0, -1)
	wantBefore := target.AddDate(0, 0, 2) // padDays+1
	if !since.Equal(wantSince) {
		t.Errorf("since = %v, want %v", since, wantSince)
	}
	if !before.Equal(wantBefore) {
		t.Errorf("before = %v, want %v", before, wantBefore)
	}
}

func TestParseTemporalWindow_WeeksAgoWiderPad(t *testing.T) {
	since, before := ParseTemporalWindow("Where did I go two weeks ago?", "2023-06-09")
	if since == nil || before == nil {
		t.Fatalf("expected non-nil window, got since=%v before=%v", since, before)
	}
	target := twDate(t, 2023, 6, 9).AddDate(0, 0, -14)
	wantSince := target.AddDate(0, 0, -3) // week pad
	wantBefore := target.AddDate(0, 0, 4)
	if !since.Equal(wantSince) {
		t.Errorf("since = %v, want %v", since, wantSince)
	}
	if !before.Equal(wantBefore) {
		t.Errorf("before = %v, want %v", before, wantBefore)
	}
}

func TestParseTemporalWindow_Yesterday(t *testing.T) {
	since, before := ParseTemporalWindow("What appointment did I have yesterday?", "2023/06/09")
	if since == nil || before == nil {
		t.Fatalf("expected non-nil window, got since=%v before=%v", since, before)
	}
	target := twDate(t, 2023, 6, 8)
	if !since.Equal(target) {
		t.Errorf("since = %v, want %v", since, target)
	}
	if !before.Equal(target.AddDate(0, 0, 1)) {
		t.Errorf("before = %v, want %v", before, target.AddDate(0, 0, 1))
	}
}

func TestParseTemporalWindow_HowManyReturnsNil(t *testing.T) {
	// "How many ... ago" questions must NOT be windowed: the answer date is the
	// thing being computed, so filtering by it would discard the gold session.
	since, before := ParseTemporalWindow("How many weeks ago did I visit the dentist?", "2023/06/09")
	if since != nil || before != nil {
		t.Fatalf("expected nil window for 'how many ago', got since=%v before=%v", since, before)
	}
}

func TestParseTemporalWindow_NoAnchorReturnsNil(t *testing.T) {
	since, before := ParseTemporalWindow("What is my favourite colour?", "2023/06/09")
	if since != nil || before != nil {
		t.Fatalf("expected nil window for non-temporal question, got since=%v before=%v", since, before)
	}
}

func TestParseTemporalWindow_UnparseableDateReturnsNil(t *testing.T) {
	since, before := ParseTemporalWindow("What did I do 3 days ago?", "not-a-date")
	if since != nil || before != nil {
		t.Fatalf("expected nil window for unparseable question_date, got since=%v before=%v", since, before)
	}
}

func TestParseTemporalWindow_EmptyQuestionDateReturnsNil(t *testing.T) {
	since, before := ParseTemporalWindow("What did I do 3 days ago?", "")
	if since != nil || before != nil {
		t.Fatalf("expected nil window for empty question_date, got since=%v before=%v", since, before)
	}
}

func TestParseTemporalWindow_WindowOrderingValid(t *testing.T) {
	// since must always be strictly before `before`.
	for _, q := range []string{
		"What did I do 1 day ago?",
		"Where was I 5 months ago?",
		"What happened 2 years ago?",
		"yesterday's plan?",
	} {
		since, before := ParseTemporalWindow(q, "2023/06/09")
		if since == nil || before == nil {
			t.Fatalf("q=%q: expected non-nil window", q)
		}
		if !since.Before(*before) {
			t.Errorf("q=%q: since %v not before %v", q, since, before)
		}
	}
}
