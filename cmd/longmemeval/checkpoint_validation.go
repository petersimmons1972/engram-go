package main

import (
	"context"
	"fmt"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

const checkpointProjectSampleSize = 8

type checkpointProjectQuery func(context.Context, string) (bool, error)

var newCheckpointProjectQuery = func(
	ctx context.Context,
	cfg *Config,
) (checkpointProjectQuery, func() error, error) {
	client, err := longmemeval.Connect(ctx, cfg.ServerURL, cfg.APIKey)
	if err != nil {
		return nil, nil, err
	}
	query := func(ctx context.Context, project string) (bool, error) {
		memories, err := client.ListProjectMemories(ctx, project, 1)
		return len(memories) > 0, err
	}
	return query, client.Close, nil
}

type checkpointProjectValidation struct {
	CheckpointPath string
	Checked        int
	Empty          int
}

func (v checkpointProjectValidation) Warning() string {
	if v.Empty == 0 || v.Empty == v.Checked {
		return ""
	}
	return fmt.Sprintf(
		"WARN checkpoint project validation: %d/%d sampled projects referenced by %q are empty; "+
			"source run used cleanup-policy=auto? generation will continue despite partial project loss",
		v.Empty,
		v.Checked,
		v.CheckpointPath,
	)
}

func validateCheckpointProjects(
	ctx context.Context,
	checkpointPath string,
	entries []longmemeval.IngestEntry,
	projectHasMemories checkpointProjectQuery,
) (checkpointProjectValidation, error) {
	result := checkpointProjectValidation{CheckpointPath: checkpointPath}
	seen := make(map[string]struct{}, checkpointProjectSampleSize)

	for _, entry := range entries {
		if result.Checked == checkpointProjectSampleSize {
			break
		}
		if _, ok := seen[entry.Project]; ok {
			continue
		}
		seen[entry.Project] = struct{}{}
		result.Checked++

		hasMemories := false
		var err error
		if entry.Project != "" {
			hasMemories, err = projectHasMemories(ctx, entry.Project)
		}
		if err != nil {
			return result, fmt.Errorf(
				"validate project %q referenced by checkpoint %q: %w",
				entry.Project,
				checkpointPath,
				err,
			)
		}
		if !hasMemories {
			result.Empty++
		}
	}

	if result.Checked > 0 && result.Empty == result.Checked {
		return result, fmt.Errorf(
			"all %d sampled projects referenced by checkpoint %q are empty "+
				"(source run used cleanup-policy=auto?); refusing to generate answers",
			result.Checked,
			checkpointPath,
		)
	}
	return result, nil
}
