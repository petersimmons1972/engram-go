package main

import (
	"context"
	"math"
	"strings"
)

// classifyRecord returns (affected, anomaly) for a record's importance value.
//
// Classification rules:
//   - affected = true: importance is a whole number in [2, 10] — the likely
//     Python int-encoding bug (1–10 scale stored instead of 0.0–1.0 float).
//   - anomaly = true: value is outside [0, 10] or is a non-integer above 10.
//     Also covers negative values.
//   - importance == 0: not affected (conservative — treat as "no confidence score").
//   - importance == 1 or 1.0: ambiguous (could be int-encoded "10%" or float
//     1.0 "100% confidence"). Conservative: NOT affected. Document this in report.
//   - float values like 0.5, 0.7 in (0, 1]: already correct, not affected.
//
// The 3-attempt failure wall: this function must never panic; all edge cases
// handled explicitly.
func classifyRecord(importance float64) (affected bool, anomaly bool) {
	switch {
	case importance < 0:
		// Negative: clearly invalid.
		return false, true
	case importance > 10:
		// Out of range [0, 10]: anomaly.
		return false, true
	case importance == 0:
		// Zero: conservative — not affected.
		return false, false
	case importance <= 1.0:
		// Includes 1, 1.0 (float). Conservative: treat as correct float encoding.
		return false, false
	case isWholeNumber(importance):
		// Whole number in [2, 10] — this is the bug pattern.
		return true, false
	case importance > 10.0:
		return false, true
	default:
		// Non-integer float in (1, 10] — anomaly (e.g. 10.5, 3.5).
		return false, true
	}
}

// isWholeNumber returns true if f has no fractional component.
func isWholeNumber(f float64) bool {
	return math.Abs(f-math.Round(f)) < 1e-9
}

// runDetect queries Engram for all instinct+sig-* records across projects,
// classifies each, and returns a DetectReport. It never calls correctRecord.
func runDetect(ctx context.Context, client engramClient, projects []string) (DetectReport, error) {
	rpt := DetectReport{
		ScannedAt: nowUTC(),
		ByProject: make(map[string]ProjectStats),
	}

	seen := map[string]bool{}
	var samples []SampleRecord

	for _, proj := range projects {
		records, err := client.queryRecords(ctx, proj)
		if err != nil {
			return rpt, err
		}

		ps := ProjectStats{}
		for _, r := range records {
			if !hasInstinctAndSig(r.Tags) {
				continue
			}
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			rpt.TotalRecords++
			ps.Scanned++

			affected, anomaly := classifyRecord(r.Importance)
			switch {
			case anomaly:
				rpt.Anomalies = append(rpt.Anomalies, AnomalyRecord{
					ID:         r.ID,
					Project:    proj,
					Importance: r.Importance,
					Tags:       r.Tags,
					Reason:     anomalyReason(r.Importance),
				})
			case affected:
				rpt.CandidatesForMigration++
				ps.Candidates++
				if len(samples) < 10 {
					samples = append(samples, SampleRecord{
						ID:           r.ID,
						Project:      proj,
						TagSignature: sigTag(r.Tags),
						Importance:   r.Importance,
					})
				}
			}
		}
		rpt.ByProject[proj] = ps
	}

	rpt.SampleAffected = samples
	return rpt, nil
}

// hasInstinctAndSig returns true if tags contain both "instinct" and a "sig-*" tag.
func hasInstinctAndSig(tags []string) bool {
	hasInstinct, hasSig := false, false
	for _, t := range tags {
		if t == "instinct" {
			hasInstinct = true
		}
		if strings.HasPrefix(t, "sig-") {
			hasSig = true
		}
	}
	return hasInstinct && hasSig
}

// sigTag returns the first sig-* tag found, or empty string.
func sigTag(tags []string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, "sig-") {
			return t
		}
	}
	return ""
}

// anomalyReason returns a human-readable description for an anomalous value.
func anomalyReason(importance float64) string {
	if importance < 0 {
		return "negative value"
	}
	if importance > 10 {
		return "value exceeds maximum of 10"
	}
	return "non-integer float in ambiguous range"
}
