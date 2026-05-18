package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// DetectReport is the JSON output of --detect-and-report mode.
type DetectReport struct {
	ScannedAt               string                 `json:"scanned_at"`
	TotalRecords            int                    `json:"total_records"`
	CandidatesForMigration  int                    `json:"candidates_for_migration"`
	ByProject               map[string]ProjectStats `json:"by_project"`
	Anomalies               []AnomalyRecord        `json:"anomalies"`
	SampleAffected          []SampleRecord         `json:"sample_affected"`
	// Note is a human-readable comment about edge-case decisions.
	Note string `json:"note,omitempty"`
}

// ProjectStats holds per-project counts.
type ProjectStats struct {
	Scanned    int `json:"scanned"`
	Candidates int `json:"candidates"`
}

// AnomalyRecord describes a record with an out-of-range importance value.
type AnomalyRecord struct {
	ID         string   `json:"id"`
	Project    string   `json:"project"`
	Importance float64  `json:"importance"`
	Tags       []string `json:"tags"`
	Reason     string   `json:"reason"`
}

// SampleRecord is a representative affected record shown in the report.
type SampleRecord struct {
	ID           string  `json:"id"`
	Project      string  `json:"project"`
	TagSignature string  `json:"tag_signature"`
	Importance   float64 `json:"importance"`
}

// writeReport writes JSON to stdout and a human summary to stderr.
func writeReport(rpt DetectReport, stdout, stderr io.Writer) error {
	rpt.Note = "Edge case: importance==1.0 is treated as NOT affected (conservative). " +
		"It may be a legitimate float 1.0 (100% confidence) rather than the int-encoding bug. " +
		"Leaving it unchanged avoids corrupting valid data."

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rpt); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}

	// Human summary to stderr.
	fmt.Fprintf(stderr, "\n=== instinct-migrate-confidence detect-and-report ===\n")
	fmt.Fprintf(stderr, "Scanned at:          %s\n", rpt.ScannedAt)
	fmt.Fprintf(stderr, "Total records:       %d\n", rpt.TotalRecords)
	fmt.Fprintf(stderr, "Candidates:          %d\n", rpt.CandidatesForMigration)
	fmt.Fprintf(stderr, "Anomalies:           %d\n", len(rpt.Anomalies))
	fmt.Fprintf(stderr, "\nBy project:\n")
	for proj, ps := range rpt.ByProject {
		if ps.Scanned > 0 {
			fmt.Fprintf(stderr, "  %-20s scanned=%d  candidates=%d\n", proj, ps.Scanned, ps.Candidates)
		}
	}
	if len(rpt.Anomalies) > 0 {
		fmt.Fprintf(stderr, "\nAnomalies (see JSON for full list):\n")
		for _, a := range rpt.Anomalies {
			fmt.Fprintf(stderr, "  [%s/%s] importance=%.4f  reason: %s\n", a.Project, a.ID, a.Importance, a.Reason)
		}
	}
	if rpt.CandidatesForMigration == 0 {
		fmt.Fprintf(stderr, "\nNo candidates found. No migration needed.\n")
	} else {
		fmt.Fprintf(stderr, "\n%d candidate(s) need correction. Run --backup-only then --apply.\n",
			rpt.CandidatesForMigration)
	}
	fmt.Fprintf(stderr, "\nNote: %s\n\n", rpt.Note)
	return nil
}

// defaultWriteReport writes to os.Stdout / os.Stderr.
func defaultWriteReport(rpt DetectReport) error {
	return writeReport(rpt, os.Stdout, os.Stderr)
}
