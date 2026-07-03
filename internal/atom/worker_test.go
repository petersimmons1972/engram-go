package atom_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/types"
)

// ── stub backend ──────────────────────────────────────────────────────────────

type stubBackend struct {
	jobs          []atom.ExtractionJob
	memories      map[string]*types.Memory
	existing      []atom.Atom
	inserted      []atom.Atom
	retired       []string
	completedJobs map[string]error
}

func newStubBackend() *stubBackend {
	return &stubBackend{
		memories:      make(map[string]*types.Memory),
		completedJobs: make(map[string]error),
	}
}

func (s *stubBackend) ClaimAtomExtractionJobs(_ context.Context, _ string, limit int) ([]atom.ExtractionJob, error) {
	n := len(s.jobs)
	if n > limit {
		n = limit
	}
	claimed := s.jobs[:n]
	s.jobs = s.jobs[n:]
	return claimed, nil
}

func (s *stubBackend) CompleteAtomExtractionJob(_ context.Context, jobID string, err error) error {
	s.completedJobs[jobID] = err
	return nil
}

func (s *stubBackend) GetMemory(_ context.Context, id string) (*types.Memory, error) {
	m, ok := s.memories[id]
	if !ok {
		return nil, nil
	}
	return m, nil
}

func (s *stubBackend) GetActiveAtoms(_ context.Context, _ string, _ string) ([]atom.Atom, error) {
	return s.existing, nil
}

func (s *stubBackend) InsertAtom(_ context.Context, a *atom.Atom) error {
	s.inserted = append(s.inserted, *a)
	return nil
}

func (s *stubBackend) RetireAtom(_ context.Context, atomID string, _ time.Time) error {
	s.retired = append(s.retired, atomID)
	return nil
}

// ── stub extractor ────────────────────────────────────────────────────────────

type stubExtractor struct {
	atoms []atom.Atom
	err   error
}

func (s *stubExtractor) Extract(_ context.Context, _ string) ([]atom.Atom, error) {
	return s.atoms, s.err
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestWorker_ProcessesFreshAtom(t *testing.T) {
	backend := newStubBackend()
	backend.jobs = []atom.ExtractionJob{{ID: "job-1", MemoryID: "mem-1", Project: "proj"}}
	backend.memories["mem-1"] = &types.Memory{ID: "mem-1", Content: "I prefer dark chocolate."}

	ext := &stubExtractor{atoms: []atom.Atom{
		{
			Type: atom.TypePreference, Subject: "the user", Predicate: "prefers",
			Value: "dark chocolate", Statement: "The user prefers dark chocolate.", Scope: atom.ScopeGlobal, Confidence: 0.9,
		},
	}}

	w := atom.NewWorker(backend, ext, atom.WorkerConfig{Projects: []string{"proj"}})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Process one batch directly (not via Run which blocks).
	// Use the exported processBatch-equivalent via a single-tick test:
	// We drive it by calling Run with a very short tick, but since we can't
	// export processBatch, we tick once through a controlled context.
	// Instead, use a 1ms poll interval to get one tick before the context times out.
	w2 := atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	})
	_ = w
	go w2.Run(ctx)
	<-ctx.Done()

	// Allow the goroutine to finish processing.
	time.Sleep(50 * time.Millisecond)

	assert.NotEmpty(t, backend.inserted, "expected at least one atom inserted")
	assert.Equal(t, "proj", backend.inserted[0].Project)
	assert.Equal(t, "mem-1", backend.inserted[0].ProvenanceMemoryID)
}

func TestWorker_SupersessionRetiresThenInserts(t *testing.T) {
	backend := newStubBackend()
	backend.jobs = []atom.ExtractionJob{{ID: "job-1", MemoryID: "mem-1", Project: "proj"}}
	backend.memories["mem-1"] = &types.Memory{ID: "mem-1", Content: "I changed my mind."}

	// Existing atom for same subject+predicate but different value.
	backend.existing = []atom.Atom{
		makeAtom("existing-1", "proj", "the user", "prefers", "coffee"),
	}

	ext := &stubExtractor{atoms: []atom.Atom{
		// New value for same predicate → triggers supersession.
		{
			Type: atom.TypePreference, Subject: "the user", Predicate: "prefers",
			Value: "tea", Statement: "The user prefers tea.", Scope: atom.ScopeGlobal, Confidence: 0.9,
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	w := atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	})
	go w.Run(ctx)
	<-ctx.Done()

	time.Sleep(50 * time.Millisecond)

	assert.Contains(t, backend.retired, "existing-1", "expected old atom to be retired")
	require.NotEmpty(t, backend.inserted)
	assert.Equal(t, "tea", backend.inserted[0].Value)
	assert.Equal(t, "existing-1", backend.inserted[0].Supersedes)
}

func TestWorker_MarkJobCompleteOnSuccess(t *testing.T) {
	backend := newStubBackend()
	backend.jobs = []atom.ExtractionJob{{ID: "job-42", MemoryID: "mem-2", Project: "proj"}}
	backend.memories["mem-2"] = &types.Memory{ID: "mem-2", Content: "some text"}
	ext := &stubExtractor{atoms: []atom.Atom{}}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w := atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	})
	go w.Run(ctx)
	<-ctx.Done()
	time.Sleep(50 * time.Millisecond)

	jobErr, seen := backend.completedJobs["job-42"]
	assert.True(t, seen, "job should be marked complete")
	assert.NoError(t, jobErr)
}

func TestWorkerSetsObservedAt(t *testing.T) {
	backend := newStubBackend()
	createdAt := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
	backend.jobs = []atom.ExtractionJob{{ID: "job-observed", MemoryID: "mem-observed", Project: "proj"}}
	backend.memories["mem-observed"] = &types.Memory{
		ID:        "mem-observed",
		Content:   "I prefer mint tea.",
		CreatedAt: createdAt,
	}
	ext := &stubExtractor{atoms: []atom.Atom{
		{
			Type: atom.TypePreference, Subject: "the user", Predicate: "prefers",
			Value: "mint tea", Statement: "The user prefers mint tea.", Scope: atom.ScopeGlobal, Confidence: 0.9,
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w := atom.NewWorker(backend, ext, atom.WorkerConfig{
		PollInterval: time.Millisecond,
		Projects:     []string{"proj"},
	})
	go w.Run(ctx)
	<-ctx.Done()
	time.Sleep(50 * time.Millisecond)

	require.NotEmpty(t, backend.inserted)
	require.NotNil(t, backend.inserted[0].ObservedAt)
	assert.True(t, backend.inserted[0].ObservedAt.Equal(createdAt))
}
