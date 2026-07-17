package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// ingestProjectCheckMaxSample bounds how many unique projects are probed before
// generation. 20 keeps healthy-run latency bounded (at most two memory_list
// limit=1 attempts per sample) while making all-empty detection certain for the
// #1292 failure mode (cleanup-policy=auto wiped every project) and near-certain
// for mass partial deletion.
const (
	ingestProjectCheckMaxSample    = 20
	ingestProjectCheckProbeTimeout = 5 * time.Second
	ingestProjectCheckRetryBackoff = 25 * time.Millisecond
)

// projectHasMemoriesFunc reports whether project still contains ≥1 memory.
// Injected for unit tests; production uses Client.ListProjectMemories(limit=1).
type projectHasMemoriesFunc func(ctx context.Context, project string) (bool, error)

// sampleIngestProjectsForCheck returns up to maxSample unique non-empty project
// names from done ingest entries. Selection is deterministic: sort unique
// names, then evenly space sample indices so large checkpoints are covered
// without scanning every project.
func sampleIngestProjectsForCheck(entries []longmemeval.IngestEntry, maxSample int) []string {
	seen := make(map[string]struct{}, len(entries))
	var unique []string
	for _, e := range entries {
		if e.Status != "done" {
			continue
		}
		p := strings.TrimSpace(e.Project)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		unique = append(unique, p)
	}
	sort.Strings(unique)
	if maxSample <= 0 || len(unique) <= maxSample {
		return unique
	}
	if maxSample == 1 {
		return []string{unique[0]}
	}
	out := make([]string, maxSample)
	last := len(unique) - 1
	for i := 0; i < maxSample; i++ {
		// Evenly space across [0, last] inclusive.
		idx := i * last / (maxSample - 1)
		out[i] = unique[idx]
	}
	return out
}

// validateIngestProjectsAlive spot-checks that checkpoint-referenced projects
// still contain memories before any generation call (#1292).
//
// Policy:
//   - 0 empty among samples → proceed
//   - any empty among samples → hard-fail by default (all-empty is the primary
//     bug case; partial emptiness still burns LLM compute for dead projects)
//   - --allow-empty-projects → log a loud WARN and proceed for both partial
//     and all-empty (intentional empty experiments / override)
//
// ckptPath is named in the error so operators can locate the bad checkpoint.
func validateIngestProjectsAlive(
	ctx context.Context,
	check projectHasMemoriesFunc,
	projects []string,
	allowEmpty bool,
	ckptPath string,
) error {
	return validateIngestProjectsAliveWithTiming(
		ctx,
		check,
		projects,
		allowEmpty,
		ckptPath,
		ingestProjectCheckProbeTimeout,
		ingestProjectCheckRetryBackoff,
	)
}

// validateIngestProjectsAliveWithTiming applies a deadline to each probe
// attempt and retries one failed attempt after a bounded backoff.
func validateIngestProjectsAliveWithTiming(
	ctx context.Context,
	check projectHasMemoriesFunc,
	projects []string,
	allowEmpty bool,
	ckptPath string,
	probeTimeout time.Duration,
	retryBackoff time.Duration,
) error {
	if len(projects) == 0 || check == nil {
		return nil
	}

	var empty []string
	for _, p := range projects {
		has, err := checkIngestProjectWithRetry(ctx, check, p, probeTimeout, retryBackoff)
		if err != nil {
			return fmt.Errorf("ingest project memory check failed for %q (checkpoint=%s): %w", p, ckptPath, err)
		}
		if !has {
			empty = append(empty, p)
		}
	}
	if len(empty) == 0 {
		return nil
	}

	msg := formatIngestProjectEmptyError(ckptPath, len(empty), len(projects), empty)
	if allowEmpty {
		log.Printf("WARN %s", msg)
		return nil
	}
	return fmt.Errorf("%s", msg)
}

func checkIngestProjectWithRetry(
	ctx context.Context,
	check projectHasMemoriesFunc,
	project string,
	probeTimeout time.Duration,
	retryBackoff time.Duration,
) (bool, error) {
	const attempts = 2

	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		hasMemories, checkErr := check(probeCtx, project)
		probeErr := probeCtx.Err()
		cancel()
		if probeErr != nil {
			checkErr = probeErr
		}
		if checkErr == nil {
			return hasMemories, nil
		}
		err = checkErr

		if attempt == attempts-1 || retryBackoff <= 0 {
			continue
		}
		timer := time.NewTimer(retryBackoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return false, ctx.Err()
		case <-timer.C:
		}
	}
	return false, err
}

// formatIngestProjectEmptyError builds the fail-fast message. It names the
// likely root cause (source run used cleanup-policy=auto?) and the checkpoint
// path so operators can recover without a 90-minute zero-recall run.
func formatIngestProjectEmptyError(ckptPath string, emptyCount, checked int, emptyProjects []string) string {
	sample := emptyProjects
	if len(sample) > 5 {
		sample = sample[:5]
	}
	kind := "partial"
	if emptyCount == checked {
		kind = "all"
	}
	return fmt.Sprintf(
		"ingest checkpoint projects appear empty (%s: %d/%d sampled projects have 0 memories; checkpoint=%s). "+
			"Likely root cause: source run used --cleanup-policy=auto? (projects deleted after generation). "+
			"Re-ingest, reuse a checkpoint from a run with --cleanup-policy=never, or pass --allow-empty-projects to override. "+
			"Empty sample projects: %v",
		kind, emptyCount, checked, ckptPath, sample,
	)
}

// checkIngestProjectsBeforeRun connects to Engram and validates that pending
// ingest-checkpoint projects still have memories. Returns a non-nil error when
// the run should abort before generation.
func checkIngestProjectsBeforeRun(cfg *Config, pending []longmemeval.IngestEntry) error {
	if cfg == nil || cfg.AtomOracle {
		return nil
	}
	projects := sampleIngestProjectsForCheck(pending, ingestProjectCheckMaxSample)
	if len(projects) == 0 {
		return nil
	}

	ctx := context.Background()
	mcpClient, err := longmemeval.Connect(ctx, cfg.ServerURL, cfg.APIKey)
	if err != nil {
		return fmt.Errorf("connect for ingest project check: %w", err)
	}
	defer func() {
		if cerr := mcpClient.Close(); cerr != nil {
			log.Printf("WARN ingest project check: mcpClient close: %v", cerr)
		}
	}()

	check := func(ctx context.Context, project string) (bool, error) {
		// limit=1: existence probe only — bounded cost per sample.
		mems, err := mcpClient.ListProjectMemories(ctx, project, 1)
		if err != nil {
			return false, err
		}
		return len(mems) > 0, nil
	}

	ckptPath := filepath.Join(cfg.OutDir, "checkpoint-ingest.jsonl")
	return validateIngestProjectsAlive(ctx, check, projects, cfg.AllowEmptyProjects, ckptPath)
}
