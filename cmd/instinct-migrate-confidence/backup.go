package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// backupLine is the JSONL schema written per record in the backup file.
type backupLine struct {
	ID         string   `json:"id"`
	Project    string   `json:"project"`
	Importance float64  `json:"importance"`
	Tags       []string `json:"tags"`
	Content    string   `json:"content,omitempty"`
	Summary    string   `json:"summary,omitempty"`
}

// runBackup dumps every instinct-tagged record (all projects) to
// <backupDir>/pre-migration-<date>.jsonl.
// Full record JSON, one per line, with project tagged in each line.
// This is the "restore source of truth" if revert fails.
func runBackup(ctx context.Context, client engramClient, projects []string, backupDir, date string) error {
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return fmt.Errorf("create backup dir %s: %w", backupDir, err)
	}

	outPath := filepath.Join(backupDir, "pre-migration-"+date+".jsonl")
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	seen := map[string]bool{}
	count := 0

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
			line := backupLine{
				ID:         r.ID,
				Project:    r.Project,
				Importance: r.Importance,
				Tags:       r.Tags,
				Content:    r.Content,
				Summary:    r.Summary,
			}
			if err := enc.Encode(line); err != nil {
				return fmt.Errorf("write backup line for %s: %w", r.ID, err)
			}
			count++
		}
	}

	fmt.Fprintf(os.Stderr, "Backup written: %s (%d records)\n", outPath, count)
	return nil
}
