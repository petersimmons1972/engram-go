package search

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// H-NEW-1: server-side date-windowed two-pass temporal recall.
//
// ParseTemporalWindow resolves a [since, before) date window from a natural-language
// question and its reference (asked-on) date. It is a deterministic, dependency-free
// port of the client-side temporalRecallWindow logic in cmd/longmemeval/run.go, moved
// server-side so retrieval — not generation — can be temporally scoped (the H16 date
// injection at generation time was falsified; disambiguation must happen at retrieval).
//
// The window is intentionally padded around the resolved target date because session
// valid_from timestamps rarely land exactly on the arithmetic anchor. Returns (nil, nil)
// when no anchor can be resolved, including the "how many X ago" case (where the answer
// IS the date being computed, so windowing would discard the gold session).

// twRelativeAgoRe matches "<N> <unit> ago" with numeric or spelled-out N.
var twRelativeAgoRe = regexp.MustCompile(`(?i)\b(\d+|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\s+(day|days|week|weeks|month|months|year|years)\s+ago\b`)

// twAgoWords maps spelled-out small numbers to their integer value.
var twAgoWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
	"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
}

// parseTWAgoAmount parses a numeric or spelled-out positive integer.
func parseTWAgoAmount(raw string) (int, bool) {
	if n, err := strconv.Atoi(raw); err == nil {
		return n, n > 0
	}
	n := twAgoWords[strings.ToLower(raw)]
	return n, n > 0
}

// twDateOnly truncates a timestamp to midnight UTC.
func twDateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// parseQuestionDate parses the LongMemEval question_date field, tolerating the
// trailing "(Weekday)" annotation and both "/" and "-" separators.
func parseQuestionDate(questionDate string) (time.Time, bool) {
	fields := strings.Fields(questionDate)
	if len(fields) == 0 {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006/01/02", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, fields[0], time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ParseTemporalWindow returns a [since, before) date window derived from the
// question's relative-time anchor and the asked-on questionDate. Both bounds are
// nil when no anchor resolves (non-temporal questions, "how many X ago" questions,
// or an unparseable/empty questionDate). The returned bounds always satisfy
// since.Before(before).
func ParseTemporalWindow(question, questionDate string) (*time.Time, *time.Time) {
	anchor, ok := parseQuestionDate(questionDate)
	if !ok {
		return nil, nil
	}
	lower := strings.ToLower(strings.TrimSpace(question))

	// "How many ... ago" computes the date as its answer; windowing by that date
	// would discard the very session that holds the answer. Leave unscoped.
	if strings.HasPrefix(lower, "how many") {
		return nil, nil
	}

	if strings.Contains(lower, "yesterday") {
		target := twDateOnly(anchor).AddDate(0, 0, -1)
		before := target.AddDate(0, 0, 1)
		return &target, &before
	}

	match := twRelativeAgoRe.FindStringSubmatch(lower)
	if len(match) != 3 {
		return nil, nil
	}
	n, ok := parseTWAgoAmount(match[1])
	if !ok {
		return nil, nil
	}
	target := twDateOnly(anchor)
	padDays := 0
	switch match[2] {
	case "day", "days":
		target = target.AddDate(0, 0, -n)
		padDays = 1
	case "week", "weeks":
		target = target.AddDate(0, 0, -7*n)
		padDays = 3
	case "month", "months":
		target = target.AddDate(0, -n, 0)
		padDays = 7
	case "year", "years":
		target = target.AddDate(-n, 0, 0)
		padDays = 30
	default:
		return nil, nil
	}
	since := target.AddDate(0, 0, -padDays)
	before := target.AddDate(0, 0, padDays+1)
	return &since, &before
}
