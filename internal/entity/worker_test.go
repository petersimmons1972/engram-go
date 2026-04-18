package entity_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petersimmons1972/engram/internal/entity"
	"github.com/petersimmons1972/engram/internal/types"
)

// workerBackend satisfies entity.WorkerBackend for Worker tests.
type workerBackend struct {
	mu         sync.Mutex
	jobs       []entity.ExtractionJob
	memories   map[string]*types.Memory
	entities   []entity.Entity
	completed  []string
	failedJobs []string
}

func (b *workerBackend) ClaimExtractionJobs(_ context.Context, project string, limit int) ([]entity.ExtractionJob, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []entity.ExtractionJob
	for _, j := range b.jobs {
		if j.Project == project && len(out) < limit {
			out = append(out, j)
		}
	}
	// Remove claimed jobs.
	claimed := make(map[string]bool, len(out))
	for _, j := range out {
		claimed[j.ID] = true
	}
	remaining := b.jobs[:0]
	for _, j := range b.jobs {
		if !claimed[j.ID] {
			remaining = append(remaining, j)
		}
	}
	b.jobs = remaining
	return out, nil
}

func (b *workerBackend) CompleteExtractionJob(_ context.Context, jobID string, err error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err != nil {
		b.failedJobs = append(b.failedJobs, jobID)
	} else {
		b.completed = append(b.completed, jobID)
	}
	return nil
}

func (b *workerBackend) GetMemory(_ context.Context, id string) (*types.Memory, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	m, ok := b.memories[id]
	if !ok {
		return nil, errors.New("memory not found")
	}
	return m, nil
}

func (b *workerBackend) GetEntitiesByProject(_ context.Context, _ string) ([]entity.Entity, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.entities, nil
}

func (b *workerBackend) UpsertEntity(_ context.Context, e *entity.Entity) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entities = append(b.entities, *e)
	return e.ID, nil
}

// stubExtractor is an Extractor that returns canned results.
type stubExtractor struct {
	entities  []entity.Entity
	relations []entity.Relation
	err       error
}

func (s *stubExtractor) Extract(_ context.Context, _ string) ([]entity.Entity, []entity.Relation, error) {
	return s.entities, s.relations, s.err
}

func newWorkerBackend() *workerBackend {
	return &workerBackend{memories: make(map[string]*types.Memory)}
}

func TestWorker_ProcessesJobSuccessfully(t *testing.T) {
	backend := newWorkerBackend()
	backend.memories["mem1"] = &types.Memory{ID: "mem1", Content: "Claude is an AI assistant."}
	backend.jobs = []entity.ExtractionJob{{ID: "job1", MemoryID: "mem1", Project: "test"}}

	ext := &stubExtractor{entities: []entity.Entity{{Name: "Claude"}}}
	w := entity.NewWorker(backend, ext, entity.WorkerConfig{
		Projects:     []string{"test"},
		PollInterval: 50 * time.Millisecond,
		BatchSize:    5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Contains(t, backend.completed, "job1")
	assert.Empty(t, backend.failedJobs)
	// Entity should have been upserted.
	require.Len(t, backend.entities, 1)
	assert.Equal(t, "Claude", backend.entities[0].Name)
	assert.Equal(t, "test", backend.entities[0].Project)
}

func TestWorker_HandlesExtractError(t *testing.T) {
	backend := newWorkerBackend()
	backend.memories["mem2"] = &types.Memory{ID: "mem2", Content: "some text"}
	backend.jobs = []entity.ExtractionJob{{ID: "job2", MemoryID: "mem2", Project: "proj"}}

	ext := &stubExtractor{err: errors.New("claude unavailable")}
	w := entity.NewWorker(backend, ext, entity.WorkerConfig{
		Projects:     []string{"proj"},
		PollInterval: 50 * time.Millisecond,
		BatchSize:    5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	// Job should be completed as failed, not left dangling.
	assert.Contains(t, backend.failedJobs, "job2")
	assert.Empty(t, backend.completed)
}

func TestWorker_HandlesMemoryNotFound(t *testing.T) {
	backend := newWorkerBackend()
	// No memory stored for "missing"
	backend.jobs = []entity.ExtractionJob{{ID: "job3", MemoryID: "missing", Project: "proj"}}

	ext := &stubExtractor{entities: []entity.Entity{{Name: "SomeEntity"}}}
	w := entity.NewWorker(backend, ext, entity.WorkerConfig{
		Projects:     []string{"proj"},
		PollInterval: 50 * time.Millisecond,
		BatchSize:    5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Contains(t, backend.failedJobs, "job3")
}

func TestWorker_DefaultConfig(t *testing.T) {
	// NewWorker must fill in zero PollInterval and BatchSize.
	w := entity.NewWorker(nil, nil, entity.WorkerConfig{})
	assert.NotNil(t, w)
}

func TestWorker_ExitsOnContextCancel(t *testing.T) {
	backend := newWorkerBackend()
	ext := &stubExtractor{}
	w := entity.NewWorker(backend, ext, entity.WorkerConfig{
		Projects:     []string{"p"},
		PollInterval: 10 * time.Second, // very long — should not fire
		BatchSize:    1,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancel")
	}
}
