package main

import (
	"context"
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

	atomBuildWorker(extractor, nil, nil, "", 0, workCh, ckptCh)

	require.Equal(t, []time.Time{time.Date(2023, 5, 9, 0, 0, 0, 0, time.UTC)}, extractor.dates)
}
