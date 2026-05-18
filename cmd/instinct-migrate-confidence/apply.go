package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// applyConfig holds paths and date for an apply run.
type applyConfig struct {
	backupDir string
	logDir    string
	date      string // YYYY-MM-DD; defaults to today
}

// migrationLogEntry is one JSONL line in the migration log.
type migrationLogEntry struct {
	ID          string    `json:"id"`
	Project     string    `json:"project"`
	Before      float64   `json:"before"`
	After       float64   `json:"after"`
	CorrectedAt time.Time `json:"corrected_at"`
}

// runApply applies the confidence migration to all affected records.
//
// Pre-flight: requires today's backup file at <backupDir>/pre-migration-<date>.jsonl.
// Idempotent: records that already have float importance ≤1.0 are skipped.
// Conversion: new_importance = old_int / 10.0 (e.g. 7 → 0.7).
// Per-record change log written to <logDir>/migration-<date>.log (JSONL).
// Rate limit: 50ms sleep between corrections.
func runApply(ctx context.Context, client engramClient, projects []string, cfg applyConfig) error {
	date := cfg.date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	// Pre-flight: backup must exist.
	backupPath := filepath.Join(cfg.backupDir, "pre-migration-"+date+".jsonl")
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf(
			"backup not found at %s — run --backup-only first before --apply: %w",
			backupPath, err,
		)
	}

	// Open migration log.
	if err := os.MkdirAll(cfg.logDir, 0700); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	logPath := filepath.Join(cfg.logDir, "migration-"+date+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open migration log: %w", err)
	}
	defer logFile.Close()
	enc := json.NewEncoder(logFile)

	seen := map[string]bool{}
	corrected, skipped, anomalies := 0, 0, 0

	for _, proj := range projects {
		records, err := client.queryRecords(ctx, proj)
		if err != nil {
			return fmt.Errorf("query project %s: %w", proj, err)
		}
		for _, r := range records {
			if !hasInstinctAndSig(r.Tags) {
				continue
			}
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true

			affected, anomaly := classifyRecord(r.Importance)
			if anomaly {
				anomalies++
				fmt.Fprintf(os.Stderr, "SKIP anomaly [%s/%s] importance=%.4f\n", r.Project, r.ID, r.Importance)
				continue
			}
			if !affected {
				skipped++
				continue
			}

			// Idempotency check: re-fetch current state before correcting.
			// Use r.Project rather than the loop variable proj — they are the
			// same in production (queryRecords stamps r.Project), but tests
			// may return records from other projects in a single queryRecords
			// call, so always honour the record's own project field.
			current, err := client.fetchRecord(ctx, r.ID, r.Project)
			if err != nil {
				fmt.Fprintf(os.Stderr, "WARN: cannot fetch current state for %s/%s: %v — skipping\n", r.Project, r.ID, err)
				skipped++
				continue
			}
			recheck, _ := classifyRecord(current.Importance)
			if !recheck {
				// Already migrated since we queried; skip.
				skipped++
				fmt.Fprintf(os.Stderr, "SKIP already-migrated [%s/%s]\n", r.Project, r.ID)
				continue
			}

			newImportance := float64(int(r.Importance)) / 10.0
			if err := client.correctRecord(ctx, r.ID, r.Project, newImportance); err != nil {
				return fmt.Errorf("correct [%s/%s]: %w", r.Project, r.ID, err)
			}

			entry := migrationLogEntry{
				ID:          r.ID,
				Project:     r.Project,
				Before:      r.Importance,
				After:       newImportance,
				CorrectedAt: time.Now().UTC(),
			}
			if err := enc.Encode(entry); err != nil {
				return fmt.Errorf("write log entry: %w", err)
			}

			corrected++
			fmt.Fprintf(os.Stderr, "CORRECTED [%s/%s] %.0f → %.2f\n", r.Project, r.ID, r.Importance, newImportance)

			// Rate limit: 50ms between corrections.
			sleepForRateLimit()
		}
	}

	fmt.Fprintf(os.Stderr, "\nApply complete: corrected=%d skipped=%d anomalies=%d\n",
		corrected, skipped, anomalies)
	fmt.Fprintf(os.Stderr, "Migration log: %s\n", logPath)
	return nil
}

// sleepForRateLimit sleeps 50ms between Engram corrections.
// Extracted as a variable so tests can stub it to a no-op.
var sleepForRateLimit = func() {
	time.Sleep(50 * time.Millisecond)
}
