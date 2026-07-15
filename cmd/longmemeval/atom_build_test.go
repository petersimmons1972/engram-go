package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/longmemeval"
)

type recordingAtomExtractor struct {
	mu    sync.Mutex
	dates []time.Time
}

func (r *recordingAtomExtractor) Extract(_ context.Context, _ string, dates ...time.Time) ([]atom.Atom, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dates = append(r.dates, dates...)
	return nil, nil
}

func TestAtomBuildWorkerThreadsSessionDateToExtractor(t *testing.T) {
	extractor := &recordingAtomExtractor{}
	workCh := make(chan atomBuildWorkItem, 1)
	ckptCh := make(chan atomBuildEntry, 1)
	workCh <- atomBuildWorkItem{
		entry: longmemeval.IngestEntry{QuestionID: "q1", Project: "proj"},
		item: longmemeval.Item{
			QuestionID:         "q1",
			HaystackSessionIDs: []string{"s1"},
			HaystackDates:      []string{"2023/05/09 (Tue) 23:30"},
			HaystackSessions: [][]longmemeval.Turn{{
				{Role: "user", Content: "I attended a meetup."},
			}},
		},
	}
	close(workCh)

	atomBuildWorker(extractor, nil, nil, "", 0, workCh, ckptCh, &atomBuildStats{})

	require.Equal(t, []time.Time{time.Date(2023, 5, 9, 0, 0, 0, 0, time.UTC)}, extractor.dates)
}

func TestRunAtomBuildRequiresEmbedURL(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	exit := runAtomBuild(&AtomBuildConfig{
		DataFile:   "unused.json",
		LLMBaseURL: "http://inference-only.example",
		LLMModel:   "test-model",
		Workers:    1,
	}, &bytes.Buffer{}, &stderr)

	require.Equal(t, 1, exit)
	require.Contains(t, stderr.String(), "--embed-url")
}

func TestAtomBuildExitCode(t *testing.T) {
	tests := []struct {
		name       string
		processed  int64
		stored     int64
		wantExit   int
		wantStderr string
	}{
		{
			name:       "processed session storing zero atoms fails",
			processed:  1,
			wantExit:   1,
			wantStderr: "stored 0 atoms",
		},
		{
			name:     "zero sessions processed succeeds",
			wantExit: 0,
		},
		{
			name:      "processed session storing atoms succeeds",
			processed: 1,
			stored:    1,
			wantExit:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &atomBuildStats{}
			stats.processed.Store(tt.processed)
			stats.stored.Store(tt.stored)
			var stderr bytes.Buffer

			exit := atomBuildExitCode(stats, &stderr)

			require.Equal(t, tt.wantExit, exit)
			if tt.wantStderr != "" {
				require.Contains(t, stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRunAtomBuildZeroSessionsProcessedExitsZero(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data, err := json.Marshal([]longmemeval.Item{{QuestionID: "q1"}})
	require.NoError(t, err)
	dataPath := filepath.Join(dir, "data.json")
	require.NoError(t, os.WriteFile(dataPath, data, 0o600))
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q1", Project: "proj", Status: "done"},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-atom-build.jsonl"), []any{
		atomBuildEntry{QuestionID: "q1", Project: "proj", Status: "done"},
	})

	var stderr bytes.Buffer
	exit := runAtomBuild(&AtomBuildConfig{
		DataFile:   dataPath,
		OutDir:     dir,
		Workers:    1,
		LLMBaseURL: "http://llm.invalid",
		LLMModel:   "test-model",
		EmbedURL:   "http://embed.invalid",
	}, &bytes.Buffer{}, &stderr)

	require.Zero(t, exit)
	require.Empty(t, stderr.String())
}

type staticAtomExtractor struct {
	atoms []atom.Atom
}

func (e staticAtomExtractor) Extract(context.Context, string, ...time.Time) ([]atom.Atom, error) {
	return e.atoms, nil
}

type failingEmbedClient struct{}

func (failingEmbedClient) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embedding endpoint returned HTTP 404")
}

func (failingEmbedClient) EmbedWithModel(context.Context, string) ([]float32, string, error) {
	return nil, "", errors.New("embedding endpoint returned HTTP 404")
}

func (failingEmbedClient) Name() string    { return "test-embedder" }
func (failingEmbedClient) Dimensions() int { return 0 }

func TestAtomBuildAllEmbeddingRequests404ExitsNonZero(t *testing.T) {
	workCh := make(chan atomBuildWorkItem, 1)
	ckptCh := make(chan atomBuildEntry, 1)
	workCh <- atomBuildWorkItem{
		entry: longmemeval.IngestEntry{QuestionID: "q1", Project: "proj"},
		item: longmemeval.Item{
			QuestionID:         "q1",
			HaystackSessionIDs: []string{"s1"},
			HaystackSessions: [][]longmemeval.Turn{{
				{Role: "user", Content: "I prefer tea."},
			}},
		},
	}
	close(workCh)
	stats := &atomBuildStats{}
	storeCalls := 0

	atomBuildWorker(
		staticAtomExtractor{atoms: []atom.Atom{{
			Type:       atom.TypePreference,
			Subject:    "the user",
			Predicate:  "prefers",
			Value:      "tea",
			Statement:  "The user prefers tea.",
			Scope:      atom.ScopeGlobal,
			Confidence: 0.9,
		}}},
		failingEmbedClient{},
		func(string, *atom.Atom, []float32) error {
			storeCalls++
			return nil
		},
		"",
		0,
		workCh,
		ckptCh,
		stats,
	)

	var stderr bytes.Buffer
	require.Equal(t, 1, atomBuildExitCode(stats, &stderr))
	require.Equal(t, int64(1), stats.processed.Load())
	require.Zero(t, stats.stored.Load())
	require.Zero(t, storeCalls)
	require.Contains(t, stderr.String(), "stored 0 atoms")
}

// A session that extracts atoms but persists none must be checkpointed "error",
// not "done". readAtomBuildSkipSet skips exactly the "done" entries, so a "done"
// with atoms_ok:0 would permanently exclude the session from later runs — the
// re-run after fixing --embed-url would find no work and exit 0 while the atoms
// are still missing. Regression test for the adversarial-review blocker on the
// #1410 fix: fail loud ONCE then silently green forever is still silent failure.
func TestAtomBuildTotalLossCheckpointsErrorNotDone(t *testing.T) {
	workCh := make(chan atomBuildWorkItem, 1)
	ckptCh := make(chan atomBuildEntry, 1)
	workCh <- atomBuildWorkItem{
		entry: longmemeval.IngestEntry{QuestionID: "q1", Project: "proj"},
		item: longmemeval.Item{
			QuestionID:         "q1",
			HaystackSessionIDs: []string{"s1"},
			HaystackSessions: [][]longmemeval.Turn{{
				{Role: "user", Content: "I prefer tea."},
			}},
		},
	}
	close(workCh)

	atomBuildWorker(
		staticAtomExtractor{atoms: []atom.Atom{{
			Type:       atom.TypePreference,
			Subject:    "the user",
			Predicate:  "prefers",
			Value:      "tea",
			Statement:  "The user prefers tea.",
			Scope:      atom.ScopeGlobal,
			Confidence: 0.9,
		}}},
		failingEmbedClient{},
		func(string, *atom.Atom, []float32) error { return nil },
		"",
		0,
		workCh,
		ckptCh,
		&atomBuildStats{},
	)
	close(ckptCh)

	entry, ok := <-ckptCh
	require.True(t, ok, "worker must checkpoint the session")
	require.Equal(t, "error", entry.Status,
		"a session that persisted 0 of its extracted atoms must not be checkpointed done")
	require.Equal(t, 1, entry.AtomsFound)
	require.Zero(t, entry.AtomsOK)
	require.Contains(t, entry.Error, "persisted 0 of 1")
}

// The other half of the same guarantee: an "error" entry must not land in the
// skip set, so the next run actually reprocesses that session.
func TestReadAtomBuildSkipSetExcludesTotalLossSessions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint-atom-build.jsonl")

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	require.NoError(t, enc.Encode(atomBuildEntry{
		QuestionID: "q-lost", Project: "proj", AtomsFound: 3, AtomsOK: 0,
		Status: "error", Error: "persisted 0 of 3 extracted atoms",
	}))
	require.NoError(t, enc.Encode(atomBuildEntry{
		QuestionID: "q-good", Project: "proj", AtomsFound: 2, AtomsOK: 2, Status: "done",
	}))
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))

	skip, err := readAtomBuildSkipSet(path)
	require.NoError(t, err)
	require.False(t, skip["q-lost"], "a total-loss session must be retried, not skipped")
	require.True(t, skip["q-good"], "a genuinely done session should still be skipped")
}
