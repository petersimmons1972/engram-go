package scorer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/runner"
	"github.com/petersimmons1972/engram/internal/types"
)

var tagSlugRe = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)

var validTypes = map[string]bool{
	"correction":       true,
	"error_resolution": true,
	"workflow":         true,
}

type rawPattern struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Domain      string  `json:"domain"`
	Evidence    string  `json:"evidence"`
	TagSig      string  `json:"tag_signature"`
	Confidence  float64 `json:"confidence"`
}

type rawResponse struct {
	Patterns []rawPattern `json:"patterns"`
}

func isValidPattern(p rawPattern) bool {
	return validTypes[p.Type] &&
		strings.TrimSpace(p.Description) != "" &&
		strings.TrimSpace(p.Evidence) != "" &&
		tagSlugRe.MatchString(p.TagSig) &&
		p.Confidence >= 0.0 && p.Confidence <= 1.0
}

// Score computes a Score from a RunResult. Pure function — no I/O.
func Score(result types.RunResult) types.Score {
	// Skipped?
	if result.Skipped {
		return types.Score{Verdict: types.VerdictSkippedVRAM, VerdictReason: result.SkipReason}
	}

	// All timed out? (requires at least one run)
	allTimedOut := len(result.Runs) > 0
	for _, r := range result.Runs {
		if !r.TimedOut {
			allTimedOut = false
			break
		}
	}
	if allTimedOut {
		return types.Score{Verdict: types.VerdictTimedOut, VerdictReason: "all runs timed out"}
	}

	// Check thinking leak (any run)
	thinkingLeak := false
	for _, r := range result.Runs {
		if r.ThinkingText != "" || runner.DetectThinkingLeak(r.RawContent) {
			thinkingLeak = true
			break
		}
	}

	// Parse JSON for each run
	type runParsed struct {
		valid    bool
		patterns []rawPattern
		latency  time.Duration
	}
	parsed := make([]runParsed, len(result.Runs))
	for i, r := range result.Runs {
		parsed[i].latency = r.Duration.Std()
		var resp rawResponse
		if err := json.Unmarshal([]byte(r.RawContent), &resp); err == nil {
			parsed[i].valid = true
			parsed[i].patterns = resp.Patterns
		}
	}

	// JSON valid if >=2 of 3 runs parsed
	validCount := 0
	for _, p := range parsed {
		if p.valid {
			validCount++
		}
	}
	jsonValid := validCount >= 2

	if !jsonValid {
		return types.Score{
			JSONValid:     false,
			Verdict:       types.VerdictFailed,
			VerdictReason: fmt.Sprintf("JSON invalid on %d of %d runs", len(parsed)-validCount, len(parsed)),
		}
	}

	if thinkingLeak {
		return types.Score{
			JSONValid:     true,
			ThinkingLeak:  true,
			Verdict:       types.VerdictNotRecommended,
			VerdictReason: "thinking mode tokens leak into JSON content field",
		}
	}

	// Median pattern count across valid runs
	counts := []int{}
	for _, p := range parsed {
		if p.valid {
			counts = append(counts, len(p.patterns))
		}
	}
	sort.Ints(counts)
	medianCount := counts[len(counts)/2]

	// Pick the run whose pattern count == median (first match)
	var medianPatterns []rawPattern
	for _, p := range parsed {
		if p.valid && len(p.patterns) == medianCount {
			medianPatterns = p.patterns
			break
		}
	}

	// Score valid patterns
	validPatterns := 0
	for _, p := range medianPatterns {
		if isValidPattern(p) {
			validPatterns++
		}
	}

	// Average latency across runs with a non-zero duration
	var totalLatency time.Duration
	latencyCount := 0
	for _, p := range parsed {
		if p.latency > 0 {
			totalLatency += p.latency
			latencyCount++
		}
	}
	var avgLatency time.Duration
	if latencyCount > 0 {
		avgLatency = totalLatency / time.Duration(latencyCount)
	}

	qualityPct := 0.0
	if len(medianPatterns) > 0 {
		qualityPct = float64(validPatterns) / float64(len(medianPatterns))
	}
	composite := (qualityPct * float64(validPatterns) * 2) - (avgLatency.Seconds() * 0.05)

	if validPatterns == 0 {
		return types.Score{
			JSONValid:     true,
			PatternCount:  medianCount,
			AvgLatency:    types.Duration(avgLatency),
			Composite:     composite,
			Verdict:       types.VerdictUsable,
			VerdictReason: "valid JSON; zero patterns detected — may improve with prompt tuning",
		}
	}

	return types.Score{
		JSONValid:     true,
		PatternCount:  medianCount,
		ValidPatterns: validPatterns,
		QualityPct:    qualityPct,
		AvgLatency:    types.Duration(avgLatency),
		Composite:     composite,
		Verdict:       types.VerdictRecommended,
		VerdictReason: fmt.Sprintf("detected %d valid pattern(s) with %.0f%% schema conformance", validPatterns, qualityPct*100),
	}
}
