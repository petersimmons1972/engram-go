package scorer_test

import (
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/scorer"
	"github.com/petersimmons1972/engram/internal/types"
)

func attempt(content string, thinking string, dur time.Duration, timedOut bool) types.RunAttempt {
	return types.RunAttempt{
		RawContent:   content,
		ThinkingText: thinking,
		Duration:     types.Duration(dur),
		TimedOut:     timedOut,
	}
}

const validPattern = `{"patterns":[{"type":"correction","description":"Use xh not curl","domain":"bash","evidence":"curl replaced with xh twice","tag_signature":"sig-curl-xh","confidence":0.9}]}`
const emptyPatterns = `{"patterns":[]}`
const invalidJSON = `not json`

func TestScore_Recommended(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt(validPattern, "", 10*time.Second, false),
			attempt(validPattern, "", 11*time.Second, false),
			attempt(validPattern, "", 12*time.Second, false),
		},
	}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictRecommended {
		t.Errorf("want Recommended, got %s (reason: %s)", s.Verdict, s.VerdictReason)
	}
	if s.ValidPatterns != 1 {
		t.Errorf("want 1 valid pattern, got %d", s.ValidPatterns)
	}
	if !s.JSONValid {
		t.Error("want JSONValid=true")
	}
}

func TestScore_Failed_InvalidJSON(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt(invalidJSON, "", 5*time.Second, false),
			attempt(invalidJSON, "", 5*time.Second, false),
			attempt(invalidJSON, "", 5*time.Second, false),
		},
	}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictFailed {
		t.Errorf("want Failed, got %s", s.Verdict)
	}
}

func TestScore_NotRecommended_ThinkingLeak(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt(validPattern, "some thinking content", 10*time.Second, false),
			attempt(validPattern, "", 10*time.Second, false),
			attempt(validPattern, "", 10*time.Second, false),
		},
	}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictNotRecommended {
		t.Errorf("want NotRecommended (thinking leak), got %s", s.Verdict)
	}
}

func TestScore_Usable_ZeroPatterns(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt(emptyPatterns, "", 8*time.Second, false),
			attempt(emptyPatterns, "", 8*time.Second, false),
			attempt(emptyPatterns, "", 8*time.Second, false),
		},
	}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictUsable {
		t.Errorf("want Usable, got %s", s.Verdict)
	}
}

func TestScore_TimedOut(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt("", "", 300*time.Second, true),
			attempt("", "", 300*time.Second, true),
			attempt("", "", 300*time.Second, true),
		},
	}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictTimedOut {
		t.Errorf("want TimedOut, got %s", s.Verdict)
	}
}

func TestScore_SkippedVRAM(t *testing.T) {
	result := types.RunResult{Skipped: true, SkipReason: "requires 17GB, available 8GB"}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictSkippedVRAM {
		t.Errorf("want SkippedVRAM, got %s", s.Verdict)
	}
	if s.VerdictReason != "requires 17GB, available 8GB" {
		t.Errorf("want skip reason forwarded, got %q", s.VerdictReason)
	}
}

func TestScore_Composite(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt(validPattern, "", 10*time.Second, false),
			attempt(validPattern, "", 10*time.Second, false),
			attempt(validPattern, "", 10*time.Second, false),
		},
	}
	s := scorer.Score(result)
	// composite = (1.0 * 1 * 2) - (10 * 0.05) = 2.0 - 0.5 = 1.5
	if s.Composite < 1.4 || s.Composite > 1.6 {
		t.Errorf("composite score out of expected range: %.2f", s.Composite)
	}
}

func TestValidPattern_TagSignatureRegex(t *testing.T) {
	good := `{"patterns":[{"type":"workflow","description":"x","domain":"bash","evidence":"y","tag_signature":"sig-git-diff-pr","confidence":0.8}]}`
	bad := `{"patterns":[{"type":"workflow","description":"x","domain":"bash","evidence":"y","tag_signature":"SIG INVALID","confidence":0.8}]}`
	if r := scorer.Score(types.RunResult{Runs: []types.RunAttempt{attempt(good, "", 5*time.Second, false), attempt(good, "", 5*time.Second, false), attempt(good, "", 5*time.Second, false)}}); r.ValidPatterns != 1 {
		t.Errorf("good tag_signature: want 1 valid, got %d", r.ValidPatterns)
	}
	if r := scorer.Score(types.RunResult{Runs: []types.RunAttempt{attempt(bad, "", 5*time.Second, false), attempt(bad, "", 5*time.Second, false), attempt(bad, "", 5*time.Second, false)}}); r.ValidPatterns != 0 {
		t.Errorf("bad tag_signature: want 0 valid, got %d", r.ValidPatterns)
	}
}
