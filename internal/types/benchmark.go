package types

import (
	"encoding/json"
	"time"
)

// Duration wraps time.Duration with human-readable JSON serialisation.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

func (d Duration) Std() time.Duration { return time.Duration(d) }

type Verdict string

const (
	VerdictRecommended    Verdict = "recommended"
	VerdictUsable         Verdict = "usable"
	VerdictNotRecommended Verdict = "not-recommended"
	VerdictFailed         Verdict = "failed"
	VerdictTimedOut       Verdict = "timeout"
	VerdictPullFailed     Verdict = "pull-failed"
	VerdictSkippedVRAM    Verdict = "skipped-vram"
)

type RunAttempt struct {
	Duration     Duration `json:"duration"`
	RawContent   string   `json:"raw_content"`
	ThinkingText string   `json:"thinking_text"`
	Error        string   `json:"error,omitempty"`
	TimedOut     bool     `json:"timed_out"`
}

type RunResult struct {
	Model        string       `json:"model"`
	ModelDigest  string       `json:"model_digest"`
	PullDuration Duration     `json:"pull_duration"`
	Runs         []RunAttempt `json:"runs"`
	CacheKey     string       `json:"cache_key"`
	Skipped      bool         `json:"skipped,omitempty"`
	SkipReason   string       `json:"skip_reason,omitempty"`
}

type Score struct {
	JSONValid     bool     `json:"json_valid"`
	PatternCount  int      `json:"pattern_count"`
	ValidPatterns int      `json:"valid_patterns"`
	QualityPct    float64  `json:"quality_pct"`
	AvgLatency    Duration `json:"avg_latency"`
	Composite     float64  `json:"composite"`
	ThinkingLeak  bool     `json:"thinking_leak"`
	Verdict       Verdict  `json:"verdict"`
	VerdictReason string   `json:"verdict_reason"`
}

type ModelResult struct {
	Model  string  `json:"model"`
	VRAMGB float64 `json:"vram_gb"`
	Tier   string  `json:"tier"`
	Vendor string  `json:"vendor"`
	Score  Score   `json:"score"`
}
