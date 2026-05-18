package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// revertConfig holds paths and date for a revert run.
type revertConfig struct {
	logDir string
	date   string // YYYY-MM-DD; defaults to today
}

// runRevert reads a migration log and reverses each correction.
//
// Idempotent: if the record's current importance already equals the `before`
// value, it is skipped.
// Writes a reversal log to <logDir>/migration-<date>.revert.log (JSONL).
func runRevert(ctx context.Context, client engramClient, logPath string, cfg revertConfig) error {
	date := cfg.date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("open migration log %s: %w", logPath, err)
	}
	defer f.Close()

	// Open reversal log.
	if err := os.MkdirAll(cfg.logDir, 0700); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	revertLogPath := filepath.Join(cfg.logDir, "migration-"+date+".revert.log")
	revertFile, err := os.OpenFile(revertLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open revert log: %w", err)
	}
	defer revertFile.Close()
	enc := json.NewEncoder(revertFile)

	reverted, skipped := 0, 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry migrationLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: malformed log line (skipping): %s\n", line)
			continue
		}

		// Idempotency check: fetch current importance.
		current, err := client.fetchRecord(ctx, entry.ID, entry.Project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: cannot fetch [%s/%s]: %v — skipping\n",
				entry.Project, entry.ID, err)
			skipped++
			continue
		}

		const epsilon = 1e-9
		diff := current.Importance - entry.Before
		if diff < epsilon && diff > -epsilon {
			// Already at original value.
			skipped++
			fmt.Fprintf(os.Stderr, "SKIP already-reverted [%s/%s]\n", entry.Project, entry.ID)
			continue
		}

		if err := client.correctRecord(ctx, entry.ID, entry.Project, entry.Before); err != nil {
			return fmt.Errorf("revert [%s/%s]: %w", entry.Project, entry.ID, err)
		}

		revertEntry := struct {
			ID          string    `json:"id"`
			Project     string    `json:"project"`
			From        float64   `json:"from"`
			To          float64   `json:"to"`
			RevertedAt  time.Time `json:"reverted_at"`
		}{
			ID:         entry.ID,
			Project:    entry.Project,
			From:       entry.After,
			To:         entry.Before,
			RevertedAt: time.Now().UTC(),
		}
		if err := enc.Encode(revertEntry); err != nil {
			return fmt.Errorf("write revert log: %w", err)
		}

		reverted++
		fmt.Fprintf(os.Stderr, "REVERTED [%s/%s] %.2f → %.0f\n",
			entry.Project, entry.ID, entry.After, entry.Before)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read migration log: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nRevert complete: reverted=%d skipped=%d\n", reverted, skipped)
	fmt.Fprintf(os.Stderr, "Revert log: %s\n", revertLogPath)
	return nil
}
